package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 自绘标题栏后系统按钮没了：三个按钮必须都在；无窗口的测试里点了也不能 panic。
func TestTitleBarButtons(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1200, 800)

	if !tt.HasText("快速扫描") {
		t.Fatalf("标题栏没有应用名：%v", tt.Texts())
	}
	for _, label := range []string{"最小化", "最大化", "关闭"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("标题栏缺少「%s」：%v", label, tt.Texts())
		}
	}
	if err := tt.Click("最大化"); err != nil {
		t.Fatal(err)
	}
}
