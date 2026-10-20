package scanner

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
	SaveCache(devs, DeviceKey(devs[0]))

	got, selected := LoadCache()
	if len(got) != 2 {
		t.Fatalf("loaded %d devices", len(got))
	}
	if selected != DeviceKey(devs[0]) {
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
	if devs, _ := LoadCache(); len(devs) != 0 {
		t.Error("bad json accepted")
	}

	write(`{"version":0,"savedAt":1,"devices":[{"ip":"1.2.3.4"}]}`)
	if devs, _ := LoadCache(); len(devs) != 0 {
		t.Error("wrong version accepted")
	}

	old := time.Now().Add(-deviceCacheTTL - time.Hour).Unix()
	data, _ := json.Marshal(deviceCacheData{
		Version: deviceCacheVersion,
		SavedAt: old,
		Devices: []cachedDevice{{IP: "192.168.1.9", Port: 80, Path: "eSCL"}},
	})
	write(string(data))
	if devs, _ := LoadCache(); len(devs) != 0 {
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
	devs, _ := LoadCache()
	if len(devs) != 1 || devs[0].IP != "192.168.1.9" || devs[0].Port != 80 || devs[0].Path != "eSCL" {
		t.Fatalf("filtered cache = %+v", devs)
	}
}

func TestDeviceCacheEmptyDoesNotOverwrite(t *testing.T) {
	withTempCache(t)
	devs := []DeviceInfo{{Name: "Keep", IP: "192.168.1.10", Port: 80, Path: "eSCL"}}
	SaveCache(devs, "")
	SaveCache(nil, "")
	if got, _ := LoadCache(); len(got) != 1 || got[0].Name != "Keep" {
		t.Fatalf("empty save clobbered cache: %+v", got)
	}
}
