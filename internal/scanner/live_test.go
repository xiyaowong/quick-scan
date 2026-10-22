package scanner

import (
	"context"
	"os"
	"testing"
	"time"
)

// 真机联调（默认跳过）：
//
//	SIMPLESCAN_LIVE=1 just test -run TestLiveDiscovery -v        # 只测 mDNS 发现
//	SIMPLESCAN_LIVE=1 SIMPLESCAN_IP=192.168.3.111 just test -run TestLiveScan -v
//
// 不写死任何地址——地址来自环境变量，仓库里不带用户局域网信息。

func liveEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("SIMPLESCAN_LIVE") != "1" {
		t.Skip("设置 SIMPLESCAN_LIVE=1 才跑真机测试")
	}
}

func TestLiveDiscovery(t *testing.T) {
	liveEnabled(t)
	ctx, cancel := context.WithTimeout(context.Background(), discoverWait+2*time.Second)
	defer cancel()
	devs, err := Discover(ctx)
	if err != nil {
		t.Fatalf("discovery: %v", err)
	}
	if len(devs) == 0 {
		t.Fatal("没有发现任何 _uscan._tcp 设备")
	}
	for _, d := range devs {
		t.Logf("found: %s  ip=%s port=%d path=%s", d.DisplayName(), d.IP, d.Port, d.Path)
	}
}

func TestLiveCache(t *testing.T) {
	liveEnabled(t)
	ip := os.Getenv("SIMPLESCAN_IP")
	if ip == "" {
		t.Skip("设置 SIMPLESCAN_IP 才跑真机缓存测试")
	}
	withTempCache(t)

	d := DeviceInfo{Name: "live", IP: ip, Port: 80, Path: "eSCL"}
	if err := Probe(context.Background(), &d); err != nil {
		t.Fatalf("probe: %v", err)
	}
	SaveCache([]DeviceInfo{d}, DeviceKey(d))

	got, selected := LoadCache()
	if len(got) != 1 {
		t.Fatalf("loaded %d devices", len(got))
	}
	if selected != DeviceKey(d) {
		t.Errorf("selected = %q, want %q", selected, DeviceKey(d))
	}
	c := got[0]
	if c.Online {
		t.Error("cached online flag must be reset")
	}
	if c.IP != ip || len(c.Colors) == 0 || len(c.Resolutions) == 0 {
		t.Fatalf("cache lost caps: %+v", c)
	}
	if err := Probe(context.Background(), &c); err != nil {
		t.Fatalf("re-probe cached device: %v", err)
	}
	if !c.Online {
		t.Fatal("cached device re-probe failed")
	}
	t.Logf("cache ok: %s colors=%v res=%v intents=%v", c.DisplayName(), c.Colors, c.Resolutions, c.Intents)
}

func TestLiveScan(t *testing.T) {
	liveEnabled(t)
	ip := os.Getenv("SIMPLESCAN_IP")
	if ip == "" {
		t.Skip("设置 SIMPLESCAN_IP 才跑真机扫描")
	}

	// 1) 发现（顺便确认 mDNS 能给出这台设备的端口/路径）
	ctx, cancel := context.WithTimeout(context.Background(), discoverWait+2*time.Second)
	devs, err := Discover(ctx)
	cancel()
	if err != nil {
		t.Logf("discovery 失败（继续用默认 eSCL 路径直连）：%v", err)
	}
	var dev DeviceInfo
	found := false
	for _, d := range devs {
		t.Logf("discovered: %s", d.DisplayName())
		if d.IP == ip {
			dev, found = d, true
		}
	}
	if !found {
		t.Logf("mDNS 没有给出 %s，按默认 80/eSCL 直连", ip)
		dev = DeviceInfo{Name: "live", IP: ip, Port: 80, Path: "eSCL"}
	}

	// 2) 能力
	if err := Probe(context.Background(), &dev); err != nil {
		t.Fatalf("probe: %v", err)
	}
	t.Logf("online=%v name=%q colors=%v sizes=%v res=%v intents=%v feeder=%v",
		dev.Online, dev.Name, dev.Colors, dev.Sizes, dev.Resolutions, dev.Intents, dev.Feeder)

	// 3) 真扫一张（A4 300dpi 彩色玻璃板）
	intent := os.Getenv("SIMPLESCAN_INTENT")
	if intent == "" {
		intent = IntentDocument
	}
	start := time.Now()
	files, err := Scan(context.Background(), &dev, Options{
		Color: ColorModeColor, DPI: 300, Size: PaperA4, Intent: intent,
	}, ScanSink{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("platen scan returned %d pages", len(files))
	}
	img, err := Decode(files[0])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	b := img.Bounds()
	t.Logf("scanned %dx%d in %v (file %s)", b.Dx(), b.Dy(), time.Since(start), files[0])
	if b.Dx() < 2000 || b.Dy() < 2000 {
		t.Fatalf("unexpected page size %dx%d", b.Dx(), b.Dy())
	}
}
