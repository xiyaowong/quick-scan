package scanner

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"
)

// 灰底黑字的样张：纸底 200，字 40。
func grayPaperJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 240, 320))
	paper := color.RGBA{R: 200, G: 200, B: 200, A: 255}
	for y := range 320 {
		for x := range 240 {
			img.Set(x, y, paper)
		}
	}
	for y := 100; y < 140; y++ {
		for x := 60; x < 180; x++ {
			img.Set(x, y, color.RGBA{R: 40, G: 40, B: 40, A: 255})
		}
	}
	return encodeTestJPEG(t, img)
}

func encodeTestJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodeRGBA(t *testing.T, data []byte) *image.RGBA {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return toRGBA(img)
}

// 区域平均灰度：避开 JPEG 边缘的振铃。
func meanAt(src *image.RGBA, x0, y0, size int) float64 {
	var sum uint32
	for y := y0; y < y0+size; y++ {
		for x := x0; x < x0+size; x++ {
			i := src.PixOffset(x, y)
			sum += uint32(src.Pix[i]) + uint32(src.Pix[i+1]) + uint32(src.Pix[i+2])
		}
	}
	return float64(sum) / float64(size*size*3)
}

func TestEnhancePageTurnsGrayPaperWhite(t *testing.T) {
	out, ok := enhancePage(grayPaperJPEG(t))
	if !ok {
		t.Fatal("灰底文档没有做增强")
	}
	img := decodeRGBA(t, out)

	if got := meanAt(img, 10, 10, 20); got < 248 {
		t.Errorf("纸底还是灰的：%.1f", got)
	}
	// 字从 40 拉到接近黑，不能被一起提亮成灰。
	if got := meanAt(img, 110, 110, 20); got > 60 {
		t.Errorf("字没压黑：%.1f", got)
	}
}

// 边缘阴影、纸色偏黄：只拉全页白点会把暗的那一边留着，按块估计才能一起铺平。
func TestEnhancePageFlattensUnevenPaper(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for y := range 240 {
		for x := range 320 {
			// 从左到右从 210 暗到 165，左侧还偏黄。
			v := uint8(210 - x*45/320)
			img.Set(x, y, color.RGBA{R: v, G: v - 5, B: v - 20, A: 255})
		}
	}
	out, ok := enhancePage(encodeTestJPEG(t, img))
	if !ok {
		t.Fatal("有纸底的页面没有做增强")
	}
	got := decodeRGBA(t, out)
	for _, x := range []int{10, 280} {
		if v := meanAt(got, x, 10, 20); v < 245 {
			t.Errorf("x=%d 的纸底没铺白：%.1f", x, v)
		}
	}
}

// 照片没有成片的纸底，不该重编码。
func TestEnhancePageKeepsPhoto(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 240, 320))
	for y := range 320 {
		for x := range 240 {
			v := uint8(20 + x*140/240)
			src.Set(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	in := encodeTestJPEG(t, src)
	if out, ok := enhancePage(in); ok || !bytes.Equal(out, in) {
		t.Fatal("照片被当成文档处理了")
	}
}

// 已经是白底的设备（自己做过背景去除）不要再重编码一次。
func TestEnhancePageKeepsWhitePaper(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 240, 320))
	for y := range 320 {
		for x := range 240 {
			src.Set(x, y, color.RGBA{R: 250, G: 250, B: 250, A: 255})
		}
	}
	in := encodeTestJPEG(t, src)
	if out, ok := enhancePage(in); ok || !bytes.Equal(out, in) {
		t.Fatal("白底页被重编码了")
	}
}

// 走完整扫描流程：文档增强并留下原图，照片原样，关掉开关也原样。
func TestScanEnhancesOnlyDocuments(t *testing.T) {
	page := grayPaperJPEG(t)
	cases := []struct {
		name    string
		intent  string
		enhance bool
		white   bool
	}{
		{"文档", "Document", true, true},
		{"照片", "Photo", true, false},
		{"关闭开关", "Document", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newFakeScanner(t, 1)
			fs.page = page
			d := fs.device()
			if err := Probe(context.Background(), &d); err != nil {
				t.Fatal(err)
			}
			files, err := Scan(context.Background(), &d, Options{
				Color: ColorModeColor, DPI: 300, Size: PaperA4, Intent: tc.intent, Enhance: tc.enhance,
			}, ScanSink{Dir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			out := readFile(t, files[0].Path)
			if !tc.white {
				if files[0].RawPath != "" {
					t.Fatal("这一档不该另存原图")
				}
				if !bytes.Equal(out, page) {
					t.Fatal("这一档不该重编码")
				}
				return
			}
			if got := meanAt(decodeRGBA(t, out), 10, 10, 20); got < 248 {
				t.Errorf("纸底还是灰的：%.1f", got)
			}
			if files[0].RawPath == "" {
				t.Fatal("增强后没有留下原图")
			}
			if _, err := os.Stat(files[0].RawPath); err != nil {
				t.Fatalf("原图没落盘：%v", err)
			}
			if raw := readFile(t, files[0].RawPath); !bytes.Equal(raw, page) {
				t.Fatal("原图被改写了")
			}
		})
	}
}
