package ui

import (
	"testing"

	"sscan/internal/scanner"
)

// 网段扫描只负责加设备：同一台不重复加，身份分配好，不要冒充在线。
func TestAddDiscoveredMergesByKey(t *testing.T) {
	withTempCache(t)
	a := newApp()

	d := scanner.DeviceInfo{Name: "HP Laser MFP 136nw", IP: "192.168.1.10", Port: 80, Path: "eSCL"}
	a.addDiscovered(d)
	a.addDiscovered(d) // 同一台又报了一次

	if len(a.devices) != 1 {
		t.Fatalf("重复设备加了两次：%+v", a.devices)
	}
	if a.devices[0].Idx == 0 {
		t.Error("设备身份没有分配")
	}
	if a.devices[0].Online {
		t.Error("扫描只说明设备在，不能直接当探测成功")
	}
	if a.devSel != 0 || a.current() == nil || a.current().Name != d.Name {
		t.Fatalf("新增设备没被选中：devSel=%d devices=%+v", a.devSel, a.devices)
	}

	a.addDiscovered(scanner.DeviceInfo{Name: "Canon", IP: "192.168.1.11", Port: 80, Path: "eSCL"})
	if len(a.devices) != 2 {
		t.Fatalf("第二台没加上：%+v", a.devices)
	}
}
