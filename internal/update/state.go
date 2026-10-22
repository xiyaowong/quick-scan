package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"quick-scan/internal/platform"
)

// 检查记录放 %TEMP%\quick-scan\：和扫描设置、设备缓存同一个口径，
// 不建配置目录、不写注册表。
const stateFileName = "update.json"

// 测试里替换成临时目录。
var stateDir = func() string { return platform.CacheDir() }

type state struct {
	CheckedAt int64 `json:"checkedAt"`
	// 下载好、等着装的新版本。
	Pending *pendingRelease `json:"pending,omitempty"`
}

type pendingRelease struct {
	Version string `json:"version"`
	Asset   string `json:"asset"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256,omitempty"`
	Size    int64  `json:"size"`
}

func statePath() string { return filepath.Join(stateDir(), stateFileName) }

func loadState() state {
	data, err := os.ReadFile(statePath())
	if err != nil {
		return state{}
	}
	var s state
	if json.Unmarshal(data, &s) != nil {
		return state{}
	}
	return s
}

func saveState(s state) {
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = platform.WriteFileAtomic(statePath(), data, 0o700)
}

// Due 距离上次成功检查是否已经超过 d。没记录时算到期。
func Due(d time.Duration) bool {
	s := loadState()
	if s.CheckedAt <= 0 {
		return true
	}
	elapsed := time.Since(time.Unix(s.CheckedAt, 0))
	// 时钟被往回拨过：当成到期，别把更新永久锁死。
	return elapsed < 0 || elapsed >= d
}

func MarkChecked() {
	s := loadState()
	s.CheckedAt = time.Now().Unix()
	saveState(s)
}

// LoadPending 返回上次已经下载好、等着装的新版本；没有就返回 nil。
func LoadPending() *Release {
	s := loadState()
	if s.Pending == nil || s.Pending.Version == "" || s.Pending.Asset == "" {
		return nil
	}
	p := s.Pending
	return &Release{Version: p.Version, Asset: p.Asset, URL: p.URL, SHA256: p.SHA256, Size: p.Size}
}

func SavePending(r *Release) {
	s := loadState()
	if r == nil {
		s.Pending = nil
	} else {
		s.Pending = &pendingRelease{Version: r.Version, Asset: r.Asset, URL: r.URL, SHA256: r.SHA256, Size: r.Size}
	}
	saveState(s)
}
