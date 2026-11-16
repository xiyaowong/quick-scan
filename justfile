# quick-scan 构建管理（Windows 64 位，Go + mygo 原生 UI）
#
# 单一产物 quick-scan.exe：mygo UI + eSCL 客户端 + 内嵌 OCR 模型。
# 扫描走局域网 eSCL（HTTP），不需要厂商 TWAIN/WIA 驱动，也没有子进程。
#
# 打包、签名、更新清单都交给官方 `go tool mygo build`；版本号只写在 mygo.json。
#
# ── 写配方前必读的两条 just/pwsh 坑 ──────────────────────────────
# 1. just 把**每一行**配方单独交给一次 `pwsh -Command`，变量/环境不跨行保留。
#    所以一个配方里要按顺序干几件事时，必须写成**一整行**，用行尾 `\` 续行、
#    行内用 `;` 分隔。写成多行的话后面几行看不到前面设的变量。
# 2. `&&` 后面接赋值（`&& $env:X='1'`）会让 pwsh 解析失败（Unexpected token）。
#    用 `;` 代替 `&&`。
set windows-shell := ["pwsh", "-NoProfile", "-Command"]

@default:
    @just --list

# ── 1. 编 Rust OCR 桥（ocrbridge staticlib，仅 64 位）──
# 清代理：本机 http_proxy 会弄坏 cargo 的 TLS 握手（schannel 报错）
ocr:
    cd ocrbridge; \
    $env:http_proxy = ''; \
    $env:https_proxy = ''; \
    $env:proxy = ''; \
    cargo build --release --target x86_64-pc-windows-msvc; \
    if ($LASTEXITCODE -ne 0) { throw 'ocrbridge 编译失败' }; \
    Write-Host '[ocr] ocrbridge.lib 完成'

# ── 2. 构建并打包（dist\windows-amd64\）──
# mygo build 负责：exe 图标/版本资源（来自 mygo.json + resources\icon.png）、
#   NSIS 安装包、免安装 tar.gz、install.ps1、更新清单（有签名密钥时）。
# 子系统保持 console（不用 -H=windowsgui）：cgo 外部链接下 mygo build 传的
#   -H=windowsgui 会被静默丢弃；控制台改在运行期脱钩（console.go），双击启动时
#   FreeConsole 摘掉，从终端启动时留着。
# CGO_LDFLAGS 只抬 PE 头里的 SubsystemVersion（clang/lld 默认写 6.0，Go 内部链接器写
#   10.0），不切换子系统类型。6.0 会被 Windows 当成 Win7 程序，拒绝创建 WS_EX_LAYERED
#   子窗口；mygo 自定义标题栏的按钮/顶边正是这种子窗口，于是启动报
#   "cannot create the window controls of a window with a hidden title bar"，贴边缩放和
#   贴靠布局也一起失效。Windows 8（6.2）起才支持，这里取 10.0 与内部链接器对齐。
# -fuse-ld=lld：不让 clang 落到 MSVC 的 link.exe 上（它读不了 Go 给的 .ld
#   响应文件，报 LNK1107）；必须放在 CC 里，Go 用 `clang -Wl,--version` 探测
#   是不是 LLD，探测不到就会继续传那个 .ld 脚本（实测 CI 上如此）。
build: ocr
    $env:CGO_ENABLED = '1'; \
    $env:GOARCH = 'amd64'; \
    $env:CGO_LDFLAGS = '-Xlinker /subsystem:console,10.0'; \
    if (-not $env:CC) { $env:CC = if (Get-Command clang -ErrorAction SilentlyContinue) { (Get-Command clang).Source } else { 'D:\Scoop\apps\llvm\current\bin\clang.exe' } }; \
    if ($env:CC -match '\s' -and $env:CC -notmatch '^"') { $env:CC = '"' + $env:CC + '"' }; \
    if ($env:CC -notmatch 'fuse-ld') { $env:CC = $env:CC + ' -fuse-ld=lld' }; \
    $ver = (Get-Content mygo.json -Raw | ConvertFrom-Json).version; \
    go tool mygo build -platform windows/amd64; \
    if ($LASTEXITCODE -ne 0) { throw '构建失败' }; \
    Write-Host "[build] quick-scan $ver 完成（dist\windows-amd64）"

