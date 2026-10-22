package ui

import (
	"context"
	"time"

	"quick-scan/internal/scanner"

	"github.com/egoist/mygo/ui"
)

// 状态只由设备自己上报，所以拿它没有别的办法，只能轮询。
// 2 秒一次：一次小的 XML 请求，换来"点扫描之前就知道忙不忙"。
const statusPollInterval = 2 * time.Second

type statusTarget struct {
	idx int // 0 = 这轮没有可查的设备
	dev *scanner.DeviceInfo
}

// startStatusPoll 起一条独立的轮询协程。它和探测、扫描流程互不相干：
// 扫描途中也照常刷新，扫描收尾时不会留下一个过期的"忙"把按钮锁死。
func (a *app) startStatusPoll() {
	if Win == nil {
		return // 单元测试不跑后台轮询
	}
	a.statusKick = make(chan struct{}, 1)
	go a.statusLoop()
}

// requestStatusRefresh 请轮询协程立刻查一次（探测成功、换设备、扫完之后）。
func (a *app) requestStatusRefresh() {
	if a.statusKick == nil {
		return
	}
	select {
	case a.statusKick <- struct{}{}:
	default: // 已经有一单在排队，不用重复
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

// pollStatusOnce 在轮询协程里跑：向主线程要一份当前设备，查完再交回主线程。
// 状态是主线程的数据，网络请求不能碰它。
func (a *app) pollStatusOnce() {
	got := make(chan statusTarget, 1)
	Win.Update(func() {
		d := a.current()
		if d == nil || a.devState != DevOnline {
			got <- statusTarget{}
			return
		}
		// 只带端点：状态查询不需要能力，也不该把这个指针带到别的线程去读。
		got <- statusTarget{idx: d.Idx, dev: &scanner.DeviceInfo{
			Idx: d.Idx, Name: d.Name, IP: d.IP, Port: d.Port, Path: d.Path,
		}}
	})

	var tg statusTarget
	select {
	case tg = <-got:
	case <-time.After(statusPollInterval): // 主线程正忙：这一轮跳过，下一轮再来
		return
	}
	if tg.dev == nil {
		return
	}

	st, err := scanner.FetchStatus(context.Background(), tg.dev)
	Win.Update(func() {
		// 查的是哪台就写哪台：期间换了设备，这份结果作废。
		cur := a.current()
		if cur == nil || cur.Idx != tg.idx {
			return
		}
		if err != nil {
			// 读不到不等于不能扫：清空状态，按钮不受影响。
			a.devStatus, a.devStatusIdx = scanner.Status{}, 0
			return
		}
		a.devStatus, a.devStatusIdx = st, tg.idx
	})
}

// statusOf 只认当前这台设备的状态：换过设备就不能拿上一台的状态冒充。
func (a *app) statusOf(d *scanner.DeviceInfo) scanner.Status {
	if d == nil || a.devStatusIdx != d.Idx {
		return scanner.Status{}
	}
	return a.devStatus
}

type scanIssue struct {
	Text   string
	Danger bool // 卡纸、停住这类要用户动手的用红色；忙只是提醒
}

// scanIssue 说明此刻为什么不能扫描。只认和扫描直接相关的状态：
// 进纸口缺纸不算——那会把玻璃板扫描一起挡住，用户也想不通。
// 状态没读到就不拦：有些设备不报 ScannerStatus，不能因此把按钮锁死。
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

// printerState 状态栏里的打印机状态。没读到就返回空，不编一个。
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
