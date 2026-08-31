# fix-fetch-timeout · 修复方案

## 现象

fetch（以及同样走网络传输的 pull / push / 删远端分支）没有任何超时限制：

网络挂起（死链路、无响应代理）、或 GCM 凭据弹窗无人应答时，操作永远不结束——
状态区停留在「正在获取…」，fetch/pull/push 按钮永久禁用。经 `use_cases::fetch`
的 per-workspace `workspace_fetch_lock` 放大：60s 自动刷新与后续所有手动同步
在该 workspace 上排队死锁，只能重启应用。

追加需求（用户在方案评审时提出）：进行中的同步操作要能**主动取消**——状态
指示区在进行中状态显示取消按钮。

## 根因（tester 分析）

同步操作全部走 git2（libgit2）同步阻塞调用，链路上四层都无超时/取消：

1. **命令层**：`lib.rs` 的 `cmd_fetch` / `cmd_pull` / `cmd_push` 直接
   `.await` `spawn_blocking`，libgit2 挂起则 future 永不完成，UI 永远 busy
   （`cmd_delete_remote_branch` 更是直接在 async 命令里跑阻塞调用，占用
   runtime worker 线程）。
2. **libgit2 无 socket 超时**：库本身不暴露连接/读超时；唯一的传输中止
   手段是 `transfer_progress` 回调返回 `false`，但
   `remote.rs::attach_transfer_progress` 硬编码返回 `true`。
3. **凭据子进程无限等待**：`credentials.rs::query_helper` 的
   `git credential fill` 用 `wait_with_output()` 等待——GCM 的 GUI 弹窗由
   GCM 自己的设置控制（`GIT_TERMINAL_PROMPT=0` 管不到），用户不点弹窗时
   子进程和整条 fetch 无限挂起（Windows 上最常见的实际场景）。
   `notify_helper`（approve/reject）同样无限等待。
4. **无取消入口**：后端没有任何针对网络操作的 cancel 命令，前端也没有
   取消 UI。

参照先例：`infrastructure/ai/provider.rs` 的 reqwest 客户端固定 60s 超时
（注释明确写着 "a hung provider must not freeze the UI"）——同样的意识
此前没有落到 git 网络操作上。

## 修复方案

三层防线（总超时兜底 + 取消标志清理 + 凭据子进程独立超时），UI 增加取消按钮：

1. **总超时 + 取消注册表（`application/sync_ops.rs` 新模块）**
   - `SYNC_OP_TIMEOUT = 180s`：单个网络同步操作的总时限。
   - per-workspace 取消标志注册表：`register(workspace_id, flag)` 返回
     `CancelGuard`（Drop 时注销，guard 移入阻塞闭包内，注册期与操作真实
     生命周期精确一致——包括已超时返回但闭包仍在收尾的命令）。
   - `cancel_workspace_ops(workspace_id)`：翻转该 workspace 全部活动标志，
     返回是否存在活动操作。
   - `timeout_error()`：`git.sync_timeout`（Network 类，带 `seconds` 参数）。
2. **命令层（`lib.rs`）**
   - 新增 `run_sync_op` 辅助：`spawn_blocking` 外包 `tokio::time::timeout`
     （tauri 2 runtime 即 tokio）。超时 → 置位取消标志（阻塞线程在下一个
     libgit2 检查点自行中止并释放 workspace fetch 锁）→ 立即返回
     `git.sync_timeout`。
   - `cmd_fetch` / `cmd_pull` / `cmd_push` / `cmd_delete_remote_branch`
     全部改走 `run_sync_op`（顺带修复了 delete 直接阻塞 runtime worker 的
     问题）；join 错误码沿用各 `cmds.*_task_join`，新增
     `cmds.delete_remote_branch_task_join`。
   - 新增 `cmd_cancel_sync(workspace_id) -> bool`：给取消按钮用。
3. **取消检查点（`remote.rs` / `credentials.rs`）**
   - `attach_transfer_progress` 增加 `cancel` 参数：回调先查标志，置位则
     返回 `false` 中止传输（同时保留进度上报）。四个网络函数
     （fetch / push / 删远端分支 / pull 内部）全部透传。
   - 操作结果若为 Err 且标志已置位 → 映射为 `git.sync_cancelled`（用户
     取消不该伪装成 libgit2 错误）。
   - `GitCredentialHelper.with_cancel(flag)`：`fill` 等待循环同时轮询取消
     标志——取消立刻杀掉凭据子进程，不用等 fill 超时。
   - `use_cases::fetch` 多 remote 遍历在取消时立即中断，不再磨完剩余 remote。
