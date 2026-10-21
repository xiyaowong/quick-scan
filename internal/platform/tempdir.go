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

// 会话目录固定在 %TEMP% 下，不跟进程号走。
// 正常退出由 Cleanup 删除；崩溃时走不到删除这一步，目录留着，下次启动据此恢复。
func SessionDir() string { return sessionDir() }

func PagesDir() string { return SessionDir() }

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
	return os.MkdirAll(PagesDir(), 0o755)
}
