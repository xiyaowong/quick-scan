# 技术笔记：eSCL 扫描

> 2026-10-19。扫描引擎从 TWAIN 换成 **eSCL**（Mopria/AirPrint 扫描协议）。
> 目标是去掉 Windows 驱动依赖与 32 位子进程：eSCL 就是局域网里的 HTTP + XML，
> 自己实现即可。代码对应：`discovery.go`（发现）、`escl.go`（协议）、`device.go`（编排）。

## 1. 设备发现（mDNS）

- 浏览服务类型 `_uscan._tcp`（明文 HTTP；TLS 的 `_uscans._tcp` 不接）。
- 不用高层 mDNS 库，在 `github.com/miekg/dns` 上自己走 DNS-SD 两步解析：
  先组播问 PTR `_uscan._tcp.local.` 拿到实例名和应答方 IP，再向应答方单播问
  SRV/TXT（QU 位）。实测 HP 116w 的 PTR 应答只有 PTR+A，不返回 SRV/TXT，
  只浏览 PTR 的库拼不出完整条目。注意 PTR 必须用普通组播（带 QU 的设备会忽略）。
- 只采用 IPv4：条目必须能解析出 A 记录，只有 AAAA 的直接跳过；
  个别实现只给主机名时，用系统解析器做一次 IPv4 兜底。
- 名字取 TXT `ty`（设备型号），没有就用服务实例名；路径取 TXT `rs`（缺省 `eSCL`）；
  端口取 SRV（缺省 80）。
- 选择器显示格式固定：**`设备名 (IPv4)`**（`DeviceInfo.DisplayName`）。
  明确离线的仍按旧行为在末尾加 `（脱机）`。
- 发现等待 4 秒（mDNS 广播，几秒足够），同一 IP+端口+路径去重。
- 启动加速：上次的设备端点与能力缓存在 `%TEMP%\SimpleMFPScan\devices.json`。
  启动先把缓存名单显示出来并立刻探测上次选中的设备，同时后台跑 mDNS；
  发现结果按 `IP:端口/路径` 合并进现有名单，不打断用户选择。
  缓存 30 天过期、最多 32 台；系统清理临时目录时丢掉也不影响正确性。

## 2. 能力探测

`GET http://<ip>:<port>/<rs>/ScannerCapabilities`，解析 `ScannerCapabilities`：

| XML | 映射 |
|---|---|
| `Platen/PlatenInputCaps` | 玻璃板能力 |
| `ADF/AdfSimplexInputCaps`（或 Duplex） | 进纸口能力；节点存在即有进纸口 |
| `SettingProfiles/SettingProfile/ColorModes/ColorMode` | `RGB24`→彩色、`Grayscale8`→灰度、`BlackAndWhite1`→黑白（也接受直接挂在 InputCaps 下的写法） |
| `SupportedResolutions/DiscreteResolutions` | dpi 列表；直接挂在 InputCaps 下或写在 `SettingProfile` 里都收；没有离散值时用 `ResolutionRange` 判断 300/600 是否在范围内 |
| `SupportedIntents/Intent` | 扫描用途（Document / TextAndGraphic / Photo / Preview …），界面全部翻成中文 |
| `MaxWidth` / `MaxHeight`（1/300 英寸） | 判断 A4/A5/Letter 能否放下（允许横竖两个方向，8 单位容差） |

规则（与旧版一致）：

- HTTP 通了就算**在线**；能力 XML 解析失败只意味着选项全开，不标离线。
- 能力缺失（字段为空）不限制选项。
- 清晰度下拉列出设备上报的全部 dpi，默认 300（没有就取最接近的一档）。
- 用途下拉列出设备上报的全部 Intent，默认 Document。

## 3. 扫描任务

1. `POST /<rs>/ScanJobs`，Content-Type `text/xml`，返回 201 + `Location`。
2. `GET <Location>/NextDocument` 取图；404/410 = 没有下一张。
   - 刚建任务时设备可能短暂 503/409，按 250ms 起指数退避重试，整体 120 秒超时。
   - 玻璃板取一张即止；进纸口循环取到 404（一批多张）。
3. `DELETE <Location>` 收尾（成功失败都删，避免任务占着扫描仪）。

ScanSettings 要点：

- 纸张尺寸按物理尺寸 × dpi 换算成像素（A4 300dpi = 2480×3508），
  超过设备 `MaxWidth/MaxHeight` 时夹到边界。
- 请求格式优先 `image/jpeg`，其次 `image/png`；只报 PDF 的设备报错（没有 PDF 渲染器）。
- 色彩下发设备原样上报的 ColorMode（如 `RGB48`），没有就发规范值。
- 进纸口下发 `InputSource=ADF` + `ADFMode=Simplex`；玻璃板 `InputSource=Platen`。
- `Intent` 下发界面选择的值；设备不支持时退回 `Document`，再不行用设备上报的第一个。

## 4. 落盘与显示

- 设备返回的 JPEG/PNG 原样写进 `%TEMP%\SimpleMFPScan_<pid>\pages\`，扩展名随格式。
- `decodeImage` 按文件头解码 JPEG/PNG/BMP；缩略图、灯箱、旋转、PDF/JPG 导出与旧版一致。
- 导出与 OCR 都走同一份原图，行为不变。

## 5. 错误处理

- 探测失败 → 设备标红、排序垫底，状态栏显示底层原因。
- 扫描失败 → 扫描按钮旁的 `scanStatus` 显示中文原因（超时、HTTP 状态、拒绝任务）。
- 扫描仪忙/缺纸由 HTTP 状态暴露，文案保持"人话"，不出现协议术语。
