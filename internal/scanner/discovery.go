// 设备发现：浏览 mDNS/DNS-SD 的 _uscan._tcp，只采用 IPv4。
//
// HP 116w 对 PTR 查询只回 PTR + A，不带 SRV/TXT，所以自己做两步解析：
// 组播问 PTR 拿实例名和应答方 IP，再向应答方单播问 SRV/TXT 拿端口和 ty/rs。
package scanner

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	esclFQDN     = "_uscan._tcp.local."
	discoverWait = 4 * time.Second // 硬上限；正常网络 1 秒内就能收齐
	phase1Wait   = 1500 * time.Millisecond
	phase2Wait   = 1200 * time.Millisecond
	defaultPort  = 80
)

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

type serviceRecord struct {
	instance string // 实例 FQDN
	host     string // SRV Target（主机 FQDN）
	port     int
	hasPTR   bool
	hasSRV   bool
	txt      map[string]string
}

type mdnsBrowser struct {
	records    map[string]*serviceRecord // 实例 FQDN → 记录
	addrs      map[string]string         // 主机 FQDN → IPv4
	responders map[string]bool           // 应答 PTR 的设备 IP（phase 2 直接问它）
	packets    chan mdnsPacket
	done       chan struct{}
	closeOnce  sync.Once
}

type mdnsPacket struct {
	msg *dns.Msg
	src net.IP
}

type mdnsQuery struct {
	dst *net.UDPAddr
	msg []byte
}

// ctx 超时即停止，返回已找到的。
func Discover(ctx context.Context) ([]DeviceInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, discoverWait)
	defer cancel()

	mc, err := openMulticastSocket()
	if err != nil {
		return nil, err
	}
	defer mc.Close()
	b := &mdnsBrowser{
		records:    map[string]*serviceRecord{},
		addrs:      map[string]string{},
		responders: map[string]bool{},
		packets:    make(chan mdnsPacket, 128),
		done:       make(chan struct{}),
	}
	defer b.close()

	// 单播查询用临时端口；只有接收组播 PTR 应答需要绑定 5353。
	uc, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("无法创建 mDNS 查询 socket：%w", err)
	}
	defer uc.Close()

	go b.readLoop(mc.read)
	go b.readLoop(func(buf []byte) (int, net.IP, error) {
		uc.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		n, src, err := uc.ReadFromUDP(buf)
		if err != nil {
			return 0, nil, err
		}
		return n, src.IP, nil
	})

	// ── phase 1：组播 PTR ──
	ptrQuery := queryPacket(esclFQDN, dns.TypePTR, false)
	mc.send(ptrQuery)
	// 组播可能丢包：600ms 后再问一次
	b.collect(ctx, phase1Wait, 600*time.Millisecond, func() {
		mc.send(ptrQuery)
	})

	// ── phase 2：向每个应答方单播 SRV/TXT ──
	var queries []mdnsQuery
	for _, rec := range b.records {
		for ip := range b.responders {
			dst := &net.UDPAddr{IP: net.ParseIP(ip), Port: 5353}
			queries = append(queries,
				mdnsQuery{dst, queryPacket(rec.instance, dns.TypeSRV, true)},
				mdnsQuery{dst, queryPacket(rec.instance, dns.TypeTXT, true)})
		}
	}
	for _, q := range queries {
		uc.WriteToUDP(q.msg, q.dst)
	}
	if len(queries) > 0 {
		b.collect(ctx, phase2Wait, 0, nil)
	}

	return b.devices(), nil
}

// multicastSocket 绑定 5353 并从默认路由网卡收发；多网卡全加入反而容易选错出口。
type multicastSocket struct {
	conn *net.UDPConn
}

func openMulticastSocket() (*multicastSocket, error) {
	ifi, err := defaultIPv4Iface()
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenMulticastUDP("udp4", ifi, mdnsGroup)
	if err != nil {
		return nil, fmt.Errorf("无法监听 mDNS（端口 5353）：%w", err)
	}
	conn.SetReadBuffer(1 << 16)
	return &multicastSocket{conn: conn}, nil
}

func (s *multicastSocket) send(buf []byte) { s.conn.WriteToUDP(buf, mdnsGroup) }

func (s *multicastSocket) read(buf []byte) (int, net.IP, error) {
	s.conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
	n, src, err := s.conn.ReadFromUDP(buf)
	if err != nil {
		return 0, nil, err
	}
	return n, src.IP, nil
}

func (s *multicastSocket) Close() { s.conn.Close() }

func defaultIPv4Iface() (*net.Interface, error) {
	// UDP Dial 只让系统选路，不发包。
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(8, 8, 8, 8), Port: 53})
	if err == nil {
		local := conn.LocalAddr().(*net.UDPAddr).IP.To4()
		conn.Close()
		if local != nil {
			if ifi := ifaceByIPv4(local); ifi != nil {
				return ifi, nil
			}
		}
	}
	// 没有默认路由时（纯内网且未配网关），退第一个有非链路本地 IPv4 的组播网卡。
	ifaces, _ := net.Interfaces()
	for i := range ifaces {
		ifi := &ifaces[i]
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 ||
			ifi.Flags&net.FlagMulticast == 0 {
			continue
		}
		if ip := ifaceIPv4(ifi); ip != nil && !ip.IsLinkLocalUnicast() {
			return ifi, nil
		}
	}
	return nil, errors.New("没有可用的 IPv4 网卡，无法发现扫描仪")
}

func ifaceByIPv4(ip net.IP) *net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for i := range ifaces {
		if ifaces[i].Flags&net.FlagMulticast == 0 {
			continue
		}
		if ifaceIPv4(&ifaces[i]).Equal(ip) {
			return &ifaces[i]
		}
	}
	return nil
}

