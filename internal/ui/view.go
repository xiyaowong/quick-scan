package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"sscan/internal/scanner"

	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"
)

func (a *app) view(c *ui.Context) {
	c.SetTheme(lightTheme())
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			a.pagesPanel(c, t)
			a.settingsPanel(c, t)
		})
		a.statusBar(c, t)
	})
	a.overlays(c, t)
}

func (a *app) statusLabel() string {
	switch a.devState {
	case DevOnline:
		return "已连接"
	case DevOffline:
		return "连不上"
	default:
		if a.detecting || a.probing || a.netScanning {
			return "连接中"
		}
		return "未连接"
	}
}

func statusColor(s DevState) ui.Color {
	switch s {
	case DevOnline:
		return colOk
	case DevOffline:
		return colDanger
	default:
		return colFaint
	}
}

func (a *app) pagesPanel(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).MinWidth(0).Padding(18, 18, 18, 14).Gap(14).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "扫描页面").FontSize(19).FontWeight(600).TextColor(colInk)
			ui.Row(c).Grow(1)
			countBadge(c, len(a.pages))
		})

		if len(a.pages) == 0 {
			a.emptyState(c, t)
			return
		}

		ui.GridView(c, &a.pageGrid, len(a.pages), 200, 280, func(i int) {
			a.pageCell(c, t, i)
		}).Grow(1)

		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icoGrip).FontSize(12).TextColor(colFaint)
			ui.Text(c, "点一下看大图 · 拖动可排序").FontSize(11).TextColor(colMuted)
		})
	})
}

func (a *app) emptyState(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Center().Children(func() {
		halo := ui.Row(c).Size(64, 64).Radius(20).Background(colAccentSoft).Center()
		halo.Children(func() {
			ui.Icon(c, icoScan).FontSize(27).TextColor(colAccent)
		})
		ui.Text(c, "还没有扫描的页面").FontSize(15).FontWeight(500).TextColor(colInk).
			Margin(18, 0, 0, 0)
		ui.Text(c, "把纸放好，点右边的「扫描」").FontSize(12.5).TextColor(colMuted).
			Margin(7, 0, 0, 0)
	})
}

// 不在这里 .Drag()：GridView 的 Reorder 已在外层挂了拖拽源，内层再挂会抢走按下事件。
func (a *app) pageCell(c *ui.Context, t *ui.Theme, i int) {
	// 删除按钮在这一帧构建时就执行，pages 已经变短；GridView 仍按本帧开头的
	// 页数继续建后面的格子，越界的先跳过——点击会触发重绘，下一遍就对齐了。
	if i >= len(a.pages) {
		return
	}
	p := &a.pages[i]
	card := ui.Column(c.Key(p.ID)).Fill().
		Background(colPanel).Border(1, colLine).Radius(8).Clip().
		Transition(ui.ElementTransition{Duration: 140 * time.Millisecond, Colors: true})
	hovered := card.Hovered()
	if hovered {
		card.Border(1, colAccentLine).Shadow(0, 4, 14, 0, shadowCard)
	}
	card.Children(func() {
		frame := ui.Column(c).Grow(1).Background(colPanelSoft).Padding(9).Center()
		frame.Children(func() {
			ui.Image(c, p.thumb()).Fill().Fit(ui.Contain).Radius(2)
			if hovered {
				ui.Row(c).Absolute().Left(0).Right(0).Bottom(8).Justify(ui.Center).PassThrough().Children(func() {
					ui.Row(c).Gap(5).AlignItems(ui.Center).Padding(5, 11, 5, 11).Radius(999).
						Background(ui.RGBA(0, 0, 0, 0.55)).Children(func() {
						ui.Icon(c, icoEye).FontSize(11).TextColor(ui.RGB(255, 255, 255))
						ui.Text(c, "点击看大图").FontSize(10.5).TextColor(ui.RGB(255, 255, 255))
					})
				})
			}
		})
		ui.Row(c).Absolute().Left(8).Top(8).Padding(2, 6, 2, 6).Radius(4).
			Background(ui.RGBA(0, 0, 0, 0.46)).Children(func() {
			ui.Text(c, padPage(i+1)).FontSize(9.5).FontWeight(600).
				TextColor(ui.RGB(255, 255, 255))
		})
		hairline(c)
		ui.Row(c).Height(42).Padding(0, 8, 0, 8).AlignItems(ui.Center).Gap(2).Children(func() {
			ui.Icon(c, icoGrip).FontSize(11).TextColor(colFaint).Tooltip("按住拖动排序")
			ui.Row(c).Grow(1)
			if iconButton(c, icoRotate, "旋转 90°", 30).Label("旋转第 " + itoa(i+1) + " 页").Clicked() {
				a.rotatePage(p.ID)
			}
			if iconButton(c, icoDownload, "导出此页", 30).Label("导出第 " + itoa(i+1) + " 页").Clicked() {
				a.openExport(i)
			}
			ui.Row(c).Width(1).Height(14).Background(colLineSoft).Margin(3, 0, 3, 0)
			if dangerIconButton(c, icoTrash, "删除此页", 30).Label("删除第 " + itoa(i+1) + " 页").Clicked() {
				a.deletePage(p.ID)
			}
		})
	})
	if card.Clicked() || card.DoubleClicked() {
		a.openLightbox(i)
	}
}

