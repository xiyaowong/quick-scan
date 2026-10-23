package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"quick-scan/internal/platform"
	"quick-scan/internal/scanner"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) openExport(single int) {
	// 灯箱的遮罩会盖住导出对话框（overlays 里有灯箱就只画灯箱），先收起来。
	a.closeLightbox()
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

func (a *app) exportDialog(c *ui.Context) {
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
			} else if n := len(a.exportTargets()); n < len(a.pages) && n > 0 {
				subtitle = fmt.Sprintf("导出选中的 %d 页（共 %d 页）", n, len(a.pages))
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
					a.exportProgressView(c)
				case PhaseDone:
					a.exportDoneView(c)
				default:
					a.exportFormView(c)
				}
			})
		})
	})
	if !open {
		a.dlg = DlgNone
	}
}

func (a *app) exportProgressView(c *ui.Context) {
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

func (a *app) exportFormView(c *ui.Context) {
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

func (a *app) exportDoneView(c *ui.Context) {
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

// 必须让用户明确选恢复或丢弃，点窗外/Esc 都不关：误关一下数据就没了。
func (a *app) recoverDialog(c *ui.Context) {
	n := len(a.recoverPages)
	open := true
	ui.Modal(c, &open, func() {
		card := ui.Column(c).Width(432).Background(colPanel).Border(1, colLine).
			Radius(18).Clip().Shadow(0, 28, 70, 0, shadowStrong)
		card.Children(func() {
			ui.Row(c).Padding(22, 24, 0, 24).Gap(13).AlignItems(ui.Start).Children(func() {
				badge := ui.Row(c).Size(40, 40).Radius(12).Background(colAccentSoft).Center().Shrink(0)
				badge.Children(func() {
					ui.Icon(c, icoScan).FontSize(19).TextColor(colAccent)
				})
				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, "发现上次未完成的扫描").FontSize(15.5).FontWeight(600).TextColor(colInk)
					ui.Text(c, fmt.Sprintf("还有 %d 页可以恢复。恢复后可以继续查看、旋转和导出。", n)).
						FontSize(12.5).TextColor(colMuted).LineHeight(1.55)
				})
			})
			ui.Row(c).Padding(20, 24, 20, 24).Justify(ui.End).Gap(9).Children(func() {
				if styledButton(c, "丢弃", nil, toneGhost, false).Height(38).Padding(0, 18, 0, 18).Clicked() {
					platform.ClearSession()
					a.recoverPages = nil
					a.dlg = DlgNone
				}
				if styledButton(c, "恢复 "+strconv.Itoa(n)+" 页", nil, tonePrimary, false).Height(38).Padding(0, 20, 0, 20).Clicked() {
					a.restoreSession()
				}
			})
		})
	})
}

// 三步之间都要 Win.Update 回主线程，所以整段只在这一条协程里跑。
func (a *app) runExport() {
	if a.exportPicking {
		return
	}
	a.exportPicking = true

	name := a.exportFilename
	if name == "" {
		name = a.defaultFilename()
	}
	job := exportJob{
		name:     name,
		format:   a.exportFmt,
		startDir: a.lastExportDir,
		pages:    a.pageExports(),
	}

	go func() {
		path, ok := a.pickExportPath(job)
		if !ok {
			return // 用户取消
		}
		a.writeExport(job, path)
	}()
}

// goroutine 只读它，不再碰 a 的其它字段。
type exportJob struct {
	name     string
	format   ExportFormat
	startDir string
	pages    []scanner.ExportPage
}

func (a *app) pageExports() []scanner.ExportPage {
	targets := a.exportTargets()
	pages := make([]scanner.ExportPage, len(targets))
	for i, p := range targets {
		pages[i] = scanner.ExportPage{Path: p.FullPath, Rot: p.Rot}
	}
	return pages
}

