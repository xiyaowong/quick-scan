package update

import (
	"context"
	"os/exec"
)

// runInstaller 静默运行 NSIS 安装包：/S 不带界面，装到同一个目录，
// 装完自己退。调用方必须已经让开 exe：Windows 不让覆盖使用中的文件。
func runInstaller(ctx context.Context, setup string) error {
	cmd := exec.CommandContext(ctx, setup, "/S")
	return cmd.Run()
}
