package ui

import (
	"errors"
	"fmt"
	"testing"

	"quick-scan/internal/scanner"

	"github.com/egoist/mygo/ui"
)

func TestStartStatusPollIsIdempotent(t *testing.T) {
	a := newApp()
	a.startStatusPoll()
	if a.statusKick != nil {
		t.Fatal("没有窗口时不该起轮询")
	}

	a.statusKick = make(chan struct{}, 1)
	orig := a.statusKick
	a.startStatusPoll()
	if a.statusKick != orig {
		t.Fatal("重复启动换掉了通道，旧协程会没人能停")
	}
}

// 状态按 Idx 归属：上一台报的忙不能拦住新设备。
func TestStatusResultDiscardedForOtherDevice(t *testing.T) {
	a := onlineApp(false)
	a.devices = append(a.devices, scanner.DeviceInfo{
		Idx: 2, Name: "Canon", IP: "192.168.3.112", Port: 80, Path: "eSCL", Online: true,
	})
	a.devSel = 1
	a.refreshState()

	// 第一台的结果迟到。
	a.devStatus, a.devStatusIdx = scanner.Status{State: "processing"}, 1
	if got := a.scanIssue().Text; got != "" {
		t.Fatalf("旧设备的状态漏到了新设备上：%q", got)
	}

	// 是隔离不是丢弃：换回去原状态还在。
	a.devSel = 0
	a.refreshState()
	if got := a.scanIssue().Text; got == "" {
		t.Fatal("换回原设备后状态不该丢")
	}
}

// 打印机脱机：ScannerStatus 连着几次都没应答，就该当场变红、灰掉扫描按钮。
func TestStatusMissMarksDeviceOffline(t *testing.T) {
	a := onlineApp(false)
	a.devStatus, a.devStatusIdx = scanner.Status{State: "idle"}, 1
	unreachable := fmt.Errorf("%w：dial tcp 192.168.3.111: i/o timeout", scanner.ErrUnreachable)

	a.applyStatusResult(1, scanner.Status{}, unreachable)
	if a.devState != DevOnline || a.scanIssue().Text != "" {
		t.Fatal("一次没应答不该当场判脱机")
	}
	a.applyStatusResult(1, scanner.Status{}, unreachable)
	if a.devState != DevOffline {
		t.Fatalf("连续没应答后应该脱机，实际 %v", a.devState)
	}
	if a.devices[0].Online {
		t.Fatal("脱机后设备还标着在线")
	}
	tt := ui.NewTester(a.view, 1200, 800)
	if !tt.HasText("连不上") {
		t.Fatalf("状态栏没变成连不上：%v", tt.Texts())
	}
	if !tt.HasText("这台打印机连不上") {
		t.Fatalf("没告诉用户为什么扫描按钮灰了：%v", tt.Texts())
	}
	a.startScan()
	if a.scanning {
		t.Fatal("脱机时居然开始扫描了")
	}
}

// 设备只是这个端点报错（403/500）不算脱机：它还活着，能力也还在。
func TestStatusHTTPErrorKeepsDeviceOnline(t *testing.T) {
	a := onlineApp(false)
	for range statusMissLimit + 2 {
		a.applyStatusResult(1, scanner.Status{}, errors.New("设备状态返回 HTTP 500"))
	}
	if a.devState != DevOnline {
		t.Fatalf("HTTP 报错不该判脱机：%v", a.devState)
	}
}

// 脱机后设备又应答：状态恢复，并重新探测把能力补回来。
func TestStatusRecoversAfterOffline(t *testing.T) {
	a := onlineApp(false)
	for range statusMissLimit {
		a.applyStatusResult(1, scanner.Status{}, fmt.Errorf("%w：timeout", scanner.ErrUnreachable))
	}
	if a.devState != DevOffline {
		t.Fatal("前提不成立：设备没被判脱机")
	}
	a.applyStatusResult(1, scanner.Status{State: "idle"}, nil)
	if a.devStatusIdx != 1 || !a.devStatus.Known() {
		t.Fatalf("恢复后状态没落上：%+v idx=%d", a.devStatus, a.devStatusIdx)
	}
}

// 扫描中途断线不在这里改状态：按钮已经变成「停止」，结果由扫描自己报。
func TestStatusMissIgnoredWhileScanning(t *testing.T) {
	a := onlineApp(false)
	a.scanning = true
	for range statusMissLimit {
		a.applyStatusResult(1, scanner.Status{}, fmt.Errorf("%w：timeout", scanner.ErrUnreachable))
	}
	if a.devState != DevOnline {
		t.Fatal("扫描中不该被轮询判脱机")
	}
}