func (a *app) pickExportPath(job exportJob) (string, bool) {
	ext, filter := "pdf", "PDF 文件"
	if job.format == ExportJPG {
		ext, filter = "jpg", "JPG 图片"
	}
	def := job.name + "." + ext
	// 首次导出或记录的目录已消失，都默认到「文档」，
	// 别让系统对话框落到深层路径。
	startDir := job.startDir
	if startDir == "" || !platform.ExportDirUsable(startDir) {
		startDir = platform.DefaultExportDir()
	}
	if startDir != "" {
		def = filepath.Join(startDir, def)
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      Win,
		Title:       "导出",
		DefaultPath: def,
		Filters:     []mygo.FileFilter{{Name: filter, Extensions: []string{ext}}},
	})
	Win.Update(func() { a.exportPicking = false })
	return path, err == nil && path != ""
}

func (a *app) writeExport(job exportJob, path string) {
	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	Win.Update(func() {
		a.exportPhase = PhaseWorking
		a.exportDoneN = 0
		a.exportTotalN = len(job.pages)
	})
	out, err := scanner.ExportPages(scanner.ExportOptions{
		Pages: job.pages,
		Dir:   dir,
		Name:  base,
		AsPDF: job.format == ExportPDF,
		OnProgress: func(done, total int) {
			Win.Update(func() {
				a.exportDoneN = done
				a.exportTotalN = total
			})
		},
	})
	Win.Update(func() {
		if err != nil {
			a.exportPhase = PhaseIdle
			a.scanStatus = "导出失败：" + err.Error()
			return
		}
		// 导出成功才记住：用户取消或失败时选中的目录不算「上次保存的位置」。
		a.lastExportDir = dir
		platform.SaveExportDir(dir)
		a.exportPhase = PhaseDone
		a.exportDone = ExportDone{
			Display:   out.Display,
			Dir:       out.Dir,
			File:      out.File,
			PageCount: out.PageCount,
		}
	})
}

func (a *app) exportTargets() []Page {
	if a.exportSingle >= 0 && a.exportSingle < len(a.pages) {
		return []Page{a.pages[a.exportSingle]}
	}
	// 勾了页就只导出勾选的；一页没勾时仍导出全部，保持原来的用法。
	if sel := a.selectedPages(); len(sel) > 0 {
		return sel
	}
	return a.pages
}

func (a *app) rotatePage(id uint64) {
	for i := range a.pages {
		if a.pages[i].ID != id {
			continue
		}
		p := &a.pages[i]
		p.Rot = (p.Rot + 90) % 360
		// 只转 240px 的缩略图。之前在这里解码整张原图再转 800 万像素，
		// 点击后会卡住一瞬；原图不需要动，导出时按 Rot 现转。
		if p.ThumbSrc != nil {
			p.ThumbRot = thumbnail(scanner.Rotate90(p.ThumbSrc, p.Rot), 240)
		}
		if a.lightbox != nil && a.lightbox.Index == i {
			// 灯箱里的图也不重解码，只换渲染角度。
			a.lightbox.Rot = p.Rot
		}
		a.saveSession()
		a.warmDragCopy(id)
		return
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
	delete(a.selected, id)
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
	a.scanStatus = ""
	a.saveSession()
}

func (a *app) openLightbox(index int) {
	if index >= len(a.pages) {
		return
	}
	a.lightbox = &Lightbox{Index: index, Rot: a.pages[index].Rot, Zoom: 1.0, Dirty: true}
	a.requestLightboxImage()
}

func (a *app) closeLightbox() { a.lightbox = nil }

// 后台解码：26MB 级原图不能在 UI 线程读。
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
	lb.Rot = a.pages[lb.Index].Rot
	path := a.pages[lb.Index].FullPath
	index := lb.Index
	go func() {
		full, err := scanner.Decode(path)
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
			// 位图保持未旋转：灯箱按 Rot 渲染，导出/OCR 按 Rot 现转。
			lb.Full = ui.NewBitmap(full)
		})
	}()
}
