# fix-history-scroll-perf · 审查报告

> 审查对象：中间栏 commit 历史列表在 macOS 上滚动空白/卡顿的修复
> 代码版本：分支 `fix/history-scroll-perf`，基线 `main@1de31df`，**改动未提交**
> 审查基准：`docs/tasks/fix-history-scroll-perf/plan.md`
> 结论：**通过，可合并**（第 2 轮复审无 🔴）；真机验收项见文末

## 审查范围

| 文件 | 状态 | 关键改动 |
|---|---|---|
| `src/App.tsx` | 改 | `handleCommitSelect` 包 `useCallback([activeRepoId])`，让行 memo 不被 App 重渲击穿 |
| `src/components/CommitGraph.tsx` | 改 | `GraphRow`/`CommitRow` memo 化；`menu` prop 改为 `CommitMenuContext` 注入；`handleSelect` 稳定化；行内 refs `useMemo`；时间格式化下沉；分页 rAF 合并 + 阈值 600→1200；`overscan` 10→14；行 wrapper `contain: "layout"` |
| `src/components/ui/ContextMenu.tsx` | 改 | 关闭态不渲染隐形锚点（`mountAnchor`）；context value `useMemo`；`asChild` 注入 handler `useCallback` |
| `src/lib/commitTime.ts` | 新 | `formatCommitTime` / `formatCommitDate`，`toLocaleDateString` 按**本地日历日**缓存 |
| `src/lib/commitTime.test.ts` | 新 | 时间分档边界、本地日边界、缓存确定性、`now` 注入、时钟回拨 |
| `src/components/CommitGraph.renderGuards.test.ts` | 新 | 8 条静态源码断言（防回退） |

无 `package.json` / lockfile / locale / `src-tauri` 改动；未新增运行时依赖；未新增 i18n key。

## 七维度结论

| 维度 | 结论 |
|---|---|
| 正确性 | ✅ `CommitRow` 的 props 在滚动 / 选中 / HEAD / refetch 四种场景下均稳定；`menu` 改 context 注入后菜单体只在打开时挂载，永远读到最新控制器（无过期闭包）；`ContextMenuTrigger` hooks 顺序合法、语义不变 |
| 安全 | ✅ 无涉密面；未改凭据 / 网络 / 文件系统路径；无新增攻击面 |
| 性能 | ✅ 主目标达成路径清晰：每 scroll 事件的重渲行数从约 45 降到 1–3；关闭态 body 级 fixed 节点从约 180–200 降到 0。`useFlushSync` 有意保持默认（它本是为避免「新暴露区域空一帧」） |
| 可维护性 | ✅ 菜单控制器经 context 下传的做法与仓库既有 `useCommitMenuActions` + 根部 `renderModals()` 模式同源；时间格式化下沉为可测纯函数 |
| 可读性 | ✅ 注释均说明「为什么」而非「是什么」；中文/英文注释与各文件既有语言一致 |
| 测试覆盖 | ✅ 235 用例全绿（新增 2 个测试文件）；守卫覆盖 memo、`overscan`、`contain`、时间下沉、锚点条件挂载、`App.tsx` 稳定性、本地日缓存键、无 `willChange`。**已知盲区**：无 DOM 渲染测试基建（vitest 跑 node 环境，无 jsdom/testing-library），「memo 是否真的生效」只能靠真机 React Profiler 确认 |
| 最佳实践 | ✅ 未改任何对外 props 签名与导出；6 个 `ContextMenuContent` 消费方行为等价；未越界改动（非目标 3 项均未触碰） |

## 问题分级

### 🔴 必须修（第 1 轮，已修）

**R1 · `commitTime.ts` 缓存键粒度错配会显示错误日期**
缓存键原为 UTC 日桶 `Math.floor(time / 86400)`，而 label 由 `toLocaleDateString()`（本地日历日）决定。
任何非零时区下，本地午夜都落在某个 UTC 桶内部 ⇒ 同桶的两个不同本地日期命中同一条缓存，后者显示前一天的日期。
实跑复现（本机 UTC+8）：本地当天 00:30 与前一天 23:30 的旧键同为 `20716`，`formatCommitDate` 对两者都返回 `2026/9/21`。
**修复**：改为按本地日历日取键（`getFullYear/getMonth/getDate`），已实跑验证两时刻分别返回 `2026/9/21` / `2026/9/20`。

### 🟡 建议修（均已处理）

