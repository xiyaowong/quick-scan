// 应用门面：cmd 只通过这里拿视图、启动和关窗判断。
package ui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Win 主窗口。后台 goroutine 通过它回主线程刷新界面；无窗口的测试里为 nil。
var Win *mygo.Window

// App 主程序实例。
type App struct{ a *app }

func New() *App { return &App{a: newApp()} }

// View mygo 每帧调用的视图函数。
func (r *App) View(c *ui.Context) { r.a.view(c) }

// Startup 先用缓存显示设备，同时后台开始发现。
func (r *App) Startup() { r.a.startup() }

// CloseAllowed 没有未导出的页面，或用户已确认退出。
func (r *App) CloseAllowed() bool {
	return r.a.quitConfirmed || len(r.a.pages) == 0
}

// AskQuit 有未导出页面时弹一次确认。
func (r *App) AskQuit() { r.a.dlg = DlgQuitConfirm }
