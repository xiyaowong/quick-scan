// 设备检测与扫描编排：发现走 mDNS，探测和扫描走 eSCL。
package ui

import (
	"context"
	"sort"

	"quick-scan/internal/platform"
	"quick-scan/internal/scanner"

	"github.com/egoist/mygo/ui"
)

func (a *app) startup() {
	if devs, selected := scanner.LoadCache(); len(devs) > 0 {
		a.applyCached(devs, selected)
		a.probeCached()
	}
	a.startDetect()
	a.startStatusPoll()
}

// netScanHint 让用户知道还在找，否则界面看起来像卡住了。
const netScanHint = "正在扫描局域网 …"

// 只发现、不探测能力。
// mDNS 没应答且没有能用的设备时，才兜底扫网段——扫描是为了找，不该每次启动都跑。
func (a *app) startDetect() {
	if a.detecting || a.netScanning {
		return
	}
	a.detecting = true
	if len(a.devices) == 0 {
		a.hint = "正在检测设备 …"
	}
	Win.Update(func() {})
	go func() {
		devs, mdnsErr := scanner.Discover(context.Background())
		Win.Update(func() {
			a.detecting = false
			a.applyDiscovery(devs, mdnsErr)
			if len(devs) == 0 && !a.anyDeviceOnline() {
				a.startSubnetScan(mdnsErr)
			}
		})
	}()
}

// mDNS 报错时先把提示留着，网段扫描还能兜底。
func (a *app) applyDiscovery(found []scanner.DeviceInfo, mdnsErr error) {
	if mdnsErr != nil {
		if len(a.devices) == 0 {
			a.applyDetect(nil)
		}
		return
	}
	if len(a.devices) > 0 {
		a.mergeDetected(found)
		return
	}
	a.applyDetect(found)
	a.hint = ""
	if len(found) > 0 {
		a.probeFirstOnline()
	}
}

// 有能用的设备就不扫，否则每次启动都会白扫一遍 254 个地址。
func (a *app) anyDeviceOnline() bool {
	for i := range a.devices {
		if a.devices[i].Online {
			return true
		}
	}
	return false
}

func (a *app) startSubnetScan(mdnsErr error) {
	if a.netScanning {
		return
	}
	a.netScanning = true
	if len(a.devices) == 0 {
		a.hint = netScanHint
	}
	Win.Update(func() {})
	go func() {
		scanErr := scanner.ScanSubnet(context.Background(), func(d scanner.DeviceInfo) {
			Win.Update(func() { a.addDiscovered(d) })
		})
		Win.Update(func() {
			a.netScanning = false
			if a.hint == netScanHint {
				a.hint = ""
			}
			if len(a.devices) == 0 {
				switch {
				case scanErr != nil:
					a.hint = scanErr.Error()
				case mdnsErr != nil:
					a.hint = mdnsErr.Error()
				}
			}
		})
	}()
}

// 只负责加进名单；探测由 probeCached / probeFirstOnline 触发。
func (a *app) addDiscovered(d scanner.DeviceInfo) {
	key := scanner.DeviceKey(d)
	if key == "" {
		return
	}
	if i := a.indexByKey(key); i >= 0 {
		if d.Name != "" {
			a.devices[i].Name = d.Name
		}
		return
	}
	d.Idx = a.allocDevIdx()
	d.Online = false
	a.devices = append(a.devices, d)
	if a.hint == netScanHint {
		a.hint = ""
	}
	a.refreshState()
	a.sortDevices()
	a.saveCache()
	if !a.probing && !a.userPicked && a.devState != DevOnline {
		a.probeFirstUnprobed()
	}
}

func (a *app) recheckAll() {
	a.userPicked = false
	a.autoQueue = nil
	for i := range a.devices {
		delete(a.probed, a.devices[i].Idx)
		a.devices[i].Online = false
	}
	a.refreshState()
	a.sortDevices()
	a.startDetect()
	a.probeFirstUnprobed()
}

// 在途的探测按代际号作废——旧结果回来时自己丢弃，不用谁去追。
func (a *app) resetDevices(devs []scanner.DeviceInfo) {
	a.devGen++
	a.probeSeq++
	a.devices = nil
	a.probed = map[int]bool{}
	a.autoQueue = nil
	a.pendingProbe = false
	a.probing = false
	a.probingIdx = 0
	a.userPicked = false
	a.nextDevIdx = 0
	a.devSel = 0
	for _, d := range devs {
		d.Idx = a.allocDevIdx()
		d.Online = false
		a.devices = append(a.devices, d)
	}
	a.hint = ""
}

