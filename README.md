# 快速扫描（quick-scan）

一个 Windows 上的局域网扫描工具：放纸 → 扫描 → 导出 PDF / JPG。

不用装厂商驱动，打印机和电脑在同一个 Wi-Fi / 网段即可。

![主界面](docs/screenshot.png)

## 功能

- 自动发现并记住局域网里的扫描仪
- 玻璃板单张扫描，或进纸口整叠逐张扫描
- 看大图、缩放、旋转、删除、拖动排序、勾选导出
- 纸张大小、清晰度、色彩、扫描内容按设备能力自动适配
- 导出成一个 PDF 或每页一张 JPG，文件名可用识别出的标题
- 意外退出后，下次启动可以恢复上次未导出的页面

![看大图](docs/screenshot-lightbox.png)

## 使用

1. 打开 `quick-scan.exe`
2. 在底部选择打印机，状态点变绿表示连得上
3. 把纸放好，点右侧「扫描」
4. 扫完点「导出」，选 PDF 或图片

## 崩溃恢复

扫描页放在 `%TEMP%\quick-scan-session\`。正常退出会清理；程序崩溃或被杀掉时，
下次启动会提示「发现上次未完成的扫描」，可以恢复或丢弃。

## 运行要求

Windows 10 / 11（64 位）。打印机需要支持 eSCL / AirPrint 扫描，并与电脑在同一网段。

## 构建

需要 Go 1.27+、Rust、LLVM `clang` 和 `windres`。做安装包还需要 NSIS（`scoop install nsis`）。

```powershell
just build      # 产物：quick-scan.exe
just installer  # 产物：dist\quick-scan-<版本>-setup.exe
just test       # 单测，不需要真机
just run        # 构建并启动
```

安装包按当前用户安装到 `%LOCALAPPDATA%\Programs\quick-scan`，不需要管理员权限，
卸载时会一并清掉快捷方式、注册表项和临时目录里的残留。

## 目录

- `cmd/quick-scan/` 程序入口、窗口生命周期
- `internal/ui/` 界面与交互
- `internal/scanner/` eSCL 发现、扫描、导出、OCR
- `internal/platform/` 会话目录、内嵌模型、控制台脱离
- `installer.nsi` NSIS 安装包脚本
