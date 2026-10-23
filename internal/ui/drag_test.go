package ui

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quick-scan/internal/scanner"

	"github.com/egoist/mygo/transfer"
)

// dragTo 把拖拽副本的落点换到临时目录，别碰真实的 %TEMP% 会话。
func dragTo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := dragDir
	dragDir = func() string { return dir }
	t.Cleanup(func() { dragDir = old })
	return dir
}

func writeTestPage(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "page_1_0.jpg")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 拖出去的东西必须是磁盘上真实存在、别人读得到的图片，名字里带得出是第几页。
func TestDragFileIsARealImage(t *testing.T) {
	dir := dragTo(t)
	src := writeTestPage(t, 40, 56)

	a := newApp()
	a.pages = []Page{{ID: 1, FullPath: src}}

	got, err := a.dragFile(0)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(got) != dir {
		t.Fatalf("副本落在 %s，应该在 %s", filepath.Dir(got), dir)
	}
	name := filepath.Base(got)
	if !strings.HasPrefix(name, "扫描_") || !strings.HasSuffix(name, "_1.jpg") {
		t.Fatalf("文件名 = %q，看不出是第 1 页", name)
	}
	want, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(got); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("副本和原图不一致：%v", err)
	}
}

// 转过方向的页面交出去也要是正的：会话里的原图不带角度。
func TestDragFileBakesRotation(t *testing.T) {
	dragTo(t)
	src := writeTestPage(t, 40, 56)

	a := newApp()
	a.pages = []Page{{ID: 1, FullPath: src, Rot: 90}}

	path, err := a.dragFile(0)
	if err != nil {
		t.Fatal(err)
	}
	img, err := scanner.Decode(path)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 56 || b.Dy() != 40 {
		t.Fatalf("拖出去的图是 %dx%d，应该转成 56x40", b.Dx(), b.Dy())
	}
	// 同一页再拖一次走缓存：不该在拖的瞬间再编码一遍。
	if again, err := a.dragFile(0); err != nil || again != path {
		t.Fatalf("第二次拖 = %q, %v", again, err)
	}
}

// 原图没了也要说人话：拖拽开始时拿不到返回值，错误留给 Done 报。
func TestDragFailureIsReported(t *testing.T) {
	dragTo(t)
	a := newApp()
	a.pages = []Page{{ID: 1, FullPath: filepath.Join(t.TempDir(), "不存在.jpg")}}

	if data := a.pageDragData(0); len(data.Formats()) != 0 {
		t.Fatalf("文件都读不到还交出了 %v", data.Formats())
	}
	a.dragDone(transfer.Result{Err: errors.New("框架自己的报错")})
	if !strings.Contains(a.hint, "拖不出去") {
		t.Fatalf("拖失败没提示：%q", a.hint)
	}
	if a.hintDetail == "" {
		t.Fatal("原始错误没留下，排查时无从下手")
	}
}

// 用户自己松手取消不算失败，不该冒出提示。
func TestDragCancelIsSilent(t *testing.T) {
	a := newApp()
	a.dragDone(transfer.Result{Canceled: true})
	if a.hint != "" {
		t.Fatalf("取消拖拽弹了提示：%q", a.hint)
	}
}

// 拖回网格：落在哪一页上就排到那一页前面，落回原处不动。
func TestDropPageReorders(t *testing.T) {
	a := newApp()
	addPages(a, 3)

	a.dropOnPage(1, 3)
	if got := pageIDs(a); !equalIDs(got, []uint64{2, 3, 1}) {
		t.Fatalf("落到第 3 页前 = %v，想要 [2 3 1]", got)
	}
	a.dropOnPage(3, 0)
	if got := pageIDs(a); !equalIDs(got, []uint64{3, 2, 1}) {
		t.Fatalf("落到第 1 页前 = %v，想要 [3 2 1]", got)
	}
	a.dropOnPage(3, 0)
	if got := pageIDs(a); !equalIDs(got, []uint64{3, 2, 1}) {
		t.Fatalf("落回原处不该动：%v", got)
	}
}

func pageIDs(a *app) []uint64 {
	out := make([]uint64, len(a.pages))
	for i, p := range a.pages {
		out[i] = p.ID
	}
	return out
}

func equalIDs(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
