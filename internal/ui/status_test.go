package ui

import (
	"strings"
	"testing"

	"quick-scan/internal/scanner"

	"github.com/egoist/mygo/ui"
)

// onlineApp 造一台已探测在线的设备，好让扫描按钮进入可判断状态的路径。
func onlineApp(feeder bool) *app {
	a := newApp()
	a.devices = []scanner.DeviceInfo{{
		Idx: 1, Name: "HP Laser MFP 116w", IP: "192.168.3.111", Port: 80, Path: "eSCL",
		Online: true, Feeder: feeder,
	}}
	a.devSel = 0
	a.probed[1] = true
	a.devState = DevOnline
	a.feeder = feeder
	return a
}

func TestScanBlockedWhilePrinterBusy(t *testing.T) {
	a := onlineApp(false)
	a.devStatus, a.devStatusIdx = scanner.Status{State: "processing"}, 1

	if got := a.scanIssue().Text; got == "" {
		t.Fatal("打印机忙的时候应该拦住扫描")
	}
	tt := ui.NewTester(a.view, 1200, 800)
	if !tt.HasText("打印机正在忙") {
		t.Fatalf("界面没说明为什么不能扫：%v", tt.Texts())
	}
	// 按钮灰了；就算还有别的路径把点击送到这里，startScan 也要再挡一次。
	a.startScan()
	if a.scanning {
		t.Fatal("忙的时候居然开始扫描了")
	}
}

// 卡纸、卡门这类要用户动手的状态用红色提示，忙只是黄色提醒。
func TestScanIssueSeverity(t *testing.T) {
	cases := []struct {
		name   string
		feeder bool
		st     scanner.Status
		text   string
		danger bool
	}{
		{"stopped", false, scanner.Status{State: "stopped"}, "打印机停了", true},
		{"adf jam", true, scanner.Status{State: "idle", ADF: "ScannerAdfJam"}, "进纸口卡纸", true},
		{"adf door", true, scanner.Status{ADF: "ScannerAdfDoorOpen"}, "盖子没关好", true},
		{"busy", false, scanner.Status{State: "processing"}, "正在忙", false},
		{"idle", false, scanner.Status{State: "idle"}, "", false},
	}
	for _, c := range cases {
		a := onlineApp(c.feeder)
		a.devStatus, a.devStatusIdx = c.st, 1
		issue := a.scanIssue()
		if !strings.Contains(issue.Text, c.text) {
			t.Errorf("%s: issue = %q, want %q", c.name, issue.Text, c.text)
		}
		if issue.Danger != c.danger {
			t.Errorf("%s: danger = %v, want %v", c.name, issue.Danger, c.danger)
		}
	}
}

// 进纸口的状态只拦进纸口：玻璃板模式下不该被进纸口的卡纸/空纸挡住。
// 进纸口缺纸尤其不能拦——用户可能只是想用玻璃板。
func TestAdfIssuesOnlyBlockFeeder(t *testing.T) {
	for _, st := range []scanner.Status{
		{State: "idle", ADF: "ScannerAdfEmpty"},
		{State: "idle", ADF: "ScannerAdfJam"},
		{State: "idle", ADF: "ScannerAdfDoorOpen"},
	} {
		a := onlineApp(false)
		a.devStatus, a.devStatusIdx = st, 1
		if got := a.scanIssue().Text; got != "" {
			t.Errorf("玻璃板不该被进纸口状态拦住：%+v → %q", st, got)
		}
	}
}

// 进纸口没纸是扫描时才知道的事实，不该提前拦：按钮/提示都不出现。
func TestAdfEmptyDoesNotBlock(t *testing.T) {
	a := onlineApp(true)
	a.devStatus, a.devStatusIdx = scanner.Status{State: "idle", ADF: "ScannerAdfEmpty"}, 1
	if got := a.scanIssue().Text; got != "" {
		t.Fatalf("进纸口没纸不该拦住扫描：%q", got)
	}
	tt := ui.NewTester(a.view, 1200, 800)
	if tt.HasText("没有纸") {
		t.Fatalf("界面不该提纸的事：%v", tt.Texts())
	}
}

// 设备换了一台，上一台报的"忙"不能继续拦住新设备。
func TestStatusTiedToDevice(t *testing.T) {
	a := onlineApp(false)
	a.devStatus, a.devStatusIdx = scanner.Status{State: "processing"}, 1
	if a.scanIssue().Text == "" {
		t.Fatal("前提不成立：状态没生效")
	}

	a.devices = append(a.devices, scanner.DeviceInfo{
		Idx: 2, Name: "Canon", IP: "192.168.3.112", Port: 80, Path: "eSCL", Online: true,
	})
	a.devSel = 1
	a.refreshState()
	if got := a.scanIssue().Text; got != "" {
		t.Fatalf("换设备后还带着上一台的状态：%q", got)
	}
	if text, _, _ := a.printerState(); text != "" {
		t.Fatalf("状态栏还显示上一台状态：%q", text)
	}
}

// 有些设备不报 ScannerStatus：读不到状态不能把按钮锁死。
func TestUnknownStatusDoesNotBlock(t *testing.T) {
	a := onlineApp(false)
	if got := a.scanIssue().Text; got != "" {
		t.Fatalf("没读到状态就不该拦：%q", got)
	}
	if text, _, _ := a.printerState(); text != "" {
		t.Fatalf("没读到状态就别编一个：%q", text)
	}
}

func TestStatusBarShowsPrinterState(t *testing.T) {
	a := onlineApp(false)
	a.devStatus, a.devStatusIdx = scanner.Status{State: "idle"}, 1
	tt := ui.NewTester(a.view, 1200, 800)
	if !tt.HasText("空闲") {
		t.Fatalf("状态栏没显示打印机状态：%v", tt.Texts())
	}

	a.devStatus = scanner.Status{State: "processing"}
	tt.Frame()
	if !tt.HasText("正在忙") || tt.HasText("空闲") {
		t.Fatalf("状态栏没跟着变：%v", tt.Texts())
	}
}
