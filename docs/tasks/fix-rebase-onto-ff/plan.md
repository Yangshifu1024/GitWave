# fix-rebase-onto-ff · 修复方案

> 分支：`fix/rebase-onto-ff`（自 main f8937f2）。关联：`docs/tasks/fix-pull-rebase-noop/`
> （PR #4 修了 rebase 不落地，本任务修其遗留的"落后场景误报 up to date"）。

## 现象

分支右键 "Rebase current onto this"，当前分支明显落后目标分支，却提示
"Rebased onto X (already up to date)"，分支纹丝不动。

## 根因

`rebase.rs::rebase_branch` 的分支判断有误：

```rust
let (ahead, _behind) = repo.graph_ahead_behind(our_oid, upstream_oid)?;
if ahead == 0 {
    return AlreadyUpToDate;   // ← 把"严格落后"也吞了
}
```

`ahead == 0` 覆盖两种情况：与 upstream 相同（真 up to date）和**严格落后**
（正确语义是 fast-forward，`git rebase` 也这么做的）。PR #4 只在 pull 路径
把 FF 判断提到了 rebase 之前（`remote.rs::pull_integrate` 的
`analysis.is_fast_forward()` 块），右键 rebase 路径（`use_cases::rebase_branch`
直通 infra）没提，最常见的"落后"场景就变成了无声空转。

顺带发现两个相邻问题，一并修掉：

1. **rebase 到自己的祖先**（ahead>0、behind=0）会走进 in-memory rebase，
   零 pick 后返回 `Clean` 且 `new_head=None`，use_cases 报
   "rebase finished without a new HEAD"。正确语义是 up to date。
2. 旧测试 `rebase_fast_forward_when_already_descendant` 名不副实：分支实际
   建在 HEAD 上（走 same-commit 早退），注释却声称测了落后场景；
   `finalize_rebase_moves_branch_and_checks_out` 把"rebase 到父提交"当成
   会产生重写（能过纯属重写提交与原提交逐字节相同）。

## 修复方案（`src-tauri/src/infrastructure/git/rebase.rs`）

按 ahead/behind 四象限重构判断：

| 场景 | ahead | behind | 行为 |
|---|---|---|---|
| 与 upstream 相同 / upstream 是祖先 | ≥0 | 0 | `AlreadyUpToDate`（不变） |
| **严格落后** | 0 | >0 | **新增 `FastForward`：`finalize_rebase` 落地 upstream** |
| 分叉 | >0 | >0 | in-memory 重写 → `Clean`（不变，use_cases 落地） |
| 冲突 | — | — | `Conflicts`（不变） |

- 枚举新增 `RebaseKind::FastForward`（serde snake_case → 前端
  `kind.replace(/_/g, " ")` 显示 "(fast forward)"）
- FF 落地复用同文件 `finalize_rebase`（set_target + set_head + force
  checkout）；两个调用方（pull、use_cases）都已有脏区守卫前置
- `remote.rs::pull_integrate` 补 `FastForward` match 臂（实际不可达：pull 的
  FF 块先行返回，仅为穷尽性）
- 前端 `api.ts` 的 `RebaseKind` 联合类型加 `"fast_forward"`

## 测试

- 新增 `rebase_strictly_behind_fast_forwards`：落后 2 个提交 → `FastForward`，
  断言 branch ref / HEAD / 工作区文件都到位（用户报告场景的回归）
- 新增 `rebase_onto_ancestor_is_up_to_date`：真正以祖先为目标 → `AlreadyUpToDate`
- 重写 `finalize_rebase_moves_branch_and_checks_out` 为真 diverged 场景
  （old 分叉出自己的提交），断言重写产出新 oid 且落地
- 全量 `cargo test`：197 passed（改前 196）；`tsc --noEmit`、`cargo fmt --check` 通过

## 范围外（另行任务）

- in-memory rebase 冲突路径 conflicts 恒为空（磁盘 index 误用）→
  `docs/tasks/fix-rebase-conflict-list/plan.md`（待排期）
