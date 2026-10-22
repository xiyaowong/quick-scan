package ui

import (
	"image"
	"path/filepath"

	"quick-scan/internal/platform"
	"quick-scan/internal/scanner"

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
	// ThumbSrc 是 240px 缩略图的像素，旋转时直接转它。转 240px 只要几十微秒，
	// 而为了转缩略图去解码 300dpi 原图要几百毫秒。
	ThumbSrc *image.RGBA
	Rot      int    // 0/90/180/270：只是渲染参数，导出/OCR 时才烘进像素
	FullPath string // 高清原图（%TEMP% 会话目录下，灯箱/导出时才读）
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
	DlgRecover
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
	Full    *ui.Bitmap // 当前显示的大图（未旋转：转的是元素，不是像素）
	Loading bool
	Zoom    float32 // 1.0 = 适配窗口
	PanX    float32 // 放大后拖动查看的偏移
	PanY    float32
	Rot     int  // 展示角度：只在渲染时转，不重解码原图
	Dirty   bool // 需要重新解码/派生

	// 删除是破坏性操作，先在这一页上问一次，不弹整窗对话框。
	ConfirmDelete bool

	ImgW, ImgH float32 // 原图像素尺寸，用来算 Contain 后真正占的区域
	img        ui.Rect // 上一帧图片容器的区域（窗口坐标）
	row        ui.Rect // 上一帧内容区的区域：输入事件坐标相对它
	dragging   bool
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

// 图框的上下留白与左右留白，与 lightboxOverlay 里的 Padding 一致：
// 算显示尺寸要用内容区，而不是整块图框。
const (
	lbPadTop  float32 = 8
	lbPadSide float32 = 44
)

func (lb *Lightbox) contentBox() ui.Rect {
	r := lb.row
	return ui.Rect{
		X: r.X + lbPadSide,
		Y: r.Y + lbPadTop,
		W: r.W - 2*lbPadSide,
		H: r.H - 2*lbPadTop,
	}
}

// displaySize 是图片（含旋转）在图框里占的宽高。
func (lb *Lightbox) displaySize() (w, h float32) {
	box := lb.contentBox()
	iw, ih := lb.ImgW, lb.ImgH
	if lb.Rot%180 != 0 {
		iw, ih = ih, iw // 转 90°/270° 后宽高互换
	}
	if box.W <= 0 || box.H <= 0 || iw <= 0 || ih <= 0 {
		return 0, 0
	}
	s := min(box.W/iw, box.H/ih)
	return iw * s, ih * s
}

// contentRect 是图片真正画出来的地方；点黑边应该关灯箱。
func (lb *Lightbox) contentRect() ui.Rect {
	box := lb.img
	if box.W <= 0 || box.H <= 0 {
		return ui.Rect{}
	}
	if lb.Zoom > 1 {
		return box
	}
	w, h := lb.ImgW, lb.ImgH
	if lb.Rot%180 != 0 {
		w, h = h, w
	}
	if w <= 0 || h <= 0 {
		return ui.Rect{}
	}
	s := min(box.W/w, box.H/h)
	cw, ch := w*s, h*s
	return ui.Rect{X: box.X + (box.W-cw)/2, Y: box.Y + (box.H-ch)/2, W: cw, H: ch}
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
	// devStatusIdx 决定这份状态算不算数：换设备后旧状态不能继续冒充。
	devStatus    scanner.Status
	devStatusIdx int
	statusKick   chan struct{} // nil = 没起轮询
	// autoQueue 非空 = 正在按名单顺序找第一台在线的；存 Idx，列表重排后仍稳定。
	autoQueue []int
	// userPicked 用户手动选过设备：接力让位，不再自动改选中项。重新检测时清零。
	userPicked   bool
	pendingProbe bool         // 探测期间又切了设备：上一个回来后补探
	probed       map[int]bool // 已探测过的设备（键是 scanner.DeviceInfo.Idx，不是列表下标）
	detecting    bool
	netScanning  bool // 正在扫网段兜底
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
	// selected 是勾选导出的页面，按 ID 记：排序/删除后不会错位到别的页。
	selected map[uint64]bool
	// pendingDelete 是右键菜单选中的删除：等这一轮格子建完再删，免得建到一半缩短 pages。
	pendingDelete uint64

	scanning     bool
	restoring    bool // 正在把上次崩溃遗留的页面解码回界面
	scanStatus   string
	scanProgress float64 // 0..1 的估算进度：设备只报扫完没有，细节靠时间和字节数推

	// recoverPages 是上次异常退出留下的页面清单；用户选恢复前只占着位置，
	// 不建缩略图也不进 pages，免得恢复到一半又把半份数据当成正常状态。
	recoverPages []platform.SessionPage

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
		selected:     map[uint64]bool{},
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
	if d == nil || a.devStatusIdx != d.Idx {
		a.devStatus, a.devStatusIdx = scanner.Status{}, 0 // 换了设备，旧状态作废
	}
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
	a.saveSession()
}

func (a *app) isSelected(id uint64) bool { return a.selected[id] }

func (a *app) toggleSelected(id uint64) {
	if a.selected[id] {
		delete(a.selected, id)
		return
	}
	a.selected[id] = true
}

func (a *app) selectedCount() int { return len(a.selected) }

func (a *app) clearSelection() { clear(a.selected) }

// pageCount 含尚未决定是否恢复的页面：关窗确认不能把这份数据漏掉。
func (a *app) pageCount() int { return len(a.pages) + len(a.recoverPages) }

// saveSession 记录页序和旋转。图片本身已经落在会话目录，崩溃后据此还原。
func (a *app) saveSession() {
	if Win == nil {
		return // 单元测试不碰真实临时目录
	}
	pages := make([]platform.SessionPage, 0, len(a.pages))
	for _, p := range a.pages {
		pages = append(pages, platform.SessionPage{File: filepath.Base(p.FullPath), Rot: p.Rot})
	}
	platform.SaveSession(pages)
}

func (a *app) selectAll() {
	for _, p := range a.pages {
		a.selected[p.ID] = true
	}
}

// selectedPages 按页面顺序返回，导出的页序才和界面上看到的一致。
func (a *app) selectedPages() []Page {
	if len(a.selected) == 0 {
		return nil
	}
	out := make([]Page, 0, len(a.selected))
	for _, p := range a.pages {
		if a.selected[p.ID] {
			out = append(out, p)
		}
	}
	return out
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
