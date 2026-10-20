// 临时目录：内嵌资产与扫描原图都放 %TEMP%，退出即清。
package platform

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const tempPrefix = "sscan_"

var tempRoot = filepath.Join(os.TempDir(), tempPrefix+strconv.Itoa(os.Getpid()))

// PagesDir 扫描原图目录。
func PagesDir() string { return filepath.Join(tempRoot, "pages") }

// CleanupStale 删除上次异常退出残留的临时目录。
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

// Cleanup 删除本进程的临时目录。
func Cleanup() {
	_ = os.RemoveAll(tempRoot)
}

// EnsurePagesDir 确保扫描原图目录存在。
func EnsurePagesDir() error {
	return os.MkdirAll(PagesDir(), 0o755)
}
