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

	// 收起扫描态、模拟收尾：停止后该回到扫描按钮。
	a.scanning = false
	a.scanStatus = "已停止。"
	a.scanResult = true
	tt2 := ui.NewTester(a.view, 1200, 800)
	if !tt2.HasText("扫　描") {
		t.Fatalf("停止后该回到扫描按钮：%v", tt2.Texts())
	}
	if !tt2.HasText("已停止。") {
		t.Fatalf("停止结果该显示出来：%v", tt2.Texts())
	}
}

// 停止按钮和扫描按钮是同一位置的同类元素：不换 Key 会继承点击态，
// 双击「扫描」的第二下会落到停止上，把刚启动的扫描停掉。
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
	if a.stopRequested {
		t.Fatal("扫描刚启动就被停掉了：两个按钮共用了元素身份")
	}
	a.stopScan() // 收尾，别让后台 goroutine 拖着
}
