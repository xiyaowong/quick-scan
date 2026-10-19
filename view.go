// 主视图：布局对照 docs/ui-mockup.html，交互语义对照设计定稿。
// mygo 原生 UI 即时模式：view 是状态的函数，每帧重建。
package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *app) view(c *ui.Context) {
	// 纯白主题（定稿 §三-1）：不跟随系统深色模式，显式覆盖
	c.SetTheme(lightTheme())
	t := c.Theme()
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			a.pagesPanel(c, t)
			a.settingsPanel(c, t)
		})
		a.statusBar(c, t)
	})
	a.overlays(c, t)
}

// ── 左侧：扫描页面区 ──
func (a *app) pagesPanel(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).Padding(14).Gap(10).Justify(ui.Start).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "扫描页面").FontSize(16).Bold()
			ui.Text(c, fmt.Sprintf("共 %d 页", len(a.pages))).TextColor(t.TextMuted).Padding(0, 0, 0, 8)
		})

		if len(a.pages) == 0 {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Text(c, "还没有扫描的页面").TextColor(t.TextMuted)
				ui.Text(c, "放好纸，点右侧「扫描」").TextColor(t.TextMuted).FontSize(13)
			})
		} else {
			ui.GridView(c, &a.pageGrid, len(a.pages), 130, 180, func(i int) {
				a.pageCell(c, t, i)
			}).Grow(1).Gap(10)
			ui.Text(c, "拖动排序 · 双击放大").TextColor(t.TextMuted).FontSize(12)
		}
	})
}

// pageCell 一张缩略图卡片
func (a *app) pageCell(c *ui.Context, t *ui.Theme, i int) {
	p := &a.pages[i]
	// 不在这里 .Drag()：GridView 的 Reorder 已在格子外层挂了拖拽源，
	// 内层再挂一个会把按下事件抢走，拖拽排序就废了。
	cell := ui.Column(c.Key(p.ID)).Gap(4)
	cell.Children(func() {
		ui.Image(c, p.thumb()).Size(120, 140).Fit(ui.Contain).
			Border(1, t.Border).Radius(4).Background(ui.RGB(250, 250, 250))
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, itoa(i+1)).FontSize(12).TextColor(t.TextMuted)
			ui.Row(c).Grow(1)
			// 导出此页（旧版沿用的 ↓ 入口；openExport(i) 只导这一页）
			if ui.Button(c, "↓").Tooltip("导出此页").Clicked() {
				a.openExport(i)
			}
			if ui.Button(c, "↻").Tooltip("旋转").Clicked() {
				a.rotatePage(p.ID)
			}
			if ui.Button(c, "✕").Tooltip("删除").Clicked() {
				a.deletePage(p.ID)
			}
		})
	})
	// 双击整张卡片放大（放在卡片上，Image 本身不接收双击）
	if cell.DoubleClicked() {
		a.openLightbox(i)
	}
}

