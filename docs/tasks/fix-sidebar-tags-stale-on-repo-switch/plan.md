# fix-sidebar-tags-stale-on-repo-switch · 切换仓库后左侧栏 tag 列表不刷新

> 状态：已修复（待冒烟 / review）
> 需求（用户，bug 报告）：左侧栏中的 tag，切换仓库不会变化。

## 根因

1. 后端契约：`list_tags / create_tag / delete_tag` 命令只接收 `workspace_id`，后端经
   `active_repo_path(ctx, workspace_id)` 解析为「该 workspace 的**当前激活仓库**」。
2. 前端 `TagsPanel` / `CommitInfoHeader` 的 react-query key 是 `["tags", workspaceId]`，
   不含 `repoId`。同一 workspace 内切换仓库时 key 不变 → 命中缓存、不 refetch →
   显示的仍是上一个仓库的 tag 列表。
3. 两个消费方共用同一个 key 前缀 `["tags", …]`，auto-refresh（`invalidateQueries()`
   全量失效）之外，切换仓库没有任何机制让它们重新拉取。

对比：`useWorkingCopy`（`["working-copy", workspaceId, repoId]`）等其余面板均把
repoId 放进 key，且组件会随 `activeRepoId` 变化重渲染/重拉取，故只有 tags 有此问题。
（该问题在 PR #12 前即存在，PR #12 的选中态依赖 tag 数据，使其更易被观察到。）

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 修复方式 | 抽共享 hook `src/hooks/useTags.ts`，queryKey 变为 `["tags", workspaceId, repoId]` | 两个消费方（TagsPanel / CommitInfoHeader）一处修复；key 变化触发 refetch，切换仓库立即拿到新仓库数据 |
| queryFn 保持只传 `workspaceId` | 与后端契约一致 | 后端按 active repo 解析；repoId 仅作为缓存分片（同 `useWorkingCopy` 的既有模式） |
| 竞态评估 | 安全，无需加锁 | 所有切换点（WorkspaceRepoTabs / ActionBar / WorktreePanel）都是先 `await set_active_repo`（后端指针）再 `setActiveRepoId`（前端 store）；refetch 必然发生在指针更新之后 |
| 单测 | 不新增 | 测试栈无 testing-library，hook 渲染级测试做不了；以 typecheck + 手动冒烟覆盖 |
| invalidate 语义 | hook 暴露 `invalidate()`，按当前 (workspaceId, repoId) 精确失效 | TagsPanel 删除后、CommitInfoHeader 标签管理器 onChanged 均改走该出口 |

## 改动清单

- 新增 `src/hooks/useTags.ts`：共享 tags query（key 含 repoId、enabled 双 id 守卫、invalidate 出口）
- `src/components/TagsPanel.tsx`：改用 `useTags()`（删除原内联 query）
- `src/components/CommitInfoHeader.tsx`：改用 `useTags()`（`refetchTags` → `invalidateTags`）

## 测试

- `npm run typecheck` / `npm test` / `npm run lint` / `npm run build` 全绿
- 手动冒烟要点：
  - workspace 内两个仓库各有不同 tag；来回切换 repo tab，Tags 面板立即跟随切换
  - 切到无 tag 仓库显示「暂无标签」；切回 tag 仓库恢复
  - 选中有 tag 的提交 → CommitInfoHeader 标签管理器列出正确仓库的 tag
  - 空仓库 / 未选仓库（activeRepoId = null）时面板不报错
