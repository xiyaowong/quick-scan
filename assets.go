// 内嵌资产：OCR 模型三件套（det/rec/dict）。
// 单文件分发：模型编进 exe，运行时释放到本进程临时目录（tempRoot，退出即清）。
package main

import (
	_ "embed"
	"os"
	"path/filepath"
	"sync"
)

//go:embed models/det.onnx
var detOnnx []byte

//go:embed models/rec.onnx
var recOnnx []byte

//go:embed models/dict.txt
var dictTxt []byte

var (
	extractOnce sync.Once
	extractErr  error
)

// ensureAssets 把内嵌模型释放到临时目录。懒执行：第一次加载模型时触发。
// tempRoot 每次启动都是新目录，无需比对直接覆写。
func ensureAssets() error {
	extractOnce.Do(func() {
		extractErr = writeAssets()
	})
	return extractErr
}

func writeAssets() error {
	// 不能调 modelDir()：它内部走 ensureAssets，sync.Once 重入会死锁。
	models := filepath.Join(tempRoot, "models")
	if err := os.MkdirAll(models, 0o755); err != nil {
		return err
	}
	for name, data := range map[string][]byte{
		"det.onnx": detOnnx,
		"rec.onnx": recOnnx,
		"dict.txt": dictTxt,
	} {
		if err := os.WriteFile(filepath.Join(models, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
