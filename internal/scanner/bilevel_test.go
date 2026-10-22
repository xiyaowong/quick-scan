package scanner

import (
	"bytes"
	"context"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
)

// 黑白不能把 BlackAndWhite1 + JPEG 下发：调试时见过设备直接 Aborted。
func TestScanBWRequestsGrayscale(t *testing.T) {
	fs := newFakeScanner(t, 1)
	d := fs.device()
	if err := Probe(context.Background(), &d); err != nil {
		t.Fatal(err)
	}
	files, err := Scan(context.Background(), &d,
		Options{Color: ColorModeBW, DPI: 300, Size: PaperA4}, ScanSink{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fs.lastCreate, "<scan:ColorMode>Grayscale8</scan:ColorMode>") {
		t.Errorf("黑白应该改要灰度，实际下发：%s", fs.lastCreate)
	}
	if strings.Contains(fs.lastCreate, "BlackAndWhite1") {
		t.Errorf("不该把 BlackAndWhite1 下发出去：%s", fs.lastCreate)
	}
	if len(files) != 1 || !strings.HasSuffix(files[0], ".png") {
		t.Fatalf("黑白页应存成 PNG：%v", files)
	}
	// 落盘的必须已经是纯黑白：只有 0 和 255 两个灰度值。
	img, err := png.Decode(bytes.NewReader(readFile(t, files[0])))
	if err != nil {
		t.Fatalf("落盘的不是 PNG：%v", err)
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += max(1, b.Dy()/64) {
		for x := b.Min.X; x < b.Max.X; x += max(1, b.Dx()/64) {
			if v := color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y; v != 0 && v != 0xff {
				t.Fatalf("二值化后还有中间灰度 %d", v)
			}
		}
	}
}

// 灰度、彩色仍按原样下发，不受黑白这条路影响。
func TestScanColorUnchanged(t *testing.T) {
	for _, key := range []string{ColorModeColor, ColorModeGray} {
		fs := newFakeScanner(t, 1)
		d := fs.device()
		if err := Probe(context.Background(), &d); err != nil {
			t.Fatal(err)
		}
		files, err := Scan(context.Background(), &d,
			Options{Color: key, DPI: 300, Size: PaperA4}, ScanSink{Dir: t.TempDir()})
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if !strings.Contains(fs.lastCreate, pickColorMode(d.caps.Platen, key)) {
			t.Errorf("%s 下发的 ColorMode 不对：%s", key, fs.lastCreate)
		}
		if len(files) != 1 || !strings.HasSuffix(files[0], ".jpg") {
			t.Errorf("%s 不该改存 PNG：%v", key, files)
		}
	}
}

// 设备只报黑白、连灰度都没有：没有可二值化的来源，只能按黑白下发。
func TestScanBWWithoutGrayKeepsDeviceMode(t *testing.T) {
	in := &inputCaps{Colors: []string{ColorModeBW}}
	mode, bilevel := scanColor(in, ColorModeBW)
	if bilevel || mode != "BlackAndWhite1" {
		t.Fatalf("scanColor = %q, %v", mode, bilevel)
	}
}

// 能力没上报：按灰度请求 + 本地二值化，比赌 BlackAndWhite1 稳。
func TestScanBWWithoutCapsRequestsGray(t *testing.T) {
	for _, in := range []*inputCaps{nil, {}} {
		mode, bilevel := scanColor(in, ColorModeBW)
		if !bilevel || mode != "Grayscale8" {
			t.Fatalf("scanColor(%+v) = %q, %v", in, mode, bilevel)
		}
	}
}

func TestOtsuThresholdSplitsDocument(t *testing.T) {
	// 纸白 190–240、字黑 20–70，阈值应落在两者之间的空档里。
	var gray []uint8
	for v := 190; v <= 240; v++ {
		for range 20 {
			gray = append(gray, uint8(v))
		}
	}
	for v := 20; v <= 70; v++ {
		for range 6 {
			gray = append(gray, uint8(v))
		}
	}
	th := otsuThreshold(gray)
	// 判据是分类对不对，而不是阈值具体落在哪个数：Otsu 会在整段空档上取同一个值。
	for _, v := range gray {
		if v > th && v < 180 {
			t.Fatalf("像素 %d 被当成白纸（阈值 %d）", v, th)
		}
		if v <= th && v > 100 {
			t.Fatalf("像素 %d 被当成字黑（阈值 %d）", v, th)
		}
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
