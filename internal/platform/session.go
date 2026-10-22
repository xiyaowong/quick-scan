package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	sessionFileName = "session.json"
	sessionVersion  = 1
)

// 测试里替换成临时目录。
var sessionDir = func() string {
	return filepath.Join(os.TempDir(), sessionDirName)
}

type SessionPage struct {
	File string `json:"file"` // 会话目录里的文件名
	Rot  int    `json:"rot,omitempty"`
}

type sessionData struct {
	Version int           `json:"version"`
	Pages   []SessionPage `json:"pages"`
}

func PagePath(file string) string {
	return filepath.Join(SessionDir(), filepath.Base(file))
}

// 只记页序和旋转；图片本身已经落在目录里。
func SaveSession(pages []SessionPage) {
	path := filepath.Join(SessionDir(), sessionFileName)
	if len(pages) == 0 {
		_ = os.Remove(path)
		return
	}
	data, err := json.Marshal(sessionData{Version: sessionVersion, Pages: pages})
	if err != nil {
		return
	}
	_ = WriteFileAtomic(path, data, 0o755)
}

// 有残留页面就返回清单。扫描中途崩溃时最后几页可能还没来得及登记，
// 所以把目录里没被清单引用的图片补在后面。
func LoadSession() ([]SessionPage, bool) {
	dir := SessionDir()
	files := pageFiles(dir)
	if len(files) == 0 {
		_ = os.RemoveAll(dir)
		return nil, false
	}
	exists := make(map[string]bool, len(files))
	for _, name := range files {
		exists[name] = true
	}

	var pages []SessionPage
	used := map[string]bool{}
	if data, err := os.ReadFile(filepath.Join(dir, sessionFileName)); err == nil {
		var c sessionData
		if json.Unmarshal(data, &c) == nil && c.Version == sessionVersion {
			for _, p := range c.Pages {
				name := filepath.Base(strings.TrimSpace(p.File))
				if name == "" || name == "." || !exists[name] || used[name] {
					continue
				}
				used[name] = true
				pages = append(pages, SessionPage{File: name, Rot: normRot(p.Rot)})
			}
		}
	}
	for _, name := range files {
		if !used[name] {
			pages = append(pages, SessionPage{File: name})
		}
	}
	if len(pages) == 0 {
		return nil, false
	}
	return pages, true
}

func ClearSession() {
	_ = os.RemoveAll(SessionDir())
}

func pageFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".jpg", ".jpeg", ".png":
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

func normRot(r int) int {
	r = ((r % 360) + 360) % 360
	if r%90 != 0 {
		return 0
	}
	return r
}
