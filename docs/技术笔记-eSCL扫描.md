# 技术笔记：eSCL 扫描

> 扫描走局域网 eSCL（Mopria / AirPrint 扫描协议），代码在 `internal/scanner`。

## 设备发现（mDNS）

- 浏览 `_uscan._tcp`，不接 TLS 的 `_uscans._tcp`。
- 在 `miekg/dns` 上自己做 DNS-SD 两步解析：组播问 PTR 拿实例名和应答方 IP，
  再向应答方单播问 SRV/TXT。HP 116w 的 PTR 应答只有 PTR+A，只刷 PTR 的库拿不到完整条目。
- 只采用 IPv4；只有 AAAA 的条目跳过，只给主机名的做一次系统解析兜底。
- 名字取 TXT `ty`，没有就用实例名；路径取 `rs`（缺省 `eSCL`），端口取 SRV（缺省 80）。
- 发现等待 4 秒，按 `IP:端口/路径` 去重。
- 缓存 `%TEMP%\quick-scan\devices.json`：启动先显示缓存并探测上次的设备，同时后台跑 mDNS，
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

## 设备状态（ScannerStatus）

`GET http://<ip>:<port>/<rs>/ScannerStatus`，取根节点的 `pwg:State` 和 `scan:AdfState`。
`Jobs` 里每条的 `pwg:JobState` 不能混进来，所以只解析这两个字段。

| 读取 | 界面 |
|---|---|
| `State=processing` 或 `AdfState=ScannerAdfProcessing` | 「正在忙」，扫描按钮置灰 |
| `State=stopped` | 「需要检查」，红色提示卡纸/盖子 |
| `AdfState=ScannerAdfJam`（进纸口模式） | 「进纸口卡纸」，红色 |
| `AdfState=ScannerAdfDoorOpen`（进纸口模式） | 「进纸口没关好」，红色 |
| 读不到 / 字段缺失 | 状态栏不显示，**不拦扫描** |

- 只跟扫描直接相关的状态：`AdfState=ScannerAdfEmpty`（进纸口没纸）**不显示也不拦**，
  否则用户会疑惑"没纸？"，而且玻璃板扫描也会被一起挡住。
- 起一条独立的轮询协程，每 2 秒查一次；探测成功、换设备、扫完之后插队再查一次。
  轮询和扫描流程互不相干，不会因为扫描卡住就停在过期的状态上。
- 状态跟着 `DeviceInfo.Idx` 记：换设备后旧状态作废，不能拿上一台的「忙」拦住新设备。
- 读不到状态不等于离线；离线仍由能力探测判断。有些不报状态的设备不能因此被锁死。

## 扫描任务

1. `POST /<rs>/ScanJobs`（`text/xml`），返回 201 + `Location`。
2. `GET <Location>/NextDocument` 取图；404/410 表示没有下一张。
   刚建任务可能短暂 503/409，按 250ms 起指数退避，整体 120 秒超时。
   玻璃板取一张即止，进纸口循环到 404。
3. 扫完后**不删任务**，让设备自己收尾（约 6 秒），JobState 记成 `Completed`。
   `DELETE <JobUri>` 的语义是「取消」，不是「删除」：处理中调用会让条目变成
   `Canceled / JobCanceledByUser`；已经结束的再调用返回 503，条目照样留着。
   设备侧的任务历史无法清理，所以 DELETE 只用在两种情况：扫描半路失败，
   或用户中途点了「停止」（不删的话设备会一直扫完整叠）。
   实测删与不删，下一轮扫描都要等设备空下来（约 6 秒），正常扫完删掉没有好处。

ScanSettings：

- 纸张尺寸按物理尺寸 × dpi 换算成像素，超过 `MaxWidth/MaxHeight` 时夹到边界。
- 优先请求 `image/jpeg`，其次 `image/png`；只报 PDF 的设备直接报错。
- 色彩下发设备上报的 ColorMode，缺省用规范值。
  **例外：黑白不下发 `BlackAndWhite1`**。实测 HP 116w 在能力里报了它，但配
  `image/jpeg` 时任务直接 `Aborted / JobCompletedWithErrors`，`ImagesCompleted=0`，
  `NextDocument` 一直回 410（只肯把黑白包进 PDF）。所以黑白改成向设备要灰度，
  本地按 Otsu 阈值二值化后存 PNG（JPEG 会把纯黑白重新变灰）。
- 玻璃板 `InputSource=Platen`；进纸口 `InputSource=ADF` + `ADFMode=Simplex`。
- Intent 不支持时退回 `Document`，再不行用设备上报的第一个。

## 落盘与错误

- 返回的 JPEG/PNG 原样写到 `%TEMP%\quick-scan_<pid>\pages\`；黑白页是二值化后的 PNG。
- 文件的扩展名按响应头 `Content-Type` 定，不按请求的格式：设备常无视请求自己换一种
  （要 `image/png` 也可能回 `application/pdf`），扩名出错文件就打不开。
- 探测失败 → 设备标红并垫底，状态栏给底层原因。
- 扫描失败 → 扫描按钮旁显示中文原因，不出现协议术语。