func (a *app) applyCached(devs []scanner.DeviceInfo, selected string) {
	a.resetDevices(devs)
	if selected != "" {
		if i := a.indexByKey(selected); i >= 0 {
			a.devSel = i
		}
	}
	a.refreshState()
}

// 先验证上次选中的设备，失败就按名单顺序继续，直到找到在线的。
func (a *app) probeCached() {
	if len(a.devices) == 0 {
		return
	}
	a.autoQueue = nil
	for i, d := range a.devices {
		if i != a.devSel {
			a.autoQueue = append(a.autoQueue, d.Idx)
		}
	}
	a.requestProbe(a.devices[a.devSel].Idx, true)
}

// 同端点保留 Idx 和能力；既没发现也没连上的缓存条目删掉。
func (a *app) mergeDetected(found []scanner.DeviceInfo) {
	// mDNS 一个都没回可能是组播被防火墙挡了，不等于设备不存在，保留缓存名单。
	if len(found) == 0 {
		a.refreshState()
		a.sortDevices()
		a.saveCache()
		return
	}
	selIdx := -1
	if a.devSel >= 0 && a.devSel < len(a.devices) {
		selIdx = a.devices[a.devSel].Idx
	}
	byKey := make(map[string]int, len(a.devices))
	for i := range a.devices {
		byKey[scanner.DeviceKey(a.devices[i])] = i
	}
	seen := make(map[string]bool, len(found))
	var added []int
	for _, d := range found {
		key := scanner.DeviceKey(d)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if i, ok := byKey[key]; ok {
			if d.Name != "" {
				a.devices[i].Name = d.Name // mDNS 的 ty 比旧缓存可靠
			}
			continue
		}
		d.Idx = a.allocDevIdx()
		d.Online = false
		a.devices = append(a.devices, d)
		byKey[key] = len(a.devices) - 1
		added = append(added, d.Idx)
	}

	kept := a.devices[:0]
	for _, d := range a.devices {
		// 正在探测的缓存设备先留着：结果回来再决定去留。
		if seen[scanner.DeviceKey(d)] || (a.probed[d.Idx] && d.Online) || d.Idx == a.probingIdx {
			kept = append(kept, d)
		}
	}
	a.devices = kept

	// 正在接力探测的话，把新发现的设备插到队首：它们刚被 mDNS 证实存在。
	if a.probing && !a.userPicked && len(added) > 0 {
		a.autoQueue = append(append([]int(nil), added...), a.autoQueue...)
	}
	if selIdx >= 0 {
		if i := a.listIndex(selIdx); i >= 0 {
			a.devSel = i
		}
	}
	if a.devSel >= len(a.devices) {
		a.devSel = 0
	}
	a.refreshState()
	a.sortDevices()
	a.saveCache()
	// 缓存设备全部失败/还没探到：继续验证发现列表里的下一台。
	if !a.probing && !a.userPicked && a.devState != DevOnline {
		a.probeFirstUnprobed()
	}
}

func (a *app) probeFirstUnprobed() bool {
	if a.probing || a.userPicked {
		return false
	}
	for i := range a.devices {
		if !a.probed[a.devices[i].Idx] {
			a.requestProbe(a.devices[i].Idx, true)
			return true
		}
	}
	return false
}

// 同一时刻只允许一个探测在飞。身份一律用 DeviceInfo.Idx：
// 列表会按在线状态重排，而设备可能重名。
func (a *app) probeSelected() {
	a.autoQueue = nil
	a.userPicked = true
	if a.devSel < 0 || a.devSel >= len(a.devices) {
		return
	}
	if a.probed[a.devices[a.devSel].Idx] {
		// 已经探过：清掉上一台留下的提示，并把参数校准到这台设备。
		if !a.probing {
			a.hint = ""
		}
		a.applyDefaults(a.devSel)
		a.refreshState()
		a.saveCache()
		return
	}
	a.requestProbe(a.devices[a.devSel].Idx, false)
}

