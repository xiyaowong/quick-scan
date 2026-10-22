package ui

import (
	"testing"

	"quick-scan/internal/scanner"
)

// 轮询只能有一条：多起一条就多一倍请求，而且没人能停它。
func TestStartStatusPollIsIdempotent(t *testing.T) {
	a := newApp()
	if a.statusKick != nil {
		t.Fatal("还没启动就有 kick 通道")
	}
	// Win 为 nil 时 startStatusPoll 直接返回：这是单元测试不跑网络的约定。
	a.startStatusPoll()
	if a.statusKick != nil {
		t.Fatal("没有窗口时不该起轮询")
	}

	// 模拟"已经起过"：再调一次不能把现有通道换掉。
	a.statusKick = make(chan struct{}, 1)
	orig := a.statusKick
	a.startStatusPoll()
	if a.statusKick != orig {
		t.Fatal("重复启动换掉了通道，旧轮询协程就再也没人能停")
	}
}

// 换设备后旧结果作废：状态按 Idx 归属，不是当前设备就不认。
// 这条决定了"上一台报的忙"不会拦住新设备。
func TestStatusResultDiscardedForOtherDevice(t *testing.T) {
	a := onlineApp(false)
	a.devices = append(a.devices, scanner.DeviceInfo{
		Idx: 2, Name: "Canon", IP: "192.168.3.112", Port: 80, Path: "eSCL", Online: true,
	})
	a.devSel = 1
	a.refreshState()

	// 第一台的结果迟到（轮询请求发出后用户换了设备）。
	a.devStatus, a.devStatusIdx = scanner.Status{State: "processing"}, 1
	if got := a.scanIssue().Text; got != "" {
		t.Fatalf("旧设备的状态漏到了新设备上：%q", got)
	}

	// 换回第一台，它的状态仍然有效——不是被清掉，只是按 Idx 隔离。
	a.devSel = 0
	a.refreshState()
	if got := a.scanIssue().Text; got == "" {
		t.Fatal("换回原设备后状态应该还在：刷新不该把它当年陈旧数据丢掉")
	}
}

