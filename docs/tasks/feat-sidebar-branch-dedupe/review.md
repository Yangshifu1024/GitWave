# review.md · feat-sidebar-branch-dedupe

> 审查人：code-reviewer 代理（七维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践）
> 审查对象：分支 `feature/sidebar-branch-dedupe` 未提交改动
> 结论：需求 1 **无 🔴，2 项 🟡 已修复**；需求 2 二轮审查 **1 项 🔴 与 2 项 🟡 已修复**，验证全绿

## 审查确认正确的点

- 过滤规则抽纯函数 + vitest 单测，符合仓库 `src/lib/*.test.ts` 惯例
- Set 成员查找，整体 O(n+m)，无性能负担
- 过滤在前端展示层、后端 `getBranches` 保持完整数据；plan.md 决策表记录清晰
- 空态边界正确：`branches.length === 0` 用原始列表判断，本地分支为空时不会有任何远程被过滤，非空数据至少渲染一组（`BranchList.tsx:452`）
- checkout 交互安全：远程分支双击被 `checkoutGate.ts:19-26` 阻断（blocked 弹窗），不存在"checkout 远程分支自动建本地分支"的路径
- 多 remote：本地 `main` 同时隐藏 `origin/main` 与 `upstream/main`，与需求及决策一致

## 🟡 问题与修复记录

### R1 · 选中恢复逻辑与渲染规则脱节（悬挂选中）— 已修复

- 原问题：refresh 后 `setSelectedName` 恢复判定作用在**未过滤**列表上。复现：选中 `origin/feat-x`（本地无同名）→ New 创建本地 `feat-x` → refresh 后该远程分支被隐藏但选中仍保留，无高亮行且不回落到 current 分支。`onBranchSelect` 目前未接线，影响限高亮；后续接线会升级为必须修复
- 修复：过滤规则抽为 `filterRemoteBranches`（`src/lib/branchNames.ts`）单一来源；恢复判定改用 `filterRemoteBranches(updated).some((b) => b.name === prev)`（`BranchList.tsx:251`），被隐藏项自然回落 current 分支

### R2 · 需求本体行为零测试覆盖 — 已修复

- 原问题：`localNames` 与 `remoteShortName` 的组合逻辑内联在组件里（无组件测试设施），纯函数测试只覆盖等式一半；渲染与恢复两处各写一份规则存在重复
- 修复：`filterRemoteBranches` 入 `src/lib/branchNames.ts`（泛型，仅约束 `{ name, kind }`，不耦合 api.ts），渲染与恢复共用；补 5 用例：同名隐藏 / 嵌套同名隐藏 / 无同名保留 / 多 remote 全隐藏 / 纯远程仓库全保留

## 🟢 建议处理情况

| 建议 | 处理 |
|---|---|
| 退化输入用例（`""`、`origin/`） | 已补（remoteShortName 第 5 例） |
| `origin/HEAD` 符号引用可能留在 REMOTE 组 | 未处理（超出本需求范围，且是否出现未实证；行为与改动前一致，无回归。若后续确认出现，可在 `filterRemoteBranches` 顺带排除短名 `HEAD`） |
| `localNames` Set 每渲染重建 / useMemo | 不需要（抽取后仅两处 O(n+m) 过滤，分支数量级小） |

## 二轮审查（需求 2 · 点击分支定位 History）

### 审查确认正确的点（代理经 tanstack-virtual 源码核实）

- loading / error / 空仓库 early-return 时 `scrollRef` 为 null，`scrollToIndex` 内部 `scrollElement?.scrollTo` 为真 no-op，无崩溃无悬挂任务
- 加载完成后补滚成立：`shaToIndex` 为 `useMemo([commits])`，数据到达生成新 Map 触发 effect 重跑
- `virtualizer` 实例跨渲染引用稳定（react 适配器 `useState(() => new Virtualizer())`），作为 effect 依赖安全
- 移除 checkoutOnto 中 `onBranchSelect` 旧调用：改动前 prop 从未传入，本就是死代码，无行为影响
- 安全维度：无新增 Tauri 命令、sha 仅用于前端索引查找；性能 O(1) Map 查找

