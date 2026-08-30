# feat-auto-update · 执行计划（F009）

关联提案：`docs/pm/features/F009-auto-update.md`。分支 `feature/f009-auto-update`（基于 main）。

## 方案概述

Tauri 2 官方 updater 体系：`tauri-plugin-updater`（Rust + JS）+ minisign 签名 + `latest.json` 清单。endpoint 指向公开仓库的 `https://github.com/Yangshifu1024/GitWave/releases/latest/download/latest.json`，publish 草稿 release 后即生效，无需任何后端服务。

## 一次性准备（已完成 / 用户操作）

1. ✅ 密钥对已生成：`~/.tauri/gitwave.key`（私钥，无密码，不入库）+ `~/.tauri/gitwave.key.pub`（公钥已写入 `tauri.conf.json`）
2. ⬜ 用户备份私钥到密码管理器（**不可再生**：GitHub secret 只写不读；丢失则无法发版，换钥则已装用户永远收不到更新）
3. ⬜ 用户设置 GitHub secret：`gh secret set TAURI_SIGNING_PRIVATE_KEY < ~/.tauri/gitwave.key`（`TAURI_SIGNING_PRIVATE_KEY_PASSWORD` 不设，渲染为空串即匹配无密码私钥）

## 代码变更

### src-tauri/
- `Cargo.toml`：+ `tauri-plugin-updater = "2"`、`tauri-plugin-process = "2"`
- `src/lib.rs`：注册两个插件；新增命令 `is_appimage()`（读 `APPIMAGE` env，Linux 安装形态判定）
- `tauri.conf.json`：`bundle.createUpdaterArtifacts: true`；`plugins.updater`（pubkey + endpoint）
- `capabilities/default.json`：+ `updater:default`、`process:allow-restart`、`os:allow-platform`

### 前端
- `src/stores/updaterStore.ts`（新）：zustand 状态镜像（phase / 版本 / 进度 / modal open）；plugin 的 `Update` 对象带方法，留在 hook 模块槽里不进 store
- `src/hooks/useUpdater.ts`（新）：
  - `useAutoUpdateSetting()`：localStorage `gitwave-auto-update`，**默认开启**（显式 `"false"` 才关）
  - 检查状态机：idle → checking → available / manual-download / up-to-date / error → downloading → ready
  - `resolveManualDownload()`：`platform() === "linux" && !isAppimage()` → deb/rpm 降级
  - 启动检查延迟 3s 静默执行：失败（无网 / 404 无清单）一律吞掉；仅手动检查报错
  - `useInstallUpdate()`：`downloadAndInstall` 进度事件 → `useRestartApp()`（`relaunch()`）
  - `useRetryUpdate()`：有 pendingUpdate 续装（下载可重试），否则重查
- `src/components/UpdateModal.tsx`（新）：复用共享 Modal/Button + HeroUI ProgressBar；按 phase 切 footer（Download & Install / Open Releases Page / Restart Now / Try Again）；release notes 链接指向 `releases/tag/v<version>`
- `src/components/SettingsModal.tsx`：General 区新增 Updates 区块（开关 + Check for updates + 状态文案 + View update 重开弹窗）
- 应用菜单（两个菜单面共用 `appMenuSpec.ts` 单一事实源）：新增自处理特殊项 `check-updates`——macOS 原生 app 菜单（About 下方，Chrome 风格分组的「Check for Updates…」）与 Win/Linux 应用内 File 菜单（About 下方）各入一条；点击 = 打开 Settings（给 "up to date" 一个可见落点）+ 执行检查，发现新版则更新弹窗自动弹出
- `src/App.tsx`：挂 `useStartupUpdateCheck()` + `<UpdateModal />`

### CI（.github/workflows/build.yml）
- 三个平台 job 的 `tauri build` step：+ `TAURI_SIGNING_PRIVATE_KEY(_PASSWORD)` env
- release job：checkout（读 package.json 版本）+ node 脚本组装 `artifacts/latest.json`——按平台精确收集一份 `.sig`（数量≠1 或空内容即 fail-fast）；platforms 键 `darwin-aarch64` / `linux-x86_64` / `windows-x86_64`，URL 指向 `releases/download/<tag>/<asset>`；deb/rpm 不进清单
- release files 增补：macOS `*.app.tar.gz(.sig)`、`artifacts/latest.json`（windows `nsis/*`、linux `appimage/*` 的既有 glob 天然覆盖 `.sig`）

### 产物形态（`createUpdaterArtifacts: true` 后）
- macOS：`bundle/macos/GitWave.app.tar.gz(.sig)`（固定名，无版本号）
- Linux：`bundle/appimage/*_amd64.AppImage(.sig)`（AppImage 本体即更新包）
- Windows：`bundle/nsis/GitWave_<ver>_x64-setup.exe(.sig)`

## 基线限制

0.4.0 及更早版本无 updater，收不到任何更新提示；v0.5.0（装上本特性的首版）起才进入链式自动更新。公钥烧在客户端内，**私钥丢失 = 发版终止；私钥更换 = 已装用户全部断更**。

## 验证

- `cargo check` / clippy / fmt；`npm run lint` / `test` / `typecheck`
- 本地 `tauri dev`：Settings 手动检查（当前线上无 latest.json → 优雅报错；启动静默检查不打扰）
- 完整链路（真实清单 → 下载 → 签名校验 → 换包重启）待 v0.5.0 发版验证；如需提前验证：本地静态服务器托管手拼 latest.json + 临时改 endpoint 指向 `http://127.0.0.1:<port>/latest.json`
- 遗留观察项：Windows 下载进度事件的 contentLength 在部分 NSIS 场景为空（进度条退化为字节计数，已兼容）
- 遗留观察项：Windows 上 `downloadAndInstall` 的 install 步骤会拉起 NSIS 安装器（installMode 默认 passive）并退出应用——「Restart Now」实际只对 macOS / AppImage 有意义，发版验证时勿误判为缺陷
- 遗留观察项：endpoint 用的 `releases/latest` 语义不含 prerelease/draft；将来若发 prerelease 需改 endpoint 策略（如固定指向 stable 清单）
