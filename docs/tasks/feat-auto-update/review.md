# feat-auto-update · Code Review

- 分支：`feature/f009-auto-update`（计划分支；当前全部改动未提交且实际位于 `main`，commit 前需先建分支，见总体评价）
- 审查方式：code-reviewer 代理，对照 `git diff` + 未跟踪新文件；插件行为核验基于本地安装源码（`@tauri-apps/plugin-updater` 2.10.1 / `@tauri-apps/plugin-os` 2.3.2 / `tauri-plugin-updater-2.10.1` cargo 源码）
- 基线复验：`npx tsc --noEmit` 零错误；`npx vitest run` 15 文件 106 用例全过（与提交说明一致）
- 结论：**建议修复 2 项 🟡 后合入**（无 🔴；其余 🟡 / 🟢 可随本 PR 或紧随其后处理）

## 审查认可项

- 插件 API 用法全部核验正确：`check(): Promise<Update | null>`；等版本不误报（插件源码 `updater.rs:532` 为 `release.version > self.current_version`）；`Update.currentVersion` / `Update.version` 字段名正确；`DownloadEvent` 三分支恰好穷尽（else 即 `Finished`）；`relaunch()` 来自 plugin-process；`platform()` 在 plugin-os v2 确为同步（读 `window.__TAURI_OS_PLUGIN_INTERNALS__`），hook 内注释属实
- `useUpdater.ts:53` 将带方法的 `Update` 对象留在模块槽、zustand 只镜像渲染态的分层设计合理，注释解释了动机；失败重试可复用 pendingUpdate 免二次 check
- capabilities 最小化到位：`updater:default`（插件文档声明的标准全集）、`process:allow-restart`（恰好覆盖 relaunch）、`os:allow-platform`（未放宽到 `os:default`），且限定 `windows: ["main"]`
- `tauri.conf.json` schema 合法：`bundle.createUpdaterArtifacts`、`plugins.updater.pubkey` / `endpoints` 字段名与位置均符合 Tauri 2 规范；pubkey 烧入客户端 + HTTPS endpoint + minisign 校验，更新链路信任模型正确
- `is_appimage()` 读 `APPIMAGE` env 是 AppImage（type 2）标准探测方式，deb/rpm 场景 env 未设置 → 正确落入 manual 分支；静默失败兜底为 `false`（降级到手动，安全方向）
- CI latest.json 脚本健壮性核验：
  - release job `checkout` **必要**（读 package.json 版本），成本低，保留合理
  - `GITHUB_REF_NAME` 在 tag push 事件下即 tag 名，release job 已有 `refs/tags/` 守卫，re-run 上下文不变
  - `readdirSync` 只按文件名过滤，软链/嵌套目录（macOS artifact 内的 `GitWave.app/` 递归上传）不会误匹配；空目录 → hits=0 → fail-fast，符合意图
  - 与 `fail_on_unmatched_files: true` 的交互正确：latest.json 在 publish 前写入恒被匹配；脚本任一 sig 数量≠1 或为空即抛错 → 整个 release job 失败、不产生 draft，fail-fast 语义完整
  - macOS 上传路径 `bundle/macos/*` 既有 glob 天然覆盖 `GitWave.app.tar.gz(.sig)`，无需改 upload step
- 组件约定一致：复用 `ui/Modal`（open/onOpenChange/title/size/footer）、`ui/Button`、`ui/Checkbox`；HeroUI `ProgressBar` compound 写法（Track/Fill + `bg-accent/25`）与 `SyncStatusArea.tsx:84` 既有用法一致；状态文案 `aria-live="polite"`
- 版本比较无降级误报：本地版本高于 manifest 时 `check()` 返回 null → 显示 up-to-date；启动检查失败全部静默（`useUpdater.ts:88`），符合 F009「失败一律静默」

## 发现与处置

### 🔴 严重（必须修复）

无。核心链路（版本比较、签名校验、事件分支、平台键映射、CI fail-fast、capability 最小化）逐项核验无误。

### 🟡 建议修复

