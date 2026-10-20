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
