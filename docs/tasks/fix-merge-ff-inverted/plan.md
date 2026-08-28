# fix: Merge into current 快进判断反转导致本地提交丢失（P0）

状态：已实现

## 事故（2026-08-28，用户真机）

main 领先 2 个提交时，对一条**已并入 main 的分支**（tip 是 main 的祖先）执行 "Merge into current"：
`merge_branch` 的快进条件写成了 `if behind == 0`（对方没有新提交 = 应为 AlreadyUpToDate），
走进了快进分支执行 `head.set_target(their_oid)` —— **main 指针被回拨**，本地 2 个提交被甩出分支历史
（reflog "gitwave: merge"）。`2e0d49e` / `a089721` 靠 reflog cherry-pick 恢复。

附带发现：正确的回归测试曾存在但被标 `#[ignore]`（"flaky"），且断言的是错误语义（合并祖先时期望
FastForward）——掩盖了本 bug。

## 根因

`src-tauri/src/infrastructure/git/merge.rs`：

- 快进条件应为 `ahead == 0`（HEAD 无独立提交才允许移动）；`behind == 0` 是"无需合并"
- 旧的 `our_oid == their_oid` 特判被 `behind == 0` 分支涵盖（等值时 ahead/behind 均为 0）
- 快进时只 `set_target` 移引用、不刷新 index/工作区 → checkout 陈旧（潜在 bug，一并修）

## 修复

- `behind == 0` → 返回 `AlreadyUpToDate`，HEAD 不动
- `ahead == 0`（且 behind > 0）→ 真·快进：`set_target` + `checkout_head(force)` 刷新工作区
- 测试：删除 ignore 的错误用例，新增 2 个回归用例：
  - 合并祖先分支 → AlreadyUpToDate 且 HEAD 不动
  - 落后分支合并 → FastForward 且 HEAD/工作区前进
- `MergeKind::FastForward` 文档同步（工作区已刷新）

## 修订 2（2026-08-28，用户需求：Merge 前弹框确认，参考 Fork）

- `merge_branch(repo, name, no_ff)` 增加 `no_ff`：对可快进合并强制创建 merge commit（tree 取 ours，双亲）
- 新增 `merge_preview`（Rust + `cmd_merge_preview`）：`graph_ahead_behind` + `repo.merge_commits` 干跑，返回 `{ up_to_date, fast_forward, conflicts }`，完全不触碰 HEAD/index/工作区
- 前端 `BranchList`：右键 "Merge into current" 不再直接合并，弹 Fork 风格确认框 —— Merge/Into 展示、Merge Option（Auto / No fast-forward）、冲突预检状态行（绿"可无冲突合并" / 红"预计 N 文件冲突" / 灰"Already up to date"禁用 Merge）
- 测试：no_ff 建双亲 merge commit、preview 三态（up-to-date / ff / 冲突），`test_helpers::write_and_stage`、`make_commit` 改 pub

## 验证

- `cargo test --lib merge` 6 通过（含 2 个新回归用例）、clippy 0 warning、fmt 通过
- 真机：对已合并分支再执行 Merge into current 应提示 Already up to date 且 main 不动
