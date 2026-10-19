// 临时目录管理：内嵌资产释放、扫描原图都放 %TEMP%，退出即清理（零持久化，定稿 §三-2）。
package main

import (
	"os"
	"path/filepath"
	"strconv"
)

var tempRoot string

func init() {
	tempRoot = filepath.Join(os.TempDir(), "SimpleMFPScan_"+strconv.Itoa(os.Getpid()))
}

// cleanupStaleTemp 启动时删除上次异常退出残留的 SimpleMFPScan_* 目录
// （pid 不同才删，避免误删运行中实例——单实例假设下可全删）
func cleanupStaleTemp() {
	tmp := os.TempDir()
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return
	}
	self := "SimpleMFPScan_" + strconv.Itoa(os.Getpid())
	for _, e := range entries {
		name := e.Name()
		if len(name) > len("SimpleMFPScan_") && name[:len("SimpleMFPScan_")] == "SimpleMFPScan_" && name != self {
			_ = os.RemoveAll(filepath.Join(tmp, name))
		}
	}
}

// cleanupTemp 退出时删整个临时目录
func cleanupTemp() {
	_ = os.RemoveAll(tempRoot)
}

// ensurePagesDir 保证扫描页目录存在
func ensurePagesDir() error {
	return os.MkdirAll(pagesDir(), 0o755)
}
