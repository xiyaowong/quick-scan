# SimpleMFPScan

轻量级 HP MFP 扫描工具。放纸 → 点扫描 → 导出 PDF/JPG。

Windows 10/11 64 位。扫描走局域网 **eSCL**（Mopria/AirPrint 扫描协议），
不需要安装厂商 TWAIN/WIA 驱动；设备需与电脑在同一网段。

## 功能

- 启动先显示上次的设备列表（`%TEMP%` 缓存）并立刻验证，同时后台 mDNS 全量刷新；选择器显示 `设备名 (IPv4)`
- 一次扫描追加一批页面；进纸口模式自动逐张取回
- 缩略图列表：拖动排序、双击放大（滚轮缩放、←→ 翻页、Esc/点遮罩关闭）、旋转、删除、单页导出
- 纸张大小 / 清晰度（列出设备支持的全部 dpi）/ 用途 / 色彩 / 放纸方式，按设备上报的能力自动禁用不可选项
- 导出合并 PDF 或每页一张 JPG（页粒度进度条）
- 扫描后自动 OCR 首页标题，给导出文件名提供候选（PP-OCRv6，本地识别，无联网）
- 不写注册表、不建配置目录：页面与释放的模型退出即清；设备缓存只放 `%TEMP%\SimpleMFPScan\`，交给系统临时目录清理

界面与交互以 `docs/设计定稿.md` 为准，布局稿见 `docs/ui-mockup.html`；
eSCL 实现细节见 `docs/技术笔记-eSCL扫描.md`。

## 架构

```
SimpleMFPScan.exe (64 位)
    ├── mygo 原生 UI                ← state.go / view.go / dialog.go
    ├── 设备发现（mDNS/DNS-SD）     ← discovery.go（miekg/dns）
    ├── eSCL 客户端（自实现 HTTP）  ← escl.go
    └── OCR（rusto-rs 静态链入）    ← ocr.go / ocrbridge/
```

单进程、单文件分发。扫描协议只有四个 HTTP 动作（能力、建任务、取图、删任务），
没有子进程，也没有运行时依赖。

## 构建

需要 Go 1.27+、Rust（`x86_64-pc-windows-msvc` target，编 OCR 静态库）、
LLVM 的 `clang`（cgo 链接）、binutils 的 `windres`（编 exe 图标资源）。

```powershell
just build        # 图标资源 → Rust OCR 静态库 → 64 位主程序
just package      # 免安装 zip（先 build）
just run          # 启动（启动即发现设备，会真的碰打印机）
just check        # go vet（stub 路径，不需要 Rust）
just test         # 单测（eSCL 用 httptest 假设备，不碰真机）
just clean
```

产物是仓库根目录下的 `SimpleMFPScan.exe`（约 20 MB）：OCR 模型内嵌其中，
启动时释放到 `%TEMP%\SimpleMFPScan_<pid>\`，退出即清。

## 开发

程序一启动就会用 mDNS 发现设备，点扫描就会真的连打印机，跑之前确认设备状态。
`just test` 覆盖 eSCL 能力解析、任务流程（玻璃板单张 / 进纸口多张）与显示格式，
不需要真机。
