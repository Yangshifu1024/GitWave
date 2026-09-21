# fix-history-scroll-perf · 中间栏历史列表滚动空白/卡顿

> 状态：已实现，复审通过（第 2 轮无 🔴），待 macOS 真机验收（分支 `fix/history-scroll-perf`，基线 `main@1de31df`，改动未提交）
> 现象（用户）：中间历史信息部分，在 macOS 上性能不佳，上下滚动时有空白/卡顿。
> 分类：缺陷修复（性能）· 缺陷流程（tester 根因分析 → 本方案）

## 现象

```
触控板惯性滚动（macOS）
┌─ sidebar ─┬─ history ─────────────────────────┐
│           │  a1b2c3d feat: …                  │
│           │  ┌──────────────────────────────┐ │
│           │  │  ░░░░░░░░ 空白带 ░░░░░░░░    │ │  ← 滚动继续，但新暴露
│           │  └──────────────────────────────┘ │     区域来不及绘制
│           │  e4f5a6b fix: …                   │
└───────────┴────────────────────────────────────┘
```

## 根因（源码级）

主因（确定）：

1. `@tanstack/react-virtual@3.14.13` 默认 `useFlushSync = true`
   （`node_modules/@tanstack/react-virtual/dist/esm/index.js:7,69-80`）：range / `isScrolling`
   变化即 `flushSync(rerender)` —— **每个 scroll 事件同步、不可批处理地重渲整个
   `CommitGraph` 子树**。
2. `GraphRow`（`src/components/CommitGraph.tsx:113`）与 `CommitRow`（`:236`）都没有 memo
   → 每次重建约 45 行（可视 ~29 行 + `overscan` 10×2，`ROW_H=28`）。

放大器：

3. 每行最多 4 套自研 `ContextMenu`（1 行菜单 + `refs.slice(0,3)` 徽章菜单）。
   `ContextMenuContent` **无条件** `createPortal(<Popover.Trigger className="fixed …"/>,
   document.body)`（`src/components/ui/ContextMenu.tsx:107-114`），HeroUI `PopoverRoot`→RAC
   `DialogTrigger` 无条件渲染 children（`react-aria-components/dist/private/Dialog.mjs:88-92`），
   只有内层 RAC `Popover` 关闭时返回 null（`Popover.mjs:72`）
   ⇒ 关闭态每行仍留 4 个 body 级 fixed 节点 + `useId`/`useMenuTriggerState`/`useOverlayTrigger`/
   `Pressable`/`usePress`/`useFocusable` 一串 hooks。视口内约 180–200 个常驻 fixed 节点。
4. `RefBadgeContextMenu` 每实例挂 `useActiveRepoState` + `useBranchCheckout` + 3 个 `useState`
   （`src/components/RefBadgeContextMenu.tsx:51-60`）⇒ 每行最多 3 徽章 ≈ 9 个 react-query
   观察者，视口内 500+ 常驻；`useAutoRefresh` 的全局 `invalidateQueries` 会同时唤醒它们。

次因：

5. memo 的前置条件未满足：`handleSelect` 每次渲染新建（`CommitGraph.tsx:451`）；`menu` 是
   `useCommitMenuActions` 每次渲染新建的对象字面量（`src/components/CommitContextMenu.tsx:538`）
   → 直接加 memo 会失效。
6. `formatTime` 对一周前的提交每次都走 `new Date(time*1000).toLocaleDateString()`
   （`CommitGraph.tsx:37`，ICU 调用）。
7. `handleScroll`（`:410-416`）每个 scroll 事件读 3 个布局属性（在 flushSync 渲染中插入
   layout flush），近底部 `setLimit(+300)` → 重取 + `shaToIndex`/`maxLane`/`computeRowArt`
   全量重算 + 全表重渲。

平台差异（静态推断，待真机确认）：WKWebView 是**进程外异步滚动** —— 主线程被 `flushSync`
风暴占住时合成器仍继续位移旧 tile，新暴露区域的 tile 来不及绘制，表现为「滚动继续但有空白带」；
macOS 触控板惯性滚动是连续高频事件（60–120/s 且松手后持续），Windows 滚轮是低频离散事件。

已排除：滚动内容上没有 `backdrop-filter` / 大范围 `box-shadow` / 通配 `transition` / `filter` /
`contain`；`pane-edge-*` 只在内阴影的 sidebar/inspector，不随滚动重绘。

## 目标（可量化）

| 指标 | 现状（静态核算） | 目标 |
|---|---|---|
| 每次 scroll 事件重渲的 `CommitRow` 数 | 约 45 | 1–3（仅进出视口的行） |
| `document.body` 下常驻 fixed 节点 | 约 180–200 | < 5 |
| 3 秒惯性滚动的长帧（>50ms） | 待真机采样 | < 3 次，且无空白带 |