func (a *app) probeFirstOnline() {
	if len(a.devices) == 0 {
		return
	}
	a.autoQueue = make([]int, 0, len(a.devices))
	for _, d := range a.devices[1:] {
		a.autoQueue = append(a.autoQueue, d.Idx)
	}
	a.requestProbe(a.devices[0].Idx, true)
}

// auto=true 表示接力，用户手动选择后作废。
func (a *app) requestProbe(idx int, auto bool) {
	if Win == nil { // 单元测试/无窗口环境：不发起真实网络探测
		return
	}
	if a.listIndex(idx) < 0 {
		a.autoQueue = nil
		return
	}
	if a.probing {
		if !auto {
			a.pendingProbe = true // 用户选的这台优先，当前这台回来就换上
		}
		return
	}
	a.probing = true
	a.probingIdx = idx
	a.probeSeq++
	seq := a.probeSeq
	cur := a.listIndex(idx)
	// 接力要一台台试，必须说清在等谁，否则又变成"卡住了？"。
	a.hint = "正在连接 " + a.devices[cur].DisplayName() + " …"
	gen := a.devGen
	dev := a.devices[cur] // 拷贝：探测在后台写，结果回主线程再落进状态
	Win.Update(func() {})
	go func() {
		err := scanner.Probe(context.Background(), &dev)
		Win.Update(func() {
			// 已有更新的探测接管：这单结果不能写回状态，也不能清掉 a.probing。
			if seq != a.probeSeq {
				return
			}
			a.probing = false
			a.probingIdx = 0
			if gen != a.devGen {
				a.autoQueue = nil
				return // 设备列表已重建（重新检测），过期结果整批丢弃
			}
			cur := a.listIndex(idx)
			if cur < 0 { // 设备没了
				a.autoQueue = nil
				return
			}
			a.probed[idx] = true
			ok := err == nil
			a.hint = ""
			if !ok {
				// 明确失败：能力留空，refreshState 会标红；接力中提示会被下一次覆盖。
				dev.Online = false
				a.hint = err.Error()
			}
			dev.Idx = idx
			dev.Name = a.devices[cur].Name // 名单里的名字更短
			a.devices[cur] = dev
			// 接力的落点：这就是"第一台在线的"，选中它并停
			if ok && auto && !a.userPicked {
				a.devSel = cur
				a.autoQueue = nil
			}
			if ok && a.devSel == cur {
				a.applyDefaults(cur)
			}
			a.refreshState()
			a.sortDevices()
			a.saveCache()
			if ok {
				a.requestStatusRefresh() // 刚连上先报一次，不等下一轮
			}
			a.advanceProbe(auto && !ok)
		})
	}()
}

// 用户中途选的优先，否则接力下一台。
func (a *app) advanceProbe(failed bool) {
	if a.pendingProbe {
		a.pendingProbe = false
		a.probeSelected()
		return
	}
	if failed && !a.userPicked {
		if len(a.autoQueue) > 0 {
			idx := a.autoQueue[0]
			a.autoQueue = a.autoQueue[1:]
			a.requestProbe(idx, true)
			return
		}
		// 队列空了但列表里可能还有 mDNS 后补进来的设备。
		if a.probeFirstUnprobed() {
			return
		}
	}
	a.autoQueue = nil
}

func (a *app) listIndex(idx int) int {
	for i := range a.devices {
		if a.devices[i].Idx == idx {
			return i
		}
	}
	return -1
}

// 在线的排前、未探测的居中、失败的垫底；devSel 跟着设备走。
func (a *app) sortDevices() {
	if len(a.devices) < 2 {
		return
	}
	rank := func(i int) int {
		d := &a.devices[i]
		switch {
		case a.probed[d.Idx] && d.Online:
			return 0 // 在线
		case !a.probed[d.Idx]:
			return 1 // 未探测
		default:
			return 2 // 明确离线
		}
	}
	selIdx := a.devices[a.devSel].Idx
	// order[i] = 排序后第 i 位对应排序前的下标（SliceStable：同档保原序）
	order := make([]int, len(a.devices))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(x, y int) bool { return rank(order[x]) < rank(order[y]) })

	newDevs := make([]scanner.DeviceInfo, len(a.devices))
	for i, old := range order {
		newDevs[i] = a.devices[old]
	}
	a.devices = newDevs
	// 选中项按下标找回。
	if i := a.listIndex(selIdx); i >= 0 {
		a.devSel = i
	}
	a.refreshState()
}

