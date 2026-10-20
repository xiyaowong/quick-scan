// 临时目录：内嵌资产与扫描原图都放 %TEMP%，退出即清。
package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const tempPrefix = "sscan_"

var tempRoot string

func init() {
	tempRoot = filepath.Join(os.TempDir(), tempPrefix+strconv.Itoa(os.Getpid()))
}

// cleanupStaleTemp 删除上次异常退出残留的临时目录。
func cleanupStaleTemp() {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}
	self := tempPrefix + strconv.Itoa(os.Getpid())
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, tempPrefix) && name != self {
			_ = os.RemoveAll(filepath.Join(os.TempDir(), name))
		}
	}
}

func cleanupTemp() {
	_ = os.RemoveAll(tempRoot)
}

func ensurePagesDir() error {
	return os.MkdirAll(pagesDir(), 0o755)
}
