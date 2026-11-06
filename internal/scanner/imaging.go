package scanner

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"

	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
)

const (
	// 增白要重编码一次，质量比导出高一档，抵掉代际损失。
	whitenJPEGQuality = 92
	// 增益上限：纸底再暗也不硬拉到白，否则噪点会变成色块。
	maxPaperGain = 2.0
)

// BMP 分支留给 OCR 测试和旧样张。
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

func Resize(src *image.RGBA, nw, nh int) *image.RGBA {
	if src.Bounds().Dx() == nw && src.Bounds().Dy() == nh {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

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

func encodeJPEG(src *image.RGBA, quality int) ([]byte, error) {
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: quality})
	return buf.Bytes(), err
}

func encodePNG(src *image.RGBA) ([]byte, error) {
	var buf bytes.Buffer
	err := png.Encode(&buf, src)
	return buf.Bytes(), err
}

// 解码 → Otsu 全局阈值 → 纯黑白 PNG。
// 灰度扫描件是双峰的（纸白、字黑），全局阈值就够了；局部自适应会在纸面噪点上
// 长出黑斑，反而比设备自己的黑白化更脏。
func bilevelPNG(jpg []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(jpg))
	if err != nil {
		return nil, err
	}
	src := toRGBA(img)
	gray := toGray(src)
	threshold := otsuThreshold(gray)
	b := src.Bounds()
	// 2 色调色板而不是 8 位灰度：同样的像素，PNG 小 ~40%，编码也快 ~40%。
	out := image.NewPaletted(image.Rect(0, 0, b.Dx(), b.Dy()), color.Palette{color.Black, color.White})
	for i, v := range gray {
		if v > threshold {
			out.Pix[i] = 1
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// whitenPage 把灰底纸张归一化成白底：按亮度直方图找纸张主峰，
// 逐通道把纸白拉到 255。已经是白底、或看不出成片纸底（多半是照片）时原样返回。
//
// 增白只是观感修正，失败时保留原图比让整页扫不出来更重要，所以这里不返回错误。
func whitenPage(data []byte) []byte {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}
	src := toRGBA(img)
	gains, ok := paperGains(src)
	if !ok {
		return data
	}
	applyGains(src, gains)
	var out []byte
	switch format {
	case "png":
		out, err = encodePNG(src)
	default:
		out, err = encodeJPEG(src, whitenJPEGQuality)
	}
	if err != nil {
		return data
	}
	return out
}

// paperGains 估计纸底每通道的白点增益。第二项为 false 表示不该增白。
func paperGains(src *image.RGBA) ([3]float32, bool) {
	var none [3]float32
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return none, false
	}
	// 最多约 20 万个样本：分布够用，不必全图。
	step := max(1, int(math.Sqrt(float64(w)*float64(h)/200000)))

	var lum [256]uint32
	total := 0
	lumAt := func(i int) uint32 {
		return (299*uint32(src.Pix[i]) + 587*uint32(src.Pix[i+1]) + 114*uint32(src.Pix[i+2])) / 1000
	}
	for y := b.Min.Y; y < b.Max.Y; y += step {
		i := src.PixOffset(b.Min.X, y)
		for x := b.Min.X; x < b.Max.X; x += step {
			lum[lumAt(i)]++
			total++
			i += step * 4
		}
	}
	if total == 0 {
		return none, false
	}
	share := func(n uint32) float64 { return float64(n) / float64(total) }

	var whiteMass uint32
	for v := 251; v <= 255; v++ {
		whiteMass += lum[v]
	}
	if share(whiteMass) > 0.25 {
		return none, false // 纸底已经白了
	}

	// 纸底是亮部里最高的一峰；没有成片的亮部就当照片放过。
	peak := 120
	for v := 121; v <= 250; v++ {
		if lum[v] > lum[peak] {
			peak = v
		}
	}
	var peakMass uint32
	for v := max(0, peak-12); v <= min(255, peak+12); v++ {
		peakMass += lum[v]
	}
	if peak < 150 || share(peakMass) < 0.25 {
		return none, false
	}

	// 主峰附近就是纸底，取通道均值当白点。
	var sum [3]uint64
	var n uint64
	for y := b.Min.Y; y < b.Max.Y; y += step {
		i := src.PixOffset(b.Min.X, y)
		for x := b.Min.X; x < b.Max.X; x += step {
			if l := int(lumAt(i)); l >= peak-8 && l <= peak+8 {
				sum[0] += uint64(src.Pix[i])
				sum[1] += uint64(src.Pix[i+1])
				sum[2] += uint64(src.Pix[i+2])
				n++
			}
			i += step * 4
		}
	}
	if n == 0 {
		return none, false
	}

	var gain [3]float32
	changed := false
	for c := range 3 {
		white := float64(sum[c]) / float64(n)
		if white <= 0 {
			return none, false
		}
		g := float32(min(255/white, maxPaperGain))
		gain[c] = max(g, 1)
		if gain[c] > 1.05 {
			changed = true
		}
	}
	if !changed {
		return none, false
	}
	return gain, true
}

func applyGains(src *image.RGBA, gain [3]float32) {
	var lut [3][256]uint8
	for c := range 3 {
		for v := range 256 {
			n := int(float32(v)*gain[c] + 0.5)
			lut[c][v] = uint8(min(n, 255))
		}
	}
	b := src.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := src.PixOffset(b.Min.X, y)
		for x := b.Min.X; x < b.Max.X; x++ {
			src.Pix[i] = lut[0][src.Pix[i]]
			src.Pix[i+1] = lut[1][src.Pix[i+1]]
			src.Pix[i+2] = lut[2][src.Pix[i+2]]
			i += 4
		}
	}
}

// Rec.601 亮度：扫描件多是灰底黑字，人眼权重最接近「看起来黑不黑」。
func toGray(src *image.RGBA) []uint8 {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	out := make([]uint8, w*h)
	for y := range h {
		for x := range w {
			i := y*src.Stride + x*4
			out[y*w+x] = uint8((299*uint32(src.Pix[i]) + 587*uint32(src.Pix[i+1]) + 114*uint32(src.Pix[i+2])) / 1000)
		}
	}
	return out
}

// Otsu：选让类间方差最大的灰度级。
func otsuThreshold(gray []uint8) uint8 {
	var hist [256]uint32
	for _, v := range gray {
		hist[v]++
	}
	total := uint64(len(gray))
	var sumAll uint64
	for v, n := range hist {
		sumAll += uint64(v) * uint64(n)
	}
	var sumB, weightB uint64
	best, bestVar := 0, -1.0
	for t := 0; t < 256; t++ {
		weightB += uint64(hist[t])
		if weightB == 0 {
			continue
		}
		weightF := total - weightB
		if weightF == 0 {
			break
		}
		sumB += uint64(t) * uint64(hist[t])
		meanB := float64(sumB) / float64(weightB)
		meanF := float64(sumAll-sumB) / float64(weightF)
		v := float64(weightB) * float64(weightF) * (meanB - meanF) * (meanB - meanF)
		if v > bestVar {
			bestVar, best = v, t
		}
	}
	return uint8(best)
}
