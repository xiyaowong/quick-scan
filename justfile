# quick-scan 构建管理（Windows 64 位，Go + mygo 原生 UI）
#
# 单一产物 quick-scan.exe：mygo UI + eSCL 客户端 + 内嵌 OCR 模型。
# 扫描走局域网 eSCL（HTTP），不需要厂商 TWAIN/WIA 驱动，也没有子进程。
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

# ── 2. 生成 Windows 资源（exe 图标 + 版本信息）──
# windres 把 app.rc 编成 COFF 对象；放到 main 包目录（cmd/quick-scan）里，
# Go 的内部链接器会自动带上。app.rc 必须保持 UTF-8 BOM（无 BOM 时 rc 静默失败）。
# --codepage=65001：windres 默认按系统 ANSI 读源码，中文会变成一串 GBK 乱码写进资源。
icon:
    windres -i app.rc -o cmd\quick-scan\rsrc_windows_amd64.syso -O coff --target=pe-x86-64 --codepage=65001; \
    if ($LASTEXITCODE -ne 0) { throw 'windres 编资源失败' }; \
    Write-Host '[icon] rsrc_windows_amd64.syso 完成'

# ── 3. 64 位主程序（OCR 静态链入）──
# -s -w -trimpath -mygo_noinspector：剥 DWARF/符号/路径，mygo 生产构建的官方姿势。
# 子系统保持 console（不用 -H=windowsgui，也不切 /subsystem:windows）：cgo 外部链接下
#   -H=windowsgui 会被静默丢弃，而 windows 子系统会让 stderr 无处可去。
#   控制台改在运行期脱钩（console.go）：双击启动时 FreeConsole 摘掉，从终端启动时留着。
# CGO_LDFLAGS 只抬 PE 头里的 SubsystemVersion（clang/lld 默认写 6.0，Go 内部链接器写
#   10.0），不切换子系统类型。6.0 会被 Windows 当成 Win7 程序，拒绝创建 WS_EX_LAYERED
#   子窗口；mygo 自定义标题栏的按钮/顶边正是这种子窗口，于是启动报
#   "cannot create the window controls of a window with a hidden title bar"，贴边缩放和
#   贴靠布局也一起失效。Windows 8（6.2）起才支持，这里取 10.0 与内部链接器对齐。
# cgo：CC 会原样交给 Go，路径带空格和额外参数都要写在这里。
# -fuse-ld=lld：不让 clang 落到 MSVC 的 link.exe 上（它读不了 Go 给的 .ld
#   响应文件，报 LNK1107）；必须放在 CC 里，Go 用 `clang -Wl,--version` 探测
#   是不是 LLD，探测不到就会继续传那个 .ld 脚本（实测 CI 上如此）。
#   引号只包 exe 本身，参数留在外面。
ui: icon ocr
    $env:CGO_ENABLED = '1'; \
    $env:GOARCH = 'amd64'; \
    $env:CGO_LDFLAGS = '-Xlinker /subsystem:console,10.0'; \
    $ver = (((Select-String -Path app.rc -Pattern 'PRODUCTVERSION\s+([\d,]+)').Matches[0].Groups[1].Value -split ',')[0..2] -join '.'); \
    if (-not $env:CC) { $env:CC = if (Get-Command clang -ErrorAction SilentlyContinue) { (Get-Command clang).Source } else { 'D:\Scoop\apps\llvm\current\bin\clang.exe' } }; \
    if ($env:CC -match '\s' -and $env:CC -notmatch '^"') { $env:CC = '"' + $env:CC + '"' }; \
    if ($env:CC -notmatch 'fuse-ld') { $env:CC = $env:CC + ' -fuse-ld=lld' }; \
    $mirror = ''; \
    if ($env:QUICK_SCAN_MIRROR) { $mirror = " -X quick-scan/internal/update.Mirror=$env:QUICK_SCAN_MIRROR" }; \
    go build -trimpath -tags mygo_noinspector -ldflags "-s -w -X quick-scan/internal/update.Version=$ver$mirror" -o quick-scan.exe ./cmd/quick-scan; \
    if ($LASTEXITCODE -ne 0) { throw '主程序编译失败' }; \
    Write-Host "[ui] quick-scan.exe 完成（$ver）"