// ── 右侧：操作区 ──
func (a *app) settingsPanel(c *ui.Context, t *ui.Theme) {
	dev := a.current()
	ui.Column(c).Width(300).Padding(14).Gap(10).Justify(ui.Start).Children(func() {
		// 纸张大小
		a.fieldLabel(c, "纸张大小", dev != nil && a.sizeSelectable(dev))
		ui.RadioGroup(c, func() {
			a.sizeRadio(c, dev, SizeA4)
			a.sizeRadio(c, dev, SizeA5)
			a.sizeRadio(c, dev, SizeLetter)
		})

		// 清晰度：设备支持多少档就列多少档
		dpis := dpiChoices(dev)
		a.fieldLabel(c, "清晰度", len(dpis) <= 1)
		curDPI := a.dpi
		if !containsInt(dpis, curDPI) {
			curDPI = dpis[0]
		}
		curDPIName := dpiLabel(curDPI)
		dpiNames := make([]string, len(dpis))
		for i, v := range dpis {
			dpiNames[i] = dpiLabel(v)
		}
		if ui.Select(c, &curDPIName, dpiNames).FillWidth().Changed() {
			for i, name := range dpiNames {
				if name == curDPIName {
					a.dpi = dpis[i]
					break
				}
			}
		}
		ui.Text(c, "数字越大越清晰，扫描也越慢。").FontSize(12).TextColor(t.TextMuted)

		// 用途（eSCL Intent）：设备支持哪些就列哪些
		intents := intentChoices(dev)
		a.fieldLabel(c, "用途", len(intents) <= 1)
		curIntent := a.intent
		if !containsString(intents, curIntent) {
			curIntent = intents[0]
		}
		curIntentName := intentLabel(curIntent)
		intentNames := make([]string, len(intents))
		for i, v := range intents {
			intentNames[i] = intentLabel(v)
		}
		if ui.Select(c, &curIntentName, intentNames).FillWidth().Changed() {
			for i, name := range intentNames {
				if name == curIntentName {
					a.intent = intents[i]
					break
				}
			}
		}
		ui.Text(c, "按内容选，拿不准就用「文档」：图文混排选「图文混排」，照片选「照片」，只想快速看效果选「快速预览」。").FontSize(12).TextColor(t.TextMuted)

		// 色彩
		a.fieldLabel(c, "色彩", false)
		ui.RadioGroup(c, func() {
			ui.Radio(c, &a.color, ColorColor, "彩色").Disabled(dev != nil && !dev.colorOK(ColorColor))
			ui.Radio(c, &a.color, ColorGray, "灰度").Disabled(dev != nil && !dev.colorOK(ColorGray))
			ui.Radio(c, &a.color, ColorBW, "黑白").Disabled(dev != nil && !dev.colorOK(ColorBW))
		})

		// 放纸方式
		a.fieldLabel(c, "放纸方式", dev != nil && !dev.Feeder)
		ui.RadioGroup(c, func() {
			ui.Radio(c, &a.feeder, false, "玻璃板").Tooltip("掀开盖子，把纸平放在玻璃板上，一次一张")
			ui.Radio(c, &a.feeder, true, "进纸口").Tooltip("整叠纸放进进纸口，自动一张张扫").
				Disabled(dev == nil || !dev.Feeder)
		})

		ui.Row(c).Height(1).Background(t.Border)

		// 扫描按钮
		scanLabel := "扫　描"
		if a.scanning {
			scanLabel = "扫描中 …"
		}
		if a.probing {
			scanLabel = "连接设备 …"
		}
		if ui.PrimaryButton(c, scanLabel).FillWidth().
			Disabled(a.scanning || a.probing || a.devState != DevOnline).Clicked() {
			a.startScan()
		}
		if a.devState == DevOffline && !a.probing {
			ui.Text(c, "这台连不上，可能已脱机——换一台，或点 ⟳ 重新检测").FontSize(12).TextColor(t.Danger)
		}
		if a.scanStatus != "" {
			ui.Text(c, a.scanStatus).FontSize(12).TextColor(t.TextMuted)
		}

		// 清空页面
		if ui.Button(c, "清空页面").FillWidth().Disabled(len(a.pages) == 0 || a.scanning).Clicked() {
			a.dlg = DlgClearConfirm
		}

		// 导出
		if ui.PrimaryButton(c, "导　出").FillWidth().Disabled(len(a.pages) == 0 || a.scanning).Clicked() {
			a.openExport(-1)
		}
	})
	ui.Row(c).Width(1).Background(t.Border)
}

func (a *app) fieldLabel(c *ui.Context, name string, fixed bool) {
	label := name
	if fixed {
		label += "（固定）"
	}
	ui.Text(c, label).FontSize(13).TextColor(c.Theme().TextMuted)
}

func (a *app) sizeRadio(c *ui.Context, dev *DeviceInfo, s PaperSize) {
	ui.Radio(c, &a.size, s, s.label()).Disabled(dev != nil && !dev.sizeOK(s))
}

// sizeSelectable 只有一种纸型可选时才置灰标"固定"（定稿 §三-5）
func (a *app) sizeSelectable(dev *DeviceInfo) bool {
	if dev == nil {
		return false
	}
	n := 0
	for _, s := range []PaperSize{SizeA4, SizeA5, SizeLetter} {
		if dev.sizeOK(s) {
			n++
		}
	}
	return n == 1
}

func dpiLabel(v int) string {
	if v == 300 {
		return "300 dpi（推荐）"
	}
	return itoa(v) + " dpi"
}

// intentLabel eSCL Intent → 界面中文；设备偶尔上报非标准值时原样显示。
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

