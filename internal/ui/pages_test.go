package ui

import (
	"image"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 删除按钮在这一帧构建时就执行，pages 已经变短，而 GridView 还按旧页数建后面的
// 格子。这里走真实点击路径：以前删 3 页里的第一页会 panic: index out of range [2]。
func TestDeleteFromGridThenExport(t *testing.T) {
	a := newApp()
	img := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 40, 56)))
	for i := 0; i < 3; i++ {
		a.pages = append(a.pages, Page{ID: uint64(i + 1), Thumb: img, FullPath: "不存在.jpg"})
	}
	tt := ui.NewTester(a.view, 1200, 800)

	if err := tt.Click("删除第 1 页"); err != nil {
		t.Fatal(err)
	}
	if len(a.pages) != 2 {
		t.Fatalf("删除后还剩 %d 页，应该是 2 页", len(a.pages))
	}
	if _, ok := tt.Find("03"); ok {
		t.Fatalf("第 3 页的格子还在：%v", tt.Texts())
	}
	if _, ok := tt.Find("02"); !ok {
		t.Fatalf("第 2 页的格子没了：%v", tt.Texts())
	}

	if err := tt.Click("导出"); err != nil {
		t.Fatal(err)
	}
	if a.dlg != DlgExport || a.exportSingle != -1 {
		t.Fatalf("dlg=%v exportSingle=%d", a.dlg, a.exportSingle)
	}
	if !tt.HasText("导出全部 2 页") {
		t.Fatalf("导出对话框页数不对：%v", tt.Texts())
	}
}

