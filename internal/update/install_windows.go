package update

import (
	"syscall"
	"unsafe"
)

// 安装目录从卸载项里读：安装程序登记为 InstallLocation，
// 一起登记了 QuietUninstallString 才说明这份安装能静默升级。
// 读不到就是免安装版或仓库里直接跑的，不参与自动更新。
const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\quick-scan`

var (
	advapi32             = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegCloseKey      = advapi32.NewProc("RegCloseKey")
)

const (
	keyRead = 0x20019 // KEY_READ
	regSZ   = 1
)

func installedDir() string {
	if procRegOpenKeyExW.Find() != nil || procRegQueryValueExW.Find() != nil || procRegCloseKey.Find() != nil {
		return ""
	}
	sub, err := syscall.UTF16PtrFromString(uninstallKey)
	if err != nil {
		return ""
	}
	var h syscall.Handle
	if r, _, _ := procRegOpenKeyExW.Call(
		uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(sub)),
		0, keyRead, uintptr(unsafe.Pointer(&h)),
	); r != 0 {
		return ""
	}
	defer procRegCloseKey.Call(uintptr(h))

	// 没有静默卸载命令的安装，装它的时候就做不到静默，升级同理。
	if regString(h, "QuietUninstallString") == "" {
		return ""
	}
	return regString(h, "InstallLocation")
}

func regString(h syscall.Handle, name string) string {
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return ""
	}
	var buf [syscall.MAX_PATH]uint16
	size := uint32(len(buf) * 2)
	var typ uint32
	if r, _, _ := procRegQueryValueExW.Call(
		uintptr(h), uintptr(unsafe.Pointer(n)), 0,
		uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)),
	); r != 0 || typ != regSZ {
		return ""
	}
	return syscall.UTF16ToString(buf[:])
}