## 改动清单

### 包 A · `src/components/CommitGraph.tsx`

1. `GraphRow`、`CommitRow` 用 `React.memo` 包裹（props 均为原始值或稳定引用）。
2. `CommitRow` 移除 `menu` prop，改为经 `CommitMenuContext` 注入的 `CommitMenuBody` 读取控制器
   —— 菜单体只在菜单打开时挂载，因此总能读到最新控制器，既无需稳定那个 400 行 hook 的返回值，
   也不会产生过期闭包。
3. `handleSelect` 改 `useCallback`；`src/App.tsx` 的 `handleCommitSelect` 同步稳定化
   （否则 App 每次重渲都会击穿行 memo）。
4. 行内 `mergeTrackedRefs` 结果用 `useMemo`。
5. 分页触发改 rAF 合并 + 阈值 600 → 1200，卸载时 `cancelAnimationFrame`。
6. 虚拟行 wrapper 加 `contain: layout`（**审查后从 `layout paint` 降级**：paint containment 会裁掉
   `GraphRow` SVG 的描边出血与横向溢出，且开发机无法验证）。内层 sizer **不加** `will-change`：
   数万 px 高 sizer 的层提升在 WKWebView 上的显存 / 光栅行为无法在开发机验证，列入真机 A/B 候选。
7. `overscan` 10 → 14。
8. 保持 `useFlushSync` 默认（它存在正是为了避免「新暴露区域空一帧」；本方案靠降低单次渲染成本
   解决卡顿，不改滚动语义）。

### 包 A · 新增 `src/lib/commitTime.ts`

把组件内 `formatTime` 抽成纯函数 `formatCommitTime(time, t, now)`，并缓存
`toLocaleDateString` 结果（**按本地日历日**缓存）。**行为不变：仍用系统 locale**，不做语言切换的顺带改动。

### 包 B · `src/components/ui/ContextMenu.tsx`（共享原语，6 个消费方统一受益）

9. `ContextMenuContent` **关闭态不渲染隐形锚点**（`const mountAnchor = ctx.isOpen;` + `{mountAnchor ?
   createPortal(…) : null}`）—— 审查后由「整体 `return null`」改为该变体：不再常驻约 200 个 body 级
   fixed 节点，锚点在菜单打开这一帧才挂载，坐标取 contextmenu 时写入的 `clientX/clientY`，与
   `docs/tasks/fix-history-menu-drift/plan.md` 的定位修复兼容（该修复的本质是「锚点 portal 到 body 以
   脱离 transform 包含块」，此处仍 portal 到 body）。
   **审查纠正**：HeroUI 样式表用 `.popover[data-exiting=true]{animation:exit …}`
   （`node_modules/@heroui/styles/dist/heroui.min.css`）驱动退场动画，RAC 靠 `isExiting` 期间保持挂载
   才能播放 —— 整体早退会把 6 个消费方的 0.1s 退场动画一并干掉，故保留 `<Popover>` 本体挂载，只条件
   渲染锚点。
   正向副作用（两个变体都有）：关闭态 RAC `Popover` 自身 `return null` ⇒ 菜单体的 children 不渲染，
   菜单体内重型 hooks 自动惰性化。
10. `ContextMenu` 的 context value 用 `useMemo`；`ContextMenuTrigger` 把**内层 handler 与 `asChild`
    注入给子元素的 handler** 都用 `useCallback` 稳定化（后者依赖 `child.props.onContextMenu`）——
    否则 `cloneElement` 每帧注入新函数，被注入元素无法 memo。

### 跨包契约

- 不改任何 `ContextMenu*` / `RefBadge*` / `CommitMenuController` 的对外 props 签名与导出；
  6 个消费方（BranchList / TagsPanel / ChangesPanel / RemotesPanel / CommitGraph /
  RefBadgeContextMenu）行为等价。
- 不新增 i18n key、不新增运行时依赖、不动 Rust/后端。

## 非目标（本轮不做）

1. **徽章菜单控制器化**（消除每行约 9 个 react-query 观察者）：正确实现必须把
   `confirm` / `mergeDialog` / `checkout.renderDialogs()` 的对话框状态上提到列表根部
   （`RefBadgeContextMenu.tsx:254-300`）—— 否则点菜单项先关闭菜单，会连带卸载确认弹窗，
   确认框永远不出现。其收益只在「行进入视口」这一次要路径上，收益/风险不划算。
