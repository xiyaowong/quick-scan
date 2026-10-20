// 导出对话框 + 灯箱图像加载 + 页面操作。
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"sscan/internal/scanner"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// openExport 打开导出对话框；single >= 0 时只导出那一页
func (a *app) openExport(single int) {
	a.exportSingle = single
	a.exportPhase = PhaseIdle
	a.exportFmt = ExportPDF
	a.exportFilename = a.defaultFilename()
	a.ocrPicked = false
	a.dlg = DlgExport
}

func (a *app) defaultFilename() string {
	return "扫描_" + time.Now().Format("20060102_1504")
}

// exportDialog 导出对话框（页面内，两步：格式 → 文件名 → 系统另存为）
func (a *app) exportDialog(c *ui.Context, t *ui.Theme) {
	open := a.dlg == DlgExport
	ui.Modal(c, &open, func() {
		card := ui.Column(c).Width(452).Background(colPanel).Border(1, colLine).
			Radius(12).Clip().Shadow(0, 24, 60, 0, shadowStrong)
		card.Children(func() {
			title := "导出"
			subtitle := fmt.Sprintf("导出全部 %d 页", len(a.pages))
			if a.exportSingle >= 0 {
				title = fmt.Sprintf("导出第 %d 页", a.exportSingle+1)
				subtitle = fmt.Sprintf("仅导出第 %d 页（共 %d 页）", a.exportSingle+1, len(a.pages))
			}
			ui.Row(c).Padding(20, 22, 0, 22).Gap(13).AlignItems(ui.Center).Children(func() {
				badge := ui.Row(c).Size(40, 40).Radius(10).Background(colAccentSoft).Center().Shrink(0)
				badge.Children(func() {
					ui.Icon(c, icoDownload).FontSize(19).TextColor(colAccent)
				})
				ui.Column(c).Gap(3).Children(func() {
					ui.Text(c, title).FontSize(16).FontWeight(600).TextColor(colInk)
					ui.Text(c, subtitle).FontSize(12).TextColor(colMuted)
				})
			})
			ui.Column(c).Padding(18, 22, 0, 22).Gap(16).Children(func() {
				switch a.exportPhase {
				case PhaseWorking:
					a.exportProgressView(c, t)
				case PhaseDone:
					a.exportDoneView(c, t)
				default:
					a.exportFormView(c, t)
				}
			})
		})
	})
	if !open {
		a.dlg = DlgNone
	}
}

func (a *app) exportProgressView(c *ui.Context, t *ui.Theme) {
	frac := 0.0
	if a.exportTotalN > 0 {
		frac = float64(a.exportDoneN) / float64(a.exportTotalN)
	}
	ui.Column(c).Gap(9).Children(func() {
		ui.Progress(c, frac).FillWidth()
		ui.Text(c, fmt.Sprintf("正在导出 … %d / %d 页", a.exportDoneN, a.exportTotalN)).
			FontSize(12).TextColor(colMuted)
	})
}

// exportFormView 格式 + 文件名 + 导出按钮
func (a *app) exportFormView(c *ui.Context, t *ui.Theme) {
	sectionLabel(c, "格式", false)
	cur := 0
	if a.exportFmt == ExportJPG {
		cur = 1
	}
	if segmented(c, &cur, []string{"PDF 文件", "每页一张图"}, nil) {
		a.exportFmt = ExportPDF
		if cur == 1 {
			a.exportFmt = ExportJPG
		}
	}
	if a.exportFmt == ExportJPG {
		caption(c, "每页存成一张图片。")
	} else {
		caption(c, "全部页面合成一个文件。")
	}

	sectionLabel(c, "文件名", false)
	ui.TextInput(c, &a.exportFilename).FillWidth().Height(40).Radius(8)

	if len(a.ocrNames) > 0 {
		sectionLabel(c, "识别到的名称", false)
		ui.Row(c).Gap(6).Wrap().Children(func() {
			for _, name := range a.ocrNames {
				n := name
				b := ui.ButtonBase(c).Height(28).Padding(0, 10, 0, 10).Radius(6).
					Background(ui.Hex("#F2F2F2"))
				if b.Hovered() {
					b.Background(ui.Hex("#E8E8E8"))
				}
				b.Children(func() {
					ui.Text(c, n).FontSize(11.5).TextColor(colInk).SingleLine()
				})
				if b.Clicked() {
					a.applyOCRName(n)
				}
			}
		})
	}

	ui.Row(c).Padding(4, 0, 20, 0).Justify(ui.End).Gap(9).Children(func() {
		if styledButton(c, "取消", nil, toneGhost, false).Height(38).Padding(0, 18, 0, 18).Clicked() {
			a.dlg = DlgNone
		}
		if styledButton(c, "导出", icoDownload, tonePrimary, false).Height(38).Clicked() {
			a.runExport()
		}
	})
}

// exportDoneView 导出完成态
func (a *app) exportDoneView(c *ui.Context, t *ui.Theme) {
	successPanel(c, "已导出", fmt.Sprintf("共 %d 页", a.exportDone.PageCount))
	if a.exportDone.Display != "" {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(10, 12, 10, 12).
			Radius(8).Background(colPanelSoft).Children(func() {
			ui.Icon(c, icoFolder).FontSize(14).TextColor(colMuted).Shrink(0)
			ui.Text(c, a.exportDone.Display).FontSize(11.5).TextColor(colMuted).
				SingleLine()
		})
	}
	ui.Row(c).Padding(4, 0, 20, 0).Justify(ui.End).Gap(9).Children(func() {
		if a.exportDone.File != "" {
			if styledButton(c, "打开文件", icoFile, toneGhost, false).Height(38).Clicked() {
				mygo.Shell.OpenPath(a.exportDone.File)
			}
		}
		if styledButton(c, "打开所在文件夹", icoFolder, toneGhost, false).Height(38).Clicked() {
			target := a.exportDone.File
			if target == "" {
				target = a.exportDone.Dir
			}
			mygo.Shell.ShowItemInFolder(target)
		}
		if styledButton(c, "完成", nil, tonePrimary, false).Height(38).Padding(0, 22, 0, 22).Clicked() {
			a.dlg = DlgNone
		}
	})
}

