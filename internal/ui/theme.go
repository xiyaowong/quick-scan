package ui

import "github.com/egoist/mygo/ui"

var (
	colBg         = ui.Hex("#F5F5F5")
	colPanel      = ui.Hex("#FFFFFF")
	colPanelSoft  = ui.Hex("#F7F7F7")
	colPanelHover = ui.Hex("#F0F0F0")
	colLine       = ui.Hex("#E7E7E7")
	colLineSoft   = ui.Hex("#EFEFEF")
	colInk        = ui.Hex("#191919")
	colMuted      = ui.Hex("#8C8C8C")
	colFaint      = ui.Hex("#B2B2B2")

	colAccent      = ui.Hex("#07C160")
	colAccentHover = ui.Hex("#06AD56")
	colAccentPress = ui.Hex("#059B4C")
	colAccentSoft  = ui.Hex("#E8F7EE")
	colAccentLine  = ui.Hex("#B7E5C9")

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

const (
	statusBarHeight = 52
	railWidth       = 348
	titleBarHeight  = 40
)

// 以 mygo 的 LightTheme 为底，保证度量齐全。
func lightTheme() *ui.Theme {
	t := ui.LightTheme()
	t.Background = colBg
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
