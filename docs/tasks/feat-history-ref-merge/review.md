# Review · feat-history-ref-merge

> history 中 commit 的分支标签（RefBadgeContextMenu）右键菜单增加「合并到当前分支」。

## 方案对齐表

| 方案条目 | 状态 | 说明 |
|---|---|---|
| 提取 `MergeConfirmDialog` 为独立组件（`src/components/MergeConfirmDialog.tsx`） | ✅ 对齐 | 与原 BranchList 内实现逐行一致，props 不变；queryKey 与 `branches.merge.*` i18n key 均保留 |
| BranchList 改为 import 共享组件，清理孤儿 import | ✅ 对齐 | 移除 `mergePreview` / `Label` / `CircleCheck` / `CircleX`；调用点 props 原样 |
| RefBadgeContextMenu `local_branch` 新增「Merge into current」 | ✅ 对齐 | GitMerge 图标、复用 `branches.menu.mergeIntoCurrent`，无新增 i18n key |
| 禁用逻辑：当前分支 / detached HEAD | ✅ 对齐 | `disabled={busy \|\| checkout.busy \|\| isCurrent \|\| !currentBranch}`，title 用 `branches.guard.current` |
| 确认后 `mergeBranch` + 状态提示 + invalidate + bumpHistory | ✅ 对齐 | 成功/冲突文案与侧栏一致；显式确认，无自动 merge（P1） |
| remote_branch / tag 菜单不变 | ✅ 对齐 | 未触碰 |

## 问题分级

- 🔴 严重：无。
- 🟡 已修复（返工提交前）：① tooltip 误用 "Cannot delete the current branch" → 改为 `branches.guard.current`；② 菜单项补齐 `busy`/`checkout.busy` 门控。
- 🟢 观察：`handleMerge` 与 `run` helper 结构重复（可后续收敛）；MergeConfirmDialog 硬编码 DOM id（既有问题，非本次引入）。

## 验证

- `pnpm typecheck` ✅
- `pnpm test`（vitest）✅ 24 files / 161 tests，含 `i18n/locales/parity.test.ts`
- `pnpm exec eslint`（3 个改动文件）✅

## 结论

**对齐，可合入**。建议合并前在应用内做一轮人工冒烟（重点：确认后状态栏文案与 history/分支列表刷新）。
