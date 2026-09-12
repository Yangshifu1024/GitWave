# fix: merge / interactive-rebase 强制覆盖与路径穿越防护

状态：待实现（Batch 1，全量修复 4 批次之首，最高优）

分支：`fix/merge-rebase-dirty-guard`（从 `main` 切出，squash merge）

## 背景

源码审查发现两处 🔴 数据丢失风险与一处 🔴 路径穿越：

1. `interactive_rebase.rs:192/204` — `execute_interactive_rebase` 在"全部 drop"与"正常执行"两条路径直接
   `repo.reset(&obj, ResetType::Hard, None)`，无 dirty worktree 检查，静默丢弃未提交修改
   （`git rebase` 会拒绝脏工作区）。`continue_interactive_rebase`（353）同样无检查。
2. `merge.rs:158/184/198-208` — FF / no-ff / 3-way 三条路径全部 `CheckoutBuilder::force()` 且无脏状态预检，
   `git merge` 会拒绝的场景下本应用直接覆盖用户未提交编辑。
3. `conflict.rs:112-138` — `resolve_conflict` 对 `path` 无校验就 `wd.join(path)` + 写文件，
   `../evil` / 绝对路径 / `..\foo` 可写出工作区外。

已确认安全的 intentional-discard 站点（本批**不改**，review 时逐条复核）：
`conflict.rs:145 abort_merge`、`working_copy.rs:449/897`、`revert.rs:118`（入口已有 `ensure_clean`）、
`branch.rs` force 路径（UI 确认 + 后端 DIRTY 检查）、`remote.rs:744`（已有 `PULL_DIRTY_WORKTREE` 门禁）、
`rebase.rs:192`（`use_cases.rs:1760 worktree_is_dirty` 兜底）。

## 修复步骤

### 1.1 抽取共享 `ensure_clean`，`merge_branch` 入口设防
- 新建 `src-tauri/src/infrastructure/git/worktree_guard.rs`（或放入 `git2_adapter.rs`），把
  `revert.rs:33-44 ensure_clean` 搬过去 `pub` 化；`revert.rs` 改调共享函数（行为不变）。
- `merge.rs:130 merge_branch` 在 `behind == 0` 早退**之后**调用 `ensure_clean()?`
  （"AlreadyUpToDate"是纯 no-op，脏工作区下也应成功，贴近 git 行为），覆盖 FF / no-ff / 3-way 三路。
- 错误码复用 `codes::git::DIRTY_WORKTREE`（前端 `errors-git.json` 中英已有文案，零 i18n 改动）。
- 测试（`merge.rs:256 mod tests`，用现有 `test_helpers`）：staged / unstaged / untracked 脏 → 均
  `Err(DIRTY_WORKTREE)`；干净 → `Ok`；`behind==0` + 脏 → `Ok(AlreadyUpToDate)`。

### 1.2 `execute_interactive_rebase` 入口设防 + `continue` 脏语义
- `interactive_rebase.rs:179` 在 `resolve_upstream` 之后、首次 `reset(Hard)` 之前加 `ensure_clean()?`
 （`plan_interactive_rebase` 只读，不加）。
- `continue_interactive_rebase`（353）**不能**无条件拦截：Edit 停顿后的用户修改是正常状态。
  方案：continue 入口若脏则先 `commit_index`（沿用 edit 的 message）再 replay remaining，
  干净则直接 replay。实施前精读 220-380 行确认 continue/replay 衔接。
- 测试：execute 脏 → 拒；continue 脏 → 自动提交后继续；continue 净 → 直接继续。

### 1.3 abort 真回滚（默认包含；不同意可砍，退为只改 UI 文案）
- `PauseState`（60-64）加 `#[serde(default)] original_head: Option<String>`（兼容旧 pause 文件）；
  `execute` 落盘前（271）记录当前 HEAD；`abort_interactive_rebase_pause`（372）若有
  `original_head` 则 `reset(Hard)` 回去 + `cleanup_state()`，否则仅删文件 + `warn` 日志。
- 前端 `BranchList.tsx:682 handleAbortIrebasePause` 文案不变（行为变对）。

### 1.4 路径穿越防护统一
- `conflict.rs:112 resolve_conflict` 与 `working_copy.rs stage_paths` 加与
  `hooks.rs:53-66` / `submodule.rs:195-200` 同手法校验：拒绝对路径、`..` 路径段、Windows `..\`。
- 新增 `codes::git::PATH_TRAVERSAL`（仿 `PATH_ESCAPES_WORKTREE`）+ 前端 `en` / `zh-CN`
  `errors-git.json` 同名 key + 跑 `parity.test.ts`。
- 测试：`../evil`、`/abs/path`、`..\evil` → 均拒；正常相对路径通过。

### 1.5 前端预检（好文案；后端 guard 是最后防线）
- `MergeConfirmDialog.tsx:32` mergePreview 旁加 dirty 预检；`InteractiveRebaseDialog.tsx:69 run` 与
  `BranchList.tsx:653` rebase 前调 `gateCheckout`（仿 `useBranchCheckout.tsx:140 request()` 取数）；
  `CommandPalette.tsx:243 checkout_branch` 同样走 `gateCheckout`。
- 测试：vitest 断言脏工作区下 merge/rebase 入口弹门禁而非直接 invoke。

## 验证

- `cd src-tauri && cargo test --all-targets` + `cargo clippy` + `cargo fmt --check`
- `pnpm typecheck` + `pnpm lint` + `pnpm test`（含 i18n parity）
- 手动：脏工作区下 merge / rebase / irebase 被拒且工作区原样；Edit→Continue 全流程走一遍；
  已合并分支回退 / no-ff 空合并两个历史回归测试仍过
- 合并前调 `@code-reviewer` 出 `review.md`（7 维度）

## 实施偏差（以实现为准）

- 1.4 未新增 `PATH_TRAVERSAL` 码：复用既有 `PATH_ESCAPES_WORKTREE`（文案完全贴合，零 i18n 改动）；
  另把 `discard_worktree_changes` 内联校验收敛到共享函数（行为一致，空串路径错误码由 GIT_ERROR 变为
  PATH_ESCAPES_WORKTREE，更准确）；`reject_escaping_syntax` 额外拒前导 `/`
 （Windows `is_absolute` 不认 `/abs/path`）。
- 1.5 BranchList rebase 走定向脏预检（非完整 gateCheckout：rebase 无分支占位语义）；
  UI 胶水逻辑无 vitest（typecheck + lint + 手动覆盖）。
- review 驱动的追加修复：Edit arm 恒写 pause（修 Edit-last 残留过期 pause 致 Continue 重放）；
  `execute` 内 squash/fixup 首项预校验（reset 之前）；`pauseCleared` 文案同步为"中止并恢复"；
  前端 working-copy queryKey 与 ActionBar 正典对齐（带 activeRepoId）。
