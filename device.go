// 设备检测与扫描编排。
//
// 全部在网络里完成（eSCL over HTTP），不再有子进程：
// 发现 = mDNS 浏览 _uscan._tcp（discovery.go），探测/扫描 = escl.go。
//
// 打开设备的动作只在 probe（用户选定了某台）和 scan（真要扫）里发生：
// 未选中的设备只出现在名单里，不为它们付连接代价。
package main

import (
	"context"
	"sort"
)

// startup 启动编排：先用缓存把上次的设备立刻显示出来并验证，
// 同时后台跑 mDNS 全量发现；发现结果回来后再无感合并。
func (a *app) startup() {
	if devs, selected := loadDeviceCache(); len(devs) > 0 {
		a.applyCached(devs, selected)
		a.probeCached()
	}
	a.startDetect()
}

// startDetect 后台浏览局域网设备。只发现、不探测能力——能力探测延迟到
// 用户选定设备之后（probeFirstOnline / probeSelected），只为真正要用的设备付费。
// 已有缓存名单时静默刷新：不显示"正在检测"，也不整表重建。
func (a *app) startDetect() {
	if a.detecting {
		return
	}
	a.detecting = true
	if len(a.devices) == 0 {
		a.hint = "正在检测设备 …"
	}
	win.Update(func() {})
	go func() {
		devs, err := discoverDevices(context.Background())
		win.Update(func() {
			a.detecting = false
			if err != nil {
				if len(a.devices) == 0 {
					a.applyDetect(nil)
					a.hint = err.Error()
				}
				return // 有缓存就静默失败，列表继续可用
			}
			if len(a.devices) == 0 {
				a.applyDetect(devs)
				// 名单到手就没什么可说的了：选中项在下拉里看得见，
				// 一台没有时状态栏自己会显示"未检测到设备"
				a.hint = ""
				if len(devs) > 0 {
					a.probeFirstOnline() // 依次试，默认选中第一台能连上的
				}
				return
			}
			a.mergeDetected(devs) // 缓存名单已显示：只合并，不打断
		})
	}()
}

// recheckAll 用户点 ⟳：忘掉旧的在线/离线结论，立刻重新验证已知设备，
// 同时再跑一轮 mDNS 发现。与启动路径一样，两件事并行。
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

// applyCached 把缓存名单放进状态，先全部当"未探测"。
// 能力字段仍然可用，所以界面选项立刻就是正确的；在线状态等探测确认。
func (a *app) applyCached(devs []DeviceInfo, selected string) {
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
	for _, d := range devs {
		d.Idx = a.allocDevIdx()
		d.Online = false
		a.devices = append(a.devices, d)
	}
	a.devSel = 0
	if selected != "" {
		if i := a.indexByKey(selected); i >= 0 {
			a.devSel = i
		}
	}
	a.hint = ""
	a.refreshState()
}

// probeCached 立刻验证缓存里的设备：先探上次选中的那台，
// 失败就按名单顺序继续，找到第一台在线的就停下（与 probeFirstOnline 同语义）。
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

// mergeDetected 把 mDNS 结果并进已有名单：
//   - 同一端点保留原 Idx、能力与探测结果，只更新名字；
//   - 新设备分配新 Idx，排在后面等待探测；
//   - 缓存里既没被发现、也没连上的端点删除（避免名单越积越脏）。
func (a *app) mergeDetected(found []DeviceInfo) {
	// mDNS 一个都没回，可能是组播被防火墙挡了，不等于设备不存在。
	// 保留缓存名单，在线与否交给 HTTP 探测判断。
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
		byKey[deviceKey(a.devices[i])] = i
	}
	seen := make(map[string]bool, len(found))
	var added []int
	for _, d := range found {
		key := deviceKey(d)
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
		if seen[deviceKey(d)] || (a.probed[d.Idx] && d.Online) || d.Idx == a.probingIdx {
			kept = append(kept, d)
		}
	}
	a.devices = kept

	// 正在接力探测的话，把新发现的设备插到队首：它们刚被 mDNS 证实存在，
	// 比缓存里可能已经离线的旧入口更值得先试。
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

// probeFirstUnprobed 找一台尚未探测的设备去验证。返回是否真的发起了探测。
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

// ── 探测调度 ──
//
// 同一时刻只有一个探测在飞（a.probing）。两个触发点：
//
//	probeFirstOnline —— 检测完，按名单顺序依次试，选中第一台能连上的
//	probeSelected    —— 用户手动选了某台，只探这一台，并立即作废自动接力
//
// 身份一律用 DeviceInfo.Idx（发现下标），不用列表位置也不用名字：
// 列表会被 sortDevices 按在线状态重排，而两台打印机重名是常事。