func padPage(n int) string { return fmt.Sprintf("%02d", n) }

func (a *app) settingsPanel(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Width(railWidth).Padding(0, 18, 18, 18).Children(func() {
		card := ui.Column(c).Fill().Background(colPanel).Border(1, colLine).Radius(12).
			Clip().Shadow(0, 1, 6, 0, shadowSoft)
		card.Children(func() {
			ui.Column(c).Padding(18, 18, 12, 18).Children(func() {
				ui.Text(c, "扫描设置").FontSize(15).FontWeight(500).TextColor(colInk)
			})

			ui.Scroll(c).Grow(1).Padding(18, 18, 10, 18).Gap(15).Children(func() {
				field := func(build func()) {
					ui.Column(c).Gap(7).Children(build)
				}
				field(func() { a.sizeField(c) })
				field(func() { a.dpiField(c) })
				field(func() { a.intentField(c) })
				field(func() { a.colorField(c) })
				field(func() { a.sourceField(c) })
			})

			hairline(c)

			ui.Column(c).Padding(12, 18, 16, 18).Gap(10).Children(func() {
				a.scanButton(c, t)
				a.scanFeedback(c)
				ui.Row(c).Gap(8).Children(func() {
					a.clearLink(c)
					a.exportButton(c)
				})
			})
		})
	})
}

func (a *app) sizeField(c *ui.Context) {
	dev := a.current()
	fixed := dev != nil && a.sizeSelectable(dev)
	sectionLabel(c, "纸张大小", fixed)
	enabled := []bool{true, true, true}
	if dev != nil {
		enabled = []bool{dev.SizeOK(SizeA4.key()), dev.SizeOK(SizeA5.key()), dev.SizeOK(SizeLetter.key())}
	}
	cur := int(a.size)
	if segmented(c, &cur, []string{"A4", "A5", "Letter"}, enabled) {
		a.size = PaperSize(cur)
	}
}

func (a *app) sizeSelectable(dev *scanner.DeviceInfo) bool {
	if dev == nil {
		return false
	}
	n := 0
	for _, s := range []PaperSize{SizeA4, SizeA5, SizeLetter} {
		if dev.SizeOK(s.key()) {
			n++
		}
	}
	return n == 1
}

