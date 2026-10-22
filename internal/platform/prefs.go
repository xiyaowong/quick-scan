package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	prefsFileName = "prefs.json"
	prefsVersion  = 1
)

// 和会话目录一样留在 %TEMP% 下：设计上不建配置目录，也不写注册表。
// 被临时文件清理掉只是回到系统默认位置，不值得为此换个地方。
var prefsDir = CacheDir

type prefsData struct {
	Version   int    `json:"version"`
	ExportDir string `json:"exportDir,omitempty"`
}

func prefsPath() string { return filepath.Join(prefsDir(), prefsFileName) }

// 只认记录本身，不确认目录还在不在：
// 记着的可能是已经离线的网络盘，启动时去 stat 会卡住界面。
func LoadExportDir() string {
	data, err := os.ReadFile(prefsPath())
	if err != nil {
		return ""
	}
	var p prefsData
	if json.Unmarshal(data, &p) != nil || p.Version != prefsVersion {
		return ""
	}
	dir := strings.TrimSpace(p.ExportDir)
	if dir == "" || !filepath.IsAbs(dir) {
		return ""
	}
	return dir
}

func ExportDirUsable(dir string) bool {
	if dir == "" {
		return false
	}
	st, err := os.Stat(dir)
	return err == nil && st.IsDir()
}

func SaveExportDir(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return
	}
	data, err := json.Marshal(prefsData{Version: prefsVersion, ExportDir: dir})
	if err != nil {
		return
	}
	_ = WriteFileAtomic(prefsPath(), data, 0o700)
}
