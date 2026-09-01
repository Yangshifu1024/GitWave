# fix-merge-banner-stuck · 代码审查报告

> 审查人：code-reviewer（重点维度：正确性 / git 数据完整性，7 维度全覆盖）
> 审查对象：分支 `fix/merge-banner-stuck` 全部未提交改动
> 结论：**无 🔴 严重问题，可合入**（🟡 ×2 均已当场修复加固）
> 实测：`cargo test --lib` 235 通过（含新增 5 例）、`cargo fmt --check`、`cargo clippy` 干净

## ✅ 优点（审查核实摘要）

- **git 语义与 CLI 精确对齐**：parents 顺序 `[HEAD, MERGE_HEAD]` 一致；空提交豁免仅作用于 merge 场景；非 merge 的 `NOTHING_TO_COMMIT` 拒绝路径零改动。
- **失败顺序设计正确，无状态错乱窗口**：所有可能失败的步骤都在 commit 之前（含 `write_tree()` 遇未解决冲突返回 `GIT_EUNMERGED` 的既有防护，已对照 vendored libgit2 源码核实）；commit 之后只有 best-effort 的 `cleanup_state()`，不存在「清了 MERGE_HEAD 却没提交」的路径。
- **损坏状态 fail-safe**：`read_merge_head` 对缺失/不可解析/指向不存在对象的 MERGE_HEAD 一律报错中止，不会退化成单 parent 的「假 merge commit」。
- **惯例一致**：MERGE_* 文件级操作风格对齐 conflict.rs；`let _ = cleanup_state()` 对齐其余 5 处调用点；`build_conflicted_merge` 用真实 `merge_branch` 制造状态而非伪造。
- 已核实 `git_repository_state_cleanup` 确实清除 MERGE_HEAD / MERGE_MSG / MERGE_MODE。

## 🔴 严重问题（必须修复）

无。

## 🟡 一般问题（建议修复）→ 均已修复

1. **被修改的空提交守卫缺回归测试**（`working_copy.rs` `merge_head.is_none() && parent_tree.id() == tree.id()`；全库原本无任何测试断言非 merge 空提交仍被拒）→ **✅ 已补**：`commit_refuses_empty_tree_without_merge`。
2. **octopus merge（多行 MERGE_HEAD）会让 commit 报错不透明**（整个文件被当单个 oid 解析失败）→ **✅ 已修**：`read_merge_head` 解析首行后检查剩余内容，多行时返回明确错误「octopus merge (multiple MERGE_HEAD entries) is not supported — finish or abort the merge with git」，并补测试 `commit_refuses_octopus_merge_head`。

## 🟢 优化建议（可选，未采纳，记录备查）

1. `is_merge_in_progress` 检查与 `read_merge_head` 读取之间的 TOCTOU（并发 abort 会让 commit 报 FS NotFound，重试自愈）——无害，保持现状。
2. cleanup_state 失败后再次提交会产生重复双亲 merge commit——与既有 5 处 best-effort 惯例一致，可经 abort / health residue 检查恢复。
3. 外部 git 遗留的 CHERRY_PICK_HEAD / REVERT_HEAD 不在本次范围。
4. conflict.rs 既有测试的手工场景搭建与 `build_conflicted_merge` 重复，后续可复用 helper。
5. MERGE_MSG 已含默认消息，后续可考虑前端预填提交输入框（范围外）。

## 加固测试（本次新增，共 5 例）

| 测试 | 覆盖 |
|---|---|
| `commit_while_merging_records_second_parent_and_cleans_up` | 双亲 merge commit + MERGE_HEAD/MERGE_MSG 清理 |
| `commit_while_merging_allows_tree_identical_to_head` | 全 "ours" 解决（树同 HEAD）仍可提交结束 merge |
| `commit_refuses_empty_tree_without_merge` | 非 merge 空提交仍被 `NOTHING_TO_COMMIT` 拒绝（守卫回归） |
| `commit_with_unparseable_merge_head_fails_and_keeps_state` | 损坏 MERGE_HEAD：报错且状态不变（fail-safe） |
| `commit_refuses_octopus_merge_head` | octopus：明确报错且状态不变 |

## 📝 总体评价

修复最小且正确：三个缺陷（不清理状态 / 缺第二 parent / 空树误拒）集中在 `commit()` 一处解决，与 git CLI 语义精确对齐，所有失败路径不产生状态错乱，损坏与 octopus MERGE_HEAD 均 fail-safe 并有可操作报错。235 个 Rust 测试全绿。可合入，合入前建议按 plan.md 冒烟清单手动验证「解决 → 提交 → banner ≤3s 消失」。