func (a *app) dpiField(c *ui.Context) {
	dpis := a.current().DPIs()
	sectionLabel(c, "清晰度", len(dpis) <= 1)
	curDPI := a.dpi
	if !slices.Contains(dpis, curDPI) {
		curDPI = dpis[0]
	}
	curName := dpiLabel(curDPI)
	names := make([]string, len(dpis))
	for i, v := range dpis {
		names[i] = dpiLabel(v)
	}
	if ui.Select(c, &curName, names).FillWidth().Height(38).Radius(9).Changed() {
		for i, name := range names {
			if name == curName {
				a.dpi = dpis[i]
				break
			}
		}
	}
	caption(c, "数字越大越清楚，扫得也越慢。")
}

func dpiLabel(v int) string {
	if v == 300 {
		return "300 dpi（推荐）"
	}
	return itoa(v) + " dpi"
}

var intentLabels = map[string]string{
	"Document":       "文档（推荐）",
	"TextAndGraphic": "图文混排",
	"Photo":          "照片",
	"Preview":        "快速预览",
	"Object":         "实物",
	"BusinessCard":   "名片",
}

func intentLabel(v string) string {
	if s, ok := intentLabels[v]; ok {
		return s
	}
	return v
}

func (a *app) intentField(c *ui.Context) {
	intents := a.current().IntentChoices()
	sectionLabel(c, "扫描内容", len(intents) <= 1)
	cur := a.intent
	if !slices.ContainsFunc(intents, func(x string) bool { return strings.EqualFold(x, cur) }) {
		cur = intents[0]
	}
	curName := intentLabel(cur)
	names := make([]string, len(intents))
	for i, v := range intents {
		names[i] = intentLabel(v)
	}
	if ui.Select(c, &curName, names).FillWidth().Height(38).Radius(9).Changed() {
		for i, name := range names {
			if name == curName {
				a.intent = intents[i]
				break
			}
		}
	}
}

func (a *app) colorField(c *ui.Context) {
	dev := a.current()
	enabled := []bool{true, true, true}
	if dev != nil {
		enabled = []bool{dev.ColorOK(ColorColor.key()), dev.ColorOK(ColorGray.key()), dev.ColorOK(ColorBW.key())}
	}
	sectionLabel(c, "色彩", false)
	cur := int(a.color)
	if segmented(c, &cur, []string{"彩色", "灰度", "黑白"}, enabled) {
		a.color = ColorMode(cur)
	}
}

func (a *app) sourceField(c *ui.Context) {
	dev := a.current()
	fixed := dev != nil && !dev.Feeder
	sectionLabel(c, "放纸方式", fixed)
	enabled := []bool{true, dev != nil && dev.Feeder}
	cur := 0
	if a.feeder {
		cur = 1
	}
	if segmented(c, &cur, []string{"玻璃板", "进纸口"}, enabled) {
		a.feeder = cur == 1
	}
	if a.feeder {
		caption(c, "整叠纸放进进纸口，自动一张张扫。")
	} else {
		caption(c, "把纸平放在玻璃板上，一次一张。")
	}
}

func (a *app) scanButton(c *ui.Context, t *ui.Theme) {
	off := a.devState != DevOnline
	busy := a.scanning || a.probing
	disabled := off || busy
	label := "扫　描"
	switch {
	case a.probing:
		label = "连接设备…"
	case a.scanning:
		label = "扫描中…"
	}

	b := ui.ButtonBase(c).FillWidth().Height(44).Radius(8).Disabled(disabled).
		Transition(ui.ElementTransition{Duration: 140 * time.Millisecond, Colors: true})
	fg := ui.RGB(255, 255, 255)
	switch {
	case disabled:
		b.Background(colLineSoft)
		fg = colFaint
	case b.Pressed():
		b.Background(colAccentPress)
	case b.Hovered():
		b.Background(colAccentHover)
	default:
		b.Background(colAccent)
	}
	if !disabled {
		b.Shadow(0, 3, 10, 0, shadowAccent)
	}
	b.Children(func() {
		ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
			if busy {
				loading.Arc(c, loading.Options{Size: 15.5, Color: fg}).Label(label)
			} else {
				ui.Icon(c, icoScan).FontSize(16).TextColor(fg)
			}
			ui.Text(c, label).FontSize(14).FontWeight(600).TextColor(fg).LetterSpacing(1.1)
		})
	})
	if b.Clicked() {
		a.startScan()
	}
}

