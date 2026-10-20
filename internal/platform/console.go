// 控制台脱钩：双击启动时摘掉临时控制台，从终端启动时保留。
//
// -H=windowsgui 在 cgo 外部链接下会被丢弃，/SUBSYSTEM:WINDOWS 又会让终端里看不到输出。
// 运行期判断把取舍还回来：控制台上只有自己 = 双击启动，脱钩；还有别人 = 终端，保留。
package platform

import (
	"syscall"
	"unsafe"
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procFreeConsole           = kernel32.NewProc("FreeConsole")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
)

// DetachIfStandalone 只在"控制台是为本进程临时新建的"时候脱钩。
// 必须在任何耗时工作之前调用：晚一步用户就会看见控制台闪一下。
func DetachIfStandalone() {
	if procGetConsoleProcessList.Find() != nil || procFreeConsole.Find() != nil {
		return
	}
	var buf [8]uint32
	// 返回值 = 挂在这个控制台上的进程数；>len(buf) 表示只写回了一部分，
	// 那种情况一定是"有很多进程"（终端），不必再问
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n > 1 {
		return // 有父终端：留着控制台，stderr 才有地方去
	}
	_, _, _ = procFreeConsole.Call()
}
