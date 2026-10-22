package scanner

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"quick-scan/internal/platform"
)

const (
	cacheVersion = 1
	cacheLimit   = 32
	cacheTTL     = 30 * 24 * time.Hour
)

// 测试里替换成临时目录。
var cacheDir = func() string { return platform.CacheDir() }

type cacheFile struct {
	Version  int            `json:"version"`
	SavedAt  int64          `json:"savedAt"` // Unix 秒；过期就整份忽略
	Selected string         `json:"selected,omitempty"`
	Devices  []cachedDevice `json:"devices"`
}

type cachedDevice struct {
	Name        string    `json:"name"`
	IP          string    `json:"ip"`
	Port        int       `json:"port"`
	Path        string    `json:"path"`
	Online      bool      `json:"online"` // 上次会话最后一次探测结果
	Colors      []string  `json:"colors,omitempty"`
	Resolutions []float32 `json:"resolutions,omitempty"`
	Sizes       []string  `json:"sizes,omitempty"`
	Intents     []string  `json:"intents,omitempty"`
	Feeder      bool      `json:"feeder,omitempty"`
}

func cachePath() string { return filepath.Join(cacheDir(), "devices.json") }

// LoadCache 返回缓存的设备名单和上次选中的键；记录过期、损坏或一个可用条目都没有时返回空。
func LoadCache() ([]DeviceInfo, string) {
	data, err := os.ReadFile(cachePath())
	if err != nil {
		return nil, ""
	}
	var c cacheFile
	if json.Unmarshal(data, &c) != nil || c.Version != cacheVersion {
		return nil, ""
	}
	if c.SavedAt <= 0 || time.Since(time.Unix(c.SavedAt, 0)) > cacheTTL {
		return nil, ""
	}
	devs := []DeviceInfo{}
	seen := map[string]bool{}
	for _, cd := range c.Devices {
		if len(devs) >= cacheLimit {
			break
		}
		d, ok := cd.deviceInfo()
		if !ok || seen[DeviceKey(d)] {
			continue
		}
		seen[DeviceKey(d)] = true
		devs = append(devs, d) // Online 先置 false：缓存的状态不算数，启动后重新探测
	}
	return devs, c.Selected
}

func SaveCache(devs []DeviceInfo, selected string) {
	out := make([]cachedDevice, 0, len(devs))
	for _, d := range devs {
		if len(out) >= cacheLimit {
			break
		}
		if cd, ok := makeCachedDevice(d); ok {
			out = append(out, cd)
		}
	}
	// 空名单不落盘：一次失败的发现不能把上次的可用设备抹掉。
	if len(out) == 0 {
		return
	}
	data, err := json.Marshal(cacheFile{
		Version:  cacheVersion,
		SavedAt:  time.Now().Unix(),
		Selected: selected,
		Devices:  out,
	})
	if err != nil {
		return
	}
	// 同目录临时文件 + rename，避免读到写了一半的 JSON。
	if err := platform.WriteFileAtomic(cachePath(), data, 0o700); err != nil {
		return
	}
}

func makeCachedDevice(d DeviceInfo) (cachedDevice, bool) {
	ip := net.ParseIP(strings.TrimSpace(d.IP)).To4()
	if ip == nil {
		return cachedDevice{}, false
	}
	port := d.Port
	if port <= 0 || port > 65535 {
		port = defaultPort
	}
	path := strings.Trim(strings.TrimSpace(d.Path), "/")
	if path == "" {
		path = defaultESCLPath
	}
	return cachedDevice{
		Name:        strings.TrimSpace(d.Name),
		IP:          ip.String(),
		Port:        port,
		Path:        path,
		Online:      d.Online,
		Colors:      append([]string(nil), d.Colors...),
		Resolutions: append([]float32(nil), d.Resolutions...),
		Sizes:       append([]string(nil), d.Sizes...),
		Intents:     append([]string(nil), d.Intents...),
		Feeder:      d.Feeder,
	}, true
}

func (c cachedDevice) deviceInfo() (DeviceInfo, bool) {
	ip := net.ParseIP(strings.TrimSpace(c.IP)).To4()
	if ip == nil {
		return DeviceInfo{}, false
	}
	port := c.Port
	if port <= 0 || port > 65535 {
		port = defaultPort
	}
	path := strings.Trim(strings.TrimSpace(c.Path), "/")
	if path == "" {
		path = defaultESCLPath
	}
	var resolutions []float32
	for _, r := range c.Resolutions {
		if r > 0 {
			resolutions = append(resolutions, r)
		}
	}
	return DeviceInfo{
		Name:        strings.TrimSpace(c.Name),
		IP:          ip.String(),
		Port:        port,
		Path:        path,
		Online:      false, // 待本次启动探测确认
		Colors:      cleanStrings(c.Colors),
		Resolutions: resolutions,
		Sizes:       cleanStrings(c.Sizes),
		Intents:     cleanStrings(c.Intents),
		Feeder:      c.Feeder,
	}, true
}

func cleanStrings(xs []string) []string {
	var out []string
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x != "" && !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

func DeviceKey(d DeviceInfo) string {
	port := d.Port
	if port <= 0 || port > 65535 {
		port = defaultPort
	}
	path := strings.Trim(strings.TrimSpace(d.Path), "/")
	if path == "" {
		path = defaultESCLPath
	}
	return strings.ToLower(d.IP + ":" + strconv.Itoa(port) + "/" + path)
}