func (a *app) scanFeedback(c *ui.Context) {
	switch {
	case a.scanning:
		// 设备不报扫到第几行，这是估算值，所以不给百分比数字。
		ui.Progress(c, a.scanProgress).FillWidth()
	case a.devState == DevOffline && !a.probing:
		notice(c, icoAlert, "这台打印机连不上，换一台试试。", colDanger, colDangerSoft)
	case a.scanStatus != "":
		ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
			ui.Text(c, a.scanStatus).FontSize(11.5).TextColor(colMuted).TextAlign(ui.Center)
		})
	}
}

func (a *app) clearLink(c *ui.Context) {
	disabled := len(a.pages) == 0 || a.scanning
	b := ui.ButtonBase(c).Grow(1).Height(38).Radius(8).Disabled(disabled).
		Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
	fg := colMuted
	if disabled {
		fg = colFaint
	} else if b.Hovered() {
		b.Background(colPanelHover)
		fg = colInk
	}
	b.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icoTrash).FontSize(13).TextColor(fg)
			ui.Text(c, "清空页面").FontSize(12).TextColor(fg)
		})
	})
	if b.Clicked() {
		a.dlg = DlgClearConfirm
	}
}

func (a *app) exportButton(c *ui.Context) {
	enabled := len(a.pages) > 0 && !a.scanning
	b := styledButton(c, "导出", icoDownload, toneOutline, !enabled).Grow(1).Height(38)
	b.Tooltip("PDF 或 JPG，导出全部页面")
	if b.Clicked() {
		a.openExport(-1)
	}
}

func (a *app) statusBar(c *ui.Context, t *ui.Theme) {
	hairline(c)
	ui.Row(c).Height(statusBarHeight).FillWidth().Padding(0, 18, 0, 18).
		Background(colPanel).AlignItems(ui.Center).Gap(10).Children(func() {
		dc := statusColor(a.devState)
		statusDot(c, dc)
		ui.Text(c, a.statusLabel()).FontSize(11.5).FontWeight(500).TextColor(colMuted)

		if len(a.devices) > 0 {
			// 显示格式固定为「设备名 (IPv4)」
			names := make([]string, len(a.devices))
			for i, d := range a.devices {
				if a.probed[d.Idx] && !d.Online {
					names[i] = d.DisplayName() + "（脱机）"
				} else {
					names[i] = d.DisplayName()
				}
			}
			cur := names[a.devSel]
			if ui.Select(c, &cur, names).Width(330).Height(34).Radius(8).Changed() {
				for i, n := range names {
					if n == cur {
						a.devSel = i
						a.refreshState()
						a.probeSelected() // 选谁才探谁——连接成本只在用户点了它时付
						break
					}
				}
			}
		} else {
			ui.Text(c, "还没找到打印机").FontSize(12).TextColor(colMuted)
		}

		btn := ui.ButtonBase(c).Height(32).Padding(0, 11, 0, 11).Radius(7).Disabled(a.detecting || a.netScanning).
			Tooltip("重新找打印机").Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
		if btn.Hovered() {
			btn.Background(colPanelHover)
		}
		spin := a.detecting || a.probing || a.netScanning
		btn.Children(func() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				if spin {
					loading.Arc(c, loading.Options{Size: 13, Color: colMuted}).Label("正在查找打印机")
				} else {
					ui.Icon(c, icoRefresh).FontSize(13).TextColor(colMuted)
				}
				ui.Text(c, "刷新").FontSize(11.5).TextColor(colMuted)
			})
		})
		if btn.Clicked() {
			a.recheckAll()
		}

		ui.Row(c).Grow(1)
		if a.hint != "" {
			ui.Text(c, a.hint).FontSize(11.5).TextColor(colMuted).
				TextAlign(ui.End).MaxWidth(430).SingleLine()
		}
	})
}

