package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func useSessionDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := sessionDir
	sessionDir = func() string { return dir }
	t.Cleanup(func() { sessionDir = old })
	return dir
}

func writePageFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	dir := useSessionDir(t)
	writePageFile(t, dir, "a.jpg")
	writePageFile(t, dir, "b.png")

	SaveSession([]SessionPage{{File: "b.png", Rot: 90}, {File: "a.jpg"}})
	got, ok := LoadSession()
	if !ok {
		t.Fatal("应有可恢复的会话")
	}
	if len(got) != 2 || got[0].File != "b.png" || got[0].Rot != 90 || got[1].File != "a.jpg" {
		t.Fatalf("恢复清单 = %+v", got)
	}
	if p := PagePath(got[0].File); p != filepath.Join(dir, "b.png") {
		t.Fatalf("PagePath = %s", p)
	}
}

// 扫描中途崩溃时，最后几页可能已经落盘、但还没进清单，也要能恢复。
func TestSessionRecoversUnlistedPages(t *testing.T) {
	dir := useSessionDir(t)
	writePageFile(t, dir, "a.jpg")
	SaveSession([]SessionPage{{File: "a.jpg"}})
	writePageFile(t, dir, "b.jpg")

	got, ok := LoadSession()
	if !ok || len(got) != 2 {
		t.Fatalf("恢复清单 = %+v, ok=%v", got, ok)
	}
	if got[0].File != "a.jpg" || got[1].File != "b.jpg" {
		t.Fatalf("未登记的页应补在末尾：%+v", got)
	}
}

// 没有任何图片就当作没有会话，顺手清掉空目录。
func TestNoSessionWithoutPages(t *testing.T) {
	dir := useSessionDir(t)
	SaveSession([]SessionPage{{File: "gone.jpg"}})
	if _, ok := LoadSession(); ok {
		t.Fatal("文件都不存在，不该提示恢复")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("空会话目录应被清掉，stat err = %v", err)
	}
}

func TestClearSession(t *testing.T) {
	dir := useSessionDir(t)
	writePageFile(t, dir, "a.jpg")
	SaveSession([]SessionPage{{File: "a.jpg"}})
	ClearSession()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("清空后目录应消失，stat err = %v", err)
	}
}
