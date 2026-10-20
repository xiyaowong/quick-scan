// 通用界面组件：把设计令牌包成可复用的小部件，
// view.go / dialog.go 只描述结构，不再逐处写颜色与悬停逻辑。
package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// ── 文本与分隔 ──

// sectionLabel 参数区的小标题：字距略宽、全大写感的标签。
func sectionLabel(c *ui.Context, text string, fixed bool) {
	ui.Row(c).AlignItems(ui.Center).Gap(7).Children(func() {
		ui.Text(c, text).FontSize(11.5).FontWeight(500).LetterSpacing(0.3).TextColor(colMuted)
		if fixed {
			ui.Text(c, "固定").FontSize(9.5).FontWeight(500).LetterSpacing(0.2).
				TextColor(colFaint).Padding(2, 5, 2, 5).Radius(4).
				Background(colPanelSoft).Border(1, colLineSoft)
		}
	})
}

// caption 参数下方的解释文字。
func caption(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(11.5).TextColor(colMuted).LineHeight(1.55)
}

func hairline(c *ui.Context) {
	ui.Row(c).Height(1).FillWidth().Background(colLineSoft)
}

// ── 按钮 ──

type buttonTone int

const (
	toneGhost buttonTone = iota
	toneOutline
	tonePrimary
	toneDanger
)

// styledButton 统一按钮外观；返回元素以便调用方继续链式设置尺寸与读取 Clicked。
func styledButton(c *ui.Context, label string, ic *ui.SVG, tone buttonTone, disabled bool) ui.Element {
	b := ui.ButtonBase(c).Height(40).Padding(0, 16, 0, 16).Radius(8).Disabled(disabled).
		Transition(ui.ElementTransition{Duration: 130 * time.Millisecond, Colors: true})

	hover := b.Hovered() && !disabled
	pressed := b.Pressed() && !disabled

	fg := colInk
	switch tone {
	case tonePrimary:
		fg = ui.RGB(255, 255, 255)
		switch {
		case disabled:
			b.Background(colLineSoft)
			fg = colFaint
		case pressed:
			b.Background(colAccentPress)
		case hover:
			b.Background(colAccentHover)
		default:
			b.Background(colAccent)
		}
		if !disabled {
			b.Shadow(0, 2, 8, 0, shadowAccent)
		}
	case toneDanger:
		fg = ui.RGB(255, 255, 255)
		switch {
		case disabled:
			b.Background(colLineSoft)
			fg = colFaint
		case pressed:
			b.Background(ui.Hex("#8F2E24"))
		case hover:
			b.Background(ui.Hex("#9C3127"))
		default:
			b.Background(colDanger)
		}
		if !disabled {
			b.Shadow(0, 5, 14, 0, ui.RGBA(178, 58, 46, 0.24))
		}
	case toneOutline:
		// 微信式次要按钮：浅灰填充、深色文字，不用彩色描边抢主按钮的注意力。
		fg = colInk
		b.Background(ui.Hex("#F2F2F2"))
		switch {
		case disabled:
			b.Background(ui.Hex("#F7F7F7"))
			fg = colFaint
		case pressed:
			b.Background(ui.Hex("#DEDEDE"))
		case hover:
			b.Background(ui.Hex("#E8E8E8"))
		}
	default: // ghost
		fg = colMuted
		b.Background(ui.Transparent)
		switch {
		case disabled:
			fg = colFaint
		case pressed:
			b.Background(colLineSoft)
		case hover:
			b.Background(colPanelHover)
			fg = colInk
		}
	}

	weight := 500
	if tone == tonePrimary || tone == toneOutline || tone == toneDanger {
		weight = 600
	}
	b.Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			if ic != nil {
				ui.Icon(c, ic).FontSize(14.5).TextColor(fg)
			}
			ui.Text(c, label).FontSize(13.5).FontWeight(weight).TextColor(fg).SingleLine()
		})
	})
	return b
}

