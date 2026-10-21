package ui

import (
	"image"
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
