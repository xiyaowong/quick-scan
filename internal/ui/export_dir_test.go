package ui

import (
	"testing"

	"quick-scan/internal/platform"
)

func TestNewRestoresLastExportDir(t *testing.T) {
	withTempCache(t)
	dir := t.TempDir()
	platform.SaveExportDir(dir)

	if got := New().a.lastExportDir; got != dir {
		t.Fatalf("启动时没接上记住的目录：%q, want %q", got, dir)
	}
}

// 没有记录时留空，交给系统默认位置——不能凭空塞一个目录。
func TestNewWithoutExportDirLeavesItEmpty(t *testing.T) {
	withTempCache(t)
	if got := New().a.lastExportDir; got != "" {
		t.Fatalf("没记录却有了目录：%q", got)
	}
}
