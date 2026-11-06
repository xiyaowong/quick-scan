package ui

import (
	"testing"

	"quick-scan/internal/scanner"
)

// withTempCache 把设备缓存写进测试临时目录，避免碰用户真实的 %TEMP%\quick-scan。
func withTempCache(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
}

func TestApplyCachedSelectsLastUsed(t *testing.T) {
	withTempCache(t)
	devs := []scanner.DeviceInfo{
		{Name: "A", IP: "192.168.1.10", Port: 80, Path: "eSCL", Online: true},
		{Name: "B", IP: "192.168.1.11", Port: 80, Path: "eSCL", Online: true},
	}
	a := newApp()
	a.applyCached(devs, scanner.DeviceKey(devs[1]))

	if a.devSel != 1 || a.current().Name != "B" {
		t.Fatalf("selected = %d %+v", a.devSel, a.current())
	}
	if a.devState != DevUnknown {
		t.Error("cached devices must start unprobed")
	}
	if len(a.probed) != 0 {
		t.Error("cache load should not mark anything probed")
	}
	if a.devices[0].Idx == a.devices[1].Idx {
		t.Error("device identities must be unique")
	}
}

func TestProbeCachedQueuesSelectedFirst(t *testing.T) {
	withTempCache(t)
	devs := []scanner.DeviceInfo{
		{Name: "A", IP: "192.168.1.10", Port: 80, Path: "eSCL"},
		{Name: "B", IP: "192.168.1.11", Port: 80, Path: "eSCL"},
		{Name: "C", IP: "192.168.1.12", Port: 80, Path: "eSCL"},
	}
	a := newApp()
	a.applyCached(devs, scanner.DeviceKey(devs[1]))
	a.probeCached()

	if len(a.autoQueue) != 2 || a.autoQueue[0] != a.devices[0].Idx || a.autoQueue[1] != a.devices[2].Idx {
		t.Fatalf("autoQueue = %v, devices = %+v", a.autoQueue, a.devices)
	}
	// Win 为 nil 时 requestProbe 不发起网络探测，状态保持不变
	if a.probing {
		t.Error("headless probe should not start")
	}
}

func TestMergeDetectedPreservesAndPrunes(t *testing.T) {
	withTempCache(t)
	a := newApp()
	a.applyCached([]scanner.DeviceInfo{
		{Name: "Cached", IP: "192.168.1.10", Port: 80, Path: "eSCL", Colors: []string{"color"}},
		{Name: "Gone", IP: "192.168.1.99", Port: 80, Path: "eSCL"},
	}, "")

	keptIdx := a.devices[0].Idx
	goneIdx := a.devices[1].Idx
	a.probed[keptIdx] = true
	a.devices[0].Online = true
	a.probed[goneIdx] = true
	a.devices[1].Online = false

	a.mergeDetected([]scanner.DeviceInfo{
		{Name: "Rediscovered", IP: "192.168.1.10", Port: 80, Path: "eSCL"},
		{Name: "New", IP: "192.168.1.12", Port: 80, Path: "eSCL"},
	})

	if len(a.devices) != 2 {
		t.Fatalf("merged devices = %+v", a.devices)
	}
	var old, fresh *scanner.DeviceInfo
	for i := range a.devices {
		switch a.devices[i].IP {
		case "192.168.1.10":
			old = &a.devices[i]
		case "192.168.1.12":
			fresh = &a.devices[i]
		}
	}
	if old == nil || fresh == nil {
		t.Fatalf("missing merged devices: %+v", a.devices)
	}
	if old.Idx != keptIdx {
		t.Errorf("existing identity changed: %d != %d", old.Idx, keptIdx)
	}
	if !old.Online || len(old.Colors) != 1 || old.Name != "Rediscovered" {
		t.Errorf("existing state not preserved: %+v", old)
	}
	if fresh.Idx == keptIdx || fresh.Idx == goneIdx {
		t.Errorf("new device reused identity: %+v", fresh)
	}
	if a.devState != DevOnline {
		t.Errorf("selection should stay on online device, state=%v", a.devState)
	}
}

