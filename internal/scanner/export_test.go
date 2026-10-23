package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUniquePathSkipsExistingNames(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "扫描.pdf")
	if err := os.WriteFile(first, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := uniquePath(dir, "扫描", "pdf"); got != filepath.Join(dir, "扫描(2).pdf") {
		t.Fatalf("已有同名文件时 = %q", got)
	}
	second := filepath.Join(dir, "扫描(2).pdf")
	if err := os.WriteFile(second, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := uniquePath(dir, "扫描", "pdf"); got != filepath.Join(dir, "扫描(3).pdf") {
		t.Fatalf("两个重名时 = %q", got)
	}
}

// Stat 失败不等于“文件不存在”：返回候选路径让写入报出真正原因，不能在这里空转。
func TestUniquePathReturnsWhenStatFails(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "不是目录")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := uniquePath(file, "x", "jpg"), filepath.Join(file, "x.jpg"); got != want {
		t.Fatalf("父路径不是目录时 = %q，want %q", got, want)
	}
}
