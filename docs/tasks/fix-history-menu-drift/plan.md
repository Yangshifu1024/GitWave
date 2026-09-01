# fix-history-menu-drift · history 提交行右键菜单严重漂移

> 状态：已实现（待冒烟：见文末手工冒烟要点）
> 现象（用户，截屏 2026-09-01）：history 中 commit 右键，菜单不出现在鼠标处，
> 而是出现在偏右且垂直偏移数百像素的位置；偏移量随滚动位置变化。

## 现象示意

```
现状（漂移）                              修复后
┌─sidebar─┬─ history ────────────        ┌─sidebar─┬─ history ────────────
│         │  [行 f1fbf12] ▲点击          │         │ [行 f1fbf12] ▲点击
│         │        ┌──────────┐          │         │   ┌──────────┐
│ ┌──────────┐     │ 新建分支… │          │         │   │ 新建分支… │ ← 菜单贴
│ │ 新建分支… │     │ 检出提交… │          │         │   │ 检出提交… │   鼠标右下
│ │ 检出提交… │     └──────────┘          │         │   └──────────┘
│ └──────────┘  锚点=行偏移+clientX/Y     │         │  锚点=clientX/Y（视口）
└─────────┴─────────────────────        └─────────┴───────────────────
```

## 根因

1. `src/components/CommitGraph.tsx`（虚拟滚动，`@tanstack/react-virtual`）每个行
   wrapper 带 `transform: translateY(${virtualRow.start}px)`（:531）。
2. 共享原语 `src/components/ui/ContextMenu.tsx` 的 `ContextMenuContent`（:93-97）
   用一个隐形锚点定位菜单：

   ```tsx
   <Popover.Trigger
     aria-hidden
     className="fixed z-popover h-px w-px overflow-hidden p-0 pointer-events-none"
     style={{ left: ctx.point.x, top: ctx.point.y }}   // clientX/clientY
   />
   ```

   它随 `<ContextMenuContent>` 内联渲染在行 wrapper **内部**。
3. CSS 规范：带 `transform` 的祖先会成为 `fixed` 后代的**包含块**。锚点的
   `left/top = clientX/clientY` 被当成相对行 wrapper 解释，HeroUI/RAC 用
   `triggerRef.getBoundingClientRect()`（calculatePosition.js:364）测量锚点定位
   Popover，菜单整体偏移 = 行 wrapper 的视口偏移（水平 ≈ 侧栏宽度，垂直 =
   `virtualRow.start − scrollTop`，随滚动变化）。
4. 影响面：同一 `ContextMenuContent` 的 6 个消费方中，**CommitGraph 行菜单**与
   **RefBadgeContextMenu**（行内 ref 徽章，同在 transform wrapper 内）漂移；
   BranchList / TagsPanel / ChangesPanel / RemotesPanel 无 transform 祖先，
   表现正常（本次修复对它们是等价重构）。

已排除：clientX/pageX 混用（全链路视口坐标一致）、placement/offset 配置、
侧栏与 history 实现分歧（同一组件）。

## 可行性验证（node_modules 源码级）

- RAC `DialogTrigger`（react-aria-components/dist/private/Dialog.js:58-86）通过
  React context（`OverlayTriggerStateContext` + `PopoverContext.triggerRef`）
  接线 Trigger 与 Popover，**与 DOM 树结构无关** → portal 不断开 context。
- 定位测量纯 DOM：`useOverlayPosition` → `calculatePosition` 对
  `triggerRef.current` 做 `getBoundingClientRect()` → trigger 挂到 body 后
  rect 即真实 `clientX/clientY`。
- HeroUI `Popover.Content` 本身已通过 react-aria `Overlay` portal 到
  `document.body`（Overlay.js:31,53），仅隐形 Trigger 留在原地。

## 改动清单

只改 `src/components/ui/ContextMenu.tsx`，6 个消费方统一生效：

1. 隐形锚点用 `createPortal(..., document.body)` 渲染，脱离 transform 祖先，
   `fixed` 坐标回归视口坐标。
2. 补回「滚动时关闭菜单」：RAC 的 `useCloseOnScroll` 靠遍历 trigger 的滚动父链
   挂 scroll 监听（useCloseOnScroll.js:35，getScrollParent），portal 到 body 后
   该链失效。在 `ContextMenu` 加 `useEffect`：`isOpen` 期间
   `window.addEventListener("scroll", close, { capture: true })`，关闭/卸载时
   移除。scroll 不冒泡，必须 capture 才能覆盖内部滚动容器。

## 同批附带修复（同一 PR，均为右键菜单缺陷）

### A. commit 菜单「检出提交」缺图标

`src/components/CommitContextMenu.tsx` 的 checkout 项是唯一没图标的菜单项；
补 `CornerDownRight`（size 14，与其余 lucide 图标一致），语义为「HEAD 移到此处」。