// runExport 系统另存为 → 后台导出
func (a *app) runExport() {
	if a.exportPicking {
		return
	}
	a.exportPicking = true
	name := a.exportFilename
	if name == "" {
		name = a.defaultFilename()
	}
	fmt_ := a.exportFmt
	startDir := a.lastExportDir
	if startDir == "" {
		home, _ := os.UserHomeDir()
		startDir = filepath.Join(home, "Documents", "扫描件")
	}
	targets := a.exportTargets()
	pages := make([]scanner.ExportPage, len(targets))
	for i, p := range targets {
		pages[i] = scanner.ExportPage{Path: p.FullPath, Rot: p.Rot}
	}

	go func() {
		ext, filter := "pdf", "PDF 文件"
		if fmt_ == ExportJPG {
			ext, filter = "jpg", "JPG 图片"
		}
		def := filepath.Join(startDir, name+"."+ext)
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      Win,
			Title:       "导出",
			DefaultPath: def,
			Filters:     []mygo.FileFilter{{Name: filter, Extensions: []string{ext}}},
		})
		Win.Update(func() { a.exportPicking = false })
		if err != nil || path == "" {
			return // 用户取消
		}
		dir := filepath.Dir(path)
		base := filepath.Base(path)
		base = base[:len(base)-len(filepath.Ext(base))] // 去扩展名

		Win.Update(func() {
			a.exportPhase = PhaseWorking
			a.exportDoneN = 0
			a.exportTotalN = len(pages)
		})
		out, expErr := scanner.ExportPages(pages, fmt_ == ExportPDF, dir, base, func(done, total int) {
			Win.Update(func() {
				a.exportDoneN = done
				a.exportTotalN = total
			})
		})
		Win.Update(func() {
			if expErr != nil {
				a.exportPhase = PhaseIdle
				a.scanStatus = "导出失败：" + expErr.Error()
				return
			}
			a.lastExportDir = dir
			a.exportPhase = PhaseDone
			a.exportDone = ExportDone{
				Display:   out.Display,
				Dir:       out.Dir,
				File:      out.File,
				PageCount: out.PageCount,
			}
		})
	}()
}

// exportTargets 要导出的页（单页或全部）
func (a *app) exportTargets() []Page {
	if a.exportSingle >= 0 && a.exportSingle < len(a.pages) {
		return []Page{a.pages[a.exportSingle]}
	}
	return a.pages
}

// ── 页面操作 ──

func (a *app) rotatePage(id uint64) {
	for i := range a.pages {
		if a.pages[i].ID == id {
			a.pages[i].Rot = (a.pages[i].Rot + 90) % 360
			// 重烘缩略图
			if full, err := scanner.Decode(a.pages[i].FullPath); err == nil {
				rot := scanner.Rotate90(full, a.pages[i].Rot)
				a.pages[i].ThumbRot = thumbnail(rot, 240)
			}
			if a.lightbox != nil && a.lightbox.Index == i {
				a.lightbox.Full = nil
				a.lightbox.Dirty = true
				a.requestLightboxImage()
			}
			return
		}
	}
}

func (a *app) deletePage(id uint64) {
	idx := -1
	for i := range a.pages {
		if a.pages[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	os.Remove(a.pages[idx].FullPath)
	a.pages = append(a.pages[:idx], a.pages[idx+1:]...)
	if a.lightbox != nil {
		if len(a.pages) == 0 {
			a.lightbox = nil
		} else {
			if a.lightbox.Index >= len(a.pages) {
				a.lightbox.Index = len(a.pages) - 1
			} else if idx < a.lightbox.Index {
				a.lightbox.Index--
			}
			a.lightbox.Full = nil
			a.lightbox.Dirty = true
			a.requestLightboxImage()
		}
	}
	// 页没了就是没了，不用文字再确认一遍
	a.scanStatus = ""
}

// ── 灯箱 ──

func (a *app) openLightbox(index int) {
	if index >= len(a.pages) {
		return
	}
	a.lightbox = &Lightbox{Index: index, Zoom: 1.0, Dirty: true}
	a.requestLightboxImage()
}

func (a *app) closeLightbox() { a.lightbox = nil }

// requestLightboxImage 保证灯箱当前页大图已加载（后台解码，26MB 级不能在 UI 线程）
func (a *app) requestLightboxImage() {
	lb := a.lightbox
	if lb == nil || !lb.Dirty || lb.Loading {
		return
	}
	if lb.Index >= len(a.pages) {
		return
	}
	lb.Loading = true
	lb.Dirty = false
	path := a.pages[lb.Index].FullPath
	rot := a.pages[lb.Index].Rot
	index := lb.Index
	go func() {
		full, err := scanner.Decode(path)
		if err == nil && rot%360 != 0 {
			full = scanner.Rotate90(full, rot)
		}
		Win.Update(func() {
			lb := a.lightbox
			if lb == nil || lb.Index != index {
				return // 期间已翻页/关闭，丢弃过期结果
			}
			lb.Loading = false
			if err != nil {
				a.scanStatus = "原图打不开：" + err.Error()
				return
			}
			bnd := full.Bounds()
			lb.ImgW, lb.ImgH = float32(bnd.Dx()), float32(bnd.Dy())
			lb.Full = ui.NewBitmap(full)
		})
	}()
}
