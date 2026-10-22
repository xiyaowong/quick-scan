package scanner

import (
	"fmt"
	"os"
	"path/filepath"

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

// ExportOptions 一次导出的全部输入。逐页导出和合成 PDF 只差 AsPDF，
// 其余字段摆在一起才看得出「同一批页面、同一个落点」。
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

// jpgOutcome 描述多文件导出的落点：文件夹里可能只多了一张，界面上要说得清。
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
	i := 2
	for {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
		p = filepath.Join(dir, fmt.Sprintf("%s(%d).%s", name, i, ext))
		i++
	}
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
		pdf.AddPage()
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		// px * 72 / dpi
		const dpi = 300.0
		pwPt := float64(w) * 72.0 / dpi
		phPt := float64(h) * 72.0 / dpi
		holder, err := gopdf.ImageHolderByBytes(jpg)
		if err != nil {
			return fmt.Errorf("PDF 图片装载失败: %w", err)
		}
		if err := pdf.ImageByHolder(holder, 0, 0, &gopdf.Rect{W: pwPt, H: phPt}); err != nil {
			return fmt.Errorf("PDF 插图失败: %w", err)
		}
		prog(i+1, len(pages))
	}
	return pdf.WritePdf(outPath)
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
