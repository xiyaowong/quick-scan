package ui

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
)

// appVersion 是构建时写进程序的版本；本地直接 go build 的副本没有版本号。
func appVersion() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return "开发版"
}

// versionButton 是状态栏右下角的版本号：点开软件信息，检查更新也在里面。
func versionButton(c *ui.Context) ui.Element {
	b := ui.ButtonBase(c).Height(22).Padding(0, 7, 0, 7).Radius(6).
		Tooltip("关于快速扫描").
		Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
	fg := colFaint
	if b.Hovered() {
		b.Background(colPanelHover)
		fg = colMuted
	}
	b.Children(func() {
		ui.Text(c, appVersion()).FontSize(11).TextColor(fg)
	})
	return b
}

func (a *app) aboutDialog(c *ui.Context) {
	open := a.dlg == DlgAbout
	ui.Modal(c, &open, func() {
		card := ui.Column(c).Width(404).Background(colPanel).Border(1, colLine).
			Radius(18).Clip().Shadow(0, 28, 70, 0, shadowStrong)
		card.Children(func() {
			ui.Row(c).Padding(22, 24, 0, 24).Gap(13).AlignItems(ui.Center).Children(func() {
				badge := ui.Row(c).Size(40, 40).Radius(12).Background(colAccentSoft).Center().Shrink(0)
				badge.Children(func() {
					ui.Icon(c, icoScan).FontSize(19).TextColor(colAccent)
				})
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, "快速扫描").FontSize(15.5).FontWeight(600).TextColor(colInk)
					ui.Text(c, "版本 "+appVersion()).FontSize(12.5).TextColor(colMuted)
				})
			})
			ui.Column(c).Padding(18, 24, 18, 24).Gap(7).Children(func() {
				caption(c, "扫描、优化、导出，全部在本机完成。")
				caption(c, "xiyaowong · MIT")
			})
			hairline(c)
			ui.Row(c).Padding(16, 24, 16, 24).Justify(ui.End).Gap(9).Children(func() {
				if styledButton(c, "关闭", nil, toneGhost, false).Height(38).Padding(0, 18, 0, 18).Clicked() {
					a.dlg = DlgNone
				}
				if styledButton(c, "检查更新", icoRefresh, tonePrimary, false).Height(38).Clicked() {
					a.dlg = DlgNone
					updater.CheckForUpdates()
				}
			})
		})
	})
	if !open {
		a.dlg = DlgNone
	}
}
