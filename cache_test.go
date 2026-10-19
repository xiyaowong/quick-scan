package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func withTempCache(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := deviceCacheDir
	deviceCacheDir = func() string { return dir }
	t.Cleanup(func() { deviceCacheDir = old })
	return dir
}

func TestDeviceCacheRoundTrip(t *testing.T) {
	withTempCache(t)
	devs := []DeviceInfo{
		{
			Name: "HP Laser MFP 116w", IP: "192.168.3.111", Port: 80, Path: "eSCL",
			Online: true, Colors: []string{"color", "gray", "bw"},
			Resolutions: []float32{100, 200, 300}, Sizes: []string{"a4", "a5", "letter"},
			Intents: []string{"Document", "Photo"}, Feeder: false,
		},
		{Name: "Second", IP: "10.0.0.2", Port: 8080, Path: "/eSCL/", Online: false},
	}
	saveDeviceCache(devs, deviceKey(devs[0]))

	got, selected := loadDeviceCache()
	if len(got) != 2 {
		t.Fatalf("loaded %d devices", len(got))
	}
	if selected != deviceKey(devs[0]) {
		t.Errorf("selected = %q", selected)
	}
	d := got[0]
	if d.Name != devs[0].Name || d.IP != devs[0].IP || d.Port != 80 || d.Path != "eSCL" {
		t.Errorf("identity = %+v", d)
	}
	if len(d.Colors) != 3 || len(d.Resolutions) != 3 || len(d.Intents) != 2 || d.Feeder {
		t.Errorf("caps = %+v", d)
	}
	if d.Online {
		t.Error("cached online state must not be trusted on load")
	}
	if got[1].Port != 8080 || got[1].Path != "eSCL" {
		t.Errorf("normalized second = %+v", got[1])
	}
}

func TestDeviceCacheRejectsBadOrStale(t *testing.T) {
	dir := withTempCache(t)
	path := filepath.Join(dir, "devices.json")

	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write("{not json")
	if devs, _ := loadDeviceCache(); len(devs) != 0 {
		t.Error("bad json accepted")
	}

	write(`{"version":0,"savedAt":1,"devices":[{"ip":"1.2.3.4"}]}`)
	if devs, _ := loadDeviceCache(); len(devs) != 0 {
		t.Error("wrong version accepted")
	}

	old := time.Now().Add(-deviceCacheTTL - time.Hour).Unix()
	data, _ := json.Marshal(deviceCacheData{
		Version: deviceCacheVersion,
		SavedAt: old,
		Devices: []cachedDevice{{IP: "192.168.1.9", Port: 80, Path: "eSCL"}},
	})
	write(string(data))
	if devs, _ := loadDeviceCache(); len(devs) != 0 {
		t.Error("expired cache accepted")
	}

	data, _ = json.Marshal(deviceCacheData{
		Version: deviceCacheVersion,
		SavedAt: time.Now().Unix(),
		Devices: []cachedDevice{
			{IP: "not-an-ip", Port: 80, Path: "eSCL"},
			{IP: "192.168.1.9", Port: 0, Path: "  "},
			{IP: "192.168.1.9", Port: 0, Path: ""}, // 与上一条重复
		},
	})
	write(string(data))
	devs, _ := loadDeviceCache()
	if len(devs) != 1 || devs[0].IP != "192.168.1.9" || devs[0].Port != 80 || devs[0].Path != "eSCL" {
		t.Fatalf("filtered cache = %+v", devs)
	}
}

func TestDeviceCacheEmptyDoesNotOverwrite(t *testing.T) {
	withTempCache(t)
	devs := []DeviceInfo{{Name: "Keep", IP: "192.168.1.10", Port: 80, Path: "eSCL"}}
	saveDeviceCache(devs, "")
	saveDeviceCache(nil, "")
	if got, _ := loadDeviceCache(); len(got) != 1 || got[0].Name != "Keep" {
		t.Fatalf("empty save clobbered cache: %+v", got)
	}
}

func TestApplyCachedSelectsLastUsed(t *testing.T) {
	withTempCache(t)
	devs := []DeviceInfo{
		{Name: "A", IP: "192.168.1.10", Port: 80, Path: "eSCL", Online: true},
		{Name: "B", IP: "192.168.1.11", Port: 80, Path: "eSCL", Online: true},
	}
	a := newApp()
	a.applyCached(devs, deviceKey(devs[1]))

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
	devs := []DeviceInfo{
		{Name: "A", IP: "192.168.1.10", Port: 80, Path: "eSCL"},
		{Name: "B", IP: "192.168.1.11", Port: 80, Path: "eSCL"},
		{Name: "C", IP: "192.168.1.12", Port: 80, Path: "eSCL"},
	}
	a := newApp()
	a.applyCached(devs, deviceKey(devs[1]))
	a.probeCached()

	if len(a.autoQueue) != 2 || a.autoQueue[0] != a.devices[0].Idx || a.autoQueue[1] != a.devices[2].Idx {
		t.Fatalf("autoQueue = %v, devices = %+v", a.autoQueue, a.devices)
	}
	// win 为 nil 时 requestProbe 不发起网络探测，状态保持不变
	if a.probing {
		t.Error("headless probe should not start")
	}
}

func TestMergeDetectedPreservesAndPrunes(t *testing.T) {
	withTempCache(t)
	a := newApp()
	a.applyCached([]DeviceInfo{
		{Name: "Cached", IP: "192.168.1.10", Port: 80, Path: "eSCL", Colors: []string{"color"}},
		{Name: "Gone", IP: "192.168.1.99", Port: 80, Path: "eSCL"},
	}, "")

	keptIdx := a.devices[0].Idx
	goneIdx := a.devices[1].Idx
	a.probed[keptIdx] = true
	a.devices[0].Online = true
	a.probed[goneIdx] = true
	a.devices[1].Online = false

	a.mergeDetected([]DeviceInfo{
		{Name: "Rediscovered", IP: "192.168.1.10", Port: 80, Path: "eSCL"},
		{Name: "New", IP: "192.168.1.12", Port: 80, Path: "eSCL"},
	})

	if len(a.devices) != 2 {
		t.Fatalf("merged devices = %+v", a.devices)
	}
	var old, fresh *DeviceInfo
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
	a.applyCached([]DeviceInfo{
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
	a.applyCached([]DeviceInfo{
		{Name: "Stale", IP: "192.168.1.50", Port: 80, Path: "eSCL"},
	}, "")
	a.mergeDetected([]DeviceInfo{
		{Name: "Found", IP: "192.168.1.51", Port: 80, Path: "eSCL"},
	})
	if len(a.devices) != 1 || a.devices[0].Name != "Found" {
		t.Fatalf("stale cache entry survived: %+v", a.devices)
	}
}
