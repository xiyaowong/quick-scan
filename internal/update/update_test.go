package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeGitHub 起一个只会给 releases/latest 的假接口。
func fakeGitHub(t *testing.T, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	old := apiURL
	apiURL = srv.URL + "/releases/latest"
	t.Cleanup(func() { apiURL = old })
}

func withVersion(t *testing.T, v string) {
	t.Helper()
	old := Version
	Version = v
	t.Cleanup(func() { Version = old })
}

func useStateDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old := stateDir
	stateDir = func() string { return dir }
	t.Cleanup(func() { stateDir = old })
}

func TestNewer(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"0.2.0", "0.1.0", true},
		{"v0.2.0", "0.1.0", true},
		{"0.1.1", "0.1.0", true},
		{"1.0", "0.9.9", true},
		{"0.1.0", "0.1.0", false},
		{"0.1.0", "0.2.0", false},
		{"0.1.0-beta", "0.1.0", false}, // 后缀不影响比较，同版本不算新
		{"", "0.1.0", false},
		{"abc", "0.1.0", false}, // 解析不了就不更新
		{"0.1.0", "abc", false},
	}
	for _, c := range cases {
		if got := newer(c.candidate, c.current); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.candidate, c.current, got, c.want)
		}
	}
}

func TestDueWithoutRecord(t *testing.T) {
	useStateDir(t)
	if !Due(10 * time.Minute) {
		t.Fatal("没有记录时应该检查")
	}
	// 坏记录也当成到期，不能让一次写坏把更新永久挡住。
	if err := os.WriteFile(statePath(), []byte("{oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !Due(10 * time.Minute) {
		t.Fatal("记录损坏时应该照常检查")
	}
}

func TestDueAfterCheck(t *testing.T) {
	useStateDir(t)
	MarkChecked()
	if Due(10 * time.Minute) {
		t.Fatal("刚检查过还在间隔内，不该再查")
	}
	if !Due(time.Nanosecond) {
		t.Fatal("超过间隔后应该再查")
	}
}

// 时钟往回拨时不能被永久锁死。
func TestDueClockRolledBack(t *testing.T) {
	useStateDir(t)
	future := time.Now().Add(24 * time.Hour).Unix()
	body := fmt.Sprintf(`{"checkedAt":%d}`, future)
	if err := os.WriteFile(statePath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if !Due(10 * time.Minute) {
		t.Fatal("时钟被拨回去后应该照常检查")
	}
}

func TestPendingRoundTrip(t *testing.T) {
	useStateDir(t)
	if LoadPending() != nil {
		t.Fatal("没存过却读出了待装版本")
	}
	r := &Release{Version: "0.2.0", Asset: "quick-scan-0.2.0-setup.exe", URL: "https://x/a.exe", SHA256: "ab", Size: 42}
	SavePending(r)
	got := LoadPending()
	if got == nil || got.Version != r.Version || got.Asset != r.Asset || got.Size != r.Size {
		t.Fatalf("LoadPending = %+v", got)
	}
	SavePending(nil)
	if LoadPending() != nil {
		t.Fatal("清掉之后还读出了待装版本")
	}
}

// 只认带版本号的安装包：releases 页面手动下载拿到的也是同一个文件。
func TestCheckPicksVersionedInstaller(t *testing.T) {
	withVersion(t, "0.1.0")
	fakeGitHub(t, `{
		"tag_name": "v0.2.0",
		"assets": [
			{"name": "quick-scan-0.2.0-win64.zip", "browser_download_url": "https://x/z.zip", "size": 10},
			{"name": "quick-scan-0.2.0-setup.exe", "browser_download_url": "https://x/setup.exe", "size": 99,
			 "digest": "sha256:`+hex.EncodeToString(make([]byte, 32))+`"}
		]
	}`)
	r, err := Check(context.Background())
	if err != nil || r == nil {
		t.Fatalf("Check = %+v %v", r, err)
	}
	if r.Version != "0.2.0" || r.Asset != "quick-scan-0.2.0-setup.exe" || r.URL != "https://x/setup.exe" || r.Size != 99 {
		t.Fatalf("挑错了产物：%+v", r)
	}
}

// 没有摘要的旧资产也能用：至少大小对得上。
func TestCheckAllowsMissingDigest(t *testing.T) {
	withVersion(t, "0.1.0")
	fakeGitHub(t, `{"tag_name":"v0.2.0","assets":[{"name":"quick-scan-0.2.0-setup.exe","browser_download_url":"https://x/s.exe","size":5}]}`)
	r, err := Check(context.Background())
	if err != nil || r == nil {
		t.Fatalf("Check = %+v %v", r, err)
	}
	if r.SHA256 != "" {
		t.Fatalf("没有摘要却填了 %q", r.SHA256)
	}
}

func TestCheckSkipsWhenUpToDate(t *testing.T) {
	withVersion(t, "0.2.0")
	fakeGitHub(t, `{"tag_name":"v0.2.0","assets":[{"name":"quick-scan-0.2.0-setup.exe","browser_download_url":"https://x/s.exe","size":5}]}`)
	if r, err := Check(context.Background()); err != nil || r != nil {
		t.Fatalf("同版本不该更新：%+v %v", r, err)
	}
}

// 发布里只有 zip、或者标签和产物名对不上：当作没有可用更新，不猜。
func TestCheckWithoutMatchingAsset(t *testing.T) {
	cases := []string{
		`{"tag_name":"v0.2.0","assets":[]}`,
		`{"tag_name":"v0.2.0","assets":[{"name":"quick-scan-0.2.0-win64.zip","browser_download_url":"https://x/z.zip","size":5}]}`,
		`{"tag_name":"v0.2.0","assets":[{"name":"quick-scan-0.2.0-setup.exe","browser_download_url":"http://x/s.exe","size":5}]}`,
	}
	for _, body := range cases {
		withVersion(t, "0.1.0")
		fakeGitHub(t, body)
		if r, err := Check(context.Background()); err != nil || r != nil {
			t.Errorf("%s → %+v %v", body, r, err)
		}
	}
}

func TestCheckBadResponses(t *testing.T) {
	withVersion(t, "0.1.0")
	for _, body := range []string{`{oops`, ``} {
		fakeGitHub(t, body)
		if _, err := Check(context.Background()); err == nil {
			t.Errorf("%q → 应该报错", body)
		}
	}
	// 连不上也只返回错误，调用方丢掉即可。
	fakeGitHub(t, ``)
	apiURL = "http://127.0.0.1:1/latest"
	if _, err := Check(context.Background()); err == nil {
		t.Fatal("连不上应该报错")
	}
}

func TestDownloadAndReady(t *testing.T) {
	old := downloadAllowed
	downloadAllowed = func(string) bool { return true } // httptest 只给 http
	t.Cleanup(func() { downloadAllowed = old })

	payload := []byte("installer-bytes")
	sum := sha256.Sum256(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	r := &Release{Version: "0.2.0", URL: srv.URL, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(payload))}
	dst := filepath.Join(t.TempDir(), "setup.exe")
	if err := r.Download(context.Background(), dst); err != nil {
		t.Fatalf("下载失败：%v", err)
	}
	if !r.Ready(dst) {
		t.Fatal("刚下载的文件应该校验通过")
	}
	// 改一个字节：不完整或过期都不能装。
	if err := os.WriteFile(dst, []byte("tampered-------"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r.Ready(dst) {
		t.Fatal("内容被改过还说就绪")
	}
}

// 校验失败时不能留下半份文件给下次启动误当成完整包。
func TestDownloadRejectsBadHash(t *testing.T) {
	old := downloadAllowed
	downloadAllowed = func(string) bool { return true }
	t.Cleanup(func() { downloadAllowed = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("bad"))
	}))
	defer srv.Close()

	r := &Release{Version: "0.2.0", URL: srv.URL, SHA256: hex.EncodeToString(make([]byte, 32)), Size: 3}
	dst := filepath.Join(t.TempDir(), "setup.exe")
	if err := r.Download(context.Background(), dst); err == nil {
		t.Fatal("哈希不对应该失败")
	}
	for _, p := range []string{dst, dst + ".part"} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%s 没清掉", p)
		}
	}
}