### 🔴 问题与修复记录

**R3 · locateRequest 从不消费，"一次性定位"退化为"每次 history 刷新强制居中" — 已修复**

- 原问题：effect 依赖 `shaToIndex`，每次 commits 重载（提交 / pull / push / 分支操作均触发 `bumpHistoryEpoch`）都会在 repoId 匹配且 sha 命中时重新居中到旧 sha——用户手动滚动后一提交就被拽回；切换仓库 A→B→A 后 A 的 log 重载也会在无点击的情况下再次居中
- 修复：CommitGraph 增加 `handledLocateSeq` ref，仅在真正执行滚动时标记 `seq`；判定抽为纯函数 `resolveLocateIndex`（`src/lib/commitLocate.ts`）。sha 缺失（异步加载中）不标记，加载完成后仍可补滚；已处理的请求对后续刷新返回 null

### 🟡 问题与修复记录

**R4 · 仓库切换窗口期点击旧仓库分支，会把异仓库 sha 写入 commitSelection — 已修复**

- 原问题：activeRepoId A→B 后、B 数据到达前，A 的分支行仍渲染可点；点击会以当前 activeRepoId（B）+ A 的 sha 设置选中，DiffViewer 请求 B 仓库不存在的 sha 报错
- 修复：BranchList 的 `activeRepoId` 变化 effect 中同步 `setBranches([])`（原仅清 `selectedName`）；只依赖 `activeRepoId` 而非 `historyEpoch`，正常分支操作刷新不闪空列表

**R5 · 需求 2 行为无测试 — 已修复**

- 修复：判定逻辑抽为纯函数 `resolveLocateIndex` 后补 5 用例（新鲜请求命中 / 已处理 one-shot / 跨仓库含 null repoId / 提交不在窗口 / 无请求）；vitest 总数 44 → 49

### 🟢 建议处理情况

| 建议 | 处理 |
|---|---|
| `{repoId, sha, seq}` 内联类型两处重复 | 已改：`LocateRequest` 从 `src/lib/commitLocate.ts` 导出，App / CommitGraph 复用 |
| checkout 成功后不定位新分支 tip | 暂不处理（产品取舍：需求限定"点击"分支；checkout 定位可作后续体验增强，见 plan.md 决策表） |

## 三段（需求 3 · 窗口 / 面板配色）

改动为纯 token 值调整 + 新增面板画布 token，未跑独立代理轮，采用机械产物级验证（提交 / PR 前建议对整分支 diff 做一次 code-reviewer 终审）：