func (a *app) applyDetect(devs []scanner.DeviceInfo) {
	a.resetDevices(devs)
	a.refreshState()
}

// li 是列表位置，不是发现下标。
func (a *app) applyDefaults(li int) {
	if li < 0 || li >= len(a.devices) {
		return
	}
	d := &a.devices[li]
	if !d.ColorOK(ColorColor.key()) {
		if d.ColorOK(ColorGray.key()) {
			a.color = ColorGray
		} else {
			a.color = ColorBW
		}
	}
	if !d.SizeOK(SizeA4.key()) {
		a.size = SizeLetter
	}
	// 清晰度/用途只在当前选择不被支持时才改，保留用户已经选好的值。
	if !d.HasDPI(a.dpi) {
		a.dpi = d.DefaultDPI()
	}
	if !d.HasIntent(a.intent) {
		a.intent = d.DefaultIntent()
	}
	a.feeder = false
}

func (a *app) startScan() {
	if a.scanning || a.restoring || a.devState != DevOnline {
		return
	}
	// 按钮已按状态灰掉；这里再挡一次，防止点击和状态更新抢在同一帧。
	if a.scanIssue().Text != "" {
		return
	}
	a.scanning = true
	a.scanProgress = 0
	if a.feeder {
		a.scanStatus = "进纸口扫描中 …"
	} else {
		a.scanStatus = "玻璃板扫描中 …"
	}
	dev := a.devices[a.devSel]
	opts := scanner.Options{
		Color:  a.color.key(),
		DPI:    a.dpi,
		Size:   a.size.key(),
		Feeder: a.feeder,
		Intent: a.intent,
	}
	_ = platform.EnsurePagesDir()
	dir := platform.SessionDir()
	a.saveCache() // 用户真正用起来的设备，下次启动优先显示

	go func() {
		_, err := scanner.Scan(context.Background(), &dev, opts, scanner.ScanSink{
			Dir: dir,
			Progress: func(f float64) {
				Win.Update(func() { a.scanProgress = f })
			},
			// 每下完一页就登记：整批扫完前崩溃也不丢已扫的页。
			OnPage: func(path string) {
				p, perr := makePage(path, 0)
				Win.Update(func() {
					if perr != nil {
						a.scanStatus = "扫描结果读取失败：" + perr.Error()
						return
					}
					a.appendPage(p)
				})
			},
		})
		Win.Update(func() {
			a.scanning = false
			if err != nil {
				a.scanStatus = err.Error()
			}
			a.requestStatusRefresh() // 扫完立刻回读，别停在扫描中的"忙"上
		})
	}()
}

// 解码耗时，调用方负责放到后台线程。
func makePage(path string, rot int) (Page, error) {
	full, err := scanner.Decode(path)
	if err != nil {
		return Page{}, err
	}
	src := thumbRGBA(full, 240)
	p := Page{
		Thumb:    ui.NewBitmap(src),
		ThumbSrc: src,
		Rot:      rot,
		FullPath: path,
	}
	if rot%360 != 0 {
		p.ThumbRot = thumbnail(scanner.Rotate90(src, rot), 240)
	}
	return p, nil
}

// 只能在主线程调：登记一页就立刻写会话清单，崩溃后能恢复到这里。
func (a *app) appendPage(p Page) {
	p.ID = a.nextPageID
	a.nextPageID++
	a.pages = append(a.pages, p)
	// 缩略图当场出现、页数在左栏标题里，不必再用文字报一遍
	a.scanStatus = ""
	if len(a.pages) == 1 {
		a.startOCRSuggest()
	}
	a.saveSession()
}

// 解码放后台，页面按清单顺序一页页出现，不在 UI 线程批量卡住。
func (a *app) restoreSession() {
	pages := a.recoverPages
	a.recoverPages = nil
	a.dlg = DlgNone
	if len(pages) == 0 {
		return
	}
	a.restoring = true
	a.scanStatus = "正在恢复上次的扫描页面 …"
	Win.Update(func() {})
	go func() {
		for _, sp := range pages {
			p, err := makePage(platform.PagePath(sp.File), sp.Rot)
			Win.Update(func() {
				if err == nil {
					a.appendPage(p)
				}
			})
		}
		Win.Update(func() {
			a.restoring = false
			a.scanStatus = ""
			a.saveSession()
		})
	}()
}