func TestMergeEmptyDiscoveryKeepsCache(t *testing.T) {
	withTempCache(t)
	a := newApp()
	a.applyCached([]scanner.DeviceInfo{
		{Name: "Cached", IP: "192.168.1.10", Port: 80, Path: "eSCL"},
	}, "")

	// mDNS 一个都没回（可能被防火墙挡了）：不能据此清空缓存名单。
	a.mergeDetected(nil)
	if len(a.devices) != 1 || a.devices[0].Name != "Cached" {
		t.Fatalf("empty discovery wiped cache: %+v", a.devices)
	}
}

func TestMergePrunesUnseenUnprobedCacheEntries(t *testing.T) {
	withTempCache(t)
	a := newApp()
	a.applyCached([]scanner.DeviceInfo{
		{Name: "Stale", IP: "192.168.1.50", Port: 80, Path: "eSCL"},
	}, "")
	a.mergeDetected([]scanner.DeviceInfo{
		{Name: "Found", IP: "192.168.1.51", Port: 80, Path: "eSCL"},
	})
	if len(a.devices) != 1 || a.devices[0].Name != "Found" {
		t.Fatalf("stale cache entry survived: %+v", a.devices)
	}
}

// 探测失败又没被本轮发现看到：这条缓存作废，删掉并写回文件。
func TestDropStaleRemovesUnconfirmedCacheEntry(t *testing.T) {
	withTempCache(t)
	a := newApp()
	a.applyCached([]scanner.DeviceInfo{
		{Name: "Stale", IP: "192.0.2.10", Port: 80, Path: "eSCL"},
		{Name: "Alive", IP: "192.0.2.11", Port: 80, Path: "eSCL"},
	}, scanner.DeviceKey(scanner.DeviceInfo{IP: "192.0.2.10", Port: 80, Path: "eSCL"}))

	if !a.dropStale(a.devices[0].Idx) {
		t.Fatal("unconfirmed cache entry should be dropped")
	}
	if len(a.devices) != 1 || a.devices[0].Name != "Alive" {
		t.Fatalf("devices = %+v", a.devices)
	}
	if a.devSel != 0 || a.current().Name != "Alive" {
		t.Fatalf("selection = %d %+v", a.devSel, a.current())
	}
	if got, _ := scanner.LoadCache(); len(got) != 1 || got[0].Name != "Alive" {
		t.Fatalf("cache still has stale entry: %+v", got)
	}
}

func TestDropStaleKeepsConfirmedAndUndetectedDevices(t *testing.T) {
	withTempCache(t)
	a := newApp()
	a.applyCached([]scanner.DeviceInfo{
		{Name: "Seen", IP: "192.0.2.10", Port: 80, Path: "eSCL"},
	}, "")
	idx := a.devices[0].Idx

	a.confirmed[idx] = true
	if a.dropStale(idx) {
		t.Fatal("confirmed device must stay")
	}
	delete(a.confirmed, idx)
	a.detecting = true
	if a.dropStale(idx) {
		t.Fatal("must not drop while discovery is still running")
	}
	if len(a.devices) != 1 {
		t.Fatalf("devices = %+v", a.devices)
	}
}

// 发现结束后的兜底清理：探测失败的未确认条目走人，在线和在探测中的留下。
func TestPruneStaleAfterDiscovery(t *testing.T) {
	withTempCache(t)
	a := newApp()
	a.applyCached([]scanner.DeviceInfo{
		{Name: "Stale", IP: "192.0.2.10", Port: 80, Path: "eSCL"},
		{Name: "Online", IP: "192.0.2.11", Port: 80, Path: "eSCL"},
	}, "")
	a.probed[a.devices[0].Idx] = true
	a.probed[a.devices[1].Idx] = true
	a.devices[1].Online = true

	a.pruneStale()
	if len(a.devices) != 1 || a.devices[0].Name != "Online" {
		t.Fatalf("devices = %+v", a.devices)
	}

	a.detecting = true
	a.probed[a.devices[0].Idx] = true
	a.devices[0].Online = false
	a.pruneStale()
	if len(a.devices) != 1 {
		t.Fatalf("must not prune while discovery is running: %+v", a.devices)
	}
}
