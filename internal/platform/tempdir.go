package platform

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// 前缀跟程序名走：改名后旧版本留下的目录不会被新版本误当成自己的。
const tempPrefix = "quick-scan_"

var tempRoot = filepath.Join(os.TempDir(), tempPrefix+strconv.Itoa(os.Getpid()))

func PagesDir() string { return filepath.Join(tempRoot, "pages") }

func CleanupStale() {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}
	self := tempPrefix + strconv.Itoa(os.Getpid())
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, tempPrefix) && name != self {
			_ = os.RemoveAll(filepath.Join(os.TempDir(), name))
		}
	}
}

func Cleanup() {
	_ = os.RemoveAll(tempRoot)
}

func EnsurePagesDir() error {
	return os.MkdirAll(PagesDir(), 0o755)
}