// 勾选后导出只带选中的页，顺序跟页面上看到的一致。
func TestExportSelectedPages(t *testing.T) {
	a := newApp()
	addPages(a, 3)
	tt := ui.NewTester(a.view, 1200, 800)

	if err := tt.Click("勾选第 1 页"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("勾选第 3 页"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("已选 2 页") {
		t.Fatalf("页头没显示勾选数：%v", tt.Texts())
	}

	if err := tt.Click("导出选中"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("导出选中的 2 页（共 3 页）") {
		t.Fatalf("导出对话框没说明只导选中页：%v", tt.Texts())
	}
	targets := a.exportTargets()
	if len(targets) != 2 || targets[0].ID != 1 || targets[1].ID != 3 {
		t.Fatalf("导出目标 = %+v", targets)
	}
}

// 删除勾选页时勾选要一起消失，否则「已选 N 页」会和实际对不上。
func TestDeleteSelectedPage(t *testing.T) {
	a := newApp()
	addPages(a, 2)
	tt := ui.NewTester(a.view, 1200, 800)

	if err := tt.Click("勾选第 1 页"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("删除第 1 页"); err != nil {
		t.Fatal(err)
	}
	if a.selectedCount() != 0 {
		t.Fatalf("删除后还剩 %d 个勾选", a.selectedCount())
	}
	if tt.HasText("已选") {
		t.Fatalf("页头还在显示勾选数：%v", tt.Texts())
	}
}

// 缩略图是系统拖拽源，按下事件落在它身上；点缩略图看大图不能因此失灵。
func TestClickThumbnailOpensLightbox(t *testing.T) {
	a := newApp()
	addPages(a, 1)
	tt := ui.NewTester(a.view, 1200, 800)

	r, ok := tt.Find("01")
	if !ok {
		t.Fatalf("没找到第 1 页的格子：%v", tt.Texts())
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H+30)
	if a.lightbox == nil {
		t.Fatal("点缩略图没有打开大图")
	}
}

// 一键复原单页：转过去再复原，方向和缩略图都要回到扫描时的样子。
func TestResetPageRotation(t *testing.T) {
	a := newApp()
	src := image.NewRGBA(image.Rect(0, 0, 170, 240))
	a.pages = []Page{{ID: 1, Thumb: ui.NewBitmap(src), ThumbSrc: src, FullPath: "不存在.jpg"}}

	a.rotatePage(1)
	if a.pages[0].Rot != 90 {
		t.Fatalf("Rot=%d", a.pages[0].Rot)
	}
	a.resetPageRot(1)
	if a.pages[0].Rot != 0 || a.pages[0].ThumbRot != nil {
		t.Fatalf("没有复原：Rot=%d ThumbRot=%v", a.pages[0].Rot, a.pages[0].ThumbRot)
	}
}

// 转过的页面才有「复原」：卡片按钮和右键菜单都不多不少。
func TestResetPageRotationOnlyWhenRotated(t *testing.T) {
	a := newApp()
	src := image.NewRGBA(image.Rect(0, 0, 170, 240))
	a.pages = []Page{{ID: 1, Thumb: ui.NewBitmap(src), ThumbSrc: src, FullPath: "不存在.jpg"}}
	tt := ui.NewTester(a.view, 1200, 800)

	if err := tt.Click("复原第 1 页"); err == nil {
		t.Fatal("没转过的页面不该出现复原按钮")
	}
	if err := tt.RightClick("01"); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(tt.Menu(), "复原旋转") {
		t.Fatalf("没转过的页面不该有复原项：%v", tt.Menu())
	}

	a.rotatePage(1)
	tt.Frame()
	if err := tt.Click("复原第 1 页"); err != nil {
		t.Fatal(err)
	}
	if a.pages[0].Rot != 0 {
		t.Fatalf("Rot=%d", a.pages[0].Rot)
	}
}

// 一键旋转所有页面、一键复原：页头入口，多页时才出现；没转过时没有「复原全部」。
func TestRotateAllPages(t *testing.T) {
	a := newApp()
	addPages(a, 3)
	tt := ui.NewTester(a.view, 1200, 800)

	if err := tt.Click("复原全部"); err == nil {
		t.Fatal("没转过时不该出现复原全部")
	}
	if err := tt.Click("旋转全部"); err != nil {
		t.Fatal(err)
	}
	for i, p := range a.pages {
		if p.Rot != 90 {
			t.Fatalf("第 %d 页 Rot=%d", i+1, p.Rot)
		}
	}
	if !a.anyRotated() {
		t.Fatal("anyRotated 应为 true")
	}
	if err := tt.Click("复原全部"); err != nil {
		t.Fatal(err)
	}
	for i, p := range a.pages {
		if p.Rot != 0 {
			t.Fatalf("复原后第 %d 页 Rot=%d", i+1, p.Rot)
		}
	}
}

func addPages(a *app, n int) {
	img := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 40, 56)))
	for i := range n {
		a.pages = append(a.pages, Page{ID: uint64(i + 1), Thumb: img, FullPath: "不存在.jpg"})
	}
}

// 右键页面弹出常用操作；删除要等这一轮格子建完再生效。
func TestPageContextMenu(t *testing.T) {
	a := newApp()
	addPages(a, 2)
	tt := ui.NewTester(a.view, 1200, 800)

	if err := tt.RightClick("01"); err != nil {
		t.Fatal(err)
	}
	want := []string{"看大图", "旋转 90°", "导出此页", "勾选此页", "-", "删除此页"}
	if got := tt.Menu(); !slices.Equal(got, want) {
		t.Fatalf("菜单 = %v，想要 %v", got, want)
	}

	if err := tt.ChooseMenuItem("勾选此页"); err != nil {
		t.Fatal(err)
	}
	if !a.isSelected(1) {
		t.Fatal("勾选没生效")
	}

	if err := tt.RightClick("01"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("取消勾选"); err != nil {
		t.Fatal(err)
	}
	if a.isSelected(1) {
		t.Fatal("取消勾选没生效")
	}

	if err := tt.RightClick("01"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("删除此页"); err != nil {
		t.Fatal(err)
	}
	if len(a.pages) != 1 {
		t.Fatalf("删除后还剩 %d 页", len(a.pages))
	}
}
