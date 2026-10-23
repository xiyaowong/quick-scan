package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupStaleCollectsLegacyLayout(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TEMP", dir)
	t.Setenv("TMP", dir)
	oldVer := version
	version = "9.9.9"
	t.Cleanup(func() { version = oldVer })

	legacySession := filepath.Join(dir, legacySessionDirName)
	if err := os.MkdirAll(legacySession, 0o755); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(legacySession, "a.jpg")
	if err := os.WriteFile(page, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyModels := filepath.Join(dir, legacyProcessPrefix+"123")
	if err := os.MkdirAll(legacyModels, 0o755); err != nil {
		t.Fatal(err)
	}
	oldModels := filepath.Join(dir, tempDirName, "models", "0.0.1")
	curModels := filepath.Join(dir, tempDirName, "models", "9.9.9")
	for _, d := range []string{oldModels, curModels} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	legacyCache := filepath.Join(dir, tempDirName, "devices.json")
	if err := os.WriteFile(legacyCache, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	CleanupStale()

	if _, err := os.Stat(filepath.Join(dir, tempDirName, "session", "a.jpg")); err != nil {
		t.Errorf("旧会话没有迁进新目录: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, tempDirName, "cache", "devices.json")); err != nil {
		t.Errorf("旧缓存没有迁进 cache\\: %v", err)
	}
	if _, err := os.Stat(legacyModels); !os.IsNotExist(err) {
		t.Error("旧版按进程分的模型目录没有删掉")
	}
	if _, err := os.Stat(oldModels); !os.IsNotExist(err) {
		t.Error("旧版本模型目录没有清掉")
	}
	if _, err := os.Stat(curModels); err != nil {
		t.Errorf("当前版本模型目录被误删: %v", err)
	}
}
