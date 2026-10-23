# 发布流程

发版只需要三步：改版本号 → 打 tag 推上去 → 等 GitHub Actions 发布。

自动更新读取的是 GitHub Releases，所以**这套流程必须走完**，手改文件或手动传包都不算发版。

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

按上面的改动决定版本号，例如 `0.2.0`。

## 2. 改版本号

只改 `app.rc`，四处都要改：

```
FILEVERSION     0,2,0,0
PRODUCTVERSION  0,2,0,0
VALUE "FileVersion",     "0.2.0"
VALUE "ProductVersion",  "0.2.0"
```

`FILEVERSION` / `PRODUCTVERSION` 是四段数字（最后一段固定 0），
字符串是三段。`justfile`、安装包和自动更新都从这里读，别的地方不用动。

改完在本地确认一次：

```powershell
just test        # 单测
just installer   # dist\quick-scan-0.2.0-setup.exe
```

## 3. 发版

```powershell
git add app.rc
git commit -m "发布 0.2.0"
git tag v0.2.0
git push origin main --tags
```

推送 tag 会触发 `.github/workflows/release.yml`：

1. 校验 tag 与 `app.rc` 的版本号一致，不一致直接失败，不会发错版本；
2. 在 Windows runner 上编译（Rust OCR 桥 + Go），跑一遍单测；
3. 打包安装包和免安装 zip；
4. 建 release（自动生成更新说明）并上传两个产物。

在 Actions 页面等绿灯，然后打开 Releases 确认：

- `quick-scan-0.2.0-setup.exe`（自动更新下载的就是它）
- `quick-scan-0.2.0-win64.zip`

## 自动更新怎么用这些产物

- 已安装的用户启动时查 `releases/latest`，发现新版本就后台下载安装包；
- 下次启动时静默装好，界面上没有任何提示或等待；
- 只认文件名 `quick-scan-<版本>-setup.exe`，改名字等于关掉自动更新；
- 免安装版（zip 解压直接跑的）不自动更新，程序会跳过。

## 出错时

- **workflow 报版本号不一致**：tag 和 `app.rc` 对不上。删掉 tag 重来：
  `git tag -d v0.2.0; git push origin :refs/tags/v0.2.0`。
- **release 已经发了但要重传**：Actions 里重跑 workflow，会覆盖同名产物。
- **发错了版本**：删掉 release 和 tag，把 `app.rc` 改回正确版本重新发。
  已经更新过的用户不会自己降级，只能等下一个版本。
