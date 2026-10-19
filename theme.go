// 纯白主题（定稿 §三-1 硬性决策：只有一套浅色主题，不跟随系统深色模式）。
// 配色对照 docs/ui-mockup.html 的 :root 设计变量：冷灰纸感中性色 + 深青 accent
// （扫描仪光条的青绿），红章作为文档世界的点睛。
package main

import "github.com/egoist/mygo/ui"

// lightTheme 以 mygo 内置 LightTheme 为底（保证度量齐全），逐项改成设计稿色值。
func lightTheme() *ui.Theme {
	t := ui.LightTheme()
	// 中性色（mockup: --win/--panel/--panel-2/--line/--line-2/--ink/--mut）
	t.Background = ui.RGB(244, 246, 247)     // --win  窗口底
	t.Surface = ui.RGB(255, 255, 255)        // --panel 控件面
	t.SurfaceHover = ui.RGB(238, 241, 243)   // --panel-2
	t.SurfacePressed = ui.RGB(228, 233, 236) // panel-2 再压深一点
	t.Border = ui.RGB(211, 218, 222)         // --line
	t.Text = ui.RGB(23, 36, 46)              // --ink
	t.TextMuted = ui.RGB(93, 107, 118)       // --mut
	// 强调色（mockup: --accent 0c7f8a 深青）
	t.Accent = ui.RGB(12, 127, 138)
	t.AccentHover = ui.RGB(10, 106, 115) // --accent-hi
	t.AccentPressed = ui.RGB(8, 92, 100)
	t.AccentText = ui.RGB(255, 255, 255) // --accent-ink
	// 语义色
	t.Danger = ui.RGB(178, 58, 46)      // --danger
	t.Success = ui.RGB(47, 158, 79)     // --ok
	t.Warning = ui.RGB(217, 154, 30)    // --warn
	t.Selection = ui.RGB(224, 240, 241) // --accent-soft
	t.Focus = t.Accent
	return t
}
