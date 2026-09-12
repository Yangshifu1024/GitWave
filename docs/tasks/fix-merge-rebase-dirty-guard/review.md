# 审查报告：fix/merge-rebase-dirty-guard

范围：`git diff main --stat` 12 个文件 + untracked `worktree_guard.rs`，对照 `plan.md` 1.1–1.5。

## ✅ 优点

- `merge.rs:152` guard 在 `behind == 0` 早退之后，三路（FF/no-ff/3-way）全覆盖；5 个测试断言 HEAD 未动、工作区原样、无 MERGE_HEAD 残留。
- `revert.rs` 改调共享 `ensure_clean`，行为逐行一致；`ensure_clean` 语义与旧实现相同（含 untracked）。
- 路径守卫覆盖空串、前导 `/`、`is_absolute`、`/` 与 `\` 下的 `..` 段；`conflict.rs` / `working_copy.rs` 各 1 个新测试均含 `..\win` 用例。
- `continue` 三态（staged→amend / unstaged+untracked→拒并保留 pause）比 plan 1.2 更保守；`PauseState.original_head` 经 `#[serde(default)]` 兼容旧文件。
- 前端 run 路径均做 fresh 复检、按钮禁用；`blockedByDirty` 排除 up-to-date（与后端 no-op 一致）；en/zh `dirtyWarning_one/_other` 对齐。

## 🔴 严重问题（必须修复）

- **位置**：`interactive_rebase.rs` `execute` 预写 pause（约 275 行）+ `Edit` 分支 `if !remaining.is_empty()`（约 347 行）
- **描述**：Edit 为最后一条 todo 时，预写文件（= 全量 todos）未被重写，返回 `PausedForEdit` 却留下过期 pause；用户 Continue 会把已落盘的 Edit 提交重放一遍（重复提交或误冲突）。旧代码 fail-closed（无文件→`NOT_PAUSED`），新代码 fail-open。
- **建议**：Edit 分支去掉 `is_empty` 条件恒写 pause（含 `remaining = []`，Continue 将直接走完返回 Clean），或 `else` 分支删文件。

## 🟡 一般问题（建议修复）

- **位置**：`merge.rs` tests；`interactive_rebase.rs` tests
- **描述**：plan 承诺的用例缺两块：merge 缺 staged-only 脏测试（仅覆盖 unstaged/untracked）；continue 缺干净工作区直接继续测试。
- **建议**：各补 1 个用例（staged 写后 `add_path` 不改工作区；clean-continue 断言 `Clean` 且 pause 清除）。

- **位置**：`interactive_rebase.rs` `replay_todos` 三处 `restore_original` + `continue` 旧 pause 路径
- **描述**：冲突后返回 `Ok(Conflicts)`，但 worktree 已 Hard-reset 回原 tip 且 pause 已删——消息指出的"冲突文件"在盘上并不存在、不可继续，与结果语义矛盾；旧 pause（`original=None`）的 continue-冲突只删文件不回滚，留下半 rebase 无恢复入口。
- **建议**：冲突恢复后改返 `Err(APPLY_CONFLICT)`（说明已自动回滚），或在文档/前端明确"auto-aborted"；`original=None` 时冲突路径保留 pause 不删并补测试。

- **位置**：`interactive_rebase.rs` `replay_todos` squash 首项校验（`i == 0`）
- **描述**：校验发生在 Hard reset + 预写 pause 之后，非法输入也会搬 HEAD 并残留 pause（新老代码皆然，新代码多残留一个文件）。
- **建议**：`execute` 内先做纯内存校验（squash/fixup 不为首、oid 可解析），再 `ensure_clean`、再落盘/reset。

- **位置**：`MergeConfirmDialog.tsx:38`、`InteractiveRebaseDialog.tsx:54`
- **描述**：queryKey 用 `["working-copy", workspaceId]`，与正典 `["working-copy", workspaceId, repoId]`（`useWorkingCopy`/`ActionBar`）不一致，形成独立缓存项；前者缺 `enabled`，对话框关闭时仍请求。
- **建议**：统一 key（含 repoId，如组件拿不到则显式注明），`MergeConfirmDialog` 加 `enabled`。

- **位置**：`BranchList.tsx:653`、`CommandPalette.tsx:244`；plan 1.4/1.5
- **描述**：plan 偏差四处：① BranchList rebase 用临时 dirty 检查而非 `gateCheckout`（缺 mergeInProgress/rebasePaused 阻塞）；② CommandPalette `gateCheckout` 写死 `isCurrent:false`、`occupiedWorktree:null`；③ 1.5 承诺的 vitest 门禁测试缺失；④ 1.4 的 `PATH_TRAVERSAL`+`errors-git.json`+parity 未做（复用 `PATH_ESCAPES_WORKTREE` 本身合理，已有中英 copy，但 plan 未同步）。
- **建议**：补 vitest；CommandPalette 传入真实 isCurrent/occupancy；plan 1.4 改为"复用既有码"，1.3 的"文案不变"同步为已变更的 `pauseCleared`。

- **位置**：`working_copy.rs` `discard_worktree_changes`（重构）
- **描述**：空串路径错误码由 `GIT_ERROR` 变为 `PATH_ESCAPES_WORKTREE`（旧流程能过两道语法检查、倒在 `status_file`；新流程首道即拒）。更合理但属行为变更。
- **建议**：在 `review.md`/PR 描述注明；如有依赖旧码的前端映射需同步（经查无）。

## 🟢 优化建议（可选）

- **位置**：`worktree_guard.rs:79` `starts_with`
- **描述**：词法比较不解析 symlink，`link_to_outside/ok.txt`（无 `..`）可写出工作区；与 `hooks.rs`/`submodule.rs` 同手法同局限。
- **建议**：注释注明该局限；或对已存在父目录做 `canonicalize` 二次校验。

- **位置**：`worktree_guard.rs:42` `classify_dirty` bitflags
- **描述**：`union`/`contains`/`intersects` 用法正确；`CONFLICTED` 归入 unstaged 使 continue 在冲突中途被拒，语义恰好。
- **建议**：无，保持；可在注释加一句说明 CONFLICTED 归类意图。

## 📝 总体评价

整体质量高：后端守卫位置准确、三路与三态覆盖完整，测试断言了"拒后原样"这一关键性质。优先修复 🔴 的 Edit-last 过期 pause（可致重复提交），再补齐 🟡 的缺失用例与 plan 偏差说明；前端 key 不统一与 gate 硬编码属低风险但应在合并前收敛。
