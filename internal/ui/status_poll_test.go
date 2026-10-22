package ui

import (
	"testing"

	"quick-scan/internal/scanner"
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
