// quick-scan：Windows 扫描工具。mygo UI + eSCL/mDNS，OCR 模型内嵌。
package main

import (
	"fmt"
	"os"

	"sscan/internal/platform"
	"sscan/internal/scanner"
	appui "sscan/internal/ui"

	"github.com/egoist/mygo"
	myui "github.com/egoist/mygo/ui"
)

func main() {
	// 双击启动时摘掉控制台；从终端启动时保留，报错能看见。
	platform.DetachIfStandalone()

	// --ocr-test <图片>：不开窗口，跑一遍 OCR 文件名候选。
	if len(os.Args) >= 3 && os.Args[1] == "--ocr-test" {
		scanner.RunOcrTest(os.Args[2])
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "启动失败:", err)
		os.Exit(1)
	}
}

func run() (err error) {
	// panic 也转成 error，从终端启动时能看到原因。
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	platform.CleanupStale()
	if err := platform.EnsureAssets(); err != nil {
		return err
	}

	app := appui.New()
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title:          "快速扫描",
			Width:          1180,
			Height:         820,
			UseContentSize: true,
			MinWidth:       920,
			MinHeight:      660,
			// 标题栏由视图自己画：窗口按钮躲进视图后，右上角才能和界面同一个风格。
			TitleBarStyle:     mygo.TitleBarHidden,
			HideWindowButtons: true,
			TitleBarHeight:    40,
			Content:           myui.View(app.View),
		})
		appui.Win = win
		// 有未导出页面时先确认再关。
		win.OnClose(func(e *mygo.CloseEvent) {
			if app.CloseAllowed() {
				return
			}
			e.PreventDefault()
			app.AskQuit()
			win.Update(func() {})
		})
		app.Startup()
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	platform.Cleanup()
	return nil
}
