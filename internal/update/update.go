// Package update 检查、下载并静默安装新版本。
//
// 一切静默：检查失败、清单坏了、下载中断都只返回错误，调用方丢掉即可。
// 宁可这次没更新成功，也不能让用户看见报错。
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"quick-scan/internal/platform"
)

// Version 构建时注入（justfile 从 app.rc 读出）。空串 = 不知道自己的版本，不检查。
var Version string

const (
	repo            = "xiyaowong/simple-scan"
	assetSuffix     = "-setup.exe"
	checkTimeout    = 10 * time.Second
	downloadTimeout = 10 * time.Minute
	installTimeout  = 2 * time.Minute
)

// 测试里替换成假服务。
var (
	apiURL          = "https://api.github.com/repos/" + repo + "/releases/latest"
	cacheDir        = defaultCacheDir
	runSetup        = runInstaller
	downloadAllowed = httpsURL
)

// Release 是比当前版本新的一次发布。
type Release struct {
	Version string
	Asset   string
	URL     string
	SHA256  string // 可能为空：GitHub 没算摘要的旧资产
	Size    int64
}

type apiRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"` // "sha256:..."
	} `json:"assets"`
}

// Check 查询 GitHub 上最新发布；没有比当前新的版本返回 nil。
func Check(ctx context.Context) (*Release, error) {
	if Version == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "quick-scan/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("更新接口返回 HTTP %d", resp.StatusCode)
	}
	var rel apiRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return nil, err
	}
	ver := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	if !newer(ver, Version) {
		return nil, nil
	}
	name := "quick-scan-" + ver + assetSuffix
	for _, a := range rel.Assets {
		if a.Name != name || a.Size <= 0 || !httpsURL(a.URL) {
			continue
		}
		sum := strings.TrimPrefix(a.Digest, "sha256:")
		if len(sum) != sha256.Size*2 {
			sum = "" // 没有摘要就只能校验大小，校验在 Download 里做
		}
		return &Release{Version: ver, Asset: name, URL: a.URL, SHA256: strings.ToLower(sum), Size: a.Size}, nil
	}
	return nil, nil
}

// Download 下载安装包到本地缓存并校验。先写 .part 再改名：
// 中断只留下 .part，不会被误当成完整文件。
func (r *Release) Download(ctx context.Context, dst string) error {
	if !downloadAllowed(r.URL) {
		return errors.New("更新地址不安全")
	}
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "quick-scan/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载返回 HTTP %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	part := dst + ".part"
	f, err := os.Create(part)
	if err != nil {
		return err
	}
	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, sum), io.LimitReader(resp.Body, r.Size+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && (n != r.Size || !r.hashOK(sum.Sum(nil))) {
		err = errors.New("更新包校验失败")
	}
	if err != nil {
		_ = os.Remove(part)
		return err
	}
	if err := os.Rename(part, dst); err != nil {
		_ = os.Remove(part)
		return err
	}
	return nil
}

// Ready 本地文件完整（大小、摘要都对）。
func (r *Release) Ready(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() != r.Size {
		return false
	}
	if r.SHA256 == "" {
		return true
	}
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return false
	}
	return r.hashOK(sum.Sum(nil))
}

func (r *Release) hashOK(sum []byte) bool {
	return r.SHA256 == "" || hex.EncodeToString(sum) == r.SHA256
}

// File 是某个版本安装包在本机的位置。
func File(version string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "update", "quick-scan-"+version+assetSuffix), nil
}

// Installable 当前这个副本能不能自动更新：必须是安装版（卸载项登记的目录里
// 运行的程序），并且安装目录可写。免安装版和仓库里直接跑的都跳过。
func Installable() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	// 正在被让位的那份临时 exe 不是安装位置，装完再启动就不该再动它。
	if strings.Contains(filepath.Base(exe), ".old-") {
		return false
	}
	dir := installedDir()
	if dir == "" || !strings.EqualFold(filepath.Clean(dir), filepath.Clean(filepath.Dir(exe))) {
		return false
	}
	f, err := os.CreateTemp(filepath.Dir(exe), ".quick-scan-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// InstallDir 实际安装目录：用户当初可能改过，安装包登记在卸载项里，
// 升级必须装回这里。
func InstallDir() string { return installedDir() }

// Run 静默安装到 dir：不带任何界面，装完即止。
// 调用前必须先把正在运行的 exe 让开（Windows 不让覆盖使用中的文件）。
func (r *Release) Run(setup, dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
	defer cancel()
	return runSetup(ctx, setup, dir)
}

// StepAside 把正在运行的 exe 改名让出位置：Windows 不让覆盖使用中的文件。
// 返回原名和让位后的名字，装不上时用 Restore 改回来。
func StepAside() (exe, old string, err error) {
	exe, err = os.Executable()
	if err != nil {
		return "", "", err
	}
	old = exe + fmt.Sprintf(".old-%d", os.Getpid())
	if err := os.Rename(exe, old); err != nil {
		return "", "", err
	}
	return exe, old, nil
}

// Restore 安装没成功时把程序改回原来的名字，并删掉装了一半的文件。
func Restore(exe, old string) {
	_ = os.Remove(exe)
	_ = os.Rename(old, exe)
}

// CleanLeftovers 清掉上次让位留下的旧 exe 和没下完的 .part。
func CleanLeftovers() {
	if dir, err := Dir(); err == nil {
		gone, _ := filepath.Glob(filepath.Join(dir, "*.old-*"))
		for _, p := range gone {
			_ = os.Remove(p)
		}
	}
	if dir, err := cacheDir(); err == nil {
		parts, _ := filepath.Glob(filepath.Join(dir, "update", "*.part"))
		for _, p := range parts {
			_ = os.Remove(p)
		}
	}
}

// Dir 程序所在目录。
func Dir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// RemoveStale 删掉除 keep 以外的安装包。
func RemoveStale(keep string) {
	dir, err := cacheDir()
	if err != nil {
		return
	}
	files, _ := filepath.Glob(filepath.Join(dir, "update", "quick-scan-*"+assetSuffix))
	for _, p := range files {
		if !strings.EqualFold(p, keep) {
			_ = os.Remove(p)
		}
	}
}

func defaultCacheDir() (string, error) {
	return platform.CacheDir(), nil
}

// httpsURL 只走 HTTPS：更新包必须经得起中间人。
func httpsURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != ""
}

// newer 版本号解析不出来时一律当"不更新"：宁可漏一次，也不能误判。
func newer(candidate, current string) bool {
	a, ok := parseVersion(candidate)
	if !ok {
		return false
	}
	b, ok := parseVersion(current)
	if !ok {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}

// parseVersion 只认点分数字，最多三段；v 前缀和 -beta 之类的后缀忽略。
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return v, false
	}
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}
