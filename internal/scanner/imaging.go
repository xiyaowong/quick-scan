// 图像处理：解码、旋转、缩放。
// 原图是 eSCL 直接返回的 JPEG/PNG；界面只加载低清缩略图，双击才读原图。
package scanner

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"

	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
)

// Decode 按文件头解码 BMP / JPEG / PNG 为 *image.RGBA。
// 扫描结果通常是 JPEG；BMP 分支保留给 OCR 测试与旧测试样张。
func Decode(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var hdr [2]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var img image.Image
	if hdr[0] == 'B' && hdr[1] == 'M' {
		img, err = bmp.Decode(f)
	} else {
		img, _, err = image.Decode(f)
	}
	if err != nil {
		return nil, err
	}
	return toRGBA(img), nil
}

func toRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok {
		return r
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// Resize 高质量缩放（CatmullRom：缩略图/OCR 预处理共用）
func Resize(src *image.RGBA, nw, nh int) *image.RGBA {
	if src.Bounds().Dx() == nw && src.Bounds().Dy() == nh {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

// resizeToLongEdge 长边压到 maxEdge（比例不变）；不放大
func resizeToLongEdge(src *image.RGBA, maxEdge int) *image.RGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	long := max(w, h)
	if long <= maxEdge {
		return src
	}
	nw := max(1, w*maxEdge/long)
	nh := max(1, h*maxEdge/long)
	return Resize(src, nw, nh)
}

// Rotate90 顺时针旋转 90° 的倍数（deg ∈ 0/90/180/270）
func Rotate90(src *image.RGBA, deg int) *image.RGBA {
	switch deg % 360 {
	case 90:
		return rot90cw(src)
	case 180:
		return rot180(src)
	case 270:
		return rot90ccw(src)
	default:
		return src
	}
}

func rot90cw(src *image.RGBA) *image.RGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			soff := y*src.Stride + x*4
			nx, ny := h-1-y, x
			doff := ny*dst.Stride + nx*4
			copy(dst.Pix[doff:doff+4], src.Pix[soff:soff+4])
		}
	}
	return dst
}

func rot90ccw(src *image.RGBA) *image.RGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			soff := y*src.Stride + x*4
			nx, ny := y, w-1-x
			doff := ny*dst.Stride + nx*4
			copy(dst.Pix[doff:doff+4], src.Pix[soff:soff+4])
		}
	}
	return dst
}

func rot180(src *image.RGBA) *image.RGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			soff := y*src.Stride + x*4
			nx, ny := w-1-x, h-1-y
			doff := ny*dst.Stride + nx*4
			copy(dst.Pix[doff:doff+4], src.Pix[soff:soff+4])
		}
	}
	return dst
}

// encodeJPEG 编码 JPG 字节
func encodeJPEG(src *image.RGBA, quality int) ([]byte, error) {
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: quality})
	return buf.Bytes(), err
}

// encodePNG 编码 PNG 字节（OCR 前的中间格式）
func encodePNG(src *image.RGBA) ([]byte, error) {
	var buf bytes.Buffer
	err := png.Encode(&buf, src)
	return buf.Bytes(), err
}
