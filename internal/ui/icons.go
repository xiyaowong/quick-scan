package ui

import "github.com/egoist/mygo/ui"

func strokeIcon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" ` +
			`stroke="#000" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">` +
			body + `</svg>`))
}

// 窗口按钮要细线：系统标题栏的按钮就是细的，粗线和界面其它图标放一起会显得笨。
func thinIcon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" ` +
			`stroke="#000" stroke-width="1.1" stroke-linecap="round" stroke-linejoin="round">` +
			body + `</svg>`))
}

var (
	icoScan = strokeIcon(
		`<path d="M4 8V5.5A1.5 1.5 0 0 1 5.5 4H8"/>` +
			`<path d="M16 4h2.5A1.5 1.5 0 0 1 20 5.5V8"/>` +
			`<path d="M20 16v2.5a1.5 1.5 0 0 1-1.5 1.5H16"/>` +
			`<path d="M8 20H5.5A1.5 1.5 0 0 1 4 18.5V16"/>` +
			`<path d="M7.5 12h9"/>`)
	icoDownload = strokeIcon(
		`<path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M5 21h14"/>`)
	icoRotate = strokeIcon(
		`<path d="M20.5 12a8.5 8.5 0 1 1-2.5-6"/><path d="M20.5 3.5V9H15"/>`)
	icoTrash = strokeIcon(
		`<path d="M4 7h16"/><path d="M10 11v6"/><path d="M14 11v6"/>` +
			`<path d="M6.5 7 7.5 20h9L17.5 7"/><path d="M9 7V4h6v3"/>`)
	icoRefresh = strokeIcon(
		`<path d="M20 11a8 8 0 1 0-.8 4.5"/><path d="M20 3.5V10h-6.5"/>`)
	icoCheckCircle = strokeIcon(
		`<circle cx="12" cy="12" r="9"/><path d="m8 12 3 3 5.5-6"/>`)
	icoAlert = strokeIcon(
		`<path d="M12 3.5 2.5 20h19L12 3.5z"/><path d="M12 9.5v5"/>` +
			`<circle cx="12" cy="17.4" r=".65" fill="#000" stroke="none"/>`)
	icoFolder = strokeIcon(
		`<path d="M3 7.5A2 2 0 0 1 5 5.5h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`)
	icoFile = strokeIcon(
		`<path d="M14 3H6.5A1.5 1.5 0 0 0 5 4.5v15A1.5 1.5 0 0 0 6.5 21h11a1.5 1.5 0 0 0 1.5-1.5V8z"/>` +
			`<path d="M14 3v5h5"/><path d="M8.5 13h7"/><path d="M8.5 17h5"/>`)
	icoGrip = ui.MustParseSVG([]byte(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#000">` +
			`<circle cx="9" cy="6" r="1.15"/><circle cx="15" cy="6" r="1.15"/>` +
			`<circle cx="9" cy="12" r="1.15"/><circle cx="15" cy="12" r="1.15"/>` +
			`<circle cx="9" cy="18" r="1.15"/><circle cx="15" cy="18" r="1.15"/></svg>`))
	icoArrowLeft  = strokeIcon(`<path d="M15 5 8 12l7 7"/>`)
	icoArrowRight = strokeIcon(`<path d="m9 5 7 7-7 7"/>`)
	icoClose      = strokeIcon(`<path d="M6 6l12 12"/><path d="M18 6 6 18"/>`)
	icoEye        = strokeIcon(
		`<path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12z"/>` +
			`<circle cx="12" cy="12" r="3"/>`)
	icoPlus  = strokeIcon(`<path d="M12 5v14"/><path d="M5 12h14"/>`)
	icoMinus = strokeIcon(`<path d="M5 12h14"/>`)
	icoCheck = strokeIcon(`<path d="m5 12.5 4.5 4.5L19 7"/>`)
	icoStop  = strokeIcon(`<rect x="6.5" y="6.5" width="11" height="11" rx="1.5"/>`)

	icoWinMin     = thinIcon(`<path d="M5 12h14"/>`)
	icoWinMax     = thinIcon(`<rect x="6.5" y="6.5" width="11" height="11" rx="1"/>`)
	icoWinRestore = thinIcon(
		`<rect x="5" y="9" width="10" height="10" rx="1"/>` +
			`<path d="M8.5 9V6.5A1.5 1.5 0 0 1 10 5h7.5A1.5 1.5 0 0 1 19 6.5V14a1.5 1.5 0 0 1-1.5 1.5H15"/>`)
	icoWinClose = thinIcon(`<path d="M6.5 6.5l11 11"/><path d="M17.5 6.5l-11 11"/>`)
)
