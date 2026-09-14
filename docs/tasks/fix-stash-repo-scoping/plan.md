# fix-stash-repo-scoping

> 同一 workspace 内切换仓库时，stash 列表跟随当前激活仓库刷新；删除 / 应用 / 弹出都作用于列表所显示的仓库。

## 现象

- 在 workspace 的仓库 tab 之间切换，侧栏 Stash 列表始终显示上一个仓库的内容，不刷新。
- 列表项点击删除（drop）报错。

## 根因（已定位）

- 后端每条 stash 命令都通过 `active_repo_path(ctx, workspace_id)` 解析仓库，取 workspace 的 `last_active_repo_id`（`src-tauri/src/application/use_cases.rs:2756`）。
- 切换仓库时 `WorkspaceRepoTabs.activateRepo()` 先 `setActiveRepo`（写后端指针）再 `setActiveRepoId`（写 UI store）（`src/components/WorkspaceRepoTabs.tsx:114`）。
- 但 `StashPanel` 的 query key 只有 `["stashes", workspaceId]`（`src/components/StashPanel.tsx:34`）。切仓库不改变 `workspaceId`，key 不变 → React Query 命中旧缓存、不重新拉取。
- 删除 / 应用 / 弹出按钮仍以 `e.index` 调用后端；此时后端已指向**新**仓库，而 index 来自**旧**仓库列表 → `stash_drop(index)` 越界/不存在而报错。极端情况下若新仓库该 index 恰好存在，会误删错误的 stash。

同类仓库级查询已有约定：`src/hooks/useTags.ts:25`（key 含 `repoId`，文件头写明原因）、`src/hooks/useActiveRepoState.ts:23`（`["working-copy", workspaceId, repoId]`）、`HealthPanel`（`["health", workspaceId, repoId]`）。Stash 是漏网者。

## 修复方案

1. 新增 `src/hooks/useStashes.ts`，对齐 `useTags`：
   - 导出 `stashesQueryKey(workspaceId, repoId)` = `["stashes", workspaceId, repoId]`；
   - `useQuery({ queryKey: stashesQueryKey(...), queryFn: listStashes, enabled: workspaceId && repoId })`；
   - 返回 `workspaceId` / `repoId` / `invalidate()`。
2. `StashPanel` 改用 `useStashes()`；`repoId` 变化时清空 `selectedOid` / `diff` / 错误，避免残留上一个仓库的预览。
3. `ActionBar` 保存 stash 后的失效改为同一 key 工厂（含 `activeRepoId`）。
4. 回归：单测 `stashesQueryKey` 在同一 workspace、不同 repo 下产生不同 key（防止再次漏掉 `repoId`）。

## 验证

- `pnpm exec prettier --check .`、`pnpm lint`、`pnpm typecheck`、`pnpm test`。
- 手工：workspace 内放两个各自有 stash 的仓库，切换 tab 列表随仓库变化；在任一仓库 drop 成功且只影响该仓库。