- 主窗口背景 `--color-bg-primary` #ececec→#ececee（用户指定 #ececee）；新增面板画布 token `--color-bg-panel: #f8f8f8`（初版命名 bg-history 只覆盖 history，用户修订纳入右侧面板后改名 bg-panel）
- 消费点：History 区（App main section + CommitGraph 节点 SVG 描边抠图）+ 右侧 Inspector 区（MainContent 三处 main 容器，原 bg-elevated，含两个空态）
- dark / tide palette 的 bg-panel 取各 palette bg-primary 同值 → 非默认 light 主题零视觉变化，不为未指定主题发明新色
- 产物验证：构建 CSS 含 `.bg-bg-panel` 工具类、`.bg-bg-panel` 上的 track 覆盖规则；6 处 `--color-bg-panel` 与 6 个 theme 块一一对应（light / dark×2 / tide light / tide dark×2）；src 与 design docs 无 bg-history 残留
- 影响面核查：`bg-bg-primary` 其余使用点均为 hover 派生色调 / 小输入框，随 #ececee 微调肉眼不可辨；DiffViewer 内部 chips / gutter（bg-secondary / bg-elevated）属面板内层次色不动
- 修订 2（面板滚动条 track）：track 原全局 `--color-scrollbar-track: var(--color-bg-secondary)`，在 #f8f8f8 面板上呈侧栏灰不一致；在 `.bg-bg-panel` 容器覆盖该自定义属性（继承至面板内全部滚动条，含 CommitGraph / DiffViewer），侧栏与窗口滚动条维持 bg-secondary 不变；dark / tide 下 track 随 bg-panel=bg-primary 同理对齐
- 修订 3（侧栏 / WorkingCopyBar = #ececee）：侧栏 aside 与 WorkingCopyBar 三种形态根容器 `bg-bg-secondary`→`bg-bg-primary`（dark 下语义跟随，#202022→#262628 轻微变化）；Toolbar / DiffViewer gutter / 菜单焦点等其余 bg-secondary 层次色不动；滚动条 track 规则由单面板覆盖泛化为四个画布类各自重声明（最近 bg 类胜出），否则 App 根容器的 bg-bg-primary 会把 #ececee track 泼到 ConflictPanel 等 bg-secondary 滚动区
- 修订 4（侧栏滚动条隐藏）：新增通用 `.no-scrollbar` 工具类（`scrollbar-width: none` + webkit 伪元素 display:none / 尺寸 0），应用于侧栏 aside；纵 / 横滚动条不显示但滚轮 / 触控板滚动保留，类特异性覆盖 tokens.css 的 `*` 全局滚动条规则；其余区域滚动条不受影响
- 修订 5（Topbar = #ececee）：Toolbar 根容器 `bg-bg-secondary`→`bg-bg-primary`；至此 chrome（顶栏 / 侧栏 / WCB / 窗口画布）统一 #ececee，bg-secondary 仅剩层次色用途（gutter / hover / 菜单焦点）
- 需求 4（diff 视图密度 / 配色）：外层 padding `p-4`→`p-2`（后修订 `pb-2`，仅保留底部）、文件卡 `mb-6`→`mb-3`；hunk 容器去 `rounded-md`；标题 / 行号配色初版切 bg-bg-primary，后按用户修订改为右栏非内容元素统一 `bg-bg-elevated`（#f4f4f5：标题栏 / 单文件路径条 / 行号列 / hunk @@ 头 / BlameView 头；文件名 chip 融入标题栏）。行号列 unified `w-12`→`w-9`、split `w-10`→`w-9`（pr 同步 2→1.5）。纯类名调整，typecheck / test（49）/ lint / format:check / build 全绿
- 需求 5（右栏宽度）：`initialInspectorWidth` 360→480、`inspectorMin` 240→360（max 720 不变）；03-layout.md 三处描述同步。纯默认 props 调整
- 需求 6（分割线 1px）：`HANDLE_PX` 2→1，两条三栏分隔同时收窄；拖拽 / 双击复位行为不变
- 需求 7（删除入口右键化）：WorkspaceList / RepoList 行内删除图标按钮移除，改为行右键 ContextMenu 文字项（Delete / Remove，destructive 无图标），沿用原确认弹窗与 mutation；radix trigger 与 ListItem onClick 共生（ChangesPanel 先例）。typecheck / test（49）/ lint / format:check / build 全绿
- typecheck / test（49）/ lint / format:check / build 全绿

## 验证

- 修复后复验：`npm run typecheck` / `test`（49 通过，含 branchNames 10 例 + commitLocate 5 例）/ `lint` / `format:check` / `build` 全绿
- 一轮审查中代理独立复跑过 branchNames 单测、typecheck、eslint；二轮审查代理核实了 tanstack-virtual 源码层面的空安全与实例稳定性

## 遗留

- 手动冒烟（dev）：同名并存隐藏 / 删本地后远程重现 / REMOTE 整组消失，见 plan.md 验收清单
- AI 不执行 commit（P1），由用户提交本分支
