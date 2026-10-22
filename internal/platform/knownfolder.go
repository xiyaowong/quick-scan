// Windows 已知目录：首次导出前没有任何记录，默认落到「文档」
// 而不是系统对话框自己的深层默认位置，新手存完才知道去哪找。
package platform

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	procSHGetKnownPath = shell32.NewProc("SHGetKnownFolderPath")
	coTaskMemFree      = syscall.NewLazyDLL("ole32.dll").NewProc("CoTaskMemFree")
)

// FOLDERID_Documents {FDD39AD0-238F-46AF-ADB4-6C85480369C7}
var folderIDDocuments = syscall.GUID{
	Data1: 0xFDD39AD0,
	Data2: 0x238F,
	Data3: 0x46AF,
	Data4: [8]byte{0xAD, 0xB4, 0x6C, 0x85, 0x48, 0x03, 0x69, 0xC7},
}

// DefaultExportDir 返回「文档」目录，拿不到（非 Windows、目录被重定向
// 到不可达的网络盘等）就返回空串，调用方回落到系统默认位置。
func DefaultExportDir() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	var ptr *uint16
	r1, _, _ := procSHGetKnownPath.Call(
		uintptr(unsafe.Pointer(&folderIDDocuments)),
		0, // dwFlags
		0, // hToken（当前用户）
		uintptr(unsafe.Pointer(&ptr)),
	)
	if r1 != 0 || ptr == nil {
		return ""
	}
	defer coTaskMemFree.Call(uintptr(unsafe.Pointer(ptr)))
	return syscall.UTF16ToString(unsafe.Slice(ptr, wcslen(ptr)))
}

// wcslen 数以 NUL 结尾的 UTF-16 字符串长度。
func wcslen(p *uint16) int {
	n := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(p), n*2)) != 0 {
		n++
	}
	return n
}
