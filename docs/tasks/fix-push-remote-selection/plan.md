# fix-push-remote-selection · plan

> 日期：2026-08-30 · 分支：`fix/push-remote-selection`

## 问题

用户新增名为 `gitlab` 的 remote 后无法从 UI 推送到它。

## 根因

**后端没有写死 origin**：`cmd_fetch/pull/push` → `use_cases` → infra 层全链路接受 `remote` 参数，仅参数为 `None` 时回落 `"origin"`（`src-tauri/src/application/use_cases.rs` 的 `fetch`/`pull`/`push`）；ahead/behind 走 git config 的 upstream 解析，天然支持任意 remote。

**元凶在前端**：

1. **push 弹窗硬编码**：`src/components/ActionBar.tsx` 的 `pushDialog` state 没有 remote 字段，确认按钮写死 `remote: "origin"`，"to" 显示也写死 `origin/${branch}` 兜底。pull 弹窗已有完整 remote 下拉，push 没有。
2. **无参 fetch 隐式只 fetch origin**：工具栏 Fetch 按钮（`useRemoteSync.ts`）、命令面板（`CommandPalette.tsx`）、自动刷新（`useAutoRefresh.ts`）都不传 remote。
3. **状态区文案写死 origin**：`status.json` 的 fetching/pushing/fetched/pushed/pulled 硬编码 "origin"。

## 方案

### A. push 弹窗支持选择 remote（仅 `ActionBar.tsx`）

1. `pushDialog` state 加 `remote: string` 字段
2. `openPushDialog()` 从 `wc.data?.upstream` 按第一个 `/` 拆初始 remote（复用 pull 弹窗推断逻辑），无 upstream 回落 `"origin"`
3. `remotesQuery.enabled` 放宽为 pull 或 push 弹窗打开时启用
4. "remote 列表到达后校正 seed" 的 useEffect 扩展为同时校正 `pushDialog.remote`；`remoteOptions` 兜底同步覆盖
5. 弹窗 body：remote 行改 Select 下拉（复用 pull 弹窗模式），"to" 行动态显示 `${remote}/${branch}`
6. 确认按钮改为 `remote: pushDialog.remote`

后端零改动。

### B. fetch `None` 语义改为 fetch 所有 remotes（`use_cases.rs::fetch`）

`remote: None` 时遍历 `infra_list_remotes` 逐个 fetch：任一失败返回第一个错误，全部成功返回 Ok，无 remote 时静默成功。前端无参调用点零改动自动获益；`RemotesPanel` 的按具体 remote fetch 不受影响。

### C. 状态区文案参数化 remote + 允许换行最多 2 行

1. `syncStore.ts`：state 加 `activeRemote`，`startOp(op, remote?)` 存储，`operationLabel(op, remote)` 插值；`endOp` 清理
2. `useRemoteSync.ts`：push/pull 从 mutation `variables` 取 `options?.remote` 传 `startOp` 与成功/失败消息；fetch 为 fetch-all 语义，文案不带 remote
3. `status.json`（en + zh-CN）：`pushing`/`pushed`/`pulled` 加 `{{remote}}` 插值；`fetching`/`fetched` 改通用文案；失败文案已不含 origin，不动
4. `SyncStatusArea.tsx`：文本 `truncate` 改 `line-clamp-2`（允许换行、最多 2 行）

## 测试

- Rust 单测：两个 remote 时 `fetch(None)` 全部更新；无 remote 不报错；全量 `cargo test` 全绿
- 前端：`tsc` + `lint` + 既有测试

## 验收清单（用户真机）

1. 仅有 gitlab remote 的仓库能从 push 弹窗推送成功
2. push 弹窗默认选中 upstream 指向的 remote，可切换，"to" 行如实显示 `gitlab/<branch>`
3. 状态区显示"正在推送到 gitlab…/已推送到 gitlab"，长文案最多 2 行
4. 自动刷新/工具栏 Fetch 后 gitlab 远端分支列表更新
