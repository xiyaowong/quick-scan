// SimpleMFPScan —— Windows 桌面扫描工具。
// 对照 docs/ui-mockup.html（布局）与 docs/设计定稿.md（交互语义）。
//
// 单进程：mygo 原生 UI + eSCL 客户端（escl.go）+ mDNS 发现（discovery.go）。
// 扫描全程走局域网 HTTP，不需要厂商驱动、不需要子进程。
// OCR 模型内嵌在 exe 里，启动时释放到 %TEMP%\SimpleMFPScan_<pid>\（退出即清）。
package main

import (
	"fmt"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	// 先脱钩再干别的：晚一步用户就会看见控制台闪一下。
	// 从终端启动时控制台留着，报错能看见；双击时摘掉，不留终端窗口。
	detachIfStandalone()

	// 隐藏 CLI：--ocr-test <图片> 走一遍 OCR 文件名候选管线（无窗口、不碰打印机），
	// 给验收/调试用。正常启动不传参，走 GUI。
	if len(os.Args) >= 3 && os.Args[1] == "--ocr-test" {
		runOcrTest(os.Args[2])
		return
	}
	if err := run(); err != nil {
		// 从终端跑时这行就看在终端里；双击启动（无控制台）时它无处可去，
		// 但进程会以非零码退出——真正的"闪退"要靠带终端启动来回溯。
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	// run 里的 panic 也兜住转成 error：至少走上面那条 stderr 的路。
	// 进程直接死掉（coredump 一类）的情况这里兜不住，也兜不住任何写盘动作——
	// 所以排查一律从终端启动，看现场输出。
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	cleanupStaleTemp()
	// 内嵌 OCR 模型先落临时目录——扫描后识别首页标题要用
	if err := ensureAssets(); err != nil {
		return err
	}

	app := newApp()
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:   "惠普MFP打印机扫描工具",
			Width:   1180,
			Height:  820,
			Content: ui.View(app.view),
		})
		// 关窗拦截：有未导出页面时先弹确认（定稿：退出前确认）
		win.OnClose(func(e *mygo.CloseEvent) {
			if app.quitConfirmed || len(app.pages) == 0 {
				return // 放行（quitConfirmed 由确认对话框置位）
			}
			e.PreventDefault()
			app.dlg = DlgQuitConfirm
			win.Update(func() {})
		})
		// 启动：先用 %TEMP% 缓存立刻显示上次的设备，同时后台 mDNS 全量刷新
		app.startup()
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	cleanupTemp() // 窗口全关后清理 %TEMP% 里的扫描页
	return nil
}

// win 是主窗口句柄，后台 goroutine 通过 win.Update 回主线程刷新界面。
var win *mygo.Window