// iconButton 24–28px 的图标按钮，用于卡片操作与状态栏。
func iconButton(c *ui.Context, ic *ui.SVG, tip string, size float32) ui.Element {
	b := ui.ButtonBase(c).Size(size, size).Radius(7).
		Tooltip(tip).Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
	hover := b.Hovered()
	if hover {
		b.Background(colPanelHover)
	}
	fg := colMuted
	if hover {
		fg = colInk
	}
	b.Children(func() {
		ui.Icon(c, ic).FontSize(size * 0.52).TextColor(fg)
	})
	return b
}

// cardButton 页面卡片上的文字按钮：给不熟悉图标的人一个明确的「查看」入口。
func cardButton(c *ui.Context, ic *ui.SVG, label string) ui.Element {
	b := ui.ButtonBase(c).Height(30).Padding(0, 8, 0, 7).Radius(6).
		Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
	hover := b.Hovered()
	if hover {
		b.Background(colPanelHover)
	}
	fg := colInk
	if hover {
		fg = colAccent
	}
	b.Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, ic).FontSize(12.5).TextColor(fg)
			ui.Text(c, label).FontSize(11.5).TextColor(fg).SingleLine()
		})
	})
	return b
}

// dangerIconButton 删除等破坏性操作：悬停时泛红。
func dangerIconButton(c *ui.Context, ic *ui.SVG, tip string, size float32) ui.Element {
	b := ui.ButtonBase(c).Size(size, size).Radius(7).
		Tooltip(tip).Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
	hover := b.Hovered()
	if hover {
		b.Background(colDangerSoft)
	}
	fg := colMuted
	if hover {
		fg = colDanger
	}
	b.Children(func() {
		ui.Icon(c, ic).FontSize(size * 0.52).TextColor(fg)
	})
	return b
}

// ── 分段控件 ──

// segmented 生成一个与 *cur 绑定的分段选择器；enabled 为 nil 表示全部可用。
// 返回 Track.Changed()，调用方据此把 cur 写回业务状态。
func segmented(c *ui.Context, cur *int, labels []string, enabled []bool) bool {
	parts := ui.SegmentedBase(c, cur, len(labels))
	track := parts.Track
	track.FillWidth().AlignItems(ui.Stretch).Padding(3).Gap(3).Radius(9).
		Background(ui.Hex("#F0F0F0")).Border(1, colLineSoft)
	track.Children(func() {
		for i, label := range labels {
			ok := enabled == nil || (i < len(enabled) && enabled[i])
			on := *cur == i
			seg := parts.Segment(i).Disabled(!ok).Grow(1).Height(30).Radius(7).
				Transition(ui.ElementTransition{Duration: 130 * time.Millisecond, Colors: true})
			hover := seg.Hovered() && ok && !on

			fg := ui.Hex("#666666")
			switch {
			case !ok:
				fg = colFaint
				seg.Background(ui.Transparent)
			case on:
				// 微信的选中段是“白色浮起的小片 + 绿色文字”，不是整块绿。
				fg = colAccent
				seg.Background(colPanel).Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.10))
			case hover:
				fg = colInk
				seg.Background(ui.Hex("#E8E8E8"))
			}
			weight := 400
			if on && ok {
				weight = 600
			}
			seg.Children(func() {
				ui.Text(c, label).FontSize(12.5).FontWeight(weight).TextColor(fg).SingleLine()
			})
		}
	})
	return track.Changed()
}

// ── 徽标与状态 ──

// countBadge “共 N 页” 的小胶囊。
func countBadge(c *ui.Context, n int) {
	ui.Row(c).AlignItems(ui.Center).Gap(5).Padding(6, 12, 6, 12).Radius(999).
		Background(colPanel).Border(1, colLine).Children(func() {
		ui.Text(c, "共").FontSize(11.5).TextColor(colMuted)
		ui.Text(c, itoa(n)).FontSize(13).FontWeight(700).TextColor(colAccent)
		ui.Text(c, "页").FontSize(11.5).TextColor(colMuted)
	})
}

