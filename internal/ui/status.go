package ui

import (
	"context"
	"time"

	"quick-scan/internal/scanner"

	"github.com/egoist/mygo/ui"
)

const statusPollInterval = time.Second

// 必须比轮询间隔宽，否则设备偶尔慢一点就被当成"读不到"。
const statusTimeout = 2 * time.Second

type statusTarget struct {
	idx int
	dev *scanner.DeviceInfo
}

// startStatusPoll 起一条独立的轮询协程：扫描卡住时它也照常刷新。
func (a *app) startStatusPoll() {
	if Win == nil {
		return // 单元测试不跑后台轮询
	}
	if a.statusKick != nil {
		return // 再起一条就没人能停旧的了
	}
	a.statusKick = make(chan struct{}, 1)
	go a.statusLoop()
}

func (a *app) requestStatusRefresh() {
	if a.statusKick == nil {
		return
	}
	select {
	case a.statusKick <- struct{}{}:
	default: // 已有一单排队
	}
}

func (a *app) statusLoop() {
	tick := time.NewTicker(statusPollInterval)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
		case <-a.statusKick:
		}
		a.pollStatusOnce()
	}
}

// pollStatusOnce 在轮询协程里跑：状态归主线程所有，网络请求不能碰它。
func (a *app) pollStatusOnce() {
	got := make(chan statusTarget, 1)
	Win.Update(func() {
		d := a.current()
		if d == nil || a.devState != DevOnline {
			got <- statusTarget{}
			return
		}
		// 只带端点：跨线程传的指针不能指向主线程还会改的结构。
		got <- statusTarget{idx: d.Idx, dev: &scanner.DeviceInfo{
			Idx: d.Idx, Name: d.Name, IP: d.IP, Port: d.Port, Path: d.Path,
		}}
	})

	var tg statusTarget
	select {
	case tg = <-got:
	case <-time.After(statusPollInterval): // 主线程正忙，跳过本轮
		return
	}
	if tg.dev == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()
	st, err := scanner.FetchStatus(ctx, tg.dev)
	Win.Update(func() {
		cur := a.current()
		if cur == nil || cur.Idx != tg.idx {
			return // 期间换了设备
		}
		if err != nil {
			a.devStatus, a.devStatusIdx = scanner.Status{}, 0
			return
		}
		a.devStatus, a.devStatusIdx = st, tg.idx
	})
}

func (a *app) statusOf(d *scanner.DeviceInfo) scanner.Status {
	if d == nil || a.devStatusIdx != d.Idx {
		return scanner.Status{}
	}
	return a.devStatus
}

type scanIssue struct {
	Text   string
	Danger bool // 要用户动手的（卡纸、停住）用红色；忙只是提醒
}

// scanIssue 说明此刻为什么不能扫描。
// 进纸口缺纸不在此列：它会把玻璃板扫描一起挡住。
// 状态没读到就不拦——有的设备根本不报 ScannerStatus。
func (a *app) scanIssue() scanIssue {
	if a.devState != DevOnline {
		return scanIssue{}
	}
	st := a.statusOf(a.current())
	switch {
	case st.Stopped():
		return scanIssue{"打印机停了，看看是不是卡纸或盖子没关好。", true}
	case a.feeder && st.ADFJam():
		return scanIssue{"进纸口卡纸了，把纸取出来再扫描。", true}
	case a.feeder && st.ADFDoorOpen():
		return scanIssue{"进纸口的盖子没关好，盖好再扫描。", true}
	case st.Busy():
		return scanIssue{"打印机正在忙，等它扫完再点扫描。", false}
	}
	return scanIssue{}
}

// printerState 状态栏里的打印机状态；没读到就返回空。
func (a *app) printerState() (text string, fg, bg ui.Color) {
	if a.devState != DevOnline {
		return "", colMuted, ui.Transparent
	}
	st := a.statusOf(a.current())
	switch {
	case st.Stopped():
		return "需要检查", colDanger, colDangerSoft
	case a.feeder && st.ADFJam():
		return "进纸口卡纸", colDanger, colDangerSoft
	case a.feeder && st.ADFDoorOpen():
		return "进纸口没关好", colDanger, colDangerSoft
	case st.Busy():
		return "正在忙", colWarn, colWarnSoft
	case st.Known():
		return "空闲", colMuted, colPanelSoft
	}
	return "", colMuted, ui.Transparent
}