func (a *app) overlays(c *ui.Context, t *ui.Theme) {
	if a.lightbox != nil {
		a.lightboxOverlay(c, t)
		return
	}
	switch a.dlg {
	case DlgExport:
		a.exportDialog(c, t)
	case DlgQuitConfirm:
		a.confirmDialog(c, t, "要退出吗？",
			fmt.Sprintf("还有 %d 页没导出，退出后这些页面就没了。", len(a.pages)),
			"退出", func() {
				a.quitConfirmed = true
				a.dlg = DlgNone
				Win.Close()
			})
	case DlgClearConfirm:
		a.confirmDialog(c, t, "要清空全部页面吗？",
			fmt.Sprintf("共 %d 页，清空后无法恢复。", len(a.pages)),
			"清空", func() {
				a.pages = nil
				a.dlg = DlgNone
			})
	}
}

func (a *app) confirmDialog(c *ui.Context, t *ui.Theme, title, detail, okLabel string, onOK func()) {
	open := a.dlg != DlgNone
	ui.Modal(c, &open, func() {
		card := ui.Column(c).Width(404).Background(colPanel).Border(1, colLine).
			Radius(18).Clip().Shadow(0, 28, 70, 0, shadowStrong)
		card.Children(func() {
			ui.Row(c).Padding(22, 24, 0, 24).Gap(13).AlignItems(ui.Start).Children(func() {
				badge := ui.Row(c).Size(40, 40).Radius(12).Background(colDangerSoft).Center().Shrink(0)
				badge.Children(func() {
					ui.Icon(c, icoAlert).FontSize(19).TextColor(colDanger)
				})
				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, title).FontSize(15.5).FontWeight(600).TextColor(colInk)
					ui.Text(c, detail).FontSize(12.5).TextColor(colMuted).LineHeight(1.55)
				})
			})
			ui.Row(c).Padding(20, 24, 20, 24).Justify(ui.End).Gap(9).Children(func() {
				if styledButton(c, "取消", nil, toneGhost, false).Height(38).Padding(0, 18, 0, 18).Clicked() {
					a.dlg = DlgNone
				}
				if styledButton(c, okLabel, nil, toneDanger, false).Height(38).Padding(0, 20, 0, 20).Clicked() {
					onOK()
				}
			})
		})
	})
	if !open {
		a.dlg = DlgNone
	}
}

