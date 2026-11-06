// 网段扫描兜底：mDNS 被防火墙、路由器或虚拟网卡挡住时，直接逐个地址问 eSCL 能力。
package scanner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

const (
	scanProbeTimeout = 800 * time.Millisecond // 局域网里的设备在线就秒回
	scanWorkers      = 64                     // 同时探几个地址；多网段共用一个总时限
	scanTotalLimit   = 8 * time.Second
	scanBodyLimit    = 64 << 10 // 能力 XML 远小于这个，只读开头就够判定
)

// 内网直连，别让系统代理把 192.168.x.x 的请求也劫走。
var scanHTTP = &http.Client{
	Transport: &http.Transport{
		Proxy:             nil,
		DisableKeepAlives: true,
		DialContext:       (&net.Dialer{Timeout: scanProbeTimeout}).DialContext,
	},
}

// onFound 可能来自不同的 goroutine，调用方自己保证线程安全。
func ScanSubnet(ctx context.Context, onFound func(DeviceInfo)) error {
	hosts, err := scanTargets()
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		return errors.New("所在网段没有可扫描的地址")
	}
	ctx, cancel := context.WithTimeout(ctx, scanTotalLimit)
	defer cancel()
	probeHosts(ctx, hosts, probeESCL, onFound)
	return nil
}

func probeHosts(ctx context.Context, hosts []net.IP,
	probe func(context.Context, DeviceInfo) (DeviceInfo, bool), onFound func(DeviceInfo)) {
	var reportMu sync.Mutex
	report := func(d DeviceInfo) {
		if onFound == nil {
			return
		}
		reportMu.Lock()
		defer reportMu.Unlock()
		onFound(d)
	}

	sem := make(chan struct{}, scanWorkers)
	var wg sync.WaitGroup
	for _, ip := range hosts {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(ip net.IP) {
			defer wg.Done()
			defer func() { <-sem }()
			d := DeviceInfo{IP: ip.String(), Port: defaultPort, Path: defaultESCLPath}
			if found, ok := probe(ctx, d); ok {
				report(found)
			}
		}(ip)
	}
	wg.Wait()
}

// scanTargets 汇总所有局域网网卡的待探地址，跨网段按地址段交错排列。
// 若按网段顺序扫，虚拟网卡（VMware 之类）会排在真网卡前面把时间预算耗光。
func scanTargets() ([]net.IP, error) {
	ifaces := lanIPv4Ifaces()
	if len(ifaces) == 0 {
		return nil, errors.New("没有可用的 IPv4 网卡，无法发现扫描仪")
	}
	type target struct {
		ip     net.IP
		rank   int
		subnet int
	}
	var targets []target
	seen := map[string]bool{}
	for si, ia := range ifaces {
		subnet := ifaceIPv4Net(ia.ifi, ia.ip)
		if subnet == nil {
			mask := net.CIDRMask(24, 32)
			subnet = &net.IPNet{IP: ia.ip.Mask(mask), Mask: mask}
		}
		for _, ip := range scanHosts(ia.ip, subnet) {
			key := ip.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			targets = append(targets, target{ip, hostRank(ip.To4()[3]), si})
		}
	}
	sort.SliceStable(targets, func(i, j int) bool {
		a, b := targets[i], targets[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.subnet != b.subnet {
			return a.subnet < b.subnet
		}
		return bytes.Compare(a.ip.To4(), b.ip.To4()) < 0
	})
	hosts := make([]net.IP, len(targets))
	for i, t := range targets {
		hosts[i] = t.ip
	}
	return hosts, nil
}

// scanHosts 按常见 DHCP 分配习惯排序，家用路由器分到的地址先扫。
func scanHosts(local net.IP, subnet *net.IPNet) []net.IP {
	ip4 := local.To4()
	if ip4 == nil || subnet == nil {
		return nil
	}
	mask := subnet.Mask
	if len(mask) == net.IPv6len { // 有的网卡把 IPv4 掩码报成 16 字节
		mask = mask[12:]
	}
	if len(mask) != net.IPv4len {
		return nil
	}
	// 比 /24 大的网络只扫本机所在的 /24：/16 一次要扫六万多个地址，没必要。
	if ones, bits := mask.Size(); bits != 32 || ones < 24 {
		mask = net.CIDRMask(24, 32)
	}
	network := ip4.Mask(mask)
	sub := &net.IPNet{IP: network, Mask: mask}
	// /24 里网络号和广播号落在 0/255，循环本来就不碰；比 /24 窄时得自己排掉。
	broadcast := make(net.IP, net.IPv4len)
	for i := range broadcast {
		broadcast[i] = network[i] | ^mask[i]
	}

	var hosts []net.IP
	for i := 1; i <= 254; i++ {
		ip := net.IPv4(network[0], network[1], network[2], byte(i))
		if !sub.Contains(ip) || ip.Equal(ip4) || ip.Equal(network) || ip.Equal(broadcast) {
			continue
		}
		hosts = append(hosts, ip)
	}
	sort.SliceStable(hosts, func(x, y int) bool {
		return hostRank(hosts[x].To4()[3]) < hostRank(hosts[y].To4()[3])
	})
	return hosts
}

// hostRank 越小越先扫：四段常见 DHCP 地址各占一档。
func hostRank(last byte) int {
	switch n := int(last); {
	case n <= 20: // 1–20
		return 0
	case n <= 29: // 10 开始 20 个地址里，21–29 是剩下的
		return 1
	case n >= 100 && n <= 119:
		return 2
	case n >= 200 && n <= 219:
		return 3
	default:
		return 4
	}
}

// 只认 200 + ScannerCapabilities：路由器管理页之类同样回 200 的网页不能算设备。
func probeESCL(ctx context.Context, d DeviceInfo) (DeviceInfo, bool) {
	if d.Port == 0 {
		d.Port = defaultPort
	}
	if d.Path == "" {
		d.Path = defaultESCLPath
	}
	ctx, cancel := context.WithTimeout(ctx, scanProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.capabilitiesURL(), nil)
	if err != nil {
		return DeviceInfo{}, false
	}
	req.Header.Set("Accept", "application/xml, text/xml")
	resp, err := scanHTTP.Do(req)
	if err != nil {
		return DeviceInfo{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return DeviceInfo{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, scanBodyLimit))
	if err != nil || !bytes.Contains(body, []byte("ScannerCapabilities")) {
		return DeviceInfo{}, false
	}
	if caps, err := parseCapabilities(body); err == nil && caps.MakeAndModel != "" {
		d.Name = caps.MakeAndModel
	}
	return d, true
}
