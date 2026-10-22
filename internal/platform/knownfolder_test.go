package platform

import "testing"

// 「文档」目录每台 Windows 都该有；拿不到时返回空串也不能崩。
func TestDefaultExportDir(t *testing.T) {
	dir := DefaultExportDir()
	if dir == "" {
		t.Skip("拿不到「文档」目录（重定向到网络盘等），跳过")
	}
	if !ExportDirUsable(dir) {
		t.Fatalf("「文档」目录不可用：%q", dir)
	}
}
