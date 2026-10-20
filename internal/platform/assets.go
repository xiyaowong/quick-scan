// 内嵌资产：OCR 模型三件套（det/rec/dict）。
// 单文件分发：模型编进 exe，运行时释放到本进程临时目录，退出即清。
package platform

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

// EnsureAssets 把内嵌模型释放到临时目录。懒执行：第一次加载模型时触发。
// tempRoot 每次启动都是新目录，无需比对直接覆写。
func EnsureAssets() error {
	extractOnce.Do(func() {
		extractErr = writeAssets()
	})
	return extractErr
}

func writeAssets() error {
	// 不能调 ModelDir()：它内部走 EnsureAssets，sync.Once 重入会死锁。
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

// ModelDir OCR 模型三件套所在目录：从内嵌资产释放到临时目录（单文件分发）。
// 释放失败时退回 exe 同目录的 models/（兼容老式并排摆放）。
func ModelDir() string {
	if err := EnsureAssets(); err == nil {
		return filepath.Join(tempRoot, "models")
	}
	exe, err := os.Executable()
	if err != nil {
		return "models"
	}
	return filepath.Join(filepath.Dir(exe), "models")
}