// ── 底部状态栏 ──
func (a *app) statusBar(c *ui.Context, t *ui.Theme) {
	ui.Row(c).Height(1).Background(t.Border)
	ui.Row(c).Height(36).Padding(0, 12, 0, 12).AlignItems(ui.Center).Gap(8).Children(func() {
		dot := ui.Text(c, "●").FontSize(12)
		switch a.devState {
		case DevOnline:
			dot.TextColor(ui.RGB(34, 180, 90))
		case DevUnknown:
			dot.TextColor(t.TextMuted) // 未探测：不冒充在线也不吓人
		default:
			dot.TextColor(t.Danger)
		}
		if len(a.devices) > 0 {
			// 显示格式固定为「设备名 (IPv4)」
			names := make([]string, len(a.devices))
			for i, d := range a.devices {
				// 明确离线的在名字上标出来，用户一眼看清为什么别选它
				if a.probed[d.Idx] && !d.Online {
					names[i] = d.DisplayName() + "（脱机）"
				} else {
					names[i] = d.DisplayName()
				}
			}
			// 当前值也取自 names——收起状态下只显示这一项，
			// 脱机标注不能只写给展开的列表看
			cur := names[a.devSel]
			if ui.Select(c, &cur, names).Width(320).Changed() {
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
			ui.Text(c, "未检测到设备").TextColor(t.TextMuted)
		}
		if ui.Button(c, "⟳").Disabled(a.detecting).Clicked() {
			a.recheckAll()
		}
		if a.hint != "" {
			ui.Text(c, a.hint).FontSize(12).TextColor(t.TextMuted)
		}
	})
}

// ── 弹层（页面内弹窗 + 灯箱）──
func (a *app) overlays(c *ui.Context, t *ui.Theme) {
	// 灯箱
	if a.lightbox != nil {
		a.lightboxOverlay(c, t)
		return
	}
	// 对话框
	switch a.dlg {
	case DlgExport:
		a.exportDialog(c, t)
	case DlgQuitConfirm:
		a.confirmDialog(c, t, "要退出吗？",
			fmt.Sprintf("还有 %d 页没导出，退出后这些页面就没了。", len(a.pages)),
			"退出", func() {
				a.quitConfirmed = true
				a.dlg = DlgNone
				win.Close()
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

// confirmDialog 通用确认弹窗
func (a *app) confirmDialog(c *ui.Context, t *ui.Theme, title, detail, okLabel string, onOK func()) {
	open := a.dlg != DlgNone
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(380).Padding(20).Gap(14).Background(t.Background).
			Radius(10).Border(1, t.Border).Children(func() {
			ui.Text(c, title).FontSize(16).Bold()
			ui.Text(c, detail).TextColor(t.TextMuted)
			ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					a.dlg = DlgNone
				}
				if ui.PrimaryButton(c, okLabel).Clicked() {
					onOK()
				}
			})
		})
	})
	if !open {
		a.dlg = DlgNone
	}
}

// ── 灯箱（双击放大）──
func (a *app) lightboxOverlay(c *ui.Context, t *ui.Theme) {
	lb := a.lightbox
	ui.Overlay(c, func() {
		// 背景层直接用 Column 撑满：Absolute 四边贴 0 就能定尺寸，
		// 不必再套一层 Fill()（percent 尺寸在绝对定位父级里量不出高度）。
		back := ui.Column(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).
			Background(ui.RGBA(0, 0, 0, 0.82)).Modal()
		back.Children(func() {
			// 顶部栏
			ui.Row(c).Padding(10).AlignItems(ui.Center).Gap(10).Children(func() {
				ui.Text(c, fmt.Sprintf("第 %d 页 / 共 %d 页", lb.Index+1, len(a.pages))).
					TextColor(ui.RGB(255, 255, 255))
				ui.Row(c).Grow(1)
				if ui.Button(c, "↓ 导出此页").Clicked() {
					a.openExport(lb.Index)
				}
				if ui.Button(c, "↻ 旋转").Clicked() {
					a.rotatePage(a.pages[lb.Index].ID)
				}
				if ui.Button(c, "✕ 关闭").Clicked() {
					a.closeLightbox()
				}
			})
			// 大图区
			// 图片行自己 Clicked() 一下（忽略结果）：把它标成 interactive，
			// 点大图才不会落到遮罩层上（遮罩的 Clicked 关灯箱）。
			imgRow := ui.Row(c).Grow(1).Center()
			imgRow.Children(func() {
				if lb.Full != nil {
					// Fill 而不是 Grow：原图是 300dpi 全尺寸（几千 DIP），
					// Grow 只会放大不会缩小，得用 Fill 把框压到父级大小，
					// 再由 Contain 等比缩进去。
					img := ui.Image(c, lb.Full).Fill().Fit(ui.Contain)
					if lb.Zoom > 1.0 {
						img.Scale(lb.Zoom, lb.Zoom)
					}
				} else {
					ui.Text(c, "正在加载 …").TextColor(ui.RGB(200, 200, 200))
				}
			})
			imgRow.Clicked()
			// 滚轮缩放（Ctrl/普通滚轮都缩；向上放大向下缩小）
			imgRow.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputScroll {
					lb.zoomBy(1.0 + ev.DY*-0.001)
					return true
				}
				return false
			})
			// 底部翻页
			ui.Row(c).Padding(10).Justify(ui.Center).Gap(12).Children(func() {
				if ui.Button(c, "← 上一页").Disabled(lb.Index == 0).Clicked() {
					a.lightboxNav(-1)
				}
				ui.Text(c, fmt.Sprintf("%.0f%% · 滚轮缩放", lb.Zoom*100)).TextColor(ui.RGB(160, 160, 160)).FontSize(12)
				if ui.Button(c, "下一页 →").Disabled(lb.Index+1 >= len(a.pages)).Clicked() {
					a.lightboxNav(1)
				}
			})
		})
		// 点击遮罩（backdrop 本身，非内部控件/大图）关闭
		if back.Clicked() {
			a.closeLightbox()
		}
		// 键盘：Esc 关闭，方向键翻页
		if back.OverlayShortcut(0, ui.KeyEscape) {
			a.closeLightbox()
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