func (a *app) probeSelected() {
	a.autoQueue = nil
	a.userPicked = true
	if a.devSel < 0 || a.devSel >= len(a.devices) {
		return
	}
	if a.probed[a.devices[a.devSel].Idx] {
		// 已经探过：不重复连。顺手清掉上一台留下的提示（连接中/失败原因），
		// 并把参数校准到这台设备的能力上。
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

// probeFirstOnline 按名单顺序找第一台在线的：失败的往下走，找到就选中它并停下。
// 全部失败则停在最后一台（它的红字提示就是全貌）。
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

// requestProbe 启动一次对设备下标 idx 的探测（忙则排队）。
// auto=true 是接力中的一环：用户一旦手动选择（userPicked）就整条作废。
func (a *app) requestProbe(idx int, auto bool) {
	if win == nil { // 单元测试/无窗口环境：不发起真实网络探测
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
	win.Update(func() {})
	go func() {
		err := probeDevice(context.Background(), &dev)
		win.Update(func() {
			// 更新的探测/整体重建已经接管：这单结果一个字都不能写回状态，
			// 尤其不能把 a.probing 清掉——那会让新探测误以为空闲。
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
			if err != nil {
				dev.Online = false
				// 明确失败：能力留空，refreshState 会标红。
				// 接力中这个提示会被下一次尝试覆盖，全失败时留下的就是最后一条。
				a.hint = err.Error()
			} else {
				// 连上了就把"正在连接 …"擦掉；状态改由状态栏绿点表达，
				// 否则这行字会一直挂在界面上（曾经就是这样卡住的）。
				a.hint = ""
			}
			dev.Idx = idx
			dev.Name = a.devices[cur].Name // 名单里的名字更短
			a.devices[cur] = dev
			ok := err == nil
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
			a.advanceProbe(auto && !ok)
		})
	}()
}

// advanceProbe 上一次探测收尾后决定下一步：用户中途选的优先，否则接力下一台。
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

// listIndex 设备下标 → 当前列表位置（-1 = 列表里没有）
func (a *app) listIndex(idx int) int {
	for i := range a.devices {
		if a.devices[i].Idx == idx {
			return i
		}
	}
	return -1
}

// sortDevices 探测成功的排前面，未探测的居中，明确失败的垫底（同档保持原序）。
// devSel 跟着设备走：先记住选中项的下标，排完再找回来。
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

	newDevs := make([]DeviceInfo, len(a.devices))
	for i, old := range order {
		newDevs[i] = a.devices[old]
	}
	a.devices = newDevs
	// 选中项按下标找回（位置已变，名字可能重复）
	if i := a.listIndex(selIdx); i >= 0 {
		a.devSel = i
	}
	a.refreshState()
}

// applyDetect 探测结果落进状态。此时只有名单没有能力（探测是按需的），
// 默认参数等 probe 拿到能力后再调（applyDefaults）。
// 列表整体重建：代际号 +1，让还在路上的旧探测结果过期作废。
func (a *app) applyDetect(devs []DeviceInfo) {
	a.devGen++
	a.probeSeq++ // 作废还在路上的旧探测
	a.devices = nil
	a.nextDevIdx = 0
	for _, d := range devs {
		d.Idx = a.allocDevIdx()
		d.Online = false
		a.devices = append(a.devices, d)
	}
	a.devSel = 0
	a.probed = map[int]bool{}
	a.autoQueue = nil
	a.pendingProbe = false
	a.probing = false
	a.probingIdx = 0
	a.userPicked = false // 新一轮检测，重新允许自动选中第一台在线的
	a.hint = ""
	a.refreshState()
}

// applyDefaults 按设备能力重置默认扫描参数（能力缺失的项兜底全开）。
// li 是当前列表位置，不是发现下标。
func (a *app) applyDefaults(li int) {
	if li < 0 || li >= len(a.devices) {
		return
	}
	d := &a.devices[li]
	if !d.colorOK(ColorColor) {
		if d.colorOK(ColorGray) {
			a.color = ColorGray
		} else {
			a.color = ColorBW
		}
	}
	if !d.sizeOK(SizeA4) {
		a.size = SizeLetter
	}
	// 清晰度/用途只在当前选择不被支持时才改，保留用户已经选好的值。
	if !d.dpiOK(a.dpi) {
		a.dpi = d.defaultDPI()
	}
	if !d.intentOK(a.intent) {
		a.intent = d.defaultIntent()
	}
	a.feeder = false
}

// startScan 开始一次扫描（后台跑 eSCL）。
func (a *app) startScan() {
	if a.scanning || a.devState != DevOnline {
		return
	}
	a.scanning = true
	if a.feeder {
		a.scanStatus = "进纸口扫描中 …"
	} else {
		a.scanStatus = "玻璃板扫描中 …"
	}
	dev := a.devices[a.devSel]
	opts := ScanOptions{
		Color:  a.color,
		DPI:    a.dpi,
		Size:   a.size,
		Feeder: a.feeder,
		Intent: a.intent,
	}
	_ = ensurePagesDir()
	dir := pagesDir()
	a.saveCache() // 用户真正用起来的设备，下次启动优先显示

	go func() {
		files, err := scanDocuments(context.Background(), &dev, opts, dir)
		win.Update(func() {
			a.scanning = false
			if err != nil {
				a.scanStatus = err.Error()
				return
			}
			for _, f := range files {
				a.addPage(f)
			}
		})
	}()
}

// addPage 扫描完成 → 解码缩略图 → 追加进页面区
func (a *app) addPage(path string) {
	full, err := decodeImage(path)
	if err != nil {
		a.scanStatus = "扫描结果读取失败：" + err.Error()
		return
	}
	p := Page{
		ID:       a.nextPageID,
		Thumb:    thumbnail(full, 240),
		Rot:      0,
		FullPath: path,
	}
	a.pages = append(a.pages, p)
	a.nextPageID++
	// 缩略图当场出现、页数在左栏标题里，不必再用文字报一遍
	a.scanStatus = ""
	if len(a.pages) == 1 {
		a.startOCRSuggest()
	}
}
