package update

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
)

// runInstaller 静默运行 NSIS 安装包：/S 不带界面，/D= 指定装到哪。
//
// NSIS 的 /D= 必须是最后一个参数，并且**不能带引号**，路径里有空格也一样。
// Go 的 exec 会给带空格的参数自动加引号，NSIS 就解析不了，所以这里自己拼
// 命令行（只给 exe 路径加引号）。路径末尾带反斜杠会把引号转义掉，去掉。
func runInstaller(ctx context.Context, setup, dir string) error {
	cmd := exec.CommandContext(ctx, setup)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: installerCmdLine(setup, dir)}
	return cmd.Run()
}

func installerCmdLine(setup, dir string) string {
	line := quoteIfNeeded(setup) + " /S"
	if dir != "" {
		line += " /D=" + strings.TrimRight(dir, `\`)
	}
	return line
}

func quoteIfNeeded(s string) string {
	if !strings.ContainsAny(s, " \t") {
		return s
	}
	return `"` + s + `"`
}
