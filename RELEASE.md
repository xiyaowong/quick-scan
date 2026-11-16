# 发布流程

发版只需要三步：改版本号 → 打 tag 推上去 → 等 GitHub Actions 发布。

版本号只有一个来源：`mygo.json`。构建、安装包、更新清单和自动更新都读它，
安装脚本和资源文件里的第二份版本号已经删掉。

## 0. 首次准备（只需做一次）

自动更新只接受用同一把密钥签名的包。密钥对用 mygo 官方命令生成：

```powershell
go tool mygo keygen
```

它把 `mygo-update.key`（私钥）和 `mygo-update.pub`（公钥）写到
`%APPDATA%\mygo\update-keys\`，并打印位置。然后：

- 把 `mygo-update.pub` 的内容填进 `mygo.json` 的 `updates.publicKey`（已填好）；
- 把私钥存进 GitHub 仓库机密，CI 发版时用它签名：

  ```powershell
  gh secret set MYGO_UPDATER_PRIVATE_KEY < "$env:APPDATA\mygo\update-keys\mygo-update.key"
  ```

私钥不要进仓库。丢了它，已安装的用户就再也收不到更新；泄露了它，
别人就能给你的用户推任意代码。

## 1. 定版本号

语义化版本 `主.次.修订`：

| 情况 | 升哪一位 | 例子 |
|---|---|---|
| 修 bug、改文案、调整界面细节 | 修订 | 0.3.1 → 0.3.2 |
| 加功能，老用户升级不需要改习惯 | 次 | 0.3.2 → 0.4.0 |
| 改了扫描/导出/配置的用法，用户得重新学 | 主 | 0.4.0 → 1.0.0 |

先看上次发到哪一版：

```powershell
git tag -l --sort=-v:refname | Select-Object -First 3
git log --oneline "$(git describe --tags --abbrev=0)..HEAD"
```

## 2. 改版本号

只改 `mygo.json` 的 `version`：

```json
"version": "1.3.0"
```

更新窗口显示的新版本说明来自 `CHANGELOG.md`：在里面加一节 `## 1.3.0`
（也认 `## [1.3.0] - 2026-11-16` 这种写法）。缺了这一节，签名构建会直接失败。

改完在本地确认一次：

```powershell
just test     # 单测
just build    # dist\windows-amd64\quick-scan.exe + 安装包
```

本地不带签名密钥时只警告、不出更新包；真正发版由 CI 签名。

## 3. 发版

```powershell
git add mygo.json CHANGELOG.md
git commit -m "发布 1.3.0"
git tag v1.3.0
git push origin main --tags
```

推送 tag 会触发 `.github/workflows/release.yml`：

1. 校验 tag 与 `mygo.json` 的版本一致，不一致直接失败；
2. 在 Windows runner 上编 Rust OCR 桥、跑单测；
3. `go tool mygo build` 编译、打包，用私钥签名更新；
4. 把安装包、更新归档、增量更新和清单上传到 `v1.3.0` 草稿 release；
5. 取消草稿状态，正式发布。

在 Actions 页面等绿灯，然后打开 Releases 确认这些产物：

- `quick-scan-setup-1.3.0-amd64.exe`（安装包）
- `quick-scan-1.3.0-windows-amd64.tar.gz`（更新归档，免安装版也可解压使用）
- `update-windows-amd64.json`（更新清单，程序读它判断有没有新版本）
- `install.ps1`（装最新版用）
- `SHA256SUMS`

文件名和清单都由 mygo 生成，不要手改；改了等于关掉自动更新。
需要本地重跑发版可用 `just release`，但得先设好
`MYGO_UPDATER_PRIVATE_KEY` 和 `GH_TOKEN`。

## 自动更新怎么工作

- 已安装的程序启动约 10 秒后在后台检查，之后每天一次；
- 有新版本时弹出更新窗口，显示更新说明，可以安装、稍后提醒或跳过这个版本；
- 安装后重启程序生效；
- 界面右下角的版本号可以点开软件信息，里面有「检查更新」，检查失败也会说明原因；
- 更新包必须通过 `mygo.json` 里公钥的签名校验，校验不过不会安装；
- 解压到可写目录的免安装版也会自我更新；放在只读目录（如 `Program Files`）里则不更新。

## 从旧方案切换

1.2.0 及更早的版本用的是自研更新（认 `quick-scan-<版本>-setup.exe`），
新方案的文件名和清单都不一样。这些老用户不会自动升级，需要手动下载一次
`quick-scan-setup-<版本>-amd64.exe` 装上；之后就会走新的自动更新。

## 出错时

- **workflow 报版本号不一致**：tag 和 `mygo.json` 对不上。删掉 tag 重来：
  `git tag -d v1.3.0; git push origin :refs/tags/v1.3.0`。
- **release 已经发了但要重传**：Actions 里手动触发 workflow，输入同一个 tag，
  会覆盖同名产物并重新发布。
- **签名失败或用户收不到更新**：确认 `MYGO_UPDATER_PRIVATE_KEY` 与
  `mygo.json` 的 `publicKey` 是同一对密钥。重新生成密钥等于换锁，
  已安装的老用户不会接受新包。
- **发错了版本**：删掉 release 和 tag，把 `mygo.json` 改回正确版本重新发。
  已经更新过的用户不会自己降级，只能等下一个版本。
