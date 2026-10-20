// 界面状态：设备名单、扫描参数、页面、弹窗与灯箱。
// 所有状态只活在本进程内存里。
package ui

import (
	"sscan/internal/scanner"

	"github.com/egoist/mygo/ui"
)

// ── 枚举 ──

// ColorMode 色彩（界面用大白话）
type ColorMode int

const (
	ColorColor ColorMode = iota // 彩色
	ColorGray                   // 灰度
	ColorBW                     // 黑白
)

func (c ColorMode) label() string {
	switch c {
	case ColorGray:
		return "灰度"
	case ColorBW:
		return "黑白"
	default:
		return "彩色"
	}
}
func (c ColorMode) key() string { // 能力里的色彩键（与驱动无关的界面语义）
	switch c {
	case ColorGray:
		return "gray"
	case ColorBW:
		return "bw"
	default:
		return "color"
	}
}

// PaperSize 纸张大小
type PaperSize int

const (
	SizeA4 PaperSize = iota
	SizeA5
	SizeLetter
)

func (s PaperSize) label() string {
	switch s {
	case SizeA5:
		return "A5（148 × 210 mm）"
	case SizeLetter:
		return "Letter（216 × 279 mm）"
	default:
		return "A4（210 × 297 mm）"
	}
}
func (s PaperSize) key() string {
	switch s {
	case SizeA5:
		return "a5"
	case SizeLetter:
		return "letter"
	default:
		return "a4"
	}
}

// DevState 设备状态：没探测过是 Unknown，不冒充在线。
type DevState int

const (
	DevUnknown DevState = iota // 灰，未探测（detect 只给名单，不连设备）
	DevOffline                 // 红，探测过且明确失败
	DevOnline                  // 绿，探测成功
)

// ExportFormat 导出格式
type ExportFormat int

const (
	ExportPDF ExportFormat = iota // 合并成一个 PDF
	ExportJPG                     // 每页一张 JPG
)

// ── 页面 ──

// Page 一张已扫描的页面
type Page struct {
	ID       uint64     // 全局递增 id（拖拽排序的稳定键）
	Thumb    *ui.Bitmap // 低清缩略图（240px 级）
	ThumbRot *ui.Bitmap // 旋转后的展示位图（Rotate 时重烘；nil = 未旋转）
	Rot      int        // 0/90/180/270
	FullPath string     // 高清原图（%TEMP% 下，灯箱/导出时才读）
}

// 当前应显示的缩略图（含旋转）。Rot==0 时用原缩略图。
func (p *Page) thumb() *ui.Bitmap {
	if p.Rot%360 == 0 {
		return p.Thumb
	}
	if p.ThumbRot == nil {
		return p.Thumb // 旋转缓存尚未生成（兜底）
	}
	return p.ThumbRot
}

// ── 对话框与灯箱 ──

// DialogKind 页面内的弹窗，不走系统消息框。
type DialogKind int

const (
	DlgNone DialogKind = iota
	DlgExport
	DlgQuitConfirm
	DlgClearConfirm
)

// ExportPhase 导出流程状态
type ExportPhase int

const (
	PhaseIdle ExportPhase = iota
	PhaseWorking
	PhaseDone
)

// ExportDone 导出完成信息
type ExportDone struct {
	Display   string // 展示文案
	Dir       string // 真实目录（"打开所在文件夹"用）
	File      string // 真实文件路径，仅单文件导出时有值
	PageCount int
}

// Lightbox 灯箱：看大图、翻页、转正、导出。
type Lightbox struct {
	Index   int
	Full    *ui.Bitmap // 当前显示的大图（已按旋转烘焙）
	Loading bool
	Zoom    float32 // 1.0 = 适配窗口
	PanX    float32 // 放大后拖动查看的偏移
	PanY    float32
	Dirty   bool // 需要重新解码/派生

	// 删除是破坏性操作，先在这一页上问一次，不弹整窗对话框。
	ConfirmDelete bool

	ImgW, ImgH float32 // 原图像素尺寸，用来算 Contain 后真正占的区域
	img        ui.Rect // 上一帧图片容器的区域（灯箱层坐标）
	dragging   bool
	dragIn     bool // 按下时是否落在图片上
	dragMoved  bool
	dragX      float32
	dragY      float32
	panStartX  float32
	panStartY  float32
}

func (lb *Lightbox) resetView() {
	lb.Zoom = 1.0
	lb.PanX, lb.PanY = 0, 0
}

func (lb *Lightbox) zoomBy(f float32) {
	lb.Zoom = clamp(lb.Zoom*f, 1.0, 8.0)
	if lb.Zoom <= 1.0 {
		lb.resetView()
		return
	}
	lb.clampPan()
}

// contentRect 是图片真正画出来的地方。Contain 会在容器里留黑边，
// 只有点在这块区域里才算点在图上，点黑边应该关灯箱。
func (lb *Lightbox) contentRect() ui.Rect {
	box := lb.img
	if box.W <= 0 || box.H <= 0 || lb.ImgW <= 0 || lb.ImgH <= 0 {
		return ui.Rect{}
	}
	if lb.Zoom > 1 {
		return box
	}
	s := min(box.W/lb.ImgW, box.H/lb.ImgH)
	w, h := lb.ImgW*s, lb.ImgH*s
	return ui.Rect{X: box.X + (box.W-w)/2, Y: box.Y + (box.H-h)/2, W: w, H: h}
}

func (lb *Lightbox) clampPan() {
	maxX := (lb.Zoom - 1) * lb.img.W / 2
	maxY := (lb.Zoom - 1) * lb.img.H / 2
	lb.PanX = clamp(lb.PanX, -maxX, maxX)
	lb.PanY = clamp(lb.PanY, -maxY, maxY)
}

