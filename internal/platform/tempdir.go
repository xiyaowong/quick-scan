package platform

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// 前缀跟程序名走：改名后旧版本留下的目录不会被新版本误当成自己的。
	tempPrefix = "quick-scan_"
	// 会话目录名不能匹配 tempPrefix，否则会被 CleanupStale 当成旧进程目录删掉。
	sessionDirName = "quick-scan-session"
)

// 进程私有的临时目录：目前只放解包出来的 OCR 模型。
var tempRoot = filepath.Join(os.TempDir(), tempPrefix+strconv.Itoa(os.Getpid()))

// 不跟进程号走：正常退出由 Cleanup 删除，崩溃时目录留着，下次启动据此恢复。
func SessionDir() string { return sessionDir() }

func CacheDir() string {
	return filepath.Join(os.TempDir(), "quick-scan")
}

func CleanupStale() {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}
	self := tempPrefix + strconv.Itoa(os.Getpid())
	for _, e := range entries {
		name := e.Name()
		if name == sessionDirName {
			continue // 可能是崩溃留下的扫描会话，不能清
		}
		if strings.HasPrefix(name, tempPrefix) && name != self {
			_ = os.RemoveAll(filepath.Join(os.TempDir(), name))
		}
	}
}

func Cleanup() {
	_ = os.RemoveAll(tempRoot)
	_ = os.RemoveAll(SessionDir())
}

func EnsurePagesDir() error {
	return os.MkdirAll(SessionDir(), 0o755)
}

// DragDir 放拖给别的程序的图片副本：对方在放下那一刻读的仍是磁盘上的文件，
// 所以不能拿内存里的图冒充。放在会话目录下，正常退出跟页面一起清掉。
func DragDir() string { return filepath.Join(SessionDir(), "drag") }
