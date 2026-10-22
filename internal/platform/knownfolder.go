// Windows 已知目录：首次导出前没有任何记录，默认落到「文档」
// 而不是系统对话框自己的深层默认位置，新手存完才知道去哪找。
package platform

import (
	"syscall"
	"unsafe"
)

var (
	shell32                  = syscall.NewLazyDLL("shell32.dll")
	procSHGetKnownFolderPath = shell32.NewProc("SHGetKnownFolderPath")
	procCoTaskMemFree        = syscall.NewLazyDLL("ole32.dll").NewProc("CoTaskMemFree")
)

// FOLDERID_Documents {FDD39AD0-238F-46AF-ADB4-6C85480369C7}
var folderIDDocuments = syscall.GUID{
	Data1: 0xFDD39AD0,
	Data2: 0x238F,
	Data3: 0x46AF,
	Data4: [8]byte{0xAD, 0xB4, 0x6C, 0x85, 0x48, 0x03, 0x69, 0xC7},
}

// DefaultExportDir 返回「文档」目录，拿不到（目录被重定向到不可达的
// 网络盘等）就返回空串，调用方回落到系统默认位置。
func DefaultExportDir() string {
	// shell32/ole32 理论上必在，但和 console.go 一样先 Find 兜底：
	// 这条路跑在导出 goroutine 里，panic 会带崩整个应用。
	if procSHGetKnownFolderPath.Find() != nil || procCoTaskMemFree.Find() != nil {
		return ""
	}
	var ptr *uint16
	r1, _, _ := procSHGetKnownFolderPath.Call(
		uintptr(unsafe.Pointer(&folderIDDocuments)),
		0, // dwFlags
		0, // hToken（当前用户）
		uintptr(unsafe.Pointer(&ptr)),
	)
	if r1 != 0 || ptr == nil {
		return ""
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(ptr)))
	// Windows 的 syscall 没有现成的 PWSTR→string（x/sys/windows 才有，
	// 不想为此引依赖），自己数到 NUL。
	return syscall.UTF16ToString(unsafe.Slice(ptr, wcslen(ptr)))
}

func wcslen(p *uint16) int {
	n := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(p), n*2)) != 0 {
		n++
	}
	return n
}
