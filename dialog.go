// 导出对话框 + 灯箱图像加载 + 页面操作。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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
		ui.Column(c).Width(440).Padding(20).Gap(14).Background(t.Background).
			Radius(10).Border(1, t.Border).Children(func() {

			title := "导出"
			if a.exportSingle >= 0 {
				title = fmt.Sprintf("导出第 %d 页", a.exportSingle+1)
			}
			ui.Text(c, title).FontSize(16).Bold()
			// 导出范围：单页与整份要一眼区分（旧版语义）
			if a.exportSingle >= 0 {
				ui.Text(c, fmt.Sprintf("仅导出第 %d 页（共 %d 页）", a.exportSingle+1, len(a.pages))).
					FontSize(12).TextColor(t.TextMuted)
			} else {
				ui.Text(c, fmt.Sprintf("导出全部 %d 页", len(a.pages))).
					FontSize(12).TextColor(t.TextMuted)
			}

			switch a.exportPhase {
			case PhaseWorking:
				// 页粒度进度条（mockup: .progress 4px 细条 + 计数文案）
				frac := 0.0
				if a.exportTotalN > 0 {
					frac = float64(a.exportDoneN) / float64(a.exportTotalN)
				}
				ui.Progress(c, frac).FillWidth()
				ui.Text(c, fmt.Sprintf("正在导出 … %d / %d 页",
					a.exportDoneN, a.exportTotalN)).TextColor(t.TextMuted)
			case PhaseDone:
				a.exportDoneView(c, t)
			default:
				a.exportFormView(c, t)
			}
		})
	})
	if !open {
		a.dlg = DlgNone
	}
}

// exportFormView 格式 + 文件名 + 导出按钮
func (a *app) exportFormView(c *ui.Context, t *ui.Theme) {
	ui.Text(c, "格式").FontSize(13).TextColor(t.TextMuted)
	ui.RadioGroup(c, func() {
		ui.Radio(c, &a.exportFmt, ExportPDF, "PDF（合并成一个文件）")
		ui.Radio(c, &a.exportFmt, ExportJPG, "图片（每页一张 JPG）")
	})

	ui.Text(c, "文件名").FontSize(13).TextColor(t.TextMuted)
	ui.TextInput(c, &a.exportFilename).FillWidth()

	// OCR 文件名候选（预留：有候选时显示）。Wrap：候选多时换行，不撑破对话框
	if len(a.ocrNames) > 0 {
		ui.Text(c, "识别到的名称（点击使用）").FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(6).Wrap().Children(func() {
			for _, name := range a.ocrNames {
				n := name
				if ui.Button(c, n).Clicked() {
					a.applyOCRName(n)
				}
			}
		})
	}

	ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
		if ui.Button(c, "取消").Clicked() {
			a.dlg = DlgNone
		}
		if ui.PrimaryButton(c, "导出").Clicked() {
			a.runExport()
		}
	})
}

// exportDoneView 导出完成态
func (a *app) exportDoneView(c *ui.Context, t *ui.Theme) {
	ui.Text(c, "已导出").FontSize(14).TextColor(ui.RGB(34, 160, 90))
	ui.Text(c, a.exportDone.Display).FontSize(13)
	ui.Row(c).Gap(10).Justify(ui.End).Children(func() {
		if a.exportDone.File != "" {
			if ui.Button(c, "打开文件").Clicked() {
				mygo.Shell.OpenPath(a.exportDone.File)
			}
		}
		if ui.Button(c, "打开所在文件夹").Clicked() {
			target := a.exportDone.File
			if target == "" {
				target = a.exportDone.Dir
			}
			mygo.Shell.ShowItemInFolder(target)
		}
		if ui.PrimaryButton(c, "完成").Clicked() {
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
	pages := a.exportTargets()

	go func() {
		ext, filter := "pdf", "PDF 文件"
		if fmt_ == ExportJPG {
			ext, filter = "jpg", "JPG 图片"
		}
		def := filepath.Join(startDir, name+"."+ext)
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      win,
			Title:       "导出",
			DefaultPath: def,
			Filters:     []mygo.FileFilter{{Name: filter, Extensions: []string{ext}}},
		})
		win.Update(func() { a.exportPicking = false })
		if err != nil || path == "" {
			return // 用户取消
		}
		dir := filepath.Dir(path)
		base := filepath.Base(path)
		base = base[:len(base)-len(filepath.Ext(base))] // 去扩展名

		win.Update(func() {
			a.exportPhase = PhaseWorking
			a.exportDoneN = 0
			a.exportTotalN = len(pages)
		})
		out, expErr := exportPages(pages, fmt_, dir, base, func(done, total int) {
			win.Update(func() {
				a.exportDoneN = done
				a.exportTotalN = total
			})
		})
		win.Update(func() {
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
			if full, err := decodeImage(a.pages[i].FullPath); err == nil {
				rot := rotate90(full, a.pages[i].Rot)
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
		full, err := decodeImage(path)
		if err == nil && rot%360 != 0 {
			full = rotate90(full, rot)
		}
		win.Update(func() {
			lb := a.lightbox
			if lb == nil || lb.Index != index {
				return // 期间已翻页/关闭，丢弃过期结果
			}
			lb.Loading = false
			if err != nil {
				a.scanStatus = "原图打不开：" + err.Error()
				return
			}
			lb.Full = ui.NewBitmap(full)
		})
	}()
}
