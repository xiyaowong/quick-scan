package platform

import (
	_ "embed"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed models/det.onnx
var detOnnx []byte

//go:embed models/rec.onnx
var recOnnx []byte

//go:embed models/dict.txt
var dictTxt []byte

var (
	// 由 main 从 update.Version 注入：本地直接 go build 时没有版本号。
	version = "dev"

	extractOnce sync.Once
	extractErr  error
)

var modelFiles = []struct {
	name string
	data []byte
}{
	{"det.onnx", detOnnx},
	{"rec.onnx", recOnnx},
	{"dict.txt", dictTxt},
}

// SetVersion 必须在第一次 OCR 前调用。解包目录按版本分开，
// 升级换模型后不会继续用到上一版留下的文件。
func SetVersion(v string) {
	if tag := versionTag(v); tag != "" {
		version = tag
	}
}

// EnsureAssets 把内嵌模型放进当前版本的目录；已经有一份完整的就直接用。
// 只在第一次 OCR 经 ModelDir 触发，启动时不写盘。
func EnsureAssets() error {
	extractOnce.Do(func() {
		extractErr = writeAssets()
	})
	return extractErr
}

// writeAssets 只能走 modelDir()：ModelDir() 内部会重入 EnsureAssets 的 sync.Once。
func writeAssets() error {
	dir := modelDir()
	if assetsReady(dir) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range modelFiles {
		if err := WriteFileAtomic(filepath.Join(dir, f.name), f.data, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// 大小对不上说明模型随版本换过，或上次写到一半：整份重写。
func assetsReady(dir string) bool {
	for _, f := range modelFiles {
		st, err := os.Stat(filepath.Join(dir, f.name))
		if err != nil || !st.Mode().IsRegular() || st.Size() != int64(len(f.data)) {
			return false
		}
	}
	return true
}

// pruneOldModels 删掉当前版本以外的模型目录：升级后旧的几 MB 不留在机器上。
func pruneOldModels() {
	dir := filepath.Join(tempRoot(), "models")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != version {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}

// 只留路径安全的字符：版本号来自构建参数，别让它变成目录穿越。
func versionTag(v string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(v) {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 || b.Len() > 32 {
		return ""
	}
	return b.String()
}

func modelDir() string { return filepath.Join(tempRoot(), "models", version) }

func ModelDir() string {
	if err := EnsureAssets(); err == nil {
		return modelDir()
	}
	exe, err := os.Executable()
	if err != nil {
		return "models"
	}
	return filepath.Join(filepath.Dir(exe), "models")
}