### B. 分支菜单 push 目标写死 origin（多 remote 子菜单方案）

`src/components/BranchList.tsx` 原 `upstreamRemote()`：无 upstream 时硬编码兜底
`"origin"`——菜单文案与实际 push 目标都随之错误，且多 remote 仓库没有选择入口。

最终方案（Fork 式子菜单，用户选定）：

```
单 remote（现状不变）                 多 remote（新增子菜单）
├─ 推送到「origin」…                  ├─ 推送…              ▸
└─ …                                 │    ├─ origin
                                      │    ├─ gitlab
                                      │    └─ upstream
                                      └─ …
无 remote：push 项置灰，文案「推送…」
```

1. 原语 `src/components/ui/ContextMenu.tsx` 新增 `ContextMenuSub`：RAC
   `SubmenuTrigger`（react-aria-components 1.20，HeroUI 未包装故显式声明依赖，
   精确锁 1.20.0 与 @heroui/react peer 一致）+ `Menu.Item` 触发项（自带
   ChevronRight，RAC 不自动渲染箭头）+ HeroUI `Popover.Content` 作二级菜单容器
   （内部渲染 RAC Popover，自动消费 SubmenuTrigger 提供的 `end top` 定位上下文，
   样式与一级菜单一致）。选中子项经 `ContextMenuItem` 关闭整个菜单。
2. BranchList push 项三分支：`remotes.length === 1` 扁平项（文案带 remote 名）；
   `> 1` 子菜单逐 remote 列出；`=== 0` 置灰「推送…」。
3. `pushConfirm` 状态携带用户选定的 remote：`{ branch, remote }`；`onPush`
   签名 `(branch, remote)`；`submitPush` 与确认弹窗描述直接使用选定值。
   原 `pushTargetRemote` 推导函数删除（选择权交还用户，无隐式默认）。
4. `["remotes", workspaceId]` 查询与 ActionBar pull/push 弹窗共享缓存。
5. i18n 新增 `branches.menu.pushGeneric`（zh「推送…」/ en "Push…"），
   双 locale 同步，parity 测试通过。

### C. 侧栏标签默认折叠，切换仓库后重置为折叠

`src/components/TagsPanel.tsx` 的 `SidebarSection` 原先走默认 `defaultOpen=true`。
改为 `defaultOpen={false}`；又因 HeroUI `Disclosure` 是非受控组件、TagsPanel 在
App.tsx 单实例挂载不会随仓库重挂，给 `SidebarSection` 加 `key={repoId}`，
切换仓库时重挂重置为折叠（不会把上个仓库的展开态带过去）。

### D. 新建 remote 不出现在分支菜单推送列表

RemotesPanel 用本地 `useState` + `listRemoteDetails` 管数据，增/删/改 remote 后
只刷本地列表，从不失效 BranchList 与 ActionBar 消费的 `["remotes", workspaceId]`
React Query 缓存——新建的 remote 到不了分支菜单（一直显示打开时的旧列表）。
修复：RemotesPanel 的 `run()`（add / edit / remove 共用出口）成功后
`invalidateQueries({ queryKey: ["remotes", workspaceId] })`，BranchList 的
活动查询立即重取，push 菜单随之更新。fetch 不改配置，本不需要失效，但共用
出口使其顺带多一次冗余重取（无害，见 review.md 🟢1）。

## 测试

- `npm run typecheck` / `npm run lint` / `npm test` 全绿
  （无组件渲染测试基建——vitest node 环境、无 testing-library，见
  package.json；DOM 级回归测试归入组件测试后续项）。
- 手工冒烟要点（`npm run tauri dev`）：
  - history 行右键：顶部行、滚动到中部后的行，菜单均贴鼠标右下弹出；
  - 行内 ref 徽章右键：同样贴鼠标；
  - 菜单开着滚动提交列表：菜单立即关闭；
  - 侧栏分支 / 标签页 / Changes / Remotes 右键：行为与修复前一致（贴鼠标）；
  - 菜单项（检出/重置/拣选/撤销/复制等）功能不回归；
  - commit 菜单「检出提交」带图标，与其他项对齐；
  - 单 remote 仓库：分支右键 push 文案带真实 remote 名，行为同前；
  - 多 remote 仓库：push 项悬停展开子菜单列出全部 remote，选哪个推哪个，
    确认弹窗与状态区结果一致；
  - 无 remote 的仓库：push 项置灰；
  - 侧栏标签：初始为折叠；展开后切换仓库，标签恢复折叠；
  - 远程卡片新增 / 重命名 / 删除 remote 后，右键本地分支的 push 菜单
    （扁平项文案或子菜单列表）立即反映最新 remote 列表；
  - 菜单开着滚动其他面板（如 history 列表）：菜单应立即关闭
    （capture scroll 行为面比 RAC 原生滚动父链更宽，属预期，见 review.md 🟢2）。
