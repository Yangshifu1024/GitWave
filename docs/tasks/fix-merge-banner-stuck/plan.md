# fix-merge-banner-stuck · 解决完冲突后冲突提示框不消失

> 状态：已修复（已过 review，待冒烟；review 结论：无严重问题可合入）
> 需求（用户，bug 报告）：顶部中间的冲突提示框，解决完毕冲突后依然不会消失，
> 只能点击 abort merge 来关闭。

## 根因（后端，非前端刷新问题）

冲突 banner 的显隐由 `useMergeConflicts` 每 3s 轮询的 `mergeInProgress` 驱动；
后端唯一判定是 `.git/MERGE_HEAD` 是否存在（`conflict.rs:153-155`）。前端链路
（解决后 `refresh()`、无条件 3s 轮询、banner 只看 `active`）均无问题。

问题出在结束 merge 的唯一正常出口——Working Copy 提交
（`src-tauri/src/infrastructure/git/working_copy.rs:205-246` `commit()`）：

1. **提交后不清理 merge 状态**：libgit2 的 `Repository::commit` 与 git CLI 不同，
   提交后不会清除 MERGE_HEAD；代码库其他收尾点都调用了 `repo.cleanup_state()`
   （merge.rs:239、revert.rs ×3、interactive_rebase.rs:128），唯独手动 commit 路径
   漏了 → MERGE_HEAD 永存 → `is_merge_in_progress` 恒真 → banner 永不消失，唯一
   能删 MERGE_HEAD 的用户入口只剩 abort merge。
2. **提交不是合法的 merge commit**（同函数、更严重）：parents 只含 HEAD
   （`working_copy.rs:241`），MERGE_HEAD（被合并分支 tip）没有成为第二个 parent
   → 被合并分支不在祖先里，历史错误，push / 后续 merge 会出问题。
3. **边界**：「空提交拒绝」（tree == HEAD tree 时报 nothing to commit）同样适用于
   merge 提交——全 "ours" 解决冲突时树等于 HEAD 树，用户无法用提交结束 merge。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 修复位置 | 仅 `working_copy.rs::commit()` | 三个缺陷同一函数；abort_merge / merge_branch 路径行为不变 |
| 第二 parent | MERGE_HEAD 存在时读取其 oid，`find_commit` 后加入 parents | 文件级读取（与 conflict.rs 对 MERGE_* 的既有文件操作风格一致，不依赖引用 dwim）；MERGE_HEAD 损坏/不可解析时按 git 错误传播 |
| 状态清理 | 提交成功后 `let _ = repo.cleanup_state()`（best-effort） | 与其余 5 处 cleanup_state 调用点惯例一致；清除 MERGE_HEAD / MERGE_MSG / MERGE_MODE |
| 空提交检查 | merge 提交豁免「tree == HEAD tree」拒绝 | git CLI 语义：MERGE_HEAD 存在时即使树相同也允许 commit（全 ours 解决仍需记录第二 parent）；unborn HEAD 分支检查不受影响 |
| 未全解决即提交 | 不新增检查 | `index.write_tree()` 遇未解决冲突条目本身会报错，天然防护（既有行为） |
| banner 消失时机 | 提交成功后 ≤3s（依赖既有轮询） | `useMergeConflicts` 是 useState + 3s setInterval，不走 react-query；commit 成功的全量 invalidateQueries 不触达它。3s 设计内延迟，可接受，不为此加重联动机制 |
| 范围外 | `useMergeConflicts.ts:27,38,41` 读 repoId 但只传 workspaceId | 多仓库 workspace 下轮询目标是后端 active repo——与用户报告无关的既有问题，记录不动 |
| 回归测试 | `test_helpers.rs` 新增 `build_conflicted_merge()`；`working_copy.rs` 测试新增 2 例 | 复用 conflict.rs 既有测试的场景构造模式（merge_branch 制造真实冲突） |

## 改动清单

- `src-tauri/src/infrastructure/git/working_copy.rs`：`commit()` 支持 merge 收尾
  （第二 parent + 提交后 cleanup_state + 空树豁免），更新 doc comment
- `src-tauri/src/infrastructure/git/test_helpers.rs`：新增 `build_conflicted_merge()`
- `working_copy.rs` 测试：新增 2 例（merge commit 双 parent + 状态清理；全 ours
  解决后树等于 HEAD 仍可提交）

## 测试

- `cargo test`（新增 2 例）+ 前端 `npm run typecheck` / `lint` / `test` 不受影响
- 手动冒烟要点：
  - 制造冲突 merge → ConflictPanel 逐个 Mark resolved → banner 变「all conflicts
    resolved, commit to finish」→ Working Copy 提交 → ≤3s banner 消失
  - 提交后的 commit graph 中该提交为双亲 merge commit，被合并分支可达
  - 全 "ours" 解决（结果与 HEAD 相同）→ 提交仍成功，banner 同样消失
  - abort merge 路径不回归
