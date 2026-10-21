package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 灯箱直接摆好大图，别走 openLightbox：那会起后台解码（测试里没有窗口）。
func lightboxApp() *app {
	a := newApp()
	img := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 400, 560)))
	a.pages = []Page{{ID: 1, Thumb: img, FullPath: "不存在.jpg"}}
	a.lightbox = &Lightbox{Index: 0, Full: img, ImgW: 400, ImgH: 560, Zoom: 1}
	return a
}

// 遮罩上的输入处理一度抢走了按下事件，灯箱里的按钮全部失效。
func TestLightboxButtonsTakeClicks(t *testing.T) {
	a := lightboxApp()
	tt := ui.NewTester(a.view, 900, 700)

	if err := tt.Click("删除此页"); err != nil {
		t.Fatal(err)
	}
	if a.lightbox == nil || !a.lightbox.ConfirmDelete {
		t.Fatalf("点「删除此页」没有进入确认：%+v", a.lightbox)
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.lightbox == nil || a.lightbox.ConfirmDelete {
		t.Fatal("点「取消」后仍在确认删除")
	}

	if err := tt.Click("删除此页"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if a.lightbox != nil || len(a.pages) != 0 {
		t.Fatalf("确认删除后 lightbox=%v pages=%d", a.lightbox, len(a.pages))
	}
}

func TestLightboxExportClosesLightbox(t *testing.T) {
	a := lightboxApp()
	tt := ui.NewTester(a.view, 900, 700)

	if err := tt.Click("导出此页"); err != nil {
		t.Fatal(err)
	}
	if a.lightbox != nil {
		t.Fatal("点「导出此页」后灯箱还开着")
	}
	if a.dlg != DlgExport || a.exportSingle != 0 {
		t.Fatalf("dlg=%v exportSingle=%d", a.dlg, a.exportSingle)
	}
	if !tt.HasText("文件名") {
		t.Fatalf("导出对话框没显示：%v", tt.Texts())
	}
}

func TestLightboxImageAndBackdropClicks(t *testing.T) {
	a := lightboxApp()
	tt := ui.NewTester(a.view, 900, 700)

	tt.ClickAt(450, 350) // 图片正中：不该关
	if a.lightbox == nil {
		t.Fatal("点图片把灯箱关了")
	}

	a.lightbox.Zoom = 2
	a.lightbox.PanX, a.lightbox.PanY = 30, 40
	tt.Frame()
	tt.ClickAt(450, 350)
	tt.ClickAt(450, 350) // 双击还原
	if a.lightbox == nil || a.lightbox.Zoom != 1 || a.lightbox.PanX != 0 || a.lightbox.PanY != 0 {
		t.Fatalf("双击没有还原：%+v", a.lightbox)
	}

	tt.ClickAt(20, 350) // 图片左侧的留白：点遮罩，关灯箱
	if a.lightbox != nil {
		t.Fatal("点遮罩没关灯箱")
	}
}

// 旋转只换渲染角度：原图不再重新解码，Full 还是同一张位图。
func TestLightboxRotateKeepsBitmap(t *testing.T) {
	a := lightboxApp()
	tt := ui.NewTester(a.view, 900, 700)
	full := a.lightbox.Full

	if err := tt.Click("旋转"); err != nil {
		t.Fatal(err)
	}
	if a.lightbox == nil || a.lightbox.Rot != 90 || a.pages[0].Rot != 90 {
		t.Fatalf("旋转没生效：%+v page rot=%d", a.lightbox, a.pages[0].Rot)
	}
	if a.lightbox.Full != full {
		t.Fatal("旋转后重新解码了原图")
	}
}

// 左红右蓝的原图转 90°（顺时针）后应该是上红下蓝。
func TestLightboxRotatesAtRenderTime(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 400, 560))
	for y := 0; y < 560; y++ {
		for x := 0; x < 400; x++ {
			c := color.RGBA{R: 255, A: 255}
			if x >= 200 {
				c = color.RGBA{B: 255, A: 255}
			}
			src.SetRGBA(x, y, c)
		}
	}
	a := newApp()
	bm := ui.NewBitmap(src)
	a.pages = []Page{{ID: 1, Thumb: bm, FullPath: "不存在.jpg"}}
	a.lightbox = &Lightbox{Index: 0, Full: bm, ImgW: 400, ImgH: 560, Zoom: 1}

	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame() // 第一帧先量出图框
	a.lightbox.Rot = 90
	tt.Frame()

	r := a.lightbox.img
	if r.W <= 0 || r.H <= 0 {
		t.Fatalf("图片区域是空的：%+v", r)
	}
	if r.W <= r.H {
		t.Fatalf("转 90° 后应该变成横的：%.0f×%.0f", r.W, r.H)
	}
	shot := tt.Image()
	top := shot.RGBAAt(int(r.X+r.W/2), int(r.Y+r.H/4))
	bottom := shot.RGBAAt(int(r.X+r.W/2), int(r.Y+r.H*3/4))
	if top.R < 200 || top.B > 60 {
		t.Fatalf("上半应该来自原图左侧（红）：%v", top)
	}
	if bottom.B < 200 || bottom.R > 60 {
		t.Fatalf("下半应该来自原图右侧（蓝）：%v", bottom)
	}
}

// 页面卡片的旋转用缓存的 240px 像素，不再解码原图（原图路径是故意不存在的）。
func TestRotatePageUsesCachedThumb(t *testing.T) {
	a := newApp()
	src := image.NewRGBA(image.Rect(0, 0, 170, 240))
	a.pages = []Page{{ID: 1, Thumb: ui.NewBitmap(src), ThumbSrc: src, FullPath: "不存在.jpg"}}

	w0, h0 := a.pages[0].Thumb.Size()
	a.rotatePage(1)
	if a.pages[0].Rot != 90 {
		t.Fatalf("Rot=%d", a.pages[0].Rot)
	}
	if a.pages[0].ThumbRot == nil {
		t.Fatal("没有生成旋转后的缩略图")
	}
	w1, h1 := a.pages[0].ThumbRot.Size()
	if w1 != h0 || h1 != w0 {
		t.Fatalf("旋转后宽高没互换：%d×%d → %d×%d", w0, h0, w1, h1)
	}
}