| # | 问题 | 处理 |
|---|---|---|
| Y1 | 计划曾声称「关闭态早退不损失开合动画」——**事实错误**：HeroUI 样式表用 `.popover[data-exiting=true]{animation:exit …}` 驱动退场，RAC 靠 `isExiting` 期间保持挂载才能播放 | 改为「保留 `<Popover>` 本体挂载、只条件渲染锚点」，退场动画保住；方案文档已订正 |
| Y2 | 相对时间文案不再随滚动自动刷新（memo 的必然代价；autoRefresh 默认关） | **有意接受**，已记入方案「已知取舍」；若体感不好再加 60s tick |
| Y3 | sizer 上 `will-change: transform`：数万 px 高元素的层提升在 WKWebView 上的显存/光栅行为无法在开发机验证 | 已删除，列入真机 A/B 候选（方案已记录） |
| Y4 | `contain: "layout paint"` 会裁掉 `GraphRow` SVG 的描边出血与横向溢出 | 已降级为 `contain: "layout"` |
| Y5 | `asChild` 分支注入的 handler 仍每帧新建，方案「被注入元素可 memo」只做了一半 | 已用 `useCallback` 稳定化 |
| Y6 | 缺守住核心不变量的回归测试 | 已补 `App.tsx` 的 `useCallback` + 直传断言 |
| Y7 | 「同一日历日」用例恒真，无法失败 | 已换成本地日边界用例；另补源码级守卫覆盖 `TZ=UTC` 宿主（CI）盲区 |
| 🟡1 | R1 的行为断言在 `TZ=UTC` 宿主（CI 默认）上对旧实现同样成立 ⇒ 守卫静默失效 | 已补与宿主时区无关的源码级守卫 |
| 🟡2 | 反回归正则漏掉带花括号的等价写法（正是本轮修掉的缺陷） | 已加强（覆盖花括号/换行/空格），并锚定 `contain` 到行 wrapper、补 `willChange` 断言 |
| 🟡3 | 关闭态锚点卸载会让 react-aria `PressResponder` 在 dev 构建打印告警（release 剥离） | 已写入方案的真机冒烟清单，标注为预期噪音 |

### 🟢 可选（未处理，留档）

- `ContextMenuTrigger` 注入 handler 的稳定化在「子元素自带行内 `onContextMenu`」的热路径上仍是名义上的（无功能/性能影响，被注入的是宿主元素，行本身已 memo）。
- `CommitMenuBody` 在 context 缺失时静默 `return null`，可改为抛错（与 `useContextMenuState` 一致）。
- `BranchList` / `ReflogPanel` / `BlameView` 仍各自持有时间格式化实现，可后续统一到 `commitTime.ts`。
- `pnpm test -- <files>` 的参数会被 `--` 吞掉（跑成全量），如需 CI 跑子集需改 `package.json`。

## 测试结论（tester）

- 被测版本：分支 `fix/history-scroll-perf`，HEAD `1de31df`，工作区 dirty（改动未提交）。
- `pnpm format:check` ✅ · `pnpm lint` ✅（0 error / 40 warning，**本次改动引入 0 条**，逐文件与 HEAD 版对照确认）· `pnpm typecheck` ✅ · `pnpm test` ✅ 31 文件 / **235 用例**。
- 后端未触及（`src-tauri` 零改动），`cargo` 门禁未执行。
- 已知基础设施缺口：无 jsdom/@testing-library，无 Playwright 配置（`test:e2e` 实际不可用）。

## 必须 macOS 真机确认

1. **缺陷本体**：触控板惯性滚动 3 轮 × 3 秒无空白带；3 秒内 >50ms 长帧 < 3 次；慢速拖拽滚动条同样无空白。
2. `document.body.querySelectorAll(':scope > .fixed').length`：关闭态应为 **0**（原 180–200），打开一套菜单时为 1。
3. **条件挂载锚点的首次打开定位**：行右键 / 徽章右键菜单首帧无抖动或偏移（异常则回退为「关闭态整体 `return null`」，代价是失去退场动画）。
4. 退场动画仍在播放（6 个消费方各试一次）；`contain: "layout"` 无可见裁剪。
5. 菜单开着滚动仍立即关闭；菜单关闭后焦点落点正常；控制台除 `PressResponder` 预期告警外无新增报错。
6. 中英切换后行内时间文案仍会刷新；autoRefresh 开/关各测一轮（确认 Y2 的陈旧在可接受范围）。
7. 回归冒烟：行右键 / 徽章右键贴鼠标；侧栏 BranchList / TagsPanel / ChangesPanel / RemotesPanel 右键正常；点分支跳转定位正常；滚到底续加载与 `endOfHistory` 文案；选中态 / HEAD 高亮；synced 徽章折叠。

## 若真机仍不达标（按性价比排序）

1. 先定性：用 React Profiler + rAF 长帧采样区分「React 提交耗时」与「合成/光栅耗时」——若为后者，方案里的候选 2/3 收益本就小，不应先做。
2. 低成本 A/B：行 wrapper 的 `paint` containment、sizer 的 `will-change: transform`（同时量内存快照）。
3. `useFlushSync: false`：唯一能直接证伪「flushSync 是主因」的实验（devtools 临时改 `node_modules` 默认值，量空白带帧数增减）。
4. 徽章菜单控制器化（须同时把 `confirm`/`mergeDialog`/`checkout.renderDialogs()` 上提到列表根部）、行菜单上提为根部单菜单 —— 回归面覆盖 `fix-history-menu-drift` 与 F011，放最后。
