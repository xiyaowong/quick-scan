// 自动更新：启动时查一次、静默下载，下次启动时静默安装。
// 用户不该为更新做任何事，所以这里不弹窗、不改提示、失败就当没发生。
package ui

import (
	"context"
	"os"
	"time"

	"quick-scan/internal/update"
)

// GitHub 对未认证请求有频率限制：两次检查至少隔这么久。
const updateCheckGap = 10 * time.Minute

// 本地开发时关掉：仓库里直接跑的程序不该被线上版本换掉。
func updatesDisabled() bool { return os.Getenv("QUICK_SCAN_NO_UPDATE") != "" }

func (a *app) startUpdateCheck() {
	if Win == nil {
		return
	}
	// 上次让位留下的旧 exe 现在不占用了，顺手清掉。
	update.CleanLeftovers()
	if updatesDisabled() || update.Version == "" || !update.Installable() {
		return
	}
	go func() {
		// 已经下好的先装上：这时窗口刚建好，用户还没来得及用。
		applyPendingUpdate()
		if !update.Due(updateCheckGap) {
			return
		}
		rel, err := update.Check(context.Background())
		if err != nil {
			return // 断网、限流、接口报错：都当没这回事
		}
		update.MarkChecked()
		if rel == nil {
			update.SavePending(nil)
			return
		}
		path, err := update.File(rel.Version)
		if err != nil {
			return
		}
		if !rel.Ready(path) {
			if err := rel.Download(context.Background(), path); err != nil {
				return
			}
		}
		update.RemoveStale(path)
		update.SavePending(rel)
	}()
}

// applyPendingUpdate 静默装好已下载的新版本，装回用户当初选的那个目录。
// 先把自己改名让开位置，安装包再写到原处：本进程继续用旧映像跑完这次，
// 下次启动就是新版，用户不用等也不用重启。任何一步失败都还原，程序照常能用。
func applyPendingUpdate() {
	r := update.LoadPending()
	if r == nil {
		return
	}
	path, err := update.File(r.Version)
	if err != nil || !r.Ready(path) {
		update.SavePending(nil)
		return
	}
	exe, old, err := update.StepAside()
	if err != nil {
		return // 让不开位置：留到下次，不打扰用户
	}
	if err := r.Run(path, update.InstallDir()); err != nil {
		update.Restore(exe, old)
		return
	}
	update.SavePending(nil)
	update.RemoveStale("")
}
