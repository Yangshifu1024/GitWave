# fix-stash-repo-scoping · review

> 审查对象：`fix/stash-repo-scoping`（`src/hooks/useStashes.ts` + `src/hooks/useStashes.test.ts` + `StashPanel.tsx` + `ActionBar.tsx`）。
> 结论：**通过**，无 🔴 级问题。

## ✅ 优点

- 根因定位准确：把 stash 查询对齐到仓库级 key 约定（同 `useTags` / `useActiveRepoState` / `HealthPanel`），一次修复同时解决“列表不刷新”和“删除报错”，并消除误删其他仓库 stash 的潜在数据风险。
- key 收敛到 `stashesQueryKey` 工厂，`StashPanel` 读取与 `ActionBar` 失效共用同一来源，从结构上杜绝 key 再次漂移。
- `repoId` 变化时清理 `selectedOid` / `diff` / `error`，避免跨仓库残留预览。
- 新增回归单测锁住“不同 repo → 不同 key”。

## 🔴 严重问题（必须修复）

无。

## 🟡 一般问题（建议修复）

- **位置**：`src/hooks/useStashes.test.ts`
- **描述**：测试只验证 key 工厂本身，无法保证组件真的把 `repoId` 传进去。若将来有人改回 `["stashes", workspaceId]` 而不走工厂，测试不会失败。
- **建议**：可接受的折中（现有测试环境无 `@testing-library/react`，组件级断言成本高）。后续若引入 RTL，再补一个“切换 repoId 触发 refetch”的用例。

## 🟢 优化建议（可选）

- **位置**：`src/hooks/useTags.ts`、`src/hooks/useActiveRepoState.ts`、`useStashes.ts`
- **描述**：仓库级 query 的“key 含 repoId + enabled 双 id + invalidate”三段式已重复三处。
- **建议**：后续可抽一个 `useRepoScopedQuery(keyPrefix, fetcher)` 泛型 hook；本次不做以免扩大改动面。

- **位置**：其他仍以 workspace 为 key 的 react-query（如 `BranchList.tsx:348` 的 `["remotes", activeWorkspaceId]`，`hooks` / `lfs-status` / `reflog`）
- **描述**：同类“切仓库不刷新”的风险可能仍存在于个别面板。分支数据本身通过 effect 依赖 `activeRepoId` 重载（`BranchList.tsx:428/466`），不受影响，但不能推断所有面板都安全。
- **建议**：另开任务做一次仓库级 key 审计，不在本次范围内。

## 📝 总体评价

改动小而聚焦，正确性和可维护性都明显提升，且符合仓库既有约定。测试覆盖对本次缺陷类型是有效的静态护栏，唯一遗憾是缺少组件级行为断言——但受当前测试栈限制，属合理取舍。