| # | 位置 | 发现与建议 |
|---|---|---|
| 1 | `src/hooks/useUpdater.ts:159-168` | **StrictMode 下启动静默检查永远不会执行**。`main.tsx:41` 启用了 StrictMode，dev 模式 effect 按挂载→清理→重挂载执行：首次 effect 设 `startedRef.current = true` 并启动定时器，cleanup 清掉定时器，第二次 effect 被 ref 早退——定时器再也不会点火。生产环境（无双调用）不受影响，但 dev 里该特性完全失效且无任何征兆。建议把 `startedRef.current = true` 移入 setTimeout 回调内（首次定时器被清理后，第二次 effect 会重新调度且仅触发一次） |
| 2 | `src/hooks/useUpdater.ts:72` | **`check()` 未设超时**。`CheckOptions.timeout` 插件已支持但未传；底层 reqwest 客户端默认无超时，离线代理挂起等场景下 Settings 手动检查按钮会永久停留在 "Checking…"（`busy` 恒真、按钮 disabled），只能重启恢复。建议 `check({ timeout: 15_000 })`，超时错误走既有 `fail()` 路径展示。`downloadAndInstall` 无取消 API 属插件限制，可在 plan.md 遗留项注明 |
| 3 | `.github/workflows/build.yml:270`（node 脚本） | **版本号三处手工同步无交叉校验，漂移即静默断更**。manifest `version` 读 `package.json`，而运行时比较基准是 `tauri.conf.json` 的 `version`（插件源码 `updater.rs:169` 取 `app.package_info().version`）。当前三处（package.json / tauri.conf.json / Cargo.toml）均为 0.4.0 一致，但发版时漏改其一的失败模式是静默的：如 tag v0.5.1 而 package.json 仍 0.5.0 → manifest 版本 ≤ 客户端版本 → `check()` 返回 null，所有用户永远收不到更新且发版时无任何报警。脚本已在读 package.json，建议追加 3 行 fail-fast：断言 `tag === \`v${version}\`` 且与 `src-tauri/tauri.conf.json` 的 version 一致 |
| 4 | `src/hooks/useUpdater.ts:99-103,105-127` | **状态机并发缺口**。`busy` 仅覆盖 `checking` 阶段：下载中 / 已就绪时 Settings 的 Check 按钮仍可点击，`beginCheck()` 会把 `downloading` 重置为 `checking`，随后在途下载的 `markReady` 与新 check 的 `markAvailable` 交错到达，最终状态取决于完成顺序（结果碰巧自洽但过程不可预测）；双击 "Download & Install" 在重渲染前也会触发两次并发 `downloadAndInstall`（无 in-flight 守卫，Windows 场景可能拉起两个安装器）。建议：install 回调加模块级 in-flight 标志；`phase ∈ {downloading, ready}` 时禁用/忽略 Check |
| 5 | `src/stores/updaterStore.ts` | **状态机缺单测**。仓库惯例 stores 均有测试（`statusAreaStore.test.ts` / `syncStore.test.ts` / `uiStore.test.ts`），updaterStore 的 phase 迁移（available→downloading→ready、fail 清洗 error、markAvailable 重置进度）无覆盖；`resolveManualDownload` 的判定逻辑（linux && 非 AppImage → manual）建议抽成纯函数后补测（hook 内含插件 import 难以直接测） |

### 🟢 可选优化

