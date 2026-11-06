package scanner

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"

	"quick-scan/internal/platform"

	"github.com/signintech/gopdf"
)

type ExportOutcome struct {
	Display   string
	Dir       string
	File      string
	PageCount int
}

type ExportPage struct {
	Path string
	Rot  int
}

type ProgressFunc func(done, total int)

// 逐页导出和合成 PDF 只差 AsPDF，其余字段摆在一起才看得出「同一批页面、同一个落点」。
type ExportOptions struct {
	Pages []ExportPage
	Dir   string // 目标目录
	Name  string // 不含扩展名的文件名
	AsPDF bool
	// OnProgress 在导出 goroutine 里逐页回调。
	OnProgress ProgressFunc
}

func ExportPages(o ExportOptions) (*ExportOutcome, error) {
	if err := os.MkdirAll(o.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("建目录失败: %w", err)
	}
	prog := o.OnProgress
	if prog == nil {
		prog = func(int, int) {}
	}
	if o.AsPDF {
		path := uniquePath(o.Dir, o.Name, "pdf")
		if err := exportPDF(o.Pages, path, prog); err != nil {
			return nil, err
		}
		return &ExportOutcome{Display: path, Dir: filepath.Dir(path), File: path, PageCount: len(o.Pages)}, nil
	}
	written, err := exportJPGs(o.Pages, o.Dir, o.Name, prog)
	if err != nil {
		return nil, err
	}
	return jpgOutcome(o.Dir, written), nil
}

// 文件夹里可能只多了一张，界面上要说得清。
func jpgOutcome(dir string, written []string) *ExportOutcome {
	n := len(written)
	var display string
	if n == 1 {
		display = filepath.Base(written[0]) + "（共 1 张）"
	} else {
		display = fmt.Sprintf("%s  ~  %s  （共 %d 张）",
			filepath.Base(written[0]), filepath.Base(written[n-1]), n)
	}
	var file string
	if n == 1 {
		file = written[0]
	}
	return &ExportOutcome{Display: display, Dir: dir, File: file, PageCount: n}
}

func uniquePath(dir, name, ext string) string {
	p := filepath.Join(dir, name+"."+ext)
	for i := 2; ; i++ {
		_, err := os.Stat(p)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return p
		case err != nil:
			// 读不出的原因（权限、父路径不是目录）留给写入时报，别在这里空转。
			return p
		}
		p = filepath.Join(dir, fmt.Sprintf("%s(%d).%s", name, i, ext))
	}
}

// fitPage 按 300dpi 把像素尺寸换成 pt，等比缩进 A4 并居中。
// 旋转成横版时换横向页面；否则横版图会被 A4 页宽截掉一截。
func fitPage(pxW, pxH int) (page *gopdf.Rect, x, y, w, h float64) {
	// px * 72 / dpi
	const dpi = 300.0
	wPt := float64(pxW) * 72.0 / dpi
	hPt := float64(pxH) * 72.0 / dpi
	page = gopdf.PageSizeA4
	if wPt > hPt {
		page = gopdf.PageSizeA4Landscape
	}
	scale := math.Min(1, math.Min(page.W/wPt, page.H/hPt))
	w, h = wPt*scale, hPt*scale
	return page, (page.W - w) / 2, (page.H - h) / 2, w, h
}

func exportPDF(pages []ExportPage, outPath string, prog ProgressFunc) error {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	for i, pw := range pages {
		img, err := Decode(pw.Path)
		if err != nil {
			return fmt.Errorf("读原图失败: %w", err)
		}
		img = Rotate90(img, pw.Rot)
		jpg, err := encodeJPEG(img, 90)
		if err != nil {
			return fmt.Errorf("JPG 编码失败: %w", err)
		}
		holder, err := gopdf.ImageHolderByBytes(jpg)
		if err != nil {
			return fmt.Errorf("PDF 图片装载失败: %w", err)
		}
		page, x, y, w, h := fitPage(img.Bounds().Dx(), img.Bounds().Dy())
		pdf.AddPageWithOption(gopdf.PageOption{PageSize: page})
		if err := pdf.ImageByHolder(holder, x, y, &gopdf.Rect{W: w, H: h}); err != nil {
			return fmt.Errorf("PDF 插图失败: %w", err)
		}
		prog(i+1, len(pages))
	}
	if err := pdf.WritePdf(outPath); err != nil {
		return fmt.Errorf("写 PDF 失败: %w", err)
	}
	return nil
}

// WritePageJPEG 把一页按旋转角度烘成 JPEG 写到 path（覆盖）。
// 会话里的原图不带角度，直接交给别的程序会躺倒；拖拽的副本走这里。
func WritePageJPEG(p ExportPage, path string) error {
	img, err := Decode(p.Path)
	if err != nil {
		return fmt.Errorf("读原图失败: %w", err)
	}
	jpg, err := encodeJPEG(Rotate90(img, p.Rot), 88)
	if err != nil {
		return fmt.Errorf("JPG 编码失败: %w", err)
	}
	return platform.WriteFileAtomic(path, jpg, 0o755)
}

func exportJPGs(pages []ExportPage, dir, name string, prog ProgressFunc) ([]string, error) {
	var written []string
	for i, pw := range pages {
		img, err := Decode(pw.Path)
		if err != nil {
			return nil, fmt.Errorf("读原图失败: %w", err)
		}
		img = Rotate90(img, pw.Rot)
		jpg, err := encodeJPEG(img, 88)
		if err != nil {
			return nil, fmt.Errorf("JPG 编码失败: %w", err)
		}
		p := uniquePath(dir, fmt.Sprintf("%s_%d", name, i+1), "jpg")
		if err := os.WriteFile(p, jpg, 0o644); err != nil {
			return nil, fmt.Errorf("写 %s 失败: %w", p, err)
		}
		written = append(written, p)
		prog(i+1, len(pages))
	}
	return written, nil
}
