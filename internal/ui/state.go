package ui

import (
	"sscan/internal/scanner"

	"github.com/egoist/mygo/ui"
)

type ColorMode int

const (
	ColorColor ColorMode = iota // 彩色
	ColorGray                   // 灰度
	ColorBW                     // 黑白
)

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

type PaperSize int

const (
	SizeA4 PaperSize = iota
	SizeA5
	SizeLetter
)

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

type ExportFormat int

const (
	ExportPDF ExportFormat = iota // 合并成一个 PDF
	ExportJPG                     // 每页一张 JPG
)

type Page struct {
	ID       uint64     // 全局递增 id（拖拽排序的稳定键）
	Thumb    *ui.Bitmap // 低清缩略图（240px 级）
	ThumbRot *ui.Bitmap // 旋转后的展示位图（Rotate 时重烘；nil = 未旋转）
	Rot      int        // 0/90/180/270
	FullPath string     // 高清原图（%TEMP% 下，灯箱/导出时才读）
}

// 旋转缓存尚未生成时退回原缩略图。
func (p *Page) thumb() *ui.Bitmap {
	if p.Rot%360 == 0 {
		return p.Thumb
	}
	if p.ThumbRot == nil {
		return p.Thumb // 旋转缓存尚未生成（兜底）
	}
	return p.ThumbRot
}

type DialogKind int

const (
	DlgNone DialogKind = iota
	DlgExport
	DlgQuitConfirm
	DlgClearConfirm
)

type ExportPhase int

const (
	PhaseIdle ExportPhase = iota
	PhaseWorking
	PhaseDone
)

type ExportDone struct {
	Display   string // 展示文案
	Dir       string // 真实目录（"打开所在文件夹"用）
	File      string // 真实文件路径，仅单文件导出时有值
	PageCount int
}

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

// contentRect 是图片真正画出来的地方；点黑边应该关灯箱。
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

type app struct {
	devices    []scanner.DeviceInfo
	devSel     int
	nextDevIdx int // 单调递增的设备身份；缓存与 mDNS 合并时保持稳定
	devGen     int // 设备列表代际号：整体重建时 +1，过期的探测结果据此丢弃
	devState   DevState
	probing    bool // 正在探测选中的设备
	probeSeq   int  // 探测序号：列表重建时 +1，旧探测回调据此不动新状态
	probingIdx int  // 正在探测的设备 Idx（0=没有）；合并时别把它当陈旧缓存删掉
	// autoQueue 非空 = 正在按名单顺序找第一台在线的；存 Idx，列表重排后仍稳定。
	autoQueue []int
	// userPicked 用户手动选过设备：接力让位，不再自动改选中项。重新检测时清零。
	userPicked   bool
	pendingProbe bool         // 探测期间又切了设备：上一个回来后补探
	probed       map[int]bool // 已探测过的设备（键是 scanner.DeviceInfo.Idx，不是列表下标）
	detecting    bool
	devGrid      ui.GridState
	// hint 是状态栏右侧唯一的一行提示：检测进度、正在连哪台、失败原因。
	hint string

	size   PaperSize
	dpi    int    // 设备支持的清晰度（见 DeviceInfo.DPIs）
	intent string // eSCL Intent，默认 Document
	color  ColorMode
	feeder bool // false=玻璃板 true=进纸口

	pages      []Page
	nextPageID uint64
	pageGrid   ui.GridState

	scanning   bool
	scanStatus string

	dlg DialogKind

	exportFmt      ExportFormat
	exportPhase    ExportPhase
	exportDone     ExportDone
	exportFilename string
	exportSingle   int // -1 = 全部页；>=0 = 只导这一页
	lastExportDir  string
	exportPicking  bool
	exportDoneN    int // 导出进度：已完成页数
	exportTotalN   int // 导出进度：总页数

	lightbox *Lightbox

	quitConfirmed bool

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

func (a *app) movePages(rows []int, to int) {
	if len(rows) == 0 {
		return
	}
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
	if a.lightbox != nil {
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
