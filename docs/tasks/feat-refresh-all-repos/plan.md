# feat-refresh-all-repos · 自动刷新扩展为「每 N 分钟刷新 workspace 内所有仓库」

## 需求

现有设置项「自动刷新」（`docs/tasks/feat-auto-refresh/plan.md`）固定每 60s 刷新一次，但刷新范围仅限当前 active 仓库（后端 `fetch` 只解析 `ws.last_active_repo_id`）。需求：

1. 间隔从固定 60s 改为可配置的 **N 分钟**（输入框配置）。
2. 刷新范围扩展为当前 workspace 内的 **所有仓库**（各自 `git fetch` 远程 + 失效缓存）。
3. N 默认为 **5**，范围 **1–1440**；保留原启用复选框。

## 决策

| 项 | 结论 |
|---|---|
| 刷新语义 | workspace 内每个仓库执行 `git fetch`（best-effort），再 `invalidateQueries`；active 仓库面板立即重读，其余仓库切换时即为最新 |
| 启用方式 | 保留复选框；其下新增分钟数字输入 |
| N 默认/范围 | 默认 5，最小 1，最大 1440 |
| 手动刷新 | ⌘R / Ctrl+R / 工具栏 Fetch **保持现状**：仅 active 仓库 |
| 失败处理 | best-effort：跳过失败仓库继续，状态区汇总 `ok/total` |
| 认证 | 后台自动刷新不带 auth、不弹凭据框；需要认证的仓库计入 failed |
| 文档流程 | 只产出工程 plan（跳过 PM 的 `docs/pm/features/F*.md` 提案） |

## 关键约束 / 风险

- 后端 `workspace_sync_lock` 是**非可重入** `std::Mutex`（`use_cases.rs:2514`），不能在已持锁时递归调用现有 `fetch` → 必须抽出持锁内的 per-repo helper。
- `run_sync_op` 硬编码 `sync_ops::SYNC_OP_TIMEOUT = 180s`（`lib.rs:1425`）。多仓库串行 fetch 可能超时，且超时会取消整个操作、丢失 best-effort 语义 → 需为该命令放宽总时限。
- `SYNC_OP_TIMEOUT` 仍保留给单仓库网络操作；新命令使用独立常量。

## 方案

### 后端

1. **`src-tauri/src/application/use_cases.rs`**
   - 抽出 `fn fetch_repo_remotes(repo, remote, on_progress, cancel, auth, request_id) -> Result<()>`，承载现 `fetch` 中 `let Some(remote) else { ... }` 的分支逻辑（多 remote best-effort、cancel 立即中止、`FETCH_AUTH_FAILED` 立即中止语义不变）；`fetch` 改为「持锁 + `active_repo_path` + 调用 helper」，对外行为完全不变。
   - 新增：
     ```rust
     pub struct WorkspaceFetchSummary {
         pub total: usize,
         pub succeeded: usize,
         pub failed: usize,
     }

     pub fn fetch_workspace_repos(
         ctx: &AppContext,
         workspace_id: &str,
         on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
         cancel: Option<CancelFlag>,
         auth: Option<InlineAuth>,
         request_id: &str,
     ) -> Result<WorkspaceFetchSummary>
     ```
     - 持 `workspace_sync_lock(workspace_id)` **一次**；`list_repos(ctx, workspace_id)`（内部已 sweep presence）取全部仓库，跳过 `status == Missing`。
     - 逐个 open + `fetch_repo_remotes`：`Ok → succeeded += 1`，`Err → failed += 1` 并继续；`cancel` 置位则停止循环，返回当前 summary。
     - `on_progress` 不是 `Clone`，用 `Arc<Mutex<_>>` 逐仓库重新 box（沿用现多 remote 写法）。
   - 单测（复用 `fresh_ctx` / `build_linear_repo` / `add_local_repo` / `set_active_repo` / `cleanup`）：
     - **两个仓库各自 origin 有新 tip** → 两个 `refs/remotes/*/main` 都前进；
     - **一个仓库 remote 坏、另一个好** → summary `{ total: 2, succeeded: 1, failed: 1 }`，好的仓库仍前进；
     - **Missing 仓库被跳过**（不进 total / 不报错）。

2. **`src-tauri/src/application/sync_ops.rs`**
   - 新增 `pub const WORKSPACE_FETCH_TIMEOUT: Duration`（建议 300s，覆盖多仓库串行 fetch）。

3. **`src-tauri/src/lib.rs`**
   - `run_sync_op` 增加 `timeout: Duration` 参数（替换内部对 `SYNC_OP_TIMEOUT` 的直接引用）；7 处既有调用点补传 `sync_ops::SYNC_OP_TIMEOUT`。
   - 新增 `cmd_fetch_workspace_repos(app, ctx, workspace_id, request_id)`：仿 `cmd_fetch`（emit `sync-progress`、`emit_storage_outcome`），传 `WORKSPACE_FETCH_TIMEOUT`，返回 `WorkspaceFetchSummary`；无 `remote` / `auth` 参数。
   - 注册进 `invoke_handler`（`lib.rs:1968` 附近）。

### 前端

