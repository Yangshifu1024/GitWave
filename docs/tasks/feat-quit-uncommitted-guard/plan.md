# feat-quit-uncommitted-guard · 技术方案

> 需求：在关闭程序时，如果当前打开的仓库有未提交的改动，需要弹框提示用户。
> 修订（用户意见）：判定范围为**当前 Workspace 中打开的全部仓库**，任一 dirty 即弹框。

## 需求范围

- 触发时机：真正退出应用时（窗口关闭按钮、File → Exit）。macOS 红点仅隐藏窗口的路径不触发。
- 判定范围：当前 active Workspace（`useWorkspaceUiStore.activeWorkspaceId`）的**全部仓库**，逐一检查未提交改动（含 staged / unstaged / untracked），任一 dirty 即弹框；全部 clean 直接退出。
- 查询失败或整体超时（500ms）：**不拦截，直接退出**——提示是保险，绝不能卡住退出。
- 弹框行为：「取消」关闭弹框回到界面（进程不退出）；「仍要退出」真正退出，**不做任何 git 操作**，改动原样保留。

## 文件级改动点

### 1. Rust 后端

- `src-tauri/src/application/use_cases.rs`
  - 新增 `DirtyRepoSummary { repo_id, nickname, path, file_count }`（serde 序列化，snake_case 对齐现有类型）。
  - 新增 `get_dirty_repos(ctx, workspace_id) -> Vec<DirtyRepoSummary>`：复用 `workspaces.list_repos()` 拿全部仓库，逐仓 `ctx.open_repo()` + `infra_wc_status()` 判定 `files.len() > 0`；missing / 打开失败 / status 失败的仓库**跳过**（不拦截）。
- `src-tauri/src/lib.rs`
  - 新增 `#[tauri::command] cmd_get_dirty_repos`，注册到 handler（紧邻 `cmd_get_working_copy`）。
  - `quit_app` 保留，但前端退出统一走拦截路径（见下）。

### 2. 前端 API

- `src/lib/api.ts`：新增 `DirtyRepoSummary` 接口 + `getDirtyRepos(workspaceId)` 封装。

### 3. 前端拦截与弹框

- 新建 `src/hooks/useQuitGuard.ts`
  - `App.tsx` 挂载一次：`getCurrentWindow().onCloseRequested(async (e) => { e.preventDefault(); ... })`。
  - 逻辑：读 active workspace → `getDirtyRepos`（500ms 总超时）→ 有 dirty 则打开确认框；无 / 失败 → `destroy()` 直接退出。
  - 暴露 `requestQuit()`：供菜单退出等场景复用同一拦截流程；暴露模块级事件或由 App 层受控渲染 Modal。
- 新建 `src/components/QuitConfirmModal.tsx`
  - 复用 `src/components/ui/Modal.tsx`（参考 `AboutModal.tsx` 模式），size="sm"。
  - 内容：Workspace 名称 + dirty 仓库列表（每行：仓库名 + 改动文件计数）。
  - 按钮：「取消」（默认焦点）/「仍要退出」。
- 修改 `src/App.tsx`：挂载 `useQuitGuard()` + 渲染受控 `<QuitConfirmModal>`。
- 现有前端 `quit_app` 调用点改走统一 `requestQuit()`。

### 4. i18n

- `src/i18n/locales/en/app.json` + `zh-CN/app.json`：新增退出确认 key（标题、正文、取消、仍要退出、文件计数），双语齐全通过 `parity.test.ts`。

## 风险与回滚

- Modal 在 `onCloseRequested` preventDefault 后渲染：窗口仍存活，理论可行；若实测有渲染问题，降级为原生 `dialog.ask()`（capability 已具备）。
- 多仓库逐一 status 在仓库较多时可能慢：由 500ms 总超时兜底，超时放行退出。
- 回滚：改动约 7 个文件，单提交 git revert 即可；无新依赖、无数据迁移。

## 验证方式

1. `cargo check`（src-tauri）+ `npm test`（含 i18n parity）+ `npm run lint`。
2. 手动验证清单：
   - 多仓库 Workspace（1 个 dirty + 1 个 clean）→ 关窗 / File → Exit 均弹框，且只列出 dirty 仓库。
   - 全部 clean → 直接退出，无弹框。
   - 「取消」→ 弹框关闭、回到界面、进程存活；commit / stash 后再退出不再弹框。
   - 「仍要退出」→ 应用退出，改动原样保留在磁盘。
   - 仓库 missing / status 失败 → 跳过该仓库，不拦截。
   - 无 active workspace → 直接退出，不弹框。

## 状态

- 提案：已接受（用户批准，2024 修订版 v2）
- 实施：见 `review.md`（开发完成后补充）
