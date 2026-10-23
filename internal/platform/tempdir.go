package platform

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// 运行期的一切都放这一个目录：缓存、会话、OCR 模型。
	// 卸载时一次删干净，旧版本留下的也只在同一处收拾。
	tempDirName = "quick-scan"

	// 旧版本布局：会话在 %TEMP%\quick-scan-session，模型在 %TEMP%\quick-scan_<pid>。
	legacySessionDirName = "quick-scan-session"
	legacyProcessPrefix  = "quick-scan_"
)

// 用函数而不是常量：测试可以整份换掉，早于测试的 Setenv 也不会被缓存住。
var tempRoot = func() string { return filepath.Join(os.TempDir(), tempDirName) }

// 不跟进程号走：正常退出由 Cleanup 删除，崩溃时目录留着，下次启动据此恢复。
func SessionDir() string { return sessionDir() }

func CacheDir() string { return filepath.Join(tempRoot(), "cache") }

// CleanupStale 把旧版本散落在 %TEMP% 下的东西收进统一目录，并删掉过期模型。
func CleanupStale() {
	migrateLegacySession()
	migrateLegacyCache()

	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), legacyProcessPrefix) {
			_ = os.RemoveAll(filepath.Join(os.TempDir(), e.Name()))
		}
	}
	pruneOldModels()
}

func Cleanup() {
	_ = os.RemoveAll(SessionDir())
}

func EnsurePagesDir() error {
	return os.MkdirAll(SessionDir(), 0o755)
}

// DragDir 放拖给别的程序的图片副本：对方在放下那一刻读的仍是磁盘上的文件，
// 所以不能拿内存里的图冒充。放在会话目录下，正常退出跟页面一起清掉。
func DragDir() string { return filepath.Join(SessionDir(), "drag") }

// 升级前崩在旧版本上时，待恢复的页面还在旧目录里，先搬进来。
func migrateLegacySession() {
	legacy := filepath.Join(os.TempDir(), legacySessionDirName)
	if _, err := os.Stat(legacy); err != nil {
		return
	}
	dst := SessionDir()
	if _, err := os.Stat(dst); err == nil {
		// 新位置已经有更新的数据，旧目录只会是升级前的残留。
		_ = os.RemoveAll(legacy)
		return
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return
	}
	// 搬不动就留着下次再试：里面可能是还没恢复的扫描页。
	_ = os.Rename(legacy, dst)
}

// 旧版本把缓存直接放在根目录；现在统一收进 cache\。
func migrateLegacyCache() {
	cache := CacheDir()
	for _, name := range []string{"devices.json", "prefs.json", "update.json"} {
		moveLegacy(filepath.Join(tempRoot(), name), filepath.Join(cache, name))
	}
	moveLegacy(filepath.Join(tempRoot(), "update"), filepath.Join(cache, "update"))
}

func moveLegacy(from, to string) {
	if _, err := os.Stat(from); err != nil {
		return
	}
	if _, err := os.Stat(to); err == nil {
		_ = os.RemoveAll(from) // 新位置已经有了，旧的不留
		return
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return
	}
	_ = os.Rename(from, to)
}
