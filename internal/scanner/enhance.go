package scanner

import (
	"bytes"
	"image"
	"math"
	"slices"
)

const (
	// 优化要重编码一次，质量比导出高一档，抵掉代际损失。
	enhanceJPEGQuality = 92
	// 纸底增益上限：再暗的纸也留一点余地，否则阴影里的噪点会变成色块。
	maxPaperGain = 3.0
	// 分块尺寸：300dpi 下 64px 约 5mm，能跨过一行字，又跟得上光照渐变。
	paperTileSize = 64
	// 每块取 85% 分位当纸底。纸面占多数，分位数不会被少量噪点或浅色内容拉高。
	paperQuantile = 85
)

// EnhanceLevel 是文档增强的力度。零值 = 标准，旧记录不需要迁移。
type EnhanceLevel int

const (
	EnhanceStandard EnhanceLevel = iota // 大部分文档
	EnhanceLight                        // 保留纸纹和浅灰
	EnhanceStrong                       // 纸底更白、文字更黑
)

// 力度只动黑白点：纸底归一化本身是还原扫描时的光照，不该随力度变。
// 白点越低，越多接近纸白的像素被压成纯白；黑点越高，文字压得越黑。
func enhanceParams(level EnhanceLevel) (white, blackCap int) {
	switch level {
	case EnhanceLight:
		return 248, 32
	case EnhanceStrong:
		return 216, 72
	default:
		return 236, 48
	}
}

// paperMap 是纸底光照图：每格每通道一个纸白值，像素按双线性插值取。
type paperMap struct {
	gw, gh int
	tile   int
	ch     [3][]uint8
	white  int
	black  uint8
}

// enhancePage 做文档增强：铺平局部光照 → 逐通道把纸底拉到白 → 拉开黑白对比。
//
// 扫描仪/官方驱动通常把这一步叫「背景去除」或「文档增强」。和只按全页直方图
// 拉白点相比，按块估计纸底能把书脊阴影、边缘变暗这类不均匀光照一起铺平，
// 也不会因为纸底只亮了一点就整页重编码。
//
// 增强只是观感修正，失败时保留原图比让整页扫不出来更重要，所以这里不返回错误。
func enhancePage(data []byte, level EnhanceLevel) ([]byte, bool) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, false
	}
	src := toRGBA(img)
	if !enhanceImage(src, level) {
		return data, false
	}
	var out []byte
	if format == "png" {
		out, err = encodePNG(src)
	} else {
		out, err = encodeJPEG(src, enhanceJPEGQuality)
	}
	if err != nil {
		return data, false
	}
	return out, true
}

// enhanceImage 直接改像素：导出时从原图重做增强，省掉一次 JPEG 往返。
func enhanceImage(src *image.RGBA, level EnhanceLevel) bool {
	pm, ok := estimatePaper(src, level)
	if !ok {
		return false
	}
	applyPaperNormalization(src, pm)
	return true
}