func ifaceIPv4(ifi *net.Interface) net.IP {
	addrs, err := ifi.Addrs()
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
			return ipn.IP.To4()
		}
	}
	return nil
}

// queryPacket 构造查询。qu=true 置 QU 位请求单播应答；PTR 必须用普通组播，否则 HP 会忽略。
func queryPacket(name string, qtype uint16, qu bool) []byte {
	m := new(dns.Msg)
	m.SetQuestion(name, qtype)
	m.RecursionDesired = false
	if qu {
		m.Question[0].Qclass |= 1 << 15
	}
	buf, _ := m.Pack()
	return buf
}

func (b *mdnsBrowser) collect(ctx context.Context, window, resendAfter time.Duration, resend func()) {
	deadline := time.NewTimer(window)
	defer deadline.Stop()
	var resendC <-chan time.Time
	if resendAfter > 0 {
		t := time.NewTimer(resendAfter)
		defer t.Stop()
		resendC = t.C
	}
	for {
		select {
		case p := <-b.packets:
			b.handle(p)
		case <-resendC:
			if resend != nil {
				resend()
			}
			resendC = nil
		case <-deadline.C:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (b *mdnsBrowser) readLoop(read func([]byte) (int, net.IP, error)) {
	buf := make([]byte, 1<<16)
	for {
		n, src, err := read(buf)
		if err != nil {
			select {
			case <-b.done:
				return
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}
		m := new(dns.Msg)
		if err := m.Unpack(buf[:n]); err != nil {
			continue
		}
		select {
		case b.packets <- mdnsPacket{msg: m, src: src}:
		case <-b.done:
			return
		}
	}
}

func (b *mdnsBrowser) close() {
	b.closeOnce.Do(func() { close(b.done) })
}

// handle 归并一条应答里的 PTR/SRV/TXT/A 记录。
func (b *mdnsBrowser) handle(p mdnsPacket) {
	rrs := append(append(append([]dns.RR{}, p.msg.Answer...), p.msg.Ns...), p.msg.Extra...)
	for _, rr := range rrs {
		switch r := rr.(type) {
		case *dns.PTR:
			if !strings.EqualFold(r.Hdr.Name, esclFQDN) {
				continue
			}
			rec := b.record(r.Ptr)
			rec.hasPTR = true
			if p.src != nil && p.src.To4() != nil {
				b.responders[p.src.To4().String()] = true
			}
		case *dns.SRV:
			rec := b.record(r.Hdr.Name)
			rec.hasSRV = true
			rec.port = int(r.Port)
			rec.host = r.Target
		case *dns.TXT:
			rec := b.record(r.Hdr.Name)
			for _, kv := range r.Txt {
				i := strings.IndexByte(kv, '=')
				if i <= 0 {
					continue
				}
				rec.txt[strings.ToLower(strings.TrimSpace(kv[:i]))] = kv[i+1:]
			}
		case *dns.A:
			if v4 := r.A.To4(); v4 != nil {
				b.addrs[strings.ToLower(r.Hdr.Name)] = v4.String()
			}
		}
	}
}

func (b *mdnsBrowser) record(instance string) *serviceRecord {
	key := strings.ToLower(instance)
	if r, ok := b.records[key]; ok {
		return r
	}
	r := &serviceRecord{instance: instance, txt: map[string]string{}}
	b.records[key] = r
	return r
}

// devices 把解析好的记录组装成设备列表（去重、排序、编下标）。
func (b *mdnsBrowser) devices() []DeviceInfo {
	var found []DeviceInfo
	seen := map[string]bool{}
	for _, rec := range b.records {
		if !rec.hasPTR || !rec.hasSRV || rec.port <= 0 {
			continue
		}
		ip := b.addrs[strings.ToLower(rec.host)]
		if ip == "" {
			ip = lookupIPv4(rec.host)
		}
		if ip == "" {
			continue // 只有 IPv6 / 解析不到：不采用
		}
		name := strings.TrimSpace(rec.txt["ty"])
		if name == "" {
			name = shortInstanceName(rec.instance)
		}
		path := strings.Trim(strings.TrimSpace(rec.txt["rs"]), "/")
		if path == "" {
			path = defaultESCLPath
		}
		key := fmt.Sprintf("%s:%d/%s", ip, rec.port, path)
		if seen[key] {
			continue
		}
		seen[key] = true
		found = append(found, DeviceInfo{
			Name: name,
			IP:   ip,
			Port: rec.port,
			Path: path,
		})
	}
	// mDNS 应答顺序随机；按名字排一下，让列表稳定、可预期
	sort.SliceStable(found, func(i, j int) bool {
		a, b := strings.ToLower(found[i].Name), strings.ToLower(found[j].Name)
		if a == b {
			return found[i].IP < found[j].IP
		}
		return a < b
	})
	for i := range found {
		found[i].Idx = i
	}
	return found
}

// lookupIPv4 兜底：SRV 没带 A 记录时，用系统解析器找 IPv4。
func lookupIPv4(host string) string {
	addrs, err := net.LookupIP(strings.TrimSuffix(host, "."))
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if v4 := a.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}

// shortInstanceName 去掉 "._uscan._tcp.local." 后缀并还原 DNS 转义（\ 空格、\( \)）。
func shortInstanceName(instance string) string {
	s := strings.TrimSuffix(instance, ".")
	s = strings.TrimSuffix(s, "._uscan._tcp.local")
	return unescapeDNSName(s)
}

func unescapeDNSName(s string) string {
	var b strings.Builder
	escaped := false
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func parseTXT(text []string) map[string]string {
	out := make(map[string]string, len(text))
	for _, kv := range text {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(kv[:i]))] = kv[i+1:]
	}
	return out
}
