package ui

import (
	"os"
	"testing"
)

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
