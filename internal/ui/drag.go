package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"quick-scan/internal/platform"
	"quick-scan/internal/scanner"

	"github.com/egoist/mygo/transfer"
)

// 缩略图是系统拖拽源：按住它拖到资源管理器、桌面或浏览器，放下就是一张真实的
// 图片文件。别的程序在放下那一刻读的是磁盘上的文件，所以这里先落盘再交出去。
//
// 卡片本身不能同时当拖拽源——一个元素只认一个拖拽源，接了系统拖拽，页面内的
// 拖动排序就再也收不到事件。所以缩略图负责拖出去，卡片其余部分留给排序。

// dragDir 放交出去的图片副本；测试换成临时目录。
var dragDir = platform.DragDir

// pageDrag 是拖拽时带上的进程内值：落回网格就按落点那一页排序。
type pageDrag struct{ id uint64 }

// dragFile 把第 i 页写成交得出去的文件，返回它的路径。
func (a *app) dragFile(i int) (string, error) {
	if i < 0 || i >= len(a.pages) {
		return "", errors.New("这一页已经不在了")
	}
	p := &a.pages[i]
	src, err := a.dragSource(p)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dragDir(), dragFileName(i+1, time.Now(), filepath.Ext(src)))
	if err := os.MkdirAll(dragDir(), 0o755); err != nil {
		return "", err
	}
	return dst, copyFile(src, dst)
}

// dragSource 返回方向已经烘进像素的文件：没转过就是会话里的原图。
func (a *app) dragSource(p *Page) (string, error) {
	rot := p.Rot % 360
	if rot == 0 {
		return p.FullPath, nil
	}
	cache := dragCachePath(p.ID, rot)
	if _, err := os.Stat(cache); err == nil {
		return cache, nil
	}
	// 还没烘过（刚转完就拖）：当场补一份，之后都走这份缓存。
	if err := scanner.WritePageJPEG(scanner.ExportPage{Path: p.FullPath, Rot: rot}, cache); err != nil {
		return "", err
	}
	return cache, nil
}

// warmDragCopy 在后台把旋转烘进缓存：转过再拖时不用当场等编码那几百毫秒。
func (a *app) warmDragCopy(id uint64) {
	if Win == nil {
		return // 单元测试里没有窗口，也没有等着的用户
	}
	i := a.pageIndex(id)
	if i < 0 {
		return
	}
	p := &a.pages[i]
	rot := p.Rot % 360
	if rot == 0 {
		return // 原图本身就能直接交出去
	}
	cache := dragCachePath(id, rot)
	if _, err := os.Stat(cache); err == nil {
		return
	}
	path := p.FullPath
	go func() { _ = scanner.WritePageJPEG(scanner.ExportPage{Path: path, Rot: rot}, cache) }()
}

// dragCachePath 是转过方向的页面的拖拽副本：同一页每个角度各一份。
// 名字带下划线开头的一段，跟交出去的那份分开，用户看不到。
func dragCachePath(id uint64, rot int) string {
	return filepath.Join(dragDir(), ".cache", fmt.Sprintf("%d_%d.jpg", id, rot))
}

// dragFileName 跟导出的默认名一个路子，后面缀上页号：拖到桌面上也认得出是第几页。
func dragFileName(page int, at time.Time, ext string) string {
	return fmt.Sprintf("扫描_%s_%d%s", at.Format("20060102_1504"), page, ext)
}

// pageDragData 在拖拽开始时准备要交出去的内容。出错时记下来，由 dragDone 报给用户：
// 这里返回不了错误，空数据只会让框架报一句看不懂的话。
func (a *app) pageDragData(i int) transfer.Data {
	path, err := a.dragFile(i)
	if err != nil {
		a.dragErr = err
		return transfer.Data{}
	}
	data, err := transfer.FileData(path)
	if err != nil {
		a.dragErr = err
		return transfer.Data{}
	}
	return data
}

// dragDone 收尾：拖失败要说一声，并指条路；用户自己松手取消不算失败。
func (a *app) dragDone(r transfer.Result) {
	err := a.dragErr
	a.dragErr = nil
	if err == nil && r.Err != nil {
		err = r.Err
	}
	if err != nil {
		a.hint = "这一页拖不出去，可以点卡片上的「导出此页」。"
		a.hintDetail = err.Error()
	}
}

// dropOnPage 把拖来的页放到第 to 页前面。落回原处不算移动。
func (a *app) dropOnPage(id uint64, to int) {
	from := a.pageIndex(id)
	if from < 0 || from == to || from+1 == to {
		return
	}
	a.movePages([]int{from}, to)
}

func (a *app) pageIndex(id uint64) int {
	for i := range a.pages {
		if a.pages[i].ID == id {
			return i
		}
	}
	return -1
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
