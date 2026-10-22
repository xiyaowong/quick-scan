package update

import "testing"

// NSIS 的 /D= 必须最后、且不能带引号：路径里有空格时 Go 会自动加引号，
// 那就解析不了（实测会静默装回默认目录）。这里锁住拼法。
func TestInstallerCmdLine(t *testing.T) {
	cases := []struct {
		setup, dir, want string
	}{
		{`C:\a\setup.exe`, `C:\Users\x\AppData\Local\Programs\quick-scan`, `C:\a\setup.exe /S /D=C:\Users\x\AppData\Local\Programs\quick-scan`},
		{`C:\My Apps\setup.exe`, `C:\My Apps\Custom`, `"C:\My Apps\setup.exe" /S /D=C:\My Apps\Custom`},
		{`C:\a\setup.exe`, ``, `C:\a\setup.exe /S`},
		{`C:\a\setup.exe`, `C:\dir\`, `C:\a\setup.exe /S /D=C:\dir`},
	}
	for _, c := range cases {
		if got := installerCmdLine(c.setup, c.dir); got != c.want {
			t.Errorf("installerCmdLine(%q, %q) = %q, want %q", c.setup, c.dir, got, c.want)
		}
	}
}
