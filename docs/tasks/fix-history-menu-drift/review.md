# fix-history-menu-drift · review

> 审查对象：commit 80d8d6c（PR #23，squash 合入 main）
> 审查方式：code-reviewer 代理，7 维度（正确性/安全/性能/可维护性/可读性/测试覆盖/最佳实践）
> 结论：**CLEAN** —— 🔴 0 / 🟡 1 / 🟢 5，无阻塞项

## ✅ 通过要点（源码级核实）

- portal 锚点方案与 RAC/HeroUI context 接线完全吻合：`DialogTrigger` 经
  `OverlayTriggerStateContext`/`PopoverContext` 接线（与 DOM 位置无关），6 个
  `ContextMenuContent` 消费方统一等价生效。
- `SubmenuTrigger` 的 `[ReactElement, ReactElement]` 约束满足；子菜单选中后经
  `ContextMenuItem.setOpen(false)` 与 RAC `onAction→onClose`（经
  `RootMenuTriggerStateContext`）双路径收敛关闭整层；ESC/外点传播正确。
- `ContextMenuSub` disabled 视觉由 HeroUI slots 兜底
  （`.menu-item[data-disabled] → status-disabled`，pointer-events:none 同时阻止
  hover 展开禁用子菜单）。
- `["remotes"]` 缓存 key 与 ActionBar 一致，BranchList 查询常驻 active，
  失效后立即 refetch，无「改完 remote 菜单仍旧」窗口。
- TagsPanel：`deleteTarget`/`deleting`/`actionError` 位于 keyed 子树之外，
  `key={repoId}` 重挂不影响删除弹窗等状态。
- Conventional Commits / 分支命名对齐任务目录；i18n 双语同步 parity 通过；
  P1 无违反；typecheck / lint / 144 tests / prettier 全绿。

## 🟡 建议修复（后续任务跟进，不阻塞合入）

- **组件级回归测试缺失**（本次行为最密集改动零自动化覆盖）：vitest 为 node
  环境、无 testing-library/jsdom（vite.config.ts 注释明确是刻意取舍）。建议立
  组件测试基建任务（RAC 需配 pointer-events polyfill），最低覆盖：
  (a) 锚点渲染进 document.body 且坐标 = clientX/Y；(b) 任意 scroll 关闭；
  (c) 子菜单选中 remote 回调 `onPush(branch, remote)`；(d) 0/1/>1 remote
  分别渲染置灰/扁平/子菜单。

## 🟢 可选打磨

1. RemotesPanel `run()` 共用出口使 fetch 也触发冗余 invalidate（无害）——已
   修订 plan.md 表述保持一致。
2. capture scroll 关菜单行为面比 RAC 原生更宽（任意面板滚动均关）——OS 原生
   菜单通行行为，未发现误伤；冒烟清单已补「菜单开着滚动其他面板」。
3. `remotes` 加载期回退 `[]` 与「真无 remote」置灰态短暂不可区分——实际几乎
   不可达，保持现状。
4. 菜单面板样式类在三处重复——可抽 `menuPanelClass` 常量（后续调风格时顺手）。
5. 手写 ChevronRight 可换 HeroUI `Menu.Item.SubmenuIndicator`——可选。
6. `react-aria-components@1.20.0` 精确锁合理；后续升级 HeroUI 时同步放宽为区间。

## Verdict

**CLEAN** —— 允许合入；🟡 测试债务立后续任务跟进。