4. **子进程超时（`process.rs` / `credentials.rs`）**
   - `wait_with_output_timeout(child, timeout, cancel)` /
     `wait_timeout(child, timeout)`：`try_wait` 25ms 轮询，超时或取消则
     kill + reap。入口即关闭 stdin（等价 `Child::wait` 的 EOF 语义）；
     输出仅在退出后读取（凭据协议载荷极小，无管道容量死锁风险）。
   - `fill` 120s、`approve/reject` 15s，超时 kill + `tracing::warn!`。
5. **前端**
   - `api.ts`：`cancelSync(workspaceId)`、`isCancelledSyncError(err)`。
   - `SyncStatusArea`：进行中且 op ∈ {fetch, pull, push} 时显示 X 取消
     按钮（本地 UI 操作 checkout/stash 等立即完成，不显示）；点击后隐藏
     防重复。
   - `useRemoteSync.handleError`：`git.sync_cancelled` 以中性 info 状态
     「已取消」展示，不算失败、不弹错误 toast。
   - i18n：`errors.git.sync_timeout` / `sync_cancelled`、
     `status.sync.cancelled`、`errors.cmds.delete_remote_branch_task_join`
     （en + zh-CN 同步，保 parity）。

## 设计取舍与已知限制

- **180s 总超时是"总量"而非"无进度"**：慢而正常的大传输超过 180s 会被
  中止（可重试）。选 180s 是增量同步通常远低于此、又不必让用户久等报错
  的折中。
- **TCP 完全无响应（如连接阶段死链路）时取消标志无检查点可触发**：阻塞
  线程与 workspace fetch 锁滞留至应用重启（libgit2 无 socket 超时 API 的
  固有限制）。命令层超时保证 UI 不卡、错误明确；最常见场景（GCM 弹窗挂
  起、传输中途停滞）已被取消标志 / fill 超时覆盖。
- 取消是"尽速"而非"瞬时"：标志在下一个 libgit2 检查点生效；极限情况下
  与总超时合并为同一个 180s 兜底。
- clone 未纳入：合法耗时长且不可预估，适合后续单独用「无进度超时」策略。

## 回归测试要点

单元测试（不触网、不触真实凭据存储；新增 7 个）：

- `process.rs`：退出型子进程正常收集输出（`git --version`）；挂起子进程
  （Windows ping / Unix sleep）在 deadline 被 kill；预置取消标志立即 kill。
- `sync_ops.rs`：注册/取消只影响目标 workspace；guard Drop 即注销；
  超时错误 Network 类 + `git.sync_timeout`。
- `remote.rs`：预置取消标志的 fetch（本地 file:// remote + 服务端新提交）
  以 `git.sync_cancelled` 失败。
- 既有 fetch/pull/push 相关测试全部改为传 `None` cancel，行为不变。

验证：`cargo fmt` / `cargo clippy --all-targets -- -D warnings` /
`cargo test --all-targets`（225 tests: 223 passed / 2 ignored）；
前端 `tsc --noEmit` + `vitest run`（121 passed，含 locale parity）。

手动验收（真机）：

- [ ] 指向不可路由地址（如 `https://10.255.255.1/x.git`）的 fetch：180s
      后状态区报「网络操作超时（180 秒）」，按钮恢复可用，可再次发起
      （临时调小 `SYNC_OP_TIMEOUT` 便于验证）
- [ ] fetch 进行中状态区出现 X 按钮；点击后立即隐藏，操作在检查点处以
      「已取消」（中性色）结束
- [ ] GCM 弹窗长时间不点：120s 后 fetch 以认证失败结束，不再无限挂起
- [ ] 正常 HTTPS / SSH 仓库的 fetch / pull / push / 删远端分支行为不变
- [ ] auto-refresh 开启：超时/取消后的下一个周期能正常恢复
- [ ] 多 remote 仓库取消：整个 fetch 立即停止，不再逐个 remote 磨完

## 分支

`fix/fetch-timeout`（worktree `D:/Code/GitWave-fetch-timeout-fix`）。

**注意**：本分支基于 `fix/fetch-gcm-credential-prompt`（012057c）叠加——
两个修复深度重叠（`query_helper`、`run_with_credentials` 调用点、
`workspace_fetch_lock` 均在后者的改动之上）。stacked PR：待 GCM PR 合入
main 后 rebase，再合入本 PR。