4. **`src/lib/api.ts`**
   - 类型 `WorkspaceFetchSummary { total: number; succeeded: number; failed: number }`。
   - `fetchWorkspaceRepos(workspaceId, options?: { requestId?: string }): Promise<WorkspaceFetchSummary>` → `invoke("cmd_fetch_workspace_repos", ...)`。

5. **`src/stores/autoRefreshStore.ts`**
   - 新 key `gitwave-auto-refresh-interval`（保留原布尔 key `gitwave-auto-refresh`，既有偏好无缝迁移）。
   - 导出 `DEFAULT_INTERVAL_MINUTES = 5` / `MIN_INTERVAL_MINUTES = 1` / `MAX_INTERVAL_MINUTES = 1440` 与纯函数 `sanitizeIntervalMinutes(raw: unknown): number`（解析失败回退默认值，越界 clamp）。
   - state 增 `intervalMinutes` + `setIntervalMinutes`（写入前 sanitize）。

6. **`src/hooks/useAutoRefresh.ts`**
   - `useAutoRefresh()` 返回值增加 `intervalMinutes` / `setIntervalMinutes`。
   - `useRefreshRepo()` **保持不变**（manual 路径，仅 active 仓库）。
   - 抽出共享本地刷新体（`bumpHistoryEpoch()` + `queryClient.invalidateQueries()`），新增 `useRefreshWorkspaceRepos()`：
     - `startOp("fetch", null, requestId)` → `fetchWorkspaceRepos(activeWorkspaceId, { requestId })` → 成功后再本地重读一次 → 按 summary 设状态：
       - `failed === 0` → `status.sync.refreshedAll`（`{ count: succeeded }`）
       - `failed > 0` → `status.sync.refreshedAllPartial`（`{ ok, total, failed }`）
     - 无 active workspace / `sync.isBusy()` → 仅本地刷新，报 `status.sync.refreshed`；
     - 异常/cancel 处理沿用现 `useRefreshRepo` 语义；`finally` 中 `endOp("fetch", requestId)`。
   - `useAutoRefreshLoop()`：改用 `useRefreshWorkspaceRepos`；`const intervalMs = intervalMinutes * 60_000`；effect 依赖加 `intervalMinutes`；保留 tick 时对 store 的实时复核（`useAutoRefreshStore.getState().autoRefresh`）。

7. **`src/components/SettingsModal.tsx`**（`GeneralSection`）
   - 保留复选框；其下新增 `Input type="number"`（`disabled={!autoRefresh}`）+ 标签与范围说明。
   - 用本地字符串 state 承接输入，`onBlur` / Enter 时经 `sanitizeIntervalMinutes` 提交（避免逐字符 clamp 导致清空跳 1），提交后回填规范化值。

8. **i18n（`en` + `zh-CN`，`src/i18n/locales/parity.test.ts` 强制成对）**
   - `settings.general`：
     - 改 `autoRefreshCheckbox`：`Refresh repository data automatically` / `自动刷新仓库数据`
     - 新增 `autoRefreshIntervalLabel`：`Interval (minutes)` / `刷新间隔（分钟）`
     - 新增 `autoRefreshIntervalHint`：`1–1440 minutes` / `1–1440 分钟`
     - 更新 `autoRefreshHint`：明确「刷新 workspace 内所有仓库的提交、分支与面板，并向各远程 fetch；绝不自动 pull 或 push」
   - `status.sync`：
     - `refreshedAll`：`Refreshed {{count}} repositories` / `已刷新 {{count}} 个仓库`
     - `refreshedAllPartial`：`Refreshed {{ok}}/{{total}} repositories ({{failed}} failed)` / `已刷新 {{ok}}/{{total}} 个仓库（{{failed}} 个失败）`

### 文档

9. 更新 `docs/tech/architecture/00-overview.md:56`：「60s 轮询（`useAutoRefresh`）」→「每 N 分钟（可配置，默认 5）轮询 workspace 内所有仓库（`useAutoRefresh`）」。

### 不做

- 不改 `syncStore`、面板 query key（`["working-copy", ws, repo]` 等已按仓库隔离，inactive 仓库挂载时自然重取）。
- 不改 `cmd_fetch` 既有语义、手动刷新路径、working-copy 2s / merge-conflict 3s 轮询。
- 绝不 pull / push。

## 验证清单

- [x] `pnpm format:check` · `pnpm lint` · `pnpm typecheck` · `pnpm test`（188 passed）
  - 扩展 `src/stores/autoRefreshStore.test.ts`：默认 5、持久化读取、`sanitizeIntervalMinutes` 低/高/NaN 回退、持久化
- [x] `cargo fmt -- --check` · `cargo clippy --all-targets -- -D warnings` · `cargo test --all-targets`（352 passed）
  - 新增 `fetch_workspace_repos` 三项单测（见上）
- [ ] 手动（HMR）：设 N=1，两仓库各自 origin 推进 → 状态区汇总、切换两仓库均为最新
- [ ] 手动：一个仓库 remote 坏 / 断网 → 其他仓库照常刷新，状态区显示 partial、不弹认证框
- [ ] 手动：修改 N 后定时器立即按新间隔重建；取消勾选即停止
