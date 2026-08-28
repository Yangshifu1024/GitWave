# feat: Branches 按分支名前缀分组（Fork 风格）

状态：已实现

## 需求来源

用户 2026-08-28 + Fork 截图：Branches 下的分支再按分支名前缀分一级（feat / feature / fix 文件夹），文件夹内显示去掉前缀的名字；无前缀分支（main）留在顶层。分支：`feature/theme-design`（沿用）。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 分组依据 | 显示名的第一段（远程分支先剥 remote 前缀） | `origin/feat/x` → 文件夹 `feat`、显示 `x`；深层 `a/b/c` → 文件夹 `a`、显示 `b/c` |
| 排序 | 前缀文件夹按字母序在前，无前缀分支在后 | 对齐 Fork 截图（feat/feature/fix 文件夹 → main） |
| 折叠状态 | 复用 `collapsedGroups`，键 `${groupKey}:${prefix}`，默认展开 | 顶层组默认规则不变（remote 组默认折叠） |
| 交互不变 | 所有 handler 仍用 `branch.name` 全名 | 选择/checkout/删除/merge/rebase 不受显示名影响；`title` 悬浮显示全名 |
| 视觉 | 11px normal-case（分支前缀大小写敏感，不 uppercase）text-secondary + Folder 图标，缩进 pl-9 | 与 LOCAL/ORIGIN（uppercase muted pl-6）再深一级 |

## 改动清单

- `src/lib/branchNames.ts`：`splitBranchPrefix()` 纯函数
- `src/lib/branchNames.test.ts`：3 个新用例（13 全过）
- `src/components/BranchList.tsx`：`BranchRow.displayName` prop；`renderGroup` 分区为 folders + roots；前缀文件夹折叠头（chevron + Folder 图标 + 计数）

## 修订（2026-08-28）

文件夹默认**折叠**；但包含当前选中分支（selectedName）的文件夹默认展开。显式点击折叠/展开始终优先于默认值。

## 验证

- vitest / typecheck / prettier / build
- 真机：feat/feature/fix 文件夹折叠展开、文件夹内 checkout/右键菜单用全名、选中高亮与 current 徽标正常
