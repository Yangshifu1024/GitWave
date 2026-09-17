# F017 · Repository Tab 溢出时滚轮横向滚动

## 背景

Workspace 内多仓库以 tab 形式展示（`WorkspaceRepoTabs`，见 F005）。仓库数量超过条带宽度后，tab 会横向溢出：

- 真正承载溢出的滚动容器是 HeroUI `ScrollShadow`（`data-slot="scroll-shadow"`），它只有 `overflow-x`，且滚动条被隐藏；
- 因此竖直滚轮（鼠标最常见的操作）完全无法移动条带，只能用触控板横滑、`Shift+滚轮`，或 HeroUI 自带的左右箭头按钮；
- 滚动条隐藏后，溢出的 tab 既看不见也很难到达——尤其是刚通过「一次添加多个仓库」加入、并被置为激活的最后一个仓库，可能直接落在视野之外。

## 提议方案

1. **滚轮横滚**：指针位于 tab 条带内时，把竖直滚轮按 1:1 像素换算为横向滚动（`deltaMode=1` 行模式 ×16，`deltaMode=2` 页模式 ×视口宽度）。条带内**始终消费**该事件，滚到两端也不再冒泡（条带独占滚轮）。
2. **触控板手势不双重处理**：`deltaX !== 0`（触控板自带横向分量）的手势交回浏览器原生，保留惯性滚动。
3. **激活 tab 自动进入视野**：切换仓库、切换 Workspace、批量添加仓库后最后一个仓库被激活、启动恢复选中态时，把激活 tab 平滑滚入可视区。只滚动条带自身的滚动容器，不用 `scrollIntoView`（避免连带滚动祖先容器）。
4. **不做**：纵向滚动条样式、拖拽到边缘自动滚动、键盘左右切换 tab、RTL 反向滚动。

## 影响

- 涉及模块：
  - 新增 `src/lib/horizontalWheel.ts`（纯函数：滚轮换算 / 夹取 / 入视野位置）
  - 新增 `src/hooks/tabStripScroller.ts`（定位 HeroUI 内部滚动容器）
  - 新增 `src/hooks/useHorizontalWheelScroll.ts`（原生非 passive 滚轮监听）
  - 新增 `src/hooks/useRevealActiveTab.ts`（激活 tab 入视野）
  - 修改 `src/components/WorkspaceRepoTabs.tsx`（接入两个 hook）
  - 不修改 `src/components/ui/Tabs.tsx`（本轮唯一调用方就在条带，保持原语不变）
- 影响版本：v0.9.x
- 是否破坏向后兼容：否（纯前端行为增强，无数据 / IPC / 文案变更，不需要 i18n 改动）

## 决策

- 状态：已合并
- 决策人：用户（直接提出需求并逐项确认方案）
- 决策日期：2026-09-17
- 关联决策：[F005](./F005-repo-tab-drag-reorder.md)（同一 tab 条带的拖动排序；两者共享 pointer / wheel 互不干扰）
