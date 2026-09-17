# feat-repo-tab-wheel-scroll

> 让 `WorkspaceRepoTabs` 在 tab 溢出条带宽度时支持鼠标滚轮横向滚动，并让激活 tab 自动进入视野。

状态：已实现。

## 需求

仓库 tab 超过一屏宽度后，鼠标滚轮（竖直滚动）应能左右横向滚动该条带。

用户逐项确认的行为边界：

| 项 | 决策 |
|---|---|
| 滚到两端 | **一直吞掉**：指针在条带上时始终 `preventDefault`，到边缘也不冒泡 |
| 触控板 | 竖直滑动走同一套逻辑；`deltaX !== 0` 的横滑手势交回浏览器原生（保留惯性） |
| 附加项 | 激活 tab 自动进入视野（含批量添加仓库后最后一个仓库被激活） |
| 实现位置 | 新 hook + `WorkspaceRepoTabs`，不改 `ui/Tabs.tsx` |
| 文档 | 本 plan + [F017](../../pm/features/F017-repo-tab-wheel-scroll.md) |

## 现状与根因

`tabstripRef` 指向的条带下方实际有三层结构，**能滚的不是 `TabsList`**：

```
div[ref=tabstripRef]                        ← 外层条带：不可滚
└─ HeroTabs root
   └─ div[data-slot=tabs-list-container]    ← TabsList 的 className 落点：overflow-x-auto 但并非滚动元素
      ├─ div[data-slot=scroll-shadow]       ← 真正的滚动容器（overflow-x-auto + 隐藏滚动条）
      │  └─ div[role=tablist]               ← 各 TabsTrigger
      ├─ button（HeroUI 左箭头）
      └─ button（HeroUI 右箭头）
```

- tab 不会被压扁：`TabsTrigger` 是 `w-auto grow`，而 HeroUI 的 `.tabs__list` 为 `w-max min-w-full`（`@heroui/styles/components/tabs.css`），所以溢出真实发生，滚动容器也确实可滚。
- 竖直滚轮不会横滚：`ScrollShadow` 在横向模式下只有 `overflow-x`（`@heroui/styles/components/scroll-shadow.css`），竖直 delta 无处可去。
- **关键坑**：React 17+ 把合成 `onWheel` 注册为 passive，`preventDefault()` 无效（并会打印告警）。因此必须用原生 `addEventListener("wheel", handler, { passive: false })`。

## 实现

| 文件 | 作用 |
|---|---|
| `src/lib/horizontalWheel.ts`（新增） | 纯函数：`wheelScrollDelta`（像素/行/页换算，横滑手势返回 0）、`clampScrollLeft`（夹取）、`scrollLeftToReveal`（入视野目标位置） |
| `src/lib/horizontalWheel.test.ts`（新增） | 15 个单测覆盖上述三个函数 |
| `src/hooks/tabStripScroller.ts`（新增） | `findTabStripScroller(root)` → `[data-slot="scroll-shadow"]`；找不到时静默降级（不抛错、不吞事件） |
| `src/hooks/useHorizontalWheelScroll.ts`（新增） | 在条带上挂原生 `{ passive: false }` wheel 监听：算 delta → `preventDefault()` → 写 `scrollLeft` |
| `src/hooks/useRevealActiveTab.ts`（新增） | 依赖 `activeWorkspaceId` / `activeRepoId` / tab 数量：查 `[role="tab"][data-selected="true"]`，用 `getBoundingClientRect` 求相对滚动内容的位置，`scrollTo({ behavior: "smooth" })` |
| `src/components/WorkspaceRepoTabs.tsx`（修改） | 接入两个 hook（`useHorizontalWheelScroll(tabstripRef, activeWorkspaceId !== null)` 与 `useRevealActiveTab({ ... tabCount: renderedRepos.length })`） |

实现要点：

- hook 挂在条带容器（已有 `tabstripRef`）而非滚动节点上：滚轮落在 tab 上或条带空白处都生效，不依赖 HeroUI 内部 ref 透传。
- `enabled` / `tabCount` 入依赖是必需的：组件在 `activeWorkspaceId` 为空时 `return null`（DOM 未挂载），且启动时 `activeRepoId` 可能先于仓库列表恢复——只有把「Workspace 出现」和「tab 数量变化」纳入依赖，监听与入视野才会在这些时刻补上。
- 入视野用 `scrollLeftToReveal` + `scrollTo` 而不是 `scrollIntoView`：后者会连带滚动祖先容器（本应用是多面板固定布局）。
- 无需 i18n 改动（无新增文案）。
- 不改 `ui/Tabs.tsx`：目前只有仓库条带使用 `TabsList`，保持原语零改动。

## 测试与验证

自动：

- `pnpm test` — 208 passed（含新增 15 个）
- `pnpm typecheck` / `pnpm lint`（0 error）/ `pnpm format:check`

手工（dev 构建，workspace 放 10+ 仓库）：

- [ ] 条带上滚轮 → 横向滚动，1:1 跟手
- [ ] 滚到最右/最左继续滚 → 条带不动，页面/其他面板也不滚（"一直吞掉"）
- [ ] 触控板两指竖滑 → 横滚；两指横滑 → 原生惯性滚动，不出现双倍位移
- [ ] `Shift+滚轮`、HeroUI 左右箭头仍正常
- [ ] 切换仓库 / 切换 Workspace / 批量添加多个仓库后 → 激活 tab 自动平滑进入视野
- [ ] 拖动排序、右键菜单、点击切换不受影响
- [ ] 仓库很少（无溢出）时，滚轮在条带上无副作用（仅被吞掉）
- [ ] 隐藏滚动条场景下（macOS）行为一致

## 风险与取舍

| 项 | 说明 | 缓解 |
|---|---|---|
| "一直吞掉" 的副作用 | 无溢出时指针停在条带空白处，滚轮完全无响应 | 用户明确选择（独占行为）；条带本身很窄，实际影响小 |
| HeroUI 内部契约 | `[data-slot="scroll-shadow"]` 若在升级中改名，滚轮与入视野会**静默**失效 | `tabStripScroller.ts` 顶部注释标明来源与失效表现；升级 HeroUI 时纳入回归清单 |
| passive 陷阱 | 用 `onWheel` 会导致 `preventDefault` 无效 | 明令原生监听 + `{ passive: false }`，hook 注释说明 |
| DOM 行为不可单测 | vitest 为 `environment: "node"`，仓库无 jsdom | 所有算术抽到 `src/lib` 覆盖；DOM 部分按上面清单手工验证 |

## 关联

- 提案：[F017 · Repository Tab 溢出时滚轮横向滚动](../../pm/features/F017-repo-tab-wheel-scroll.md)
- 相邻功能：[F005 Repository Tab 拖动排序](../../pm/features/F005-repo-tab-drag-reorder.md)（同一条带的 pointer 交互，与本方案的 wheel 交互互不干扰）
