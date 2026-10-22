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

// ScanPrefs 是跨会话记忆的扫描设置，存界面枚举值而非设备能力键：
// 换设备后由界面按设备能力重新校准。
type ScanPrefs struct {
	Color  int    `json:"color,omitempty"`
	Size   int    `json:"size,omitempty"`
	DPI    int    `json:"dpi,omitempty"`
	Intent string `json:"intent,omitempty"`
	Feeder bool   `json:"feeder,omitempty"`
}

type prefsData struct {
	Version   int         `json:"version"`
	ExportDir string      `json:"exportDir,omitempty"`
	Scan      *ScanPrefs  `json:"scan,omitempty"`
}

func prefsPath() string { return filepath.Join(prefsDir(), prefsFileName) }

// 只认记录本身，不确认目录还在不在：
// 记着的可能是已经离线的网络盘，启动时去 stat 会卡住界面。
func loadPrefs() prefsData {
	data, err := os.ReadFile(prefsPath())
	if err != nil {
		return prefsData{}
	}
	var p prefsData
	if json.Unmarshal(data, &p) != nil || p.Version != prefsVersion {
		return prefsData{}
	}
	return p
}

func savePrefs(p prefsData) {
	p.Version = prefsVersion // loadPrefs 失败时带回的是零值，这里统一盖章
	data, err := json.Marshal(p)
	if err != nil {
		return
	}
	_ = WriteFileAtomic(prefsPath(), data, 0o700)
}

func LoadExportDir() string {
	dir := strings.TrimSpace(loadPrefs().ExportDir)
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

// 空串是「没有位置」而不是「清空记忆」：别让一次异常调用抹掉用户的目录。
func SaveExportDir(dir string) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return
	}
	p := loadPrefs()
	p.ExportDir = dir
	savePrefs(p)
}

func LoadScanPrefs() *ScanPrefs {
	if p := loadPrefs(); p.Scan != nil {
		return p.Scan
	}
	return nil
}

func SaveScanPrefs(s ScanPrefs) {
	p := loadPrefs()
	p.Scan = &s
	savePrefs(p)
}
