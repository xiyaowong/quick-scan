package scanner

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
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

func TestWhitenPageTurnsGrayPaperWhite(t *testing.T) {
	out := whitenPage(grayPaperJPEG(t))
	img := decodeRGBA(t, out)

	if got := meanAt(img, 10, 10, 20); got < 248 {
		t.Errorf("纸底还是灰的：%.1f", got)
	}
	// 字从 40 拉到约 51，不能被一起提亮成灰。
	if got := meanAt(img, 110, 110, 20); got > 80 {
		t.Errorf("字被提亮了：%.1f", got)
	}
}

// 照片没有成片的纸底，不该重编码。
func TestWhitenPageKeepsPhoto(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 240, 320))
	for y := range 320 {
		for x := range 240 {
			v := uint8(20 + x*140/240)
			src.Set(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	in := buf.Bytes()
	if out := whitenPage(in); !bytes.Equal(out, in) {
		t.Fatal("照片被当成文档处理了")
	}
}

// 已经是白底的设备（自己做过背景去除）不要再重编码一次。
func TestWhitenPageKeepsWhitePaper(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 240, 320))
	for y := range 320 {
		for x := range 240 {
			src.Set(x, y, color.RGBA{R: 250, G: 250, B: 250, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	in := buf.Bytes()
	if out := whitenPage(in); !bytes.Equal(out, in) {
		t.Fatal("白底页被重编码了")
	}
}

// 走完整扫描流程：文档增白，照片原样，关掉开关也原样。
func TestScanWhitensOnlyDocuments(t *testing.T) {
	page := grayPaperJPEG(t)
	cases := []struct {
		name   string
		intent string
		whiten bool
		white  bool
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
				Color: ColorModeColor, DPI: 300, Size: PaperA4, Intent: tc.intent, Whiten: tc.whiten,
			}, ScanSink{Dir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			out := readFile(t, files[0])
			if !tc.white {
				if !bytes.Equal(out, page) {
					t.Fatal("这一档不该重编码")
				}
				return
			}
			if got := meanAt(decodeRGBA(t, out), 10, 10, 20); got < 248 {
				t.Errorf("纸底还是灰的：%.1f", got)
			}
		})
	}
}
