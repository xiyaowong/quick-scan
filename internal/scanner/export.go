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

func ExportPages(pages []ExportPage, asPDF bool, dir, name string, prog ProgressFunc) (*ExportOutcome, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("建目录失败: %w", err)
	}
	if prog == nil {
		prog = func(int, int) {}
	}
	if asPDF {
		path := uniquePath(dir, name, "pdf")
		if err := exportPDF(pages, path, prog); err != nil {
			return nil, err
		}
		return &ExportOutcome{Display: path, Dir: filepath.Dir(path), File: path, PageCount: len(pages)}, nil
	}
	written, err := exportJPGs(pages, dir, name, prog)
	if err != nil {
		return nil, err
	}
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
	return &ExportOutcome{Display: display, Dir: dir, File: file, PageCount: n}, nil
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
