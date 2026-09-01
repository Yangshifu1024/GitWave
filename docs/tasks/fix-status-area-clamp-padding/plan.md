# fix-status-area-clamp-padding · 状态区文字两行截断 + padding + 省略号

> 状态：已修复（待冒烟）
> 需求（用户）：状态指示区域文字最多两行，并且增加 padding，多余文字显示为省略号。

## 现状与根因

组件 `src/components/SyncStatusArea.tsx`（ActionBar 中央悬浮卡片，固定 `h-12 w-72 overflow-hidden`）：

1. `line-clamp-2` 本已存在于文字 span 上（Tailwind v4 内置类），但 span 是内层
   `flex` 容器的 flex item，默认 `min-width: auto` 使长文本无法收缩——溢出被卡片
   `overflow-hidden` 直接裁掉而非显示省略号，截断在长文本下实际不可靠。
2. 卡片与内层容器 padding 均为 0（`p-0`），文字贴卡片左右边缘。

## 改动清单

- `src/components/SyncStatusArea.tsx` 内层文字容器（原 90–101 行）：
  - padding 改为互斥条件 `cancellable ? "pl-3 pr-8" : "px-3"`（常态左右 12px；
    可取消时右侧 32px 避让取消按钮）。互斥写法不依赖 Tailwind 的 px/pr 覆盖顺序。
  - 文字 span 增加 `min-w-0`，使 flex item 可收缩，`line-clamp-2` 的两行截断 +
    省略号稳定生效。
- 取消按钮与底部进度条均为 absolute 定位，不受 padding 影响，无需改动。

## 测试

- `npm run typecheck` / `npm run lint` / `npm test` 全绿（无渲染测试基建，样式靠冒烟）
- 手动冒烟要点：
  - idle 态贴一段超长文字（或长分支名）：最多两行、末尾省略号，文字不贴卡片边缘
  - 同步中长 remote 名标签同样两行省略
  - fetch/pull/push 进行中：取消按钮（X）出现，文字不与 X 重叠（右侧 32px 预留）
  - 常规短文案（分支名 / 「刷新完成」等）仍垂直居中、单行显示
