package scanner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// 常见 DHCP 段按 1–20、21–29、100–119、200–219 排前面，其余从小到大。
func TestScanHostsOrder(t *testing.T) {
	local := net.ParseIP("192.168.1.5")
	mask := net.CIDRMask(24, 32)
	hosts := scanHosts(local, &net.IPNet{IP: local.Mask(mask), Mask: mask})

	if len(hosts) != 253 { // /24 的 254 个主机地址，去掉本机
		t.Fatalf("待扫地址 = %d，want 253", len(hosts))
	}
	want := map[int]byte{
		0: 1, 18: 20, // 1–20（跳过本机 .5）
		19: 21, 27: 29, // 10 开始 20 个的剩余部分
		28: 100, 47: 119,
		48: 200, 67: 219,
		68: 30, 252: 254, // 其余地址从小到大
	}
	for i, octet := range want {
		if got := hosts[i].To4()[3]; got != octet {
			t.Errorf("第 %d 个地址 = .%d，want .%d", i, got, octet)
		}
	}
}

// 比 /24 窄的网络只扫该子网，不能扫到隔壁去。
func TestScanHostsNarrowSubnet(t *testing.T) {
	local := net.ParseIP("192.168.1.70")
	mask := net.CIDRMask(26, 32)
	hosts := scanHosts(local, &net.IPNet{IP: local.Mask(mask), Mask: mask})

	if len(hosts) != 61 { // .65–.126，去掉本机
		t.Fatalf("待扫地址 = %d，want 61", len(hosts))
	}
	for _, ip := range hosts {
		if last := ip.To4()[3]; last < 65 || last > 126 {
			t.Fatalf("扫到了子网外的地址 .%d", last)
		}
	}
	if hosts[0].To4()[3] != 100 || hosts[len(hosts)-1].To4()[3] != 126 {
		t.Errorf("首尾 = .%d/.%d，want .100/.126",
			hosts[0].To4()[3], hosts[len(hosts)-1].To4()[3])
	}
}

// 并发探测不能漏掉命中，回调也要串行（调用方可能直接改界面状态）。
func TestProbeHostsReportsEveryHit(t *testing.T) {
	hosts := []net.IP{
		net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 2), net.IPv4(10, 0, 0, 3),
	}
	probe := func(_ context.Context, d DeviceInfo) (DeviceInfo, bool) {
		if d.IP == "10.0.0.2" || d.IP == "10.0.0.3" {
			return d, true
		}
		return DeviceInfo{}, false
	}
	var mu sync.Mutex
	var got []string
	probeHosts(context.Background(), hosts, probe, func(d DeviceInfo) {
		mu.Lock()
		got = append(got, d.IP)
		mu.Unlock()
	})
	if len(got) != 2 {
		t.Fatalf("命中 = %v，want 10.0.0.2 / 10.0.0.3", got)
	}
}

func TestProbeESCLFindsScanner(t *testing.T) {
	fs := newFakeScanner(t, 1)
	d := fs.device()
	got, ok := probeESCL(context.Background(), d)
	if !ok {
		t.Fatal("eSCL 设备没被认出来")
	}
	if got.Name != "HP Laser MFP 136nw" || got.IP != d.IP || got.Port != d.Port {
		t.Fatalf("探测结果 = %+v", got)
	}
}

// 普通网页也回 200，不能当成扫描仪。
func TestProbeESCLIgnoresOtherServers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html><body>router admin</body></html>")
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	host, portStr, _ := net.SplitHostPort(u.Host)
	var port int
	fmt.Sscan(portStr, &port)
	if _, ok := probeESCL(context.Background(),
		DeviceInfo{IP: host, Port: port, Path: defaultESCLPath}); ok {
		t.Fatal("普通网页被当成了扫描仪")
	}
}