2. 行级菜单「上提为列表根部单一菜单」：关闭态早退已吃掉大部分边际收益，回归面涉及
   `fix-history-menu-drift` 与 F011。
3. `computeRowArt` 增量计算、后端分页游标、jsdom/@testing-library 渲染计数测试基建。

## 测试

自动（开发机）：

```bash
pnpm format:check && pnpm lint && pnpm typecheck && pnpm test
```

- 新增 `src/lib/commitTime.test.ts`：时间分档边界（59s/60s/1h/24h/7d）、缓存确定性、`now` 注入、
  **本地日边界**（本地当天 00:30 与前一天 23:30 必须得到不同 label —— 旧 UTC 日桶实现在 UTC+8 下
  两者键相同，已实跑复现）、时钟回拨（负 diff → justNow）。
- 新增 `src/components/CommitGraph.renderGuards.test.ts`：静态源码断言（两行组件被 memo 包裹、
  `overscan: 14`、行 wrapper 带 `contain: "layout"`、时间格式化已下沉、锚点只在菜单打开时挂载、
  `App.tsx` 的 `handleCommitSelect` 仍被 `useCallback` 包裹且直传），防止后人无意回退性能修复
  （与 `reactVersionParity.test.ts` / `appMenuSpec.test.ts` 同风格）。
- 复审补强的两条守卫：`commitTime.ts` 的缓存键必须由本地日历组件（`getFullYear/getMonth/getDate`）
  构成且不得退回 UTC 日桶 —— 行为断言在 `TZ=UTC` 宿主（CI）上对旧实现同样成立，故由源码级守卫兜底；
  `CommitGraph.tsx` 不得再出现 `willChange`（大 sizer 的层提升是未验证的内存风险）。
- `code-reviewer` 对照本方案出对齐表与分级结论；`tester` 实际执行测试命令出报告。

真机（macOS，手工冒烟）：

- Web Inspector 控制台 `document.body.querySelectorAll(':scope > .fixed').length`：关闭态应为 **0**
  （原 180–200），打开一套菜单时为 1。
- 触控板惯性滚动 3 轮 × 3 秒无空白带；慢速拖拽滚动条同样无空白。
- 回归：行右键 / 徽章右键贴鼠标；菜单开着滚动立即关闭；侧栏分支 / 标签 / Changes / Remotes
  右键正常；点分支跳转定位正常；滚到底续加载与 `endOfHistory` 文案；选中态 / HEAD 高亮；
  synced 徽章折叠；中英切换文案正常。
- 对照：同一仓库同一滚动起点、release 构建、autoRefresh 开 / 关各测一轮。
- 预期噪音（不是缺陷）：dev 构建下 react-aria `PressResponder` 会因关闭态锚点卸载而打印告警，
  历史列表下可能刷出多条；release 构建被剥离。冒烟时不要误判为问题（复审 🟡3）。

## 已知取舍与真机 A/B 候选

审查阶段确认的取舍（非缺陷，留档）：

- **相对时间文案不再随滚动自动刷新**：改动前 `formatTime` 每次滚动都重算（正是性能问题的一部分），
  现在只在行自身重渲时重算 ⇒ 在 autoRefresh 关闭、用户又不动列表时，「刚刚 / N 分钟前」可能长时间
  不变。这是 memo 的必然代价，本轮接受；若体感不好，可加 60s 低频 tick 让窗口每分钟重渲一次。
- **`will-change: transform`（sizer）与 `contain: layout paint`（行 wrapper）未默认开启**：二者都无法
  在 Windows 开发机上验证（WKWebView 的层提升 / 显存与 paint 裁剪行为）。真机 A/B 候选按性价比排序：
  1. 行 wrapper 的 `paint` containment（若确认 SVG 描边出血与横向溢出都不可见）；
  2. sizer 的 `will-change: transform`（需同时量内存快照）；
  3. `useFlushSync: false`（唯一能直接证伪「flushSync 是主因」的实验：在 devtools 临时改
     `node_modules/@tanstack/react-virtual/dist/esm/index.js` 做 A/B，量「空白带帧数」是增是减）；
  4. 徽章菜单控制器化（须同时把对话框状态上提，见「非目标 1」）。

## 回滚

改动仅限 3 个前端文件（`src/App.tsx` / `src/components/CommitGraph.tsx` /
`src/components/ui/ContextMenu.tsx`）+ 3 个新增 lib/test 文件；无后端、无数据 / schema 变更、无依赖变更 ——
直接丢弃分支即可。若真机发现菜单首次打开定位抖动，退化为「关闭态整体 `return null`」变体
（代价：失去退场动画）；若 `contain: layout` 造成任何可见裁剪，直接删掉该样式属性即可。
