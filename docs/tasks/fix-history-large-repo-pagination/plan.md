# fix-history-large-repo-pagination · 大仓库 History 只显示有限提交

> 状态：实施完成（待手动冒烟；未提交）
> 问题（用户，2026-08-28）：打开一个大型仓库时，History 只能显示几十条。

## 根因

`CommitGraph` 写死 `getCommitLog(workspaceId, 200)`：一次取 200 条、无分页、无加载更多。
后端 `commit_log(repo, max)` 本身正确（推入所有本地 + 远程分支 tip，topo+time 遍历，
`max` 参数可变），因此大仓库历史在 200 条处被硬性截断，滚动到底即墙。

## 修复（纯前端，CommitGraph 分页）

- 初始加载 200 条（`INITIAL_LIMIT`）；滚动接近底部（600px 提前量）自动追加 300 条
  （`PAGE_SIZE`）——后端以更大的 `max` 重走同一条确定性遍历，前缀结果稳定，整体替换列表
- 上下文（workspace / repo / epoch）切换时窗口重置为初始 200（`fetchKey` + ref 判定，
  避免 limit 变更与切换互相干扰）；加载期间旧列表保留，不再整屏闪烁 "Loading history…"
- 短页（返回条数 < 请求 limit）即到达根，显示 "End of history · N commits"；
  加载中显示 "Loading older commits…"
- 分支点击定位（locate）重试机制不受影响（shaToIndex 随列表更新）

## 验证

- [x] `npm run typecheck` / `lint` / `format:check` / `build` 全绿；`npm test` 43 通过
- [ ] 手动冒烟：大型仓库滚动到底自动追加、每次 +300、最终显示 End of history；小仓库直接显示 End；切仓库/commit 后窗口重置且不串仓库；定位分支 tip 仍居中选中
