package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func usePrefsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := prefsDir
	prefsDir = func() string { return dir }
	t.Cleanup(func() { prefsDir = old })
	return dir
}

func TestExportDirRoundTrip(t *testing.T) {
	usePrefsDir(t)
	target := t.TempDir()

	if got := LoadExportDir(); got != "" {
		t.Fatalf("没存过却读出了 %q", got)
	}
	SaveExportDir(target)
	if got := LoadExportDir(); got != target {
		t.Fatalf("LoadExportDir = %q, want %q", got, target)
	}
}

// 记录被删掉的目录（或换了盘符）时，导出前的确认要拦住它。
func TestExportDirUsable(t *testing.T) {
	usePrefsDir(t)
	gone := filepath.Join(t.TempDir(), "deleted")
	if err := os.Mkdir(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	SaveExportDir(gone)
	if got := LoadExportDir(); got != gone {
		t.Fatalf("LoadExportDir = %q, want %q", got, gone)
	}
	if !ExportDirUsable(gone) {
		t.Fatal("目录还在却说不能用")
	}
	if err := os.Remove(gone); err != nil {
		t.Fatalf("前提不成立：目录没删掉 %v", err)
	}
	if ExportDirUsable(gone) {
		t.Fatal("被删的目录还能用")
	}
	if ExportDirUsable("") {
		t.Fatal("空路径不能用")
	}
}

func TestExportDirRejectsBadRecord(t *testing.T) {
	dir := usePrefsDir(t)
	path := filepath.Join(dir, prefsFileName)
	for _, raw := range []string{
		"{not json",
		`{"version":0,"exportDir":"C:\\扫描"}`,
		`{"version":1,"exportDir":"relative\\dir"}`,
		`{"version":1,"exportDir":"   "}`,
	} {
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := LoadExportDir(); got != "" {
			t.Fatalf("%s → %q", raw, got)
		}
	}
}

// 空串是「没有位置」而不是「清空记忆」：别让一次异常调用抹掉用户的目录。
func TestSaveExportDirIgnoresEmpty(t *testing.T) {
	usePrefsDir(t)
	target := t.TempDir()
	SaveExportDir(target)
	SaveExportDir("")
	if got := LoadExportDir(); got != target {
		t.Fatalf("空值抹掉了记录：%q", got)
	}
}

// 两类记忆各自保存：单独存扫描设置不能抹掉导出目录，反之亦然。
func TestScanPrefsRoundTrip(t *testing.T) {
	usePrefsDir(t)

	if got := LoadScanPrefs(); got != nil {
		t.Fatalf("没存过却读出了 %+v", got)
	}
	SaveExportDir(t.TempDir())
	SaveScanPrefs(ScanPrefs{Color: 2, Size: 1, DPI: 600, Intent: "Photo", Feeder: true})

	got := LoadScanPrefs()
	if got == nil || *got != (ScanPrefs{Color: 2, Size: 1, DPI: 600, Intent: "Photo", Feeder: true}) {
		t.Fatalf("LoadScanPrefs = %+v", got)
	}
	if LoadExportDir() == "" {
		t.Fatal("存扫描设置抹掉了导出目录")
	}
}
