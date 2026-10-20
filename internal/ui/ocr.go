// OCR 文件名候选的后台编排：识别在 scanner 里，这里只负责跑和落状态。
package ui

import "sscan/internal/scanner"

// startOCRSuggest 首页扫完后自动跑 OCR 文件名候选（后台，失败静默）。
func (a *app) startOCRSuggest() {
	if a.ocrRunning || len(a.pages) == 0 {
		return
	}
	a.ocrRunning = true
	page := a.pages[0]
	go func() {
		names, err := scanner.Suggest(page.FullPath, page.Rot)
		Win.Update(func() {
			a.ocrRunning = false
			if err != nil || len(names) == 0 {
				return // 静默：候选是锦上添花，失败不打扰
			}
			a.ocrNames = names
		})
	}()
}

// applyOCRName 应用 OCR 候选到文件名输入框：第一次替换，后续追加。
func (a *app) applyOCRName(name string) {
	if a.ocrPicked && a.exportFilename != "" {
		a.exportFilename += name
	} else {
		a.exportFilename = name
	}
	a.ocrPicked = true
}
