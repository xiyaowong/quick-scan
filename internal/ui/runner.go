// 应用门面：cmd 只通过这里拿视图、启动和关窗判断。
package ui

import (
	"quick-scan/internal/platform"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Win 主窗口。后台 goroutine 通过它回主线程刷新界面；无窗口的测试里为 nil。
var Win *mygo.Window

type App struct{ a *app }

// 上次异常退出留下的会话在这里转成待恢复状态。
func New() *App {
	a := newApp()
	a.lastExportDir = platform.LoadExportDir()
	if s := platform.LoadScanPrefs(); s != nil {
		a.restoreScanPrefs(s)
	}
	if pages, ok := platform.LoadSession(); ok {
		a.recoverPages = pages
		a.dlg = DlgRecover
	}
	return &App{a: a}
}

// View mygo 每帧调用的视图函数。
func (r *App) View(c *ui.Context) { r.a.view(c) }

// Startup 先用缓存显示设备，同时后台开始发现。
func (r *App) Startup() { r.a.startup() }

// CloseAllowed 没有未导出的页面，或用户已确认退出。
// 走到这里窗口就要关了：把用户这次的扫描设置记下来，下次打开不用重选。
func (r *App) CloseAllowed() bool {
	if r.a.quitConfirmed || r.a.pageCount() == 0 {
		r.a.saveScanPrefs()
		return true
	}
	return false
}

// AskQuit 有未导出页面时弹一次确认；待恢复的页面先让用户决定去留。
func (r *App) AskQuit() {
	if len(r.a.recoverPages) > 0 {
		r.a.dlg = DlgRecover
		return
	}
	r.a.dlg = DlgQuitConfirm
}