// ── 主状态 ──

type app struct {
	// 设备
	devices    []scanner.DeviceInfo
	devSel     int
	nextDevIdx int // 单调递增的设备身份；缓存与 mDNS 合并时保持稳定
	devGen     int // 设备列表代际号：整体重建时 +1，过期的探测结果据此丢弃
	devState   DevState
	probing    bool // 正在探测选中的设备
	probeSeq   int  // 探测序号：列表重建时 +1，旧探测回调据此不动新状态
	probingIdx int  // 正在探测的设备 Idx（0=没有）；合并时别把它当陈旧缓存删掉
	// autoQueue 非空 = 正在"按名单顺序找第一台在线的"接力（见 probeFirstOnline）。
	// 存驱动下标 Idx：列表会被排序重排，Idx 才是稳定句柄（同名设备也分得开）。
	autoQueue []int
	// userPicked 用户手动选过设备：接力让位，不再自动改选中项。重新检测时清零。
	userPicked   bool
	pendingProbe bool         // 探测期间又切了设备：上一个回来后补探
	probed       map[int]bool // 已探测过的设备（键是 scanner.DeviceInfo.Idx，不是列表下标）
	detecting    bool
	devGrid      ui.GridState
	// hint 状态栏右侧唯一的一行提示（检测进度 / 正在连哪台 / 驱动报的失败原因）。
	// 只放"别处看不到"的信息：设备名单在下拉里，状态在灯上，动作在按钮上。
	hint string

	// 扫描参数（界面右侧）
	size   PaperSize
	dpi    int    // 设备支持的清晰度（见 DeviceInfo.DPIs）
	intent string // eSCL Intent，默认 Document
	color  ColorMode
	feeder bool // false=玻璃板 true=进纸口

	// 页面
	pages      []Page
	nextPageID uint64
	pageGrid   ui.GridState

	// 扫描任务
	scanning   bool
	scanStatus string

	// 弹窗
	dlg DialogKind

	// 导出对话框
	exportFmt      ExportFormat
	exportPhase    ExportPhase
	exportDone     ExportDone
	exportFilename string
	exportSingle   int // -1 = 全部页；>=0 = 只导这一页
	lastExportDir  string
	exportPicking  bool
	exportDoneN    int // 导出进度：已完成页数
	exportTotalN   int // 导出进度：总页数

	// 灯箱
	lightbox *Lightbox

	// 清空 / 退出确认
	quitConfirmed bool

	// OCR 文件名候选（预留能力，库后定）
	ocrNames   []string
	ocrRunning bool
	ocrPicked  bool
}

func newApp() *app {
	a := &app{
		devSel:       0,
		devState:     DevUnknown,
		probed:       map[int]bool{},
		size:         SizeA4,
		dpi:          300,
		intent:       scanner.DefaultIntent,
		color:        ColorColor,
		nextPageID:   1,
		exportSingle: -1,
	}
	a.pageGrid.Reorder = func(rows []int, to int) { a.movePages(rows, to) }
	a.pageGrid.Label = func(i int) string { return pageLabel(i) }
	a.devGrid.Label = func(i int) string { return a.devices[i].Name }
	return a
}

func (a *app) current() *scanner.DeviceInfo {
	if a.devSel >= 0 && a.devSel < len(a.devices) {
		return &a.devices[a.devSel]
	}
	return nil
}

// allocDevIdx 分配一个稳定的设备身份。缓存还是 mDNS 来的都走这里，
// 合并时同一个端点保持原 Idx，正在飞的探测结果仍然有效。
func (a *app) allocDevIdx() int {
	a.nextDevIdx++
	return a.nextDevIdx
}

func (a *app) indexByKey(key string) int {
	for i := range a.devices {
		if scanner.DeviceKey(a.devices[i]) == key {
			return i
		}
	}
	return -1
}

func (a *app) saveCache() {
	selected := ""
	if d := a.current(); d != nil {
		selected = scanner.DeviceKey(*d)
	}
	scanner.SaveCache(a.devices, selected)
}

// refreshState 根据当前选中设备的探测结果刷新状态灯。
func (a *app) refreshState() {
	d := a.current()
	if d == nil {
		a.devState = DevUnknown
		return
	}
	if !a.probed[d.Idx] {
		a.devState = DevUnknown
		return
	}
	if d.Online {
		a.devState = DevOnline
	} else {
		a.devState = DevOffline
	}
}

// movePages 拖拽排序：把 rows 里的页整体移到 to 之前
func (a *app) movePages(rows []int, to int) {
	if len(rows) == 0 {
		return
	}
	// 取出要移动的页（保持相对顺序）
	set := map[int]bool{}
	for _, r := range rows {
		set[r] = true
	}
	var moving, rest []Page
	for i, p := range a.pages {
		if set[i] {
			moving = append(moving, p)
		} else {
			rest = append(rest, p)
		}
	}
	// to 是相对原列表的下标；换算成 rest 里的插入位
	insert := to
	for _, r := range rows {
		if r < to {
			insert--
		}
	}
	if insert < 0 {
		insert = 0
	}
	if insert > len(rest) {
		insert = len(rest)
	}
	out := append([]Page{}, rest[:insert]...)
	out = append(out, moving...)
	out = append(out, rest[insert:]...)
	a.pages = out
	// 灯箱下标跟随
	if a.lightbox != nil {
		// 找到灯箱那一页的新下标
		target := -1
		_ = target // 简化：换页后灯箱重新请求即可，下标失配由请求时兜底
		a.lightbox.Dirty = true
	}
}

func pageLabel(i int) string { return "第 " + itoa(i+1) + " 页" }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
