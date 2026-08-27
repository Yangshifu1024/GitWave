# feat-branch-list-collapsible-groups · 左侧栏分支分组折叠

> 状态：实施完成（待手动冒烟；未提交）
> 需求（用户，2026-08-28）：左侧栏分支中——1) 本地分支可折叠；2) 远程分支按不同 remote 分组后可折叠。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 分组结构 | Local 一个可折叠组 + 每个 remote 一个可折叠组（origin / upstream …） | 移除原独立 "Remote" 静态组头，remote 名即组名，与 "Local" 平行 |
| 组头 | chevron + 大写名称 + `(数量)`，整行可点 | 样式沿用原组头（uppercase / tracking-wider / text-muted），加 hover |
| 折叠状态 | 组件内 `Record<groupKey, boolean>`（key：`local` / `remote:{name}`） | 会话内记忆，切仓库不重置；Local 默认展开，远程组默认折叠 |
| 远程分组来源 | 可见远程分支（同名去重后）按首段 remote 分组，首次出现顺序 | 被本地同名隐藏的远程分支不产生空组 |
| 排序/位置 | Local 组在最上，其后按 remote 首现顺序，组间 mt-2 | 与原布局节奏一致 |

## 改动清单

- `src/components/BranchList.tsx`：`remoteGroups`（按 remote 分桶）+ `collapsedGroups` 状态 + `renderGroup()`（可折叠组头 + 分支行）；`ChevronDown` / `ChevronRight` 导入

## 验证

- [x] `npm run typecheck` / `lint` / `format:check` / `test`（43）/ `build` 全绿
- [ ] 手动冒烟：Local 折叠/展开；多 remote 仓库按 origin / upstream 分组各自折叠；折叠后选中分支再次展开仍高亮；单 remote / 无远程仓库表现正常
