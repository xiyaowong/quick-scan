// 控制台脱钩：双击启动时把本进程从临时开的控制台上摘下来，从终端启动时留着。
//
// 为什么不用链接器标志：`-H=windowsgui` 在 cgo 外部链接下会被静默丢弃
// （见 justfile 的 ui 配方），而 `-Wl,/SUBSYSTEM:WINDOWS` 虽然能改对子系统，
// 却把"在终端里跑能看到输出"变成不可能——取舍被写死在构建期。
//
// 运行期判断把取舍还回来：进程是控制台程序，启动瞬间看一眼控制台上挂了几个
// 进程——只有自己，说明这个控制台是双击时系统新建的，脱钩；还有别人（cmd、
// pwsh），说明是从终端启动的，留着，报错就能被看见。
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
