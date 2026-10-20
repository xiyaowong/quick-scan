// 微信风格的浅色主题：浅灰画布、白色卡片，绿色只给主操作与选中态。
// 界面层统一从这里取色，view.go 不写死色值。
package ui

import "github.com/egoist/mygo/ui"

// 设计令牌。
var (
	colBg         = ui.Hex("#F5F5F5") // 窗口底：微信的浅灰画布
	colPanel      = ui.Hex("#FFFFFF") // 内容卡片
	colPanelSoft  = ui.Hex("#F7F7F7") // 输入框、分段轨道、缩略图底
	colPanelHover = ui.Hex("#F0F0F0") // 悬停
	colLine       = ui.Hex("#E7E7E7") // 卡片描边
	colLineSoft   = ui.Hex("#EFEFEF") // 分隔线
	colInk        = ui.Hex("#191919") // 主文字
	colMuted      = ui.Hex("#8C8C8C") // 次要文字
	colFaint      = ui.Hex("#B2B2B2") // 占位、禁用

	colAccent      = ui.Hex("#07C160") // 微信绿
	colAccentHover = ui.Hex("#06AD56")
	colAccentPress = ui.Hex("#059B4C")
	colAccentSoft  = ui.Hex("#E8F7EE") // 浅绿底
	colAccentLine  = ui.Hex("#B7E5C9") // 浅绿描边

	colDanger     = ui.Hex("#FA5151")
	colDangerSoft = ui.Hex("#FFF0F0")
	colOk         = ui.Hex("#07C160")
	colOkSoft     = ui.Hex("#E8F7EE")
	colWarn       = ui.Hex("#FA9D3B")

	shadowSoft   = ui.RGBA(0, 0, 0, 0.04)
	shadowCard   = ui.RGBA(0, 0, 0, 0.08)
	shadowStrong = ui.RGBA(0, 0, 0, 0.18)
	shadowAccent = ui.RGBA(7, 193, 96, 0.22)
)

// 版式度量。
const (
	TitleBarHeight  = 46
	statusBarHeight = 52
	railWidth       = 348
)

// lightTheme 以 mygo 内置 LightTheme 为底（保证度量齐全），逐项改成微信风格色值。
func lightTheme() *ui.Theme {
	t := ui.LightTheme()
	t.Background = colBg
	// 控件默认是浅灰填充的面，和微信的设置项一致；卡片自己用 colPanel 覆盖。
	t.Surface = colPanelSoft
	t.SurfaceHover = colPanelHover
	t.SurfacePressed = ui.Hex("#E9E9E9")
	t.Border = colLine
	t.Text = colInk
	t.TextMuted = colMuted
	t.Accent = colAccent
	t.AccentHover = colAccentHover
	t.AccentPressed = colAccentPress
	t.AccentText = ui.Hex("#FFFFFF")
	t.Danger = colDanger
	t.Success = colOk
	t.Warning = colWarn
	t.Selection = ui.RGBA(7, 193, 96, 0.20)
	t.Focus = ui.RGBA(7, 193, 96, 0.50)
	t.Inverse = ui.Hex("#1F1F1F")
	t.InverseText = ui.Hex("#FFFFFF")
	t.Scrollbar = ui.RGBA(0, 0, 0, 0.18)
	t.ScrollbarWidth = 7
	t.Radius = 8
	t.Spacing = 5
	t.FontSize = 13.5
	return t
}
