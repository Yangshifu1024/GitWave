# feat-sidebar-single-item-collapse · 左侧栏 Remotes / Worktrees 仅一项时默认折叠

> 状态：已实现（待冒烟 / review）
> 需求（用户）：左侧栏中，remote 和 Worktree 只有一项时也默认隐藏。

## 背景

左侧栏各卡片由 `SidebarSection`（HeroUI Disclosure）渲染，`defaultOpen` 只在
首次挂载时生效（uncontrolled `defaultExpanded`）。此前 Remotes / Worktrees 只要
有内容就默认展开：

- 单 remote（最常见的只有 `origin`）时，卡片常驻两行垂直空间却几乎没有信息量；
- 单 worktree 即当前仓库自身（main worktree），展开态同样冗余。

既有先例：BranchList 的 remote 分组默认折叠、Health / Recovery / Tags 卡片默认
`defaultOpen={false}`，本需求把 Remotes / Worktrees 也纳入"低价值默认收起"。

## 方案

`RemotesPanel` / `WorktreePanel` 的 `defaultOpen` 改为 `items.length > 1`：

- `<= 1` 项 → 默认折叠（卡片头保留，点击可展开；Remotes 头部右键加 remote 入口不变）
- `> 1` 项 → 默认展开（维持现状）

因 `defaultOpen` 是 uncontrolled 默认值，跨过阈值（加/删 remote、切到
worktree 数不同的仓库）时需重新生效，仿照 `TagsPanel` 的 per-repo `key` 模式，
给 `SidebarSection` 加 `key={items.length > 1 ? "multi" : "single"}`：
仅在跨阈值时重挂载并应用新默认值；同侧手动展开/折叠的用户选择不受影响。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| "隐藏"的含义 | 默认折叠（卡片头保留），而非整卡移除 | 整卡移除会连带丢掉入口：Remotes 头部右键是加 remote 的唯一侧栏入口；折叠后仍可一键展开，且与 Health / Recovery 的"默认收起"一致 |
| 阈值方向 | `> 1` 展开，`<= 1` 折叠 | 只改用户点名的两个面板；Tags / Submodules / Stash 维持各自现状，不在本次范围 |
| 默认值重生效方式 | 按 `items.length > 1` 阈值加 `key` 重挂载 | `Disclosure.defaultExpanded` 仅初始化生效；沿用 TagsPanel 既有 `key` + `defaultOpen` 模式，未引入受控状态 |
| 空 / 加载态 | 不变 | `collapsible={false}` 仍渲染静态头（Remotes 空态右键可加 remote；Worktrees 加载中不闪折叠态） |

## 改动清单

- `src/components/RemotesPanel.tsx`：`SidebarSection` 增加 `defaultOpen={items.length > 1}` 与阈值 `key`
- `src/components/WorktreePanel.tsx`：同上（抽 `defaultOpen` 局部变量）

## 测试

- `npm run typecheck` / `npm test` / `npm run lint` 全绿
- 手动冒烟要点：
  - 单 remote 仓库：Remotes 卡片默认折叠，点头部展开可见 origin；右键头部仍可加 remote，加到第二个后卡片自动展开
  - 多 remote 仓库删除到只剩一个：卡片自动折叠
  - 普通（无额外 worktree）仓库：Worktrees 卡片默认折叠；新建一个 worktree（共 2 个）后默认展开
  - 手动展开单 remote 卡片后不做其他操作：保持展开，不闪回