// estimatePaper 判断这是不是一页纸，并估出纸底光照图。第二项为 false 表示不该增强。
func estimatePaper(src *image.RGBA, level EnhanceLevel) (*paperMap, bool) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 16 || h < 16 {
		return nil, false
	}

	// 全局亮度直方图：判断有没有成片纸底，并估计最暗的墨色。
	// 最多约 20 万个样本：分布够用，不必全图。
	var lum [256]uint32
	step := max(1, int(math.Sqrt(float64(w)*float64(h)/200000)))
	var total uint32
	for y := 0; y < h; y += step {
		i := src.PixOffset(b.Min.X, b.Min.Y+y)
		for x := 0; x < w; x += step {
			lum[lumaAt(src.Pix[i:])]++
			total++
			i += step * 4
		}
	}
	if total == 0 {
		return nil, false
	}
	var bright uint32
	for v := 160; v <= 255; v++ {
		bright += lum[v]
	}
	// 没有成片的亮部就当照片放过：天空、白墙不该被当成纸底。
	// 门槛放得比「纸白」低：阴影里的纸张也可能只到 160 上下。
	if float64(bright)/float64(total) < 0.4 {
		return nil, false
	}
	ink := histPercentile(&lum, total, 2)

	pm := newPaperMap(w, h)
	measureTiles(src, pm)
	for c := range 3 {
		close3(pm.ch[c], pm.gw, pm.gh)
	}

	// 纸底已经是白的（各通道亮度接近 255）就不再重编码。
	paperTiles, maxGain := 0, 1.0
	minBg := int(math.Ceil(255 / maxPaperGain))
	for i := range pm.ch[0] {
		l := (299*int(pm.ch[0][i]) + 587*int(pm.ch[1][i]) + 114*int(pm.ch[2][i])) / 1000
		if l < 120 {
			continue
		}
		paperTiles++
		for c := range 3 {
			g := 255 / float64(max(int(pm.ch[c][i]), minBg))
			if g > maxGain {
				maxGain = g
			}
		}
	}
	if paperTiles == 0 || maxGain < 1.03 {
		return nil, false
	}

	// 墨色换算到归一化后的亮度，再按力度封顶：纸面越亮，墨越黑。
	white, blackCap := enhanceParams(level)
	paperLum := medianTileLuma(pm)
	black := int(float64(ink)*255/float64(max(paperLum, 1)) + 0.5)
	black = min(black, blackCap, white-16)
	pm.white = white
	pm.black = uint8(max(black, 0))
	return pm, true
}

func newPaperMap(w, h int) *paperMap {
	gw := (w + paperTileSize - 1) / paperTileSize
	gh := (h + paperTileSize - 1) / paperTileSize
	pm := &paperMap{gw: gw, gh: gh, tile: paperTileSize}
	for c := range 3 {
		pm.ch[c] = make([]uint8, gw*gh)
	}
	return pm
}

// measureTiles 每块取逐通道 85% 分位当纸底。分位数而不是均值：
// 字块会把均值拉黑，分位数仍落在纸面上。
func measureTiles(src *image.RGBA, pm *paperMap) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	var hist [3][256]uint32
	sample := max(1, pm.tile/32)
	for ty := 0; ty < pm.gh; ty++ {
		y0, y1 := ty*pm.tile, min((ty+1)*pm.tile, h)
		for tx := 0; tx < pm.gw; tx++ {
			x0, x1 := tx*pm.tile, min((tx+1)*pm.tile, w)
			for c := range 3 {
				clear(hist[c][:])
			}
			n := 0
			for y := y0; y < y1; y += sample {
				i := src.PixOffset(b.Min.X+x0, b.Min.Y+y)
				for x := x0; x < x1; x += sample {
					hist[0][src.Pix[i]]++
					hist[1][src.Pix[i+1]]++
					hist[2][src.Pix[i+2]]++
					n++
					i += sample * 4
				}
			}
			at := ty*pm.gw + tx
			for c := range 3 {
				pm.ch[c][at] = uint8(histPercentile(&hist[c], uint32(n), paperQuantile))
			}
		}
	}
}

// close3 做 3×3 形态学闭运算：先膨胀再腐蚀。
// 膨胀把字块里的纸底从邻块补进来，腐蚀再把阴影边缘拉回原高度，
// 这样密集文字不会把纸底估暗，光照渐变也不会被抹平。
func close3(g []uint8, gw, gh int) {
	tmp := make([]uint8, len(g))
	max3(g, tmp, gw, gh)
	min3(tmp, g, gw, gh)
}

func max3(src, dst []uint8, gw, gh int) {
	for y := 0; y < gh; y++ {
		ym, yp := max(0, y-1), min(gh-1, y+1)
		for x := 0; x < gw; x++ {
			xm, xp := max(0, x-1), min(gw-1, x+1)
			v := src[y*gw+x]
			for yy := ym; yy <= yp; yy++ {
				for xx := xm; xx <= xp; xx++ {
					v = max(v, src[yy*gw+xx])
				}
			}
			dst[y*gw+x] = v
		}
	}
}