# 全量构建
build: ui
    @Write-Host ''
    @Write-Host '完成：quick-scan.exe（64 位单文件：含 OCR 模型）'

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

# 跑主程序（会真的用 mDNS 找设备并连打印机）
# QUICK_SCAN_NO_UPDATE：这里是仓库里直接跑的开发版，别让它去下载、安装线上版本。
run: ui
    $env:QUICK_SCAN_NO_UPDATE = '1'; \
    .\quick-scan.exe

# ── 发布产物（dist\quick-scan-<版本>-win64.zip，内含单个 exe）──
# 版本号直接读 app.rc 的 PRODUCTVERSION，避免和资源里的版本漂移。
package: build
    $ver = (((Select-String -Path app.rc -Pattern 'PRODUCTVERSION\s+([\d,]+)').Matches[0].Groups[1].Value -split ',')[0..2] -join '.'); \
    $stage = "dist\quick-scan-$ver"; \
    if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }; \
    New-Item -ItemType Directory -Force $stage | Out-Null; \
    if (-not (Test-Path quick-scan.exe)) { throw 'quick-scan.exe 不存在，先跑 just build' }; \
    Copy-Item quick-scan.exe "$stage\quick-scan.exe"; \
    if (Test-Path README.md) { Copy-Item README.md $stage }; \
    $zip = "dist\quick-scan-$ver-win64.zip"; \
    if (Test-Path $zip) { Remove-Item $zip -Force }; \
    Compress-Archive -Path $stage -DestinationPath $zip; \
    Write-Host ''; \
    Write-Host "打包完成：$zip（$([math]::Round((Get-Item $zip).Length / 1MB, 1)) MB）"

# ── 安装包（dist\quick-scan-<版本>-setup.exe，NSIS + 简体中文）──
# 安装包只打根目录的 quick-scan.exe（upx 的压缩版在 dist\，不能进安装包）
installer: build
    if (-not (Get-Command makensis -ErrorAction SilentlyContinue)) { throw '未找到 makensis：scoop install nsis' }; \
    New-Item -ItemType Directory -Force dist | Out-Null; \
    $ver = (((Select-String -Path app.rc -Pattern 'PRODUCTVERSION\s+([\d,]+)').Matches[0].Groups[1].Value -split ',')[0..2] -join '.'); \
    makensis /V2 /DVERSION=$ver installer.nsi; \
    if ($LASTEXITCODE -ne 0) { throw 'makensis 编安装包失败' }; \
    Write-Host "安装包完成：dist\quick-scan-$ver-setup.exe"

# ── UPX 压缩发布版（可选）──
# 只用 NRV + --best：体积约减半，启动多 ~0.2 秒。
# 不要加 --lzma：只再小 1.7MB，启动却多 ~0.8 秒（实测：0.30 → 0.51 → 1.12 秒）。
# 产物 dist\quick-scan.exe；根目录的 quick-scan.exe 保持未压缩，方便排查。
upx: build
    if (-not (Get-Command upx -ErrorAction SilentlyContinue)) { throw '未找到 upx：winget install UPX.UPX' }; \
    New-Item -ItemType Directory -Force dist | Out-Null; \
    Copy-Item quick-scan.exe dist\quick-scan.exe -Force; \
    $before = (Get-Item dist\quick-scan.exe).Length; \
    upx --best --no-progress dist\quick-scan.exe; \
    if ($LASTEXITCODE -ne 0) { throw 'UPX 压缩失败' }; \
    upx -t dist\quick-scan.exe; \
    if ($LASTEXITCODE -ne 0) { throw '压缩后完整性校验失败' }; \
    $after = (Get-Item dist\quick-scan.exe).Length; \
    Write-Host ("[upx] {0:N1} MB → {1:N1} MB（{2:N0}%）" -f ($before/1MB), ($after/1MB), (100*$after/$before))

# 清理构建产物
# 每个目标都先 Test-Path：删不存在的路径会让进程退出码变 1，just 会当成配方失败。
clean:
    foreach ($f in 'quick-scan.exe','cmd\quick-scan\rsrc_windows_amd64.syso') { if (Test-Path $f) { Remove-Item -Force $f } }; \
    foreach ($d in 'ocrbridge\target','dist') { if (Test-Path $d) { Remove-Item -Recurse -Force $d } }; \
    Write-Host '[clean] 完成'

# 列出 Go 包
members:
    go list ./...
