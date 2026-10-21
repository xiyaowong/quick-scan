package ui

import (
	"image"
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

func addPages(a *app, n int) {
	img := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 40, 56)))
	for i := range n {
		a.pages = append(a.pages, Page{ID: uint64(i + 1), Thumb: img, FullPath: "不存在.jpg"})
	}
}
