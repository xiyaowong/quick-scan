package ui

import (
	"testing"

	"quick-scan/internal/platform"
)

// 待恢复的页面还没进 pages，但也算"没处理的数据"：不能悄悄关窗把它删掉。
func TestPendingRecoveryBlocksClose(t *testing.T) {
	r := &App{a: newApp()}
	r.a.recoverPages = []platform.SessionPage{{File: "a.jpg"}, {File: "b.jpg"}}
	if r.CloseAllowed() {
		t.Fatal("有待恢复页面时不该允许直接关窗")
	}
	r.a.recoverPages = nil
	if !r.CloseAllowed() {
		t.Fatal("没有页面时应该允许关窗")
	}
}
