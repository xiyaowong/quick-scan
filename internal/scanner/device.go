// 设备模型：发现阶段的端点，加上探测后得到的扫描能力。
package scanner

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// DeviceInfo 一台 eSCL 扫描仪。
// 列表会按在线状态重排，所以**位置不能当句柄**——Idx 才是（发现时的下标）。
type DeviceInfo struct {
	Idx         int       // 发现顺序下标：probe/scan 调度靠它找回设备
	Name        string    // 设备名（mDNS TXT ty，缺省用服务实例名）
	IP          string    // IPv4
	Port        int       // eSCL 端口（mDNS SRV，缺省 80）
	Path        string    // eSCL 资源路径（TXT rs，缺省 "eSCL"）
	Online      bool      // 探测成功
	Colors      []string  // "color"|"gray"|"bw"
	Resolutions []float32 // dpi 列表（设备支持多少就列多少）
	Sizes       []string  // "a4"|"a5"|"letter"
	Intents     []string  // eSCL Intent 原样值（Document / TextAndGraphic / Photo / Preview …）
	Feeder      bool      // 有无进纸口（ADF）

	caps *scannerCapabilities // 探测到的原始能力，扫描时下发任务用
}

func (d *DeviceInfo) DisplayName() string {
	name := d.Name
	if name == "" {
		name = d.IP
	}
	return fmt.Sprintf("%s (%s)", name, d.IP)
}

func (d *DeviceInfo) ColorOK(key string) bool {
	if d == nil || len(d.Colors) == 0 {
		return true // 能力缺失时兜底全开
	}
	return slices.Contains(d.Colors, key)
}

func (d *DeviceInfo) SizeOK(key string) bool {
	if d == nil || len(d.Sizes) == 0 {
		return true
	}
	return slices.Contains(d.Sizes, key)
}

func (d *DeviceInfo) DPIs() []int {
	if d == nil || len(d.Resolutions) == 0 {
		return []int{300, 600}
	}
	var out []int
	for _, r := range d.Resolutions {
		if v := int(r + 0.5); v > 0 && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}

func (d *DeviceInfo) HasDPI(v int) bool {
	if d == nil || len(d.Resolutions) == 0 {
		return true
	}
	for _, r := range d.Resolutions {
		if int(r+0.5) == v {
			return true
		}
	}
	return false
}

func (d *DeviceInfo) DefaultDPI() int {
	choices := d.DPIs()
	best := choices[0]
	for _, v := range choices {
		if absInt(v-300) < absInt(best-300) {
			best = v
		}
	}
	return best
}

func (d *DeviceInfo) IntentChoices() []string {
	if d == nil || len(d.Intents) == 0 {
		return []string{IntentDocument}
	}
	return d.Intents
}

func (d *DeviceInfo) HasIntent(v string) bool {
	if d == nil || len(d.Intents) == 0 {
		return v == "" || strings.EqualFold(v, IntentDocument)
	}
	return slices.ContainsFunc(d.Intents, func(x string) bool { return strings.EqualFold(x, v) })
}

func (d *DeviceInfo) DefaultIntent() string {
	if d == nil || len(d.Intents) == 0 {
		return IntentDocument
	}
	for _, x := range d.Intents {
		if strings.EqualFold(x, IntentDocument) {
			return x
		}
	}
	return d.Intents[0]
}

func absInt(v int) int {
	return max(v, -v)
}
