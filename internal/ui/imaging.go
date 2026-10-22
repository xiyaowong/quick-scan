// 缩略图：界面只存 240px 级的低清位图，原图留给灯箱与导出。
package ui

import (
	"image"

	"quick-scan/internal/scanner"

	"github.com/egoist/mygo/ui"
)

func thumbnail(src *image.RGBA, maxDim int) *ui.Bitmap {
	return ui.NewBitmap(thumbRGBA(src, maxDim))
}

func thumbRGBA(src *image.RGBA, maxDim int) *image.RGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	scale := float32(maxDim) / float32(max(w, h))
	if scale >= 1 {
		return src
	}
	nw, nh := max(1, int(float32(w)*scale)), max(1, int(float32(h)*scale))
	// 300dpi 原图（约 2500px）直接近邻缩会丢行，统一走 CatmullRom
	return scanner.Resize(src, nw, nh)
}