// statusDot 带柔光底的状态圆点。
func statusDot(c *ui.Context, color ui.Color) {
	halo := ui.Row(c).Size(20, 20).Radius(10).Center().Background(color.Alpha(0.12))
	halo.Children(func() {
		ui.Row(c).Size(8, 8).Radius(4).Background(color)
	})
}

// notice 参数区里的提示条：图标 + 多行文字。
func notice(c *ui.Context, ic *ui.SVG, text string, fg, bg ui.Color) {
	ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(9).Padding(10, 12, 10, 12).
		Radius(9).Background(bg).Children(func() {
		ui.Icon(c, ic).FontSize(14).TextColor(fg).Margin(1, 0, 0, 0).Shrink(0)
		ui.Text(c, text).FontSize(11.5).TextColor(fg).LineHeight(1.5)
	})
}

// successPanel 导出完成后的浅色成功卡片。
func successPanel(c *ui.Context, title, sub string) {
	ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(11).Padding(13, 14, 13, 14).
		Radius(11).Background(colOkSoft).Children(func() {
		ui.Icon(c, icoCheckCircle).FontSize(19).TextColor(colOk).Margin(1, 0, 0, 0).Shrink(0)
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(colInk)
			if sub != "" {
				ui.Text(c, sub).FontSize(11.5).TextColor(colMuted).LineHeight(1.4)
			}
		})
	})
}

// glassButton 灯箱里的半透明深色按钮。
func glassButton(c *ui.Context, label string, ic *ui.SVG, disabled bool) ui.Element {
	b := ui.ButtonBase(c).Height(34).Padding(0, 13, 0, 13).Radius(8).Disabled(disabled).
		Background(ui.RGBA(255, 255, 255, 0.10)).Border(1, ui.RGBA(255, 255, 255, 0.14)).
		Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
	fg := ui.RGBA(255, 255, 255, 0.92)
	switch {
	case disabled:
		fg = ui.RGBA(255, 255, 255, 0.32)
	case b.Hovered():
		b.Background(ui.RGBA(255, 255, 255, 0.18))
	}
	b.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if ic != nil {
				ui.Icon(c, ic).FontSize(13).TextColor(fg)
			}
			ui.Text(c, label).FontSize(12.5).TextColor(fg).SingleLine()
		})
	})
	return b
}

// navButton 灯箱底部的翻页按钮：比普通玻璃按钮高一档，好点。
func navButton(c *ui.Context, label string, ic *ui.SVG, disabled bool) ui.Element {
	return glassButton(c, label, ic, disabled).Height(38).Padding(0, 14, 0, 14)
}

// glassIconButton 灯箱里的方形图标按钮（缩放等）。
func glassIconButton(c *ui.Context, ic *ui.SVG, tip string, size float32) ui.Element {
	b := ui.ButtonBase(c).Size(size, size).Radius(8).Tooltip(tip).
		Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
	if b.Hovered() {
		b.Background(ui.RGBA(255, 255, 255, 0.18))
	}
	b.Children(func() {
		ui.Icon(c, ic).FontSize(size * 0.5).TextColor(ui.RGBA(255, 255, 255, 0.85))
	})
	return b
}

// glassDangerButton 灯箱里的删除确认：红底，和普通操作拉开距离。
func glassDangerButton(c *ui.Context, label string, ic *ui.SVG, disabled bool) ui.Element {
	b := ui.ButtonBase(c).Height(34).Padding(0, 14, 0, 14).Radius(8).Disabled(disabled).
		Background(colDanger).Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
	switch {
	case disabled:
		b.Background(ui.RGBA(255, 255, 255, 0.1))
	case b.Hovered():
		b.Background(ui.Hex("#E04848"))
	}
	fg := ui.RGB(255, 255, 255)
	if disabled {
		fg = ui.RGBA(255, 255, 255, 0.32)
	}
	b.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if ic != nil {
				ui.Icon(c, ic).FontSize(13).TextColor(fg)
			}
			ui.Text(c, label).FontSize(12.5).FontWeight(500).TextColor(fg).SingleLine()
		})
	})
	return b
}
