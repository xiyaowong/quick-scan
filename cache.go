// 设备缓存：上次发现的打印机端点 + 能力，放在 %TEMP% 下的固定目录。
//
// 目的只有一个——启动时先把上次的打印机名单显示出来，同时后台做 mDNS 全量发现，
// 列表到手后无感合并。缓存不是配置：不写注册表、不建用户目录，系统清理临时目录
// （存储感知/磁盘清理）时随手丢掉也不影响正确性。每次启动都会重新探测与发现。
package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	deviceCacheVersion = 1
	deviceCacheLimit   = 32
	deviceCacheTTL     = 30 * 24 * time.Hour
)

// deviceCacheDir 是缓存所在目录。测试里可整体替换。
var deviceCacheDir = func() string {
	return filepath.Join(os.TempDir(), "sscan")
}

type deviceCacheData struct {
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

func deviceCachePath() string { return filepath.Join(deviceCacheDir(), "devices.json") }

// loadDeviceCache 读上次的设备名单。任何异常都当成"没有缓存"，
// 缓存只用于提速，绝不能影响启动或正确性。
func loadDeviceCache() ([]DeviceInfo, string) {
	data, err := os.ReadFile(deviceCachePath())
	if err != nil {
		return nil, ""
	}
	var c deviceCacheData
	if json.Unmarshal(data, &c) != nil || c.Version != deviceCacheVersion {
		return nil, ""
	}
	if c.SavedAt <= 0 || time.Since(time.Unix(c.SavedAt, 0)) > deviceCacheTTL {
		return nil, ""
	}
	var out []DeviceInfo
	seen := map[string]bool{}
	for _, cd := range c.Devices {
		if len(out) >= deviceCacheLimit {
			break
		}
		d, ok := cd.deviceInfo()
		if !ok || seen[deviceKey(d)] {
			continue
		}
		seen[deviceKey(d)] = true
		out = append(out, d) // Online 先置 false：缓存的状态不算数，启动后重新探测
	}
	return out, c.Selected
}

// saveDeviceCache 原子写入当前名单。空名单不覆盖旧缓存——
// 发现失败时保留上一次的线索，下次启动仍然可以先显示出来。
func saveDeviceCache(devs []DeviceInfo, selected string) {
	out := make([]cachedDevice, 0, len(devs))
	for _, d := range devs {
		if len(out) >= deviceCacheLimit {
			break
		}
		if cd, ok := makeCachedDevice(d); ok {
			out = append(out, cd)
		}
	}
	if len(out) == 0 {
		return
	}
	data, err := json.Marshal(deviceCacheData{
		Version:  deviceCacheVersion,
		SavedAt:  time.Now().Unix(),
		Selected: selected,
		Devices:  out,
	})
	if err != nil {
		return
	}
	dir := deviceCacheDir()
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	// 同目录临时文件 + rename：避免读到写了一半的 JSON。
	f, err := os.CreateTemp(dir, "devices-*.tmp")
	if err != nil {
		return
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp)
		return
	}
	if os.Rename(tmp, deviceCachePath()) != nil {
		_ = os.Remove(tmp)
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
		if x != "" && !containsString(out, x) {
			out = append(out, x)
		}
	}
	return out
}

// deviceKey 端点的身份：IP + 端口 + eSCL 路径。列表排序/显示名都不参与。
func deviceKey(d DeviceInfo) string {
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
