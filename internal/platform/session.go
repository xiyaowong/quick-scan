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
	return filepath.Join(tempRoot(), "session")
}

type SessionPage struct {
	File    string `json:"file"`              // 会话目录里的文件名
	RawFile string `json:"rawFile,omitempty"` // 增强前的原图；空 = 没有单独的
	Rot     int    `json:"rot,omitempty"`
}

type sessionData struct {
	Version int           `json:"version"`
	Pages   []SessionPage `json:"pages"`
}

func PagePath(file string) string {
	return filepath.Join(SessionDir(), filepath.Base(file))
}

// RawPageName 是同目录里原图的名字：主文件扩展名前加 .orig。
// 放在同一个目录、用后缀区分，恢复会话时不必再多一层目录。
func RawPageName(file string) string {
	ext := filepath.Ext(file)
	return strings.TrimSuffix(file, ext) + ".orig" + ext
}

func isRawPageName(name string) bool {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return strings.EqualFold(filepath.Ext(base), ".orig")
}

// primaryPageName 是 .orig 文件对应的最终页名字。
func primaryPageName(name string) string {
	ext := filepath.Ext(name)
	return strings.TrimSuffix(strings.TrimSuffix(name, ext), ".orig") + ext
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
	exists := pageFileSet(dir)
	files := pageFiles(exists)
	if len(files) == 0 {
		_ = os.RemoveAll(dir)
		return nil, false
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
				raw := filepath.Base(strings.TrimSpace(p.RawFile))
				if raw == "" || !exists[raw] {
					raw = existingRaw(exists, name)
				}
				used[raw] = raw != ""
				pages = append(pages, SessionPage{File: name, RawFile: raw, Rot: normRot(p.Rot)})
			}
		}
	}
	for _, name := range files {
		if !used[name] {
			pages = append(pages, SessionPage{File: name, RawFile: existingRaw(exists, name)})
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

// pageFileSet 是目录里所有图片（含原图），用来校验清单和配对原图。
func pageFileSet(dir string) map[string]bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make(map[string]bool, len(entries))
	for _, e := range entries {
		if !e.IsDir() && isPageImage(e.Name()) {
			names[e.Name()] = true
		}
	}
	return names
}

// pageFiles 列出最终页；原图跟着最终页一起出现，只有最终页没写成
// （扫描中途崩溃）时才单独当一页。
func pageFiles(names map[string]bool) []string {
	out := make([]string, 0, len(names))
	for name := range names {
		if isRawPageName(name) && names[primaryPageName(name)] {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func existingRaw(exists map[string]bool, file string) string {
	raw := RawPageName(file)
	if exists[raw] {
		return raw
	}
	return ""
}

func isPageImage(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png":
		return true
	}
	return false
}

func normRot(r int) int {
	r = ((r % 360) + 360) % 360
	if r%90 != 0 {
		return 0
	}
	return r
}