| # | 位置 | 发现与建议 |
|---|---|---|
| 1 | `src/hooks/useUpdater.ts:78,124` | `Update extends Resource`（持有 Rust 侧 rid），`pendingUpdate` 被新 check 结果替换或安装完成时未调用 `update.close()`，依赖 GC 回收；重复手动检查场景会短暂累积 rid。可在替换前 `void old.update.close()` |
| 2 | `src-tauri/Cargo.toml:54`、`src/hooks/useUpdater.ts:56-67` | 为一次同步 `platform()` 引入整个 plugin-os 依赖 + `os:allow-platform` 权限；可让 `is_appimage` 命令直接返回安装形态（如 `"appimage" \| "package" \| "native"`），前端删掉 plugin-os，权限面更小 |
| 3 | `src/stores/updaterStore.ts:61` | `Progress` 事件每个 chunk 都 `setProgress` → 每 chunk 一次 re-render；下载高峰可达每秒数十次。可按 ~100ms 节流或 rAF 合帧（当前 UI 面积小，非必须） |
| 4 | `.github/workflows/build.yml:298` | manifest 中 macOS URL 硬编码 `GitWave.app.tar.gz`，而 linux/windows 从 sig 文件名派生；统一为 `path.basename(macSig).replace(/\.sig$/, "")` 更一致（productName 改名时少一处手工同步） |
| 5 | `src/components/UpdateModal.tsx:16,120` | `releaseNotesUrl` 假设 tag 带 `v` 前缀（当前约定成立，若出现非 v 前缀 tag 则 404）；`error` 阶段弹窗标题仍是 "Update available"，检查失败场景可改为 "Update check failed" 之类措辞 |
| 6 | `src-tauri/capabilities/default.json:13` | `updater:default` 含前端未用的 `allow-download` / `allow-install` 分离权限；追求极致最小化可显式列 `updater:allow-check` + `updater:allow-download-and-install`。无实际风险（download-and-install 本就包含两者） |
| 7 | `docs/tasks/feat-auto-update/plan.md` | Windows 上 `downloadAndInstall` 的 install 步骤会拉起 NSIS 安装器并退出应用（installMode 默认 passive），"ready → Restart Now" 路径实际只对 macOS / AppImage 有意义；建议在该文档遗留观察项补一句，避免发版验证时误判为缺陷。另注：`releases/latest` 语义不包含 prerelease/draft，若未来发 prerelease 需改 endpoint 策略 |
| 8 | `src/hooks/useUpdater.ts:18`、`src/components/UpdateModal.tsx:16`、`src-tauri/tauri.conf.json:57` | 仓库地址 `Yangshifu1024/GitWave` 硬编码三处（endpoint / RELEASES_URL / releaseNotesUrl 派生）；可收敛为单一常量或构建期注入，仓库迁移时少改两处 |

## 复验记录

- `npx tsc --noEmit`：零错误
- `npx vitest run`：15 文件 106 用例全过
- 插件行为核验（本地源码）：`check()` 签名与等版本返回 null（`tauri-plugin-updater-2.10.1/src/updater.rs:530-538`）；`DownloadEvent` 类型定义；plugin-os `platform()` 同步实现
- 未复跑 cargo check / clippy / fmt / cargo test / eslint / prettier（提交说明已验证，且本次审查未发现会影响其结果的写法）

## 总体评价

实现质量高：插件 API 用法、配置 schema、capability 最小化、CI fail-fast 逐项核验均符合规范与 F009 意图，状态机分层与注释清晰。最需要优先处理的两件事：一是 🟡1（StrictMode 下启动检查失效，影响 dev 可信度），二是 🟡3（版本号三处同步缺 CI 断言，是发版链路上唯一可能「静默失败且后果不可逆」的点）——两者改动都很小，建议随本 PR 落掉。另注意：全部改动当前未提交且位于 `main`，按 AGENTS.md 约定 commit 前应先创建 `feature/f009-auto-update` 分支（F009 文档与 plan.md 均以此分支名为准）。

## 修复记录（审查后跟进，2026-08-30）

- 🟡1–🟡5 全部修复：StrictMode 启动检查（started 标志移入定时器）、`check({ timeout: 15_000 })`、CI tag↔package.json 版本 fail-fast 断言、`installInFlight` 并发守卫 + Check 按钮 busy 扩展到 downloading/ready、新增 `updaterStore.test.ts`（vitest 106 → 111）
- 🟢① 已修：`pendingUpdate` 替换前与安装成功后调用 `close()` 释放 Rust 侧句柄（失败保留以便重试）
- 🟢④ 已修：latest.json 的 macOS URL 改为从 sig 文件名派生，与 linux/windows 一致
- 🟢⑤ 标题部分已修：error 阶段弹窗标题改为 "Update check"；v 前缀假设保留（当前发版约定明文）
- 🟢⑦ 已补文档：plan.md 遗留观察项增加 Windows NSIS install 语义与 `releases/latest` 不含 prerelease 两条
- 🟢②③⑥⑧ 暂不处理（收益低于改动成本），理由见审查对话
- 修复后复验：tsc / eslint / prettier（含 build.yml）/ vitest 111 全绿
