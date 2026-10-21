# 技术笔记：eSCL 扫描

> 扫描走局域网 eSCL（Mopria / AirPrint 扫描协议），代码在 `internal/scanner`。

## 设备发现（mDNS）

- 浏览 `_uscan._tcp`，不接 TLS 的 `_uscans._tcp`。
- 在 `miekg/dns` 上自己做 DNS-SD 两步解析：组播问 PTR 拿实例名和应答方 IP，
  再向应答方单播问 SRV/TXT。HP 116w 的 PTR 应答只有 PTR+A，只刷 PTR 的库拿不到完整条目。
- 只采用 IPv4；只有 AAAA 的条目跳过，只给主机名的做一次系统解析兜底。
- 名字取 TXT `ty`，没有就用实例名；路径取 `rs`（缺省 `eSCL`），端口取 SRV（缺省 80）。
- 发现等待 4 秒，按 `IP:端口/路径` 去重。
- 缓存 `%TEMP%\sscan\devices.json`：启动先显示缓存并探测上次的设备，同时后台跑 mDNS，
  结果按端点合并，不打断用户选择。缓存 30 天过期，最多 32 台，丢了不影响正确性。

## 网段扫描兜底（mDNS 无应答时）

防火墙、访客网络、VPN 抢默认路由都会让组播出不去。mDNS 阶段 1 一个应答都没有、
且**缓存里没有已经连上的设备**时，才改用默认出口网卡扫一遍所在网段——
有可用设备就不扫，避免每次启动都白跑一遍。

- 只扫默认网卡所在的 /24（网卡掩码更宽也只扫本机 /24），跳过本机、网络号和广播号。
- 地址顺序按常见 DHCP 段：`1–20`、`21–29`、`100–119`、`200–219`，其余从小到大。
- `GET http://IP/eSCL/ScannerCapabilities`，单地址 800ms，40 个并发，整轮上限 8 秒。
- 回 200 且带 `ScannerCapabilities` 才算扫描仪；名字取 `MakeAndModel`。
- 扫完整个网段，但**找到一台就立刻回调**：设备列表马上可用，其余地址在后台继续扫，
  结果逐台合并。扫描本身不碰选中、探测和缓存逻辑，那些仍由 UI 层决定。
- 端口只试 80、路径只试 `eSCL`；非标准端口的设备仍靠 mDNS。

## 能力探测

`GET http://<ip>:<port>/<rs>/ScannerCapabilities`：

| XML | 映射 |
|---|---|
| `Platen/PlatenInputCaps` | 玻璃板 |
| `ADF/AdfSimplexInputCaps` | 进纸口；节点存在即有 |
| `SettingProfiles/.../ColorModes` | `RGB24` 彩色、`Grayscale8` 灰度、`BlackAndWhite1` 黑白 |
| `SupportedResolutions` | dpi 列表；没有离散值时用 `ResolutionRange` 判断 |
| `SupportedIntents` | 扫描内容，界面翻成中文 |
| `MaxWidth` / `MaxHeight` | 判断 A4/A5/Letter 能否放下（有 8 单位容差） |

- HTTP 通了就算在线；能力解析失败只表示选项全开，不标离线。
- 字段缺失不限制选项。

## 扫描任务

1. `POST /<rs>/ScanJobs`（`text/xml`），返回 201 + `Location`。
2. `GET <Location>/NextDocument` 取图；404/410 表示没有下一张。
   刚建任务可能短暂 503/409，按 250ms 起指数退避，整体 120 秒超时。
   玻璃板取一张即止，进纸口循环到 404。
3. `DELETE <Location>` 收尾，成功失败都删。

ScanSettings：

- 纸张尺寸按物理尺寸 × dpi 换算成像素，超过 `MaxWidth/MaxHeight` 时夹到边界。
- 优先请求 `image/jpeg`，其次 `image/png`；只报 PDF 的设备直接报错。
- 色彩下发设备上报的 ColorMode，缺省用规范值。
- 玻璃板 `InputSource=Platen`；进纸口 `InputSource=ADF` + `ADFMode=Simplex`。
- Intent 不支持时退回 `Document`，再不行用设备上报的第一个。

## 落盘与错误

- 返回的 JPEG/PNG 原样写到 `%TEMP%\sscan_<pid>\pages\`。
- 探测失败 → 设备标红并垫底，状态栏给底层原因。
- 扫描失败 → 扫描按钮旁显示中文原因，不出现协议术语。