# ── 3. 发版构建：同时签名并上传到 GitHub 草稿 release ──
# 需要环境变量 MYGO_UPDATER_PRIVATE_KEY（签名密钥）和 GH_TOKEN（上传）。
release: ocr
    if (-not $env:MYGO_UPDATER_PRIVATE_KEY) { throw '缺少 MYGO_UPDATER_PRIVATE_KEY，更新包无法签名' }; \
    if (-not $env:GH_TOKEN) { throw '缺少 GH_TOKEN，无法上传 release' }; \
    $env:CGO_ENABLED = '1'; \
    $env:GOARCH = 'amd64'; \
    $env:CGO_LDFLAGS = '-Xlinker /subsystem:console,10.0'; \
    if (-not $env:CC) { $env:CC = if (Get-Command clang -ErrorAction SilentlyContinue) { (Get-Command clang).Source } else { 'D:\Scoop\apps\llvm\current\bin\clang.exe' } }; \
    if ($env:CC -match '\s' -and $env:CC -notmatch '^"') { $env:CC = '"' + $env:CC + '"' }; \
    if ($env:CC -notmatch 'fuse-ld') { $env:CC = $env:CC + ' -fuse-ld=lld' }; \
    go tool mygo build -platform windows/amd64 -upload; \
    if ($LASTEXITCODE -ne 0) { throw '发版构建失败' }

# 快速类型检查 + 单测。默认走无 cgo 的 stub 路径（不需要 Rust 库）；
# `just check 1` 连 OCR 真实现一起 vet，需要先跑 just ocr。
check cgo='0':
    $env:CGO_ENABLED = '{{ cgo }}'; \
    $env:GOARCH = 'amd64'; \
    if (-not $env:CC) { $env:CC = if (Get-Command clang -ErrorAction SilentlyContinue) { (Get-Command clang).Source } else { 'D:\Scoop\apps\llvm\current\bin\clang.exe' } }; \
    go vet ./...; \
    if ($LASTEXITCODE -ne 0) { throw 'go vet 失败' }; \
    Write-Host "[check] go vet 通过（CGO_ENABLED={{ cgo }}）"

# 单测（纯 Go 路径，不碰真机：eSCL 用 httptest 假设备）
test:
    $env:CGO_ENABLED = '0'; \
    go test ./...; \
    if ($LASTEXITCODE -ne 0) { throw '测试失败' }

# 开发时直接跑仓库里的副本：普通 go build 不带更新配置，不会自己升级。
run: ocr
    $env:CGO_ENABLED = '1'; \
    $env:GOARCH = 'amd64'; \
    $env:CGO_LDFLAGS = '-Xlinker /subsystem:console,10.0'; \
    if (-not $env:CC) { $env:CC = if (Get-Command clang -ErrorAction SilentlyContinue) { (Get-Command clang).Source } else { 'D:\Scoop\apps\llvm\current\bin\clang.exe' } }; \
    if ($env:CC -match '\s' -and $env:CC -notmatch '^"') { $env:CC = '"' + $env:CC + '"' }; \
    if ($env:CC -notmatch 'fuse-ld') { $env:CC = $env:CC + ' -fuse-ld=lld' }; \
    go build -trimpath -tags mygo_noinspector -o quick-scan.exe ./cmd/quick-scan; \
    if ($LASTEXITCODE -ne 0) { throw '主程序编译失败' }; \
    .\quick-scan.exe

# 清理构建产物
# 每个目标都先 Test-Path：删不存在的路径会让进程退出码变 1，just 会当成配方失败。
clean:
    foreach ($f in 'quick-scan.exe','cmd\quick-scan\mygo_windows_amd64.syso') { if (Test-Path $f) { Remove-Item -Force $f } }; \
    foreach ($d in 'ocrbridge\target','dist') { if (Test-Path $d) { Remove-Item -Recurse -Force $d } }; \
    Write-Host '[clean] 完成'

# 列出 Go 包
members:
    go list ./...