func (a *app) lightboxOverlay(c *ui.Context, t *ui.Theme) {
	lb := a.lightbox
	white := func(a float32) ui.Color { return ui.RGBA(255, 255, 255, a) }
	ui.Overlay(c, func() {
		back := ui.Column(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).
			Background(ui.RGBA(9, 17, 22, 0.97)).Modal()

		back.Children(func() {
			ui.Row(c).Height(64).Padding(0, 20, 0, 20).
				AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, fmt.Sprintf("第 %d 页", lb.Index+1)).
					FontSize(15).FontWeight(600).TextColor(white(0.95))
				ui.Text(c, fmt.Sprintf("共 %d 页", len(a.pages))).
					FontSize(12).TextColor(white(0.5)).Margin(2, 6, 0, 0)

				ui.Row(c).Grow(1)

				if lb.ConfirmDelete {
					ui.Text(c, "删除这一页？").FontSize(12.5).TextColor(white(0.72))
					if glassButton(c, "取消", nil, false).Clicked() {
						lb.ConfirmDelete = false
					}
					if glassDangerButton(c, "删除", icoTrash, false).Clicked() {
						lb.ConfirmDelete = false
						a.deletePage(a.pages[lb.Index].ID)
						return
					}
				} else {
					if glassButton(c, "旋转", icoRotate, false).Clicked() {
						a.rotatePage(a.pages[lb.Index].ID)
					}
					if glassButton(c, "导出此页", icoDownload, false).Clicked() {
						a.openExport(lb.Index)
					}
					if glassButton(c, "删除此页", icoTrash, false).Clicked() {
						lb.ConfirmDelete = true
					}
				}

				close := ui.ButtonBase(c).Size(38, 38).Radius(19).
					Background(ui.RGBA(255, 255, 255, 0.1)).Tooltip("关闭（Esc）").
					Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
				if close.Hovered() {
					close.Background(ui.RGBA(255, 255, 255, 0.2))
				}
				close.Children(func() {
					ui.Icon(c, icoClose).FontSize(16).TextColor(white(0.92))
				})
				if close.Clicked() {
					a.closeLightbox()
				}
			})

			// Fill + Contain 把原图压进窗口，放大后再偏移查看。
			imgRow := ui.Row(c).Grow(1).MinHeight(0).Center().Padding(8, 44, 8, 44).Clip()
			imgRow.Children(func() {
				if lb.Full == nil {
					ui.Column(c).Gap(10).Center().Children(func() {
						loading.Arc(c, loading.Options{Size: 22.5, Color: white(0.7)}).Label("正在打开")
						ui.Text(c, "正在打开 …").FontSize(12).TextColor(white(0.55))
					})
					return
				}
				w, h := lb.displaySize()
				if w <= 0 || h <= 0 { // 图框还没量过：先铺满，下一帧再按旋转后的大小摆
					lb.img = ui.Image(c, lb.Full).Fill().Fit(ui.Contain).Radius(3).Bounds()
					return
				}
				// 旋转只转里面的图：纸面留在窗口坐标里，阴影和拖拽偏移才不会被转歪。
				paper := ui.Row(c).Size(w, h).Center().
					Shadow(0, 24, 60, 0, ui.RGBA(0, 0, 0, 0.5))
				if lb.Zoom > 1.0 || lb.PanX != 0 || lb.PanY != 0 {
					paper.Translate(lb.PanX, lb.PanY).Scale(lb.Zoom, lb.Zoom)
				}
				paper.Children(func() {
					img := ui.Image(c, lb.Full).Fit(ui.Contain).Radius(3).Shrink(0)
					// 转 90°/270° 后宽高互换，所以先按未旋转的边长摆满再转过来。
					if lb.Rot%180 != 0 {
						img.Size(h, w)
					} else {
						img.Fill()
					}
					if r := lb.Rot % 360; r != 0 {
						img.Rotate(float32(r))
					}
				})
				// 图片区域按摆位算，不用 Bounds：Bounds 是上一帧的，刚转完会慢一帧。
				cb := lb.contentBox()
				lb.img = ui.Rect{
					X: cb.X + cb.W/2 + lb.Zoom*lb.PanX - lb.Zoom*w/2,
					Y: cb.Y + cb.H/2 + lb.Zoom*lb.PanY - lb.Zoom*h/2,
					W: lb.Zoom * w,
					H: lb.Zoom * h,
				}
			})
			lb.row = imgRow.Bounds()

			// 拖拽/缩放只挂在内容区：挂在遮罩上会把按下事件抢走，里面的按钮就收不到点击。
			imgRow.HandleInput(func(ev ui.InputEvent) bool {
				switch ev.Kind {
				case ui.InputScroll:
					lb.zoomBy(1 - ev.DY*0.0015)
					return true
				case ui.InputPointerDown:
					// 事件坐标相对内容区，contentRect 用的是窗口坐标。
					r := lb.contentRect()
					inImage := r.W > 0 && r.Contains(ev.X+lb.row.X, ev.Y+lb.row.Y)
					if ev.Button != 0 || !inImage {
						return false // 没按在图上：交给遮罩当普通点击，按钮也才收得到
					}
					if ev.Clicks >= 2 {
						lb.resetView()
						return true
					}
					lb.dragging = true
					lb.dragX, lb.dragY = ev.X, ev.Y
					lb.panStartX, lb.panStartY = lb.PanX, lb.PanY
					return true
				case ui.InputPointerMove:
					if !lb.dragging {
						return false
					}
					dx, dy := ev.X-lb.dragX, ev.Y-lb.dragY
					if lb.Zoom > 1.0 {
						lb.PanX, lb.PanY = lb.panStartX+dx, lb.panStartY+dy
						lb.clampPan()
					}
					return true
				case ui.InputPointerUp:
					if !lb.dragging {
						return false
					}
					lb.dragging = false
					return true
				}
				return false
			})

			// 点遮罩空白处：正在确认删除就先撤回，否则关灯箱。
			if back.Clicked() {
				if lb.ConfirmDelete {
					lb.ConfirmDelete = false
				} else {
					a.closeLightbox()
				}
			}

			back.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputScroll { // 指针不在内容区时也要能缩放
					lb.zoomBy(1 - ev.DY*0.0015)
					return true
				}
				return false
			})

			ui.Row(c).Height(72).Padding(0, 20, 0, 16).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Width(210)

				ui.Row(c).Grow(1).Justify(ui.Center).Children(func() {
					nav := ui.Row(c).Gap(2).AlignItems(ui.Center).Padding(4, 4, 4, 4).Radius(12).
						Background(ui.RGBA(255, 255, 255, 0.08)).
						Border(1, ui.RGBA(255, 255, 255, 0.12))
					nav.Children(func() {
						if navButton(c, "上一页", icoArrowLeft, lb.Index == 0).Clicked() {
							a.lightboxNav(-1)
						}
						ui.Row(c).Width(76).Justify(ui.Center).Children(func() {
							ui.Text(c, fmt.Sprintf("%d / %d", lb.Index+1, len(a.pages))).
								FontSize(12.5).FontWeight(500).TextColor(white(0.9))
						})
						if navButton(c, "下一页", icoArrowRight, lb.Index+1 >= len(a.pages)).Clicked() {
							a.lightboxNav(1)
						}
					})
				})

				ui.Row(c).Width(210).Justify(ui.End).AlignItems(ui.Center).Gap(2).Children(func() {
					if glassIconButton(c, icoMinus, "缩小", 32).Clicked() {
						lb.zoomBy(1 / 1.3)
					}
					pct := ui.ButtonBase(c).Height(32).Padding(0, 8, 0, 8).Radius(7).
						Tooltip("点一下还原大小")
					if pct.Hovered() {
						pct.Background(ui.RGBA(255, 255, 255, 0.12))
					}
					pct.Children(func() {
						ui.Text(c, fmt.Sprintf("%.0f%%", lb.Zoom*100)).
							FontSize(11.5).TextColor(white(0.72)).SingleLine()
					})
					if pct.Clicked() {
						lb.resetView()
					}
					if glassIconButton(c, icoPlus, "放大", 32).Clicked() {
						lb.zoomBy(1.3)
					}
				})
			})
		})

		if back.OverlayShortcut(0, ui.KeyEscape) {
			if lb.ConfirmDelete {
				lb.ConfirmDelete = false
			} else {
				a.closeLightbox()
			}
		}
		if back.OverlayShortcut(0, ui.KeyLeft) {
			a.lightboxNav(-1)
		}
		if back.OverlayShortcut(0, ui.KeyRight) {
			a.lightboxNav(1)
		}
	})
	a.requestLightboxImage()
}

func (a *app) lightboxNav(d int) {
	lb := a.lightbox
	if lb == nil {
		return
	}
	ni := lb.Index + d
	if ni < 0 || ni >= len(a.pages) {
		return
	}
	lb.Index = ni
	lb.Full = nil
	lb.Dirty = true
	lb.resetView()
	a.requestLightboxImage()
}
