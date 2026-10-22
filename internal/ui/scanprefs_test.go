package ui

import (
	"testing"

	"quick-scan/internal/platform"
	"quick-scan/internal/scanner"
)

func TestRestoreScanPrefs(t *testing.T) {
	a := newApp()
	a.restoreScanPrefs(&platform.ScanPrefs{Color: 1, Size: 2, DPI: 600, Intent: "Photo", Feeder: true})
	if a.color != ColorGray || a.size != SizeLetter || a.dpi != 600 || a.intent != "Photo" || !a.feeder {
		t.Fatalf("恢复后的设置不对: %+v", a)
	}
}

// 损坏的记录宁可丢掉也不能带出非法枚举值。
func TestRestoreScanPrefsIgnoresInvalid(t *testing.T) {
	a := newApp()
	a.restoreScanPrefs(&platform.ScanPrefs{Color: 9, Size: -1, DPI: 0, Intent: ""})
	if a.color != ColorColor || a.size != SizeA4 || a.dpi != 300 || a.intent != scanner.IntentDocument || a.feeder {
		t.Fatalf("非法值没有回退到默认: %+v", a)
	}
}

func TestSaveScanPrefsRoundTrip(t *testing.T) {
	a := newApp()
	a.color = ColorBW
	a.size = SizeA5
	a.dpi = 150
	a.intent = "Photo"
	a.feeder = true
	a.saveScanPrefs()

	b := newApp()
	if s := platform.LoadScanPrefs(); s != nil {
		b.restoreScanPrefs(s)
	}
	if b.color != ColorBW || b.size != SizeA5 || b.dpi != 150 || b.intent != "Photo" || !b.feeder {
		t.Fatalf("存取一轮后设置变了: %+v", b)
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
