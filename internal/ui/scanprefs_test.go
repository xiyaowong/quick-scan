package ui

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"quick-scan/internal/platform"
	"quick-scan/internal/scanner"
)

func TestRestoreScanPrefs(t *testing.T) {
	a := newApp()
	a.restoreScanPrefs(&platform.ScanPrefs{Color: 1, Size: 2, DPI: 600, Intent: "Photo", Feeder: true, EnhanceLevel: 2})
	if a.color != ColorGray || a.size != SizeLetter || a.dpi != 600 || a.intent != "Photo" || !a.feeder ||
		a.enhanceLevel != scanner.EnhanceStrong {
		t.Fatalf("恢复后的设置不对: %+v", a)
	}
}

// 损坏的记录宁可丢掉也不能带出非法枚举值。
func TestRestoreScanPrefsIgnoresInvalid(t *testing.T) {
	a := newApp()
	a.restoreScanPrefs(&platform.ScanPrefs{Color: 9, Size: -1, DPI: 0, Intent: "", EnhanceLevel: 9})
	if a.color != ColorColor || a.size != SizeA4 || a.dpi != 300 || a.intent != scanner.IntentDocument || a.feeder ||
		a.enhanceLevel != scanner.EnhanceStandard {
		t.Fatalf("非法值没有回退到默认: %+v", a)
	}
}

// 真正扫过才保存：startScan 里落盘，读回来要和当时一致。
// Win 在测试里是 nil，Update 只把回调排进不会执行的事件队列，安全。
func TestStartScanSavesScanPrefs(t *testing.T) {
	withTempCache(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	_, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	a := newApp()
	a.devices = []scanner.DeviceInfo{{Idx: 1, Name: "假设备", IP: "127.0.0.1", Port: port, Path: "eSCL", Online: true}}
	a.devSel = 0
	a.probed[1] = true
	a.devState = DevOnline
	a.color = ColorBW
	a.size = SizeA5
	a.dpi = 150
	a.intent = "Photo"
	a.feeder = true
	a.enhance = false
	a.enhanceLevel = scanner.EnhanceLight

	a.startScan()

	got := platform.LoadScanPrefs()
	if got == nil {
		t.Fatal("startScan 没把扫描设置写盘")
	}
	if got.Color != int(ColorBW) || got.Size != int(SizeA5) || got.DPI != 150 || got.Intent != "Photo" || !got.Feeder {
		t.Fatalf("写盘的设置和当时的界面不一致: %+v", got)
	}
	if !got.NoEnhance {
		t.Fatal("关掉自动优化后，记录里没有保留原样")
	}
	if got.EnhanceLevel != int(scanner.EnhanceLight) {
		t.Fatalf("优化强度没有写盘: %+v", got)
	}
}

// 旧记录没有 keepBackground 字段：新增开关不能把老用户的默认改成关。
func TestRestoreScanPrefsDefaultsEnhanceOn(t *testing.T) {
	a := newApp()
	a.restoreScanPrefs(&platform.ScanPrefs{})
	if !a.enhance || a.enhanceLevel != scanner.EnhanceStandard {
		t.Fatal("旧记录恢复后没有回到默认优化和标准强度")
	}
	a.restoreScanPrefs(&platform.ScanPrefs{NoEnhance: true})
	if a.enhance {
		t.Fatal("记录里关掉了优化，恢复后却还开着")
	}
}

// 换到没有进纸口的设备时校准要关掉 feeder；有的设备要保留。
func TestApplyDefaultsKeepsFeederWhenSupported(t *testing.T) {
	a := newApp()
	a.feeder = true
	a.devices = []scanner.DeviceInfo{{Name: "无进纸口", Online: true}}
	a.applyDefaults(0)
	if a.feeder {
		t.Fatal("设备没有进纸口，校准后 feeder 还是 true")
	}

	a.feeder = true
	a.devices = []scanner.DeviceInfo{{Name: "有进纸口", Online: true, Feeder: true}}
	a.applyDefaults(0)
	if !a.feeder {
		t.Fatal("设备有进纸口，校准不该改掉用户的 feeder 选择")
	}
}
