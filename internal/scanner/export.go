// 导出：PDF（合并成一个文件）或每页一张 JPG。
// 原图是 eSCL 返回的 JPEG，导出时按旋转烘焙后重新编码。PDF 用 gopdf（成熟库）。
// 进度：页粒度回调（done/total），导出对话框用它画进度条。
package scanner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/signintech/gopdf"
)

// ExportOutcome 导出结果
type ExportOutcome struct {
	Display   string
	Dir       string
	File      string
	PageCount int
}

// ExportPage 一页的导出输入（路径 + 旋转角）
type ExportPage struct {
	Path string
	Rot  int
}

// ProgressFunc 页粒度进度回调：done 从 1 数到 total
type ProgressFunc func(done, total int)

// ExportPages 导出总入口。asPDF=false 时每页存一张 JPG。
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

// uniquePath 已存在则加 (2)(3)… 后缀，不覆盖用户文件
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

// exportPDF 导出为单个 PDF（gopdf：每页一个 A4，JPG 直嵌）
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
		// 300dpi 扫描图 → pt 尺寸：px * 72 / dpi
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

// exportJPGs 导出为多张 JPG：name_1.jpg, name_2.jpg …
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
