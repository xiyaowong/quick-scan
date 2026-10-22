package platform

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic 同目录写临时文件再改名：崩溃或断电时不会留下半份 JSON。
// dirPerm 只管目录：%TEMP% 下的记录用 0700，会话目录要能被资源管理器打开，用 0755。
func WriteFileAtomic(path string, data []byte, dirPerm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	// 先 Close 再 Rename：Windows 上打开着的文件改不了名。
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		_ = os.Remove(tmp)
		return werr
	}
	return nil
}