func min3(src, dst []uint8, gw, gh int) {
	for y := 0; y < gh; y++ {
		ym, yp := max(0, y-1), min(gh-1, y+1)
		for x := 0; x < gw; x++ {
			xm, xp := max(0, x-1), min(gw-1, x+1)
			v := src[y*gw+x]
			for yy := ym; yy <= yp; yy++ {
				for xx := xm; xx <= xp; xx++ {
					v = min(v, src[yy*gw+xx])
				}
			}
			dst[y*gw+x] = v
		}
	}
}

func medianTileLuma(pm *paperMap) int {
	vals := make([]int, 0, len(pm.ch[0]))
	for i := range pm.ch[0] {
		l := (299*int(pm.ch[0][i]) + 587*int(pm.ch[1][i]) + 114*int(pm.ch[2][i])) / 1000
		if l >= 120 {
			vals = append(vals, l)
		}
	}
	if len(vals) == 0 {
		return 255
	}
	slices.Sort(vals)
	return vals[len(vals)/2]
}

// applyPaperNormalization 把光照图变成逐像素增益，再过一条黑白点曲线。
// 增益查表：背景值和像素值的组合只有 256×256 种，每像素只剩查表。
func applyPaperNormalization(src *image.RGBA, pm *paperMap) {
	var level [256]uint8
	black, white := int(pm.black), pm.white
	span := white - black
	for v := range 256 {
		level[v] = uint8(clampInt((v-black)*255/span, 0, 255))
	}

	minBg := int(math.Ceil(255 / maxPaperGain))
	var lut [3][256 * 256]uint8
	for c := range 3 {
		for bg := range 256 {
			gain := 255 / float64(max(bg, minBg))
			base := bg << 8
			for v := range 256 {
				n := int(float64(v)*gain + 0.5)
				lut[c][base+v] = level[min(n, 255)]
			}
		}
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	gw, gh := pm.gw, pm.gh
	x0s := make([]int, w)
	x1s := make([]int, w)
	wxs := make([]float64, w)
	for x := range w {
		fx := (float64(x)+0.5)/float64(pm.tile) - 0.5
		x0 := int(math.Floor(fx))
		wx := fx - float64(x0)
		if x0 < 0 {
			x0, wx = 0, 0
		} else if x0 >= gw-1 {
			x0, wx = gw-1, 0
		}
		x0s[x], wxs[x] = x0, wx
		x1s[x] = min(x0+1, gw-1)
	}

	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)/float64(pm.tile) - 0.5
		y0 := int(math.Floor(fy))
		wy := fy - float64(y0)
		if y0 < 0 {
			y0, wy = 0, 0
		} else if y0 >= gh-1 {
			y0, wy = gh-1, 0
		}
		y1 := min(y0+1, gh-1)
		r0, r1 := y0*gw, y1*gw
		i := src.PixOffset(b.Min.X, b.Min.Y+y)
		for x := 0; x < w; x++ {
			x0, x1, wx := x0s[x], x1s[x], wxs[x]
			for c := range 3 {
				g00 := float64(pm.ch[c][r0+x0])
				g10 := float64(pm.ch[c][r0+x1])
				g01 := float64(pm.ch[c][r1+x0])
				g11 := float64(pm.ch[c][r1+x1])
				g := (g00*(1-wx)+g10*wx)*(1-wy) + (g01*(1-wx)+g11*wx)*wy
				bg := int(g + 0.5)
				src.Pix[i+c] = lut[c][(bg<<8)|int(src.Pix[i+c])]
			}
			i += 4
		}
	}
}

func lumaAt(p []byte) uint8 {
	return uint8((299*uint32(p[0]) + 587*uint32(p[1]) + 114*uint32(p[2])) / 1000)
}

// histPercentile 返回直方图里累计占比达到 pct（0..100）的灰度级。
func histPercentile(hist *[256]uint32, n uint32, pct int) int {
	if n == 0 {
		return 0
	}
	target := (n*uint32(pct) + 99) / 100
	if target == 0 {
		target = 1
	}
	var sum uint32
	for v := 0; v < 256; v++ {
		sum += hist[v]
		if sum >= target {
			return v
		}
	}
	return 255
}

func clampInt(v, lo, hi int) int {
	return max(lo, min(v, hi))
}
