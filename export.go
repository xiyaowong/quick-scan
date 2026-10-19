// 导出：PDF（合并成一个文件）或每页一张 JPG。
// 原图是 eSCL 返回的 JPEG，导出时按旋转烘焙后重新编码。PDF 用 gopdf（成熟库）。
// 进度：页粒度回调（done/total），导出对话框用它画进度条。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/signintech/gopdf"
)

// exportOutcome 导出结果
type exportOutcome struct {
	Display   string
	Dir       string
	File      string
	PageCount int
}

// pageWork 一页的导出输入（路径 + 旋转角）
type pageWork struct {
	Path string
	Rot  int
}

// exportProgress 页粒度进度回调：done 从 1 数到 total
type exportProgress func(done, total int)

// exportPages 导出总入口
func exportPages(pages []Page, fmtOpt ExportFormat, dir, name string, prog exportProgress) (*exportOutcome, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("建目录失败: %w", err)
	}
	works := make([]pageWork, len(pages))
	for i, p := range pages {
		works[i] = pageWork{Path: p.FullPath, Rot: p.Rot}
	}
	if prog == nil {
		prog = func(int, int) {}
	}
	switch fmtOpt {
	case ExportPDF:
		path := uniquePath(dir, name, "pdf")
		if err := exportPDF(works, path, prog); err != nil {
			return nil, err
		}
		return &exportOutcome{Display: path, Dir: filepath.Dir(path), File: path, PageCount: len(pages)}, nil
	default: // ExportJPG
		written, err := exportJPGs(works, dir, name, prog)
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
		return &exportOutcome{Display: display, Dir: dir, File: file, PageCount: n}, nil
	}
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
func exportPDF(pages []pageWork, outPath string, prog exportProgress) error {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	for i, pw := range pages {
		img, err := decodeImage(pw.Path)
		if err != nil {
			return fmt.Errorf("读原图失败: %w", err)
		}
		img = rotate90(img, pw.Rot)
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
func exportJPGs(pages []pageWork, dir, name string, prog exportProgress) ([]string, error) {
	var written []string
	for i, pw := range pages {
		img, err := decodeImage(pw.Path)
		if err != nil {
			return nil, fmt.Errorf("读原图失败: %w", err)
		}
		img = rotate90(img, pw.Rot)
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
