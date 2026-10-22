package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 扫描中按钮要是「停止」，停下后能回到「扫描」，并说明保留了几页。
func TestStopButtonDuringScan(t *testing.T) {
	a := onlineApp(false)
	a.scanning = true
	tt := ui.NewTester(a.view, 1200, 800)
	if !tt.HasText("停　止") {
		t.Fatalf("扫描中该显示停止按钮：%v", tt.Texts())
	}

	// 停止已发出但还没收尾：按钮该是「正在停止…」且不可再点。
	a.stopping = true
	tt2 := ui.NewTester(a.view, 1200, 800)
	if !tt2.HasText("正在停止…") {
		t.Fatalf("停止后该有反馈：%v", tt2.Texts())
	}
}

// 扫描刚开始的一下点击不能把扫描停掉：停止按钮就长在「扫描」原来的位置，
// 双击的第二下、按住回车的重复都会落到它上面。
func TestScanClickDoesNotStopScan(t *testing.T) {
	withTempCache(t)
	a := onlineApp(false)
	// 端口 1 必然拒绝连接：扫描立刻失败收尾，不依赖网络和超时。
	a.devices[0].IP, a.devices[0].Port = "127.0.0.1", 1
	tt := ui.NewTester(a.view, 1200, 800)

	if err := tt.Click("扫　描"); err != nil {
		t.Fatal(err)
	}
	if !a.scanning {
		t.Fatal("点击没启动扫描")
	}
	// 同一位置再按一次（双击的后半拍）：不能停。
	if err := tt.Click("停　止"); err == nil {
		if a.stopRequested {
			t.Fatal("双击把刚启动的扫描停掉了")
		}
	}
	a.stopScan() // 收尾，别让后台 goroutine 拖着
}
