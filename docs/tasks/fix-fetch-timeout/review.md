# fix-fetch-timeout · 代码审查报告

审查人：code-reviewer（按 `.opencode/agents/code-reviewer.md` 章程，7 维度）
对象：worktree `D:/Code/GitWave-fetch-timeout-fix` 未提交改动（fix/fetch-timeout，
基于 fix/fetch-gcm-credential-prompt 叠加）。

## 结论：可合入（无 🔴）

## 已核对的重点路径（无缺陷）

- **CancelGuard 生命周期**（`application/sync_ops.rs`）：guard 移入阻塞闭包，
  注册期与操作真实生命周期精确一致；超时后闭包仍在收尾时标志依然可触达；
  闭包 panic 经 unwind 正常注销。
- **超时 detach 语义**（`lib.rs::run_sync_op`）：tokio spawn_blocking 任务本就
  不可 abort，超时分支只置标志并立即返回，闭包在下一个 libgit2 检查点自行
  退出并释放 workspace fetch 锁；`Result<Result<T, AppError>, JoinError>`
  展平正确。
- **取消与凭据存储交互**（`credentials.rs`）：取消杀掉 `fill` →
  `FillOnce.answer` 为 None → `approve/reject` no-op，不会替被取消的 prompt
  误存/误清凭据（已有测试锁定）。
- **取消/完成竞态**：`cancelled_if_flagged` 只改写 Err 结果，已成功操作不被
  污染；迟到 `sync-progress` 事件被 `fading`/`activeOp===null` 守卫拦截。
- **子进程处理**（`process.rs`）：入口关 stdin（EOF 语义与 `Child::wait`
  一致）、超时/取消 kill+reap 无僵尸、输出仅退出后读（载荷小 + kill 兜底，
  无管道死锁）。
- 锁 poison 恢复三处一致；`Ordering::Relaxed` 对单向翻转标志足够；
  i18n en/zh-CN 键 parity 与 `seconds` 插值双向一致。

## 🟡 发现并已修复

1. **auto-refresh 取消展示不一致**（`src/hooks/useAutoRefresh.ts:89`）：
   ⌘R / 自动刷新的 fetch 取消后以红色 danger 显示「已取消」，与手动同步
   路径的中性处理不一致。→ 已改为复用 `isCancelledSyncError`，中性 info 展示。
2. **测试缺口**：
   - 多 remote「取消即断」无守护 → 新增
     `fetch_all_remotes_stops_immediately_when_cancelled`（use_cases.rs），
     断言整批以 `git.sync_cancelled` 失败且后续 remote 未被 fetch。
   - `isCancelledSyncError` 无断言 → 新增 `src/lib/api.test.ts`（正/负用例）。

## 🟢 采纳的优化

- `sync_ops.rs`：guard 注销后空 Vec 连同 workspace key 一并从注册表移除。
- `SyncStatusArea.tsx`：取消判断改为类型安全的 `isCancellableOp` 谓词（替代
  `Set<string>`，拼写错误可被编译器捕获）；`cancelSync` 失败不再完全静默
  （`console.warn`）；注释修正为如实描述 branch-delete 流程混含本地/远端
  步骤故暂不纳入可取消集。

## 🟢 未采纳（记录备查）

- pull 的本地集成阶段（merge/rebase/checkout）不加取消检查点：网络数据已
  到手、本地操作快且不损坏数据；fetch 阶段取消经 Err 分支正确恢复 stash
  （已有测试覆盖）。接受现状。
- aria-label 的英文硬编码（`Cancel operation`）：与既有 `ProgressBar`
  `"Syncing"` 同模式，非本次引入；后续随 i18n 统一处理。
- `"delete"`（含远端删除）纳入取消按钮：见上，其网络分支已有 180s 超时
  兜底，后续可单独评估。

## 验证

- `cargo fmt` / `cargo clippy --all-targets -- -D warnings` 全绿。
- `cargo test --all-targets`：224 passed / 2 ignored（基线 216 + 新增 9：
  process 3、sync_ops 3、remote 取消 1、use_cases 取消即断 1、api 2 为前端）。
- 前端 `tsc --noEmit`、`vitest run`（123 passed，含 locale parity）、
  eslint、prettier 全绿。
