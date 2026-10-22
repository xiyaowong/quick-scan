package ui

import (
	"os"
	"testing"
)

// 本地开发时（just run）不更新：仓库里的 exe 不该被线上版本换掉。
func TestUpdateDisabledByEnv(t *testing.T) {
	t.Setenv("QUICK_SCAN_NO_UPDATE", "1")
	if !updatesDisabled() {
		t.Fatal("设了环境变量还去更新")
	}
	os.Unsetenv("QUICK_SCAN_NO_UPDATE")
	if updatesDisabled() {
		t.Fatal("没设环境变量却说关掉了更新")
	}
}
