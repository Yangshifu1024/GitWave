# feat-sidebar-branch-dedupe · 侧栏分支增强 + 窗口 / History 配色

> 状态：实施完成（review 通过，待手动冒烟；分支未提交，由用户 commit）
> 需求 1（用户，2026-08-27）：左侧栏中的分支部分，如果本地已经存在同名分支，则远程部分不再出现该分支。
> 需求 2（用户追加，2026-08-27，同分支实施）：点击左侧栏分支时，右侧 History 自动定位到对应提交并选中，且该提交显示在屏幕中间一行。
> 需求 3（用户追加，2026-08-27，同分支实施；同日修订：右侧面板一并纳入）：主窗口背景更换为 #ececee；中间和右侧面板背景修改为 #f8f8f8。
> 需求 4（用户追加，2026-08-27，同分支实施）：右侧 diff 视图——减小每个 diff 项四周 padding；标题、行号背景与 topbar 一致；缩小行号宽度；移除 diff 容器圆角。
> 需求 5（用户追加，2026-08-27，同分支实施）：右边栏默认宽度调整为 480，最小宽度 360。
> 需求 6（用户追加，2026-08-27，同分支实施）：三栏分割线宽度调整为 1px。
> 分支：`feature/sidebar-branch-dedupe`

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 过滤层级 | 前端展示层（BranchList.tsx） | 需求限定"左侧栏的分支部分"，属 UI 行为；`getBranches` 当前唯一消费者就是该组件，后端 API 保持完整数据便于未来其他消费者（如分支切换器） |
| 同名判定 | 远程分支名去掉首段 `<remote>/` 得短名，与本地分支名精确匹配 | `origin/main`→`main`、`origin/feat/x`→`feat/x`；libgit2 `branch.name()` 对远程分支返回 remote-tracking 简名（`origin/main` 风格），去首段即短名 |
| 多 remote 行为 | 全部隐藏 | 本地 `main` 存在时 `origin/main`、`upstream/main` 均不显示；与 remote 无关，符合需求字面语义 |
| REMOTE 分组空态 | 整组不渲染 | 复用现有 `remoteBranches.length > 0` 判断，无需额外处理 |
| 不建 F 编号 PM 文档 | 直接 task plan.md | 跟随 feat-wcbar-discard-ignore 先例：用户小需求直接建任务目录 |

### 需求 2（点击分支定位 History）

| 决策点 | 结论 | 说明 |
|---|---|---|
| 定位的提交 | 分支 tip（`branch.last_commit_sha`） | BranchList 已持有该字段，无需从 commit refs 反查 |
| 选中语义 | 复用 App `handleCommitSelect` | 与在 History 点提交一致：commit 高亮 + DiffViewer 显示该提交 diff + 清空 workdir 选中（App 状态提升模式，带 repoId 守卫） |
| 滚动信号 | App 持有 `locateRequest {repoId, sha, seq}` 传 prop | 瞬态命令用状态提升（模式 A）；`seq` 自增使重复点击同一分支也能重新居中；repoId 守卫防跨仓库残留 |
| 居中方式 | `virtualizer.scrollToIndex(index, { align: "center" })` | CommitGraph 已有 `shaToIndex` Map + tanstack-virtual 实例；行高固定 28px，offset 精确 |
| 提交不在日志窗口（200 条外） | 静默 no-op | `shaToIndex` 未命中即返回；不滚动不报错 |
| checkout（双击）不触发定位 | 仅单击选中分支触发 | 需求限定"点击"；checkoutOnto 中原未接线的 `onBranchSelect?.(name)` 一并移除（从未有消费者，语义收敛为"点击分支行"） |
| 定位请求一次性消费 | CommitGraph 以 `handledLocateSeq` ref 标记已处理 `seq` | review 🔴 修复：否则每次 history 刷新（提交 / pull / 分支操作）都会把视口拽回旧 sha；判定抽纯函数 `resolveLocateIndex`（src/lib/commitLocate.ts）可单测 |
| 异步加载时序 | effect 依赖 `shaToIndex`（useMemo on commits） | 点击早于日志加载完成时，加载完成后 effect 重跑补滚动；sha 始终不在 200 条窗口内则静默 no-op |
| 仓库切换竞态 | `activeRepoId` 变化 effect 中同步清空 branches | review 🟡 修复：防止旧仓库分支行在重载窗口期被点击、把异仓库 sha 写入 commitSelection；仅依赖 repoId 不依赖 epoch，常规刷新不闪空列表 |

### 需求 3（窗口 / 面板配色）

| 决策点 | 结论 | 说明 |
|---|---|---|
| token 对应 | 主窗口背景 = `--color-bg-primary`（html/body + 应用根容器）；中 / 右面板此前分别共用 bg-primary 与 bg-elevated | 需求 3 前面板底色跟随窗口色，必须拆独立 token 才能单独配色 |
| 改动方式 | `bg-primary` #ececec→#ececee；新增 `--color-bg-panel: #f8f8f8`（light） | 面板画布 token：History 区（App main section + CommitGraph 节点 SVG 描边抠图）+ Inspector 区（MainContent 三处 main 容器） |
| token 命名 | `bg-history` → `bg-panel`（初版只覆盖 history，修订纳入右侧面板后改为通用面板语义） | Tailwind 类 `bg-bg-panel` |
| dark / tide palette | bg-panel 均取各 palette 的 bg-primary 同值 | 用户只给 light 值；dark 与 tide 视觉零变化，不为未指定的主题发明新色 |
| hover 色调随 token 微调 | 不特殊处理 | `hover:bg-bg-primary/60` 等派生色跟随 #ececee，肉眼不可辨；DiffViewer 内部 chips / gutter 仍用 bg-secondary / bg-elevated，属面板内层次色不动 |
| 面板滚动条 track（修订 2） | `.bg-bg-panel` 容器上覆盖 `--color-scrollbar-track: var(--color-bg-panel)` | track 原全局取 bg-secondary（侧栏灰），与面板 #f8f8f8 不一致；自定义属性继承至面板内全部滚动条（CommitGraph / DiffViewer），侧栏与窗口滚动条不变 |
| 侧栏 / WorkingCopyBar 配色（修订 3） | 两处 `bg-bg-secondary` 类改 `bg-bg-primary`（#ececee），token 值不动 | 用户指定与主窗口同色；Toolbar、DiffViewer gutter、菜单焦点等其余 bg-secondary 层次色不动；dark 下侧栏 / WCB 语义跟随 = bg-primary（#262628，原 #202022，轻微变化） |
| 滚动条 track 规则泛化（修订 3） | 四个画布类（primary / secondary / elevated / panel）各自重声明 track | 仅给 bg-bg-primary 加规则会经 App 根容器泼到 ConflictPanel 等 bg-secondary 滚动区造成新错位；按"最近画布类胜出"继承后，侧栏 / WCB / 面板 / 弹层滚动条均与所在底色一致 |
| Topbar 配色（修订 5） | Toolbar 根容器 `bg-bg-secondary` 类改 `bg-bg-primary`（#ececee） | 整个 chrome（顶栏 / 侧栏 / WCB / 窗口画布）统一 #ececee，面板保持 #f8f8f8；dark 下语义跟随 bg-primary |
| diff 密度（需求 4） | 外层 `p-4`→`p-2`、文件卡 `mb-6`→`mb-3`；hunk 容器去 `rounded-md` | 每个文件 diff 卡四周留白减半，多文件间隔同步收紧；hunk 白底代码块方角 |
| diff 标题 / 行号配色（需求 4） | 文件标题栏（含单文件模式路径条）与 unified 行号列 `bg-bg-secondary`→`bg-bg-primary` | 与 topbar 同为 #ececee；hunk `@@` 头与 BlameView 头保持 bg-secondary 未动（用户未点名，需要可后续跟进） |
| 行号宽度（需求 4） | unified `w-12 pr-2`→`w-9 pr-1.5`；split `w-10 pr-2`→`w-9 pr-1.5` | 两种模式行号列统一 36px，3-4 位行号仍容纳 |
| 右栏宽度（需求 5） | `initialInspectorWidth` 360→480、`inspectorMin` 240→360，`inspectorMax` 保持 720 | ThreePaneLayout 默认 props；双击分隔条恢复 480；docs/design/03-layout.md 三处 ~360px 同步 ~480px |
| 分割线宽度（需求 6） | `HANDLE_PX` 2→1 | 两条分隔（侧栏-中栏、中栏-右栏）同时收窄 1px，拖拽行为不变 |
| 侧栏滚动条隐藏（修订 4） | 新增 `.no-scrollbar` 工具类应用于侧栏 aside | `scrollbar-width: none` + `::-webkit-scrollbar { display: none }`，纵 / 横向滚动条均不显示，滚轮 / 触控板滚动保留；类特异性高于 tokens.css 中 `*` 全局滚动条规则 |

## 改动清单

### 前端（纯前端改动，无 Rust 变更）
- `src/lib/branchNames.ts`（新建）：
  - 纯函数 `remoteShortName(name: string): string` — 去掉首个 `/` 及之前的部分；无 `/` 原样返回
  - 纯函数 `filterRemoteBranches<T extends { name; kind }>(branches: T[]): T[]` — 隐藏与本地分支同短名的远程分支（多 remote 全部隐藏），本地分支原样透传；返回值即"可见分支列表"
- `src/lib/branchNames.test.ts`（新建）：vitest 用例，跟随同目录 `ignorePattern.test.ts` 惯例（`remoteShortName` 5 例 + `filterRemoteBranches` 5 例）
- `src/components/BranchList.tsx`：
  - 渲染分组：`visibleBranches = filterRemoteBranches(branches)`，再按 `kind` 拆 Local / Remote 两组（过滤规则单一来源）
  - 选中恢复（refresh 后 `setSelectedName`）：改判 `filterRemoteBranches(updated).some((b) => b.name === prev)`，被隐藏的远程分支选中项回落到 current 分支，避免"悬挂选中"（review 修复项）
  - 需求 2：`onBranchSelect` 签名改为 `(branch: BranchInfo) => void`；`handleSelect` 按 name 查到分支对象后回调；移除 checkoutOnto 中未接线的旧调用

### 需求 2 接线
- `src/lib/commitLocate.ts`（新建）：`LocateRequest` 类型 + 纯函数 `resolveLocateIndex`（one-shot / repoId 守卫 / 窗口外 no-op 判定）；配 `commitLocate.test.ts`
- `src/App.tsx`：新增 `locateRequest: LocateRequest | null` state + `locateSeq` ref；`handleBranchSelect` 复用 `handleCommitSelect` 选中提交并发出定位请求；`<BranchList onBranchSelect={...} />` 与 `<CommitGraph locateRequest={...} />` 接线
- `src/components/CommitGraph.tsx`：新增 `locateRequest` prop；`handledLocateSeq` ref 消费后经 `resolveLocateIndex` 取索引，`virtualizer.scrollToIndex(index, { align: "center" })` 居中
- `src/components/BranchList.tsx`：仓库切换 effect 同步清空 branches（防旧仓库行残留可点）

### 需求 3 配色
- `src/styles/tokens.css`：light `--color-bg-primary` #ececec→#ececee；新增 `--color-bg-panel`（light #f8f8f8 / dark #262628 / tide light #f4f6f8 / tide dark #161b20，共 6 个 theme 块）；滚动条 track 规则泛化为四个画布类各自重声明（修订 2/3）；新增 `.no-scrollbar` 工具类（修订 4）
- `src/App.tsx`：main 区（History 面板容器）`bg-bg-primary`→`bg-bg-panel`；MainContent 三处 main 容器 `bg-bg-elevated`→`bg-bg-panel`（右侧面板，含两个空态）；侧栏 aside `bg-bg-secondary`→`bg-bg-primary`（修订 3）+ `no-scrollbar`（修订 4）
- `src/components/ui/WorkingCopyBar.tsx`：三种形态根容器 `bg-bg-secondary`→`bg-bg-primary`（修订 3，共 3 处）
- `src/components/Toolbar.tsx`：根容器 `bg-bg-secondary`→`bg-bg-primary`（修订 5）

### 需求 4 diff 视图密度
- `src/components/DiffViewer.tsx`：
  - 文件列表外层 `p-4`→`p-2`；文件卡 `mb-6`→`mb-3`
  - hunk 容器去 `rounded-md`（保留 border / overflow-hidden / mb-3）
  - 文件标题栏与单文件路径条 `bg-bg-secondary`→`bg-bg-primary`
  - unified 行号列 `w-12 pr-2 pl-1 bg-bg-secondary`→`w-9 pr-1.5 pl-1 bg-bg-primary`（2 处）；split 行号列 `w-10 pr-2`→`w-9 pr-1.5`（6 处）

### 需求 5 右栏宽度
- `src/components/ui/ThreePaneLayout.tsx`：`initialInspectorWidth` 360→480、`inspectorMin` 240→360（`inspectorMax` 720 不变）
- `docs/design/03-layout.md`：三处 `Inspector (~360px)` → `~480px`

### 需求 6 分割线
- `src/components/ui/ThreePaneLayout.tsx`：`HANDLE_PX` 2→1（网格模板与 ResizeHandle 宽度同源，两条分隔一起变）
- `src/components/CommitGraph.tsx`：节点圆描边 `var(--color-bg-primary)`→`var(--color-bg-panel)`（SVG 内联 var()，跟随面板画布抠图）
- `docs/design/01-tokens.md`：1.1 / 1.2 / 1.3 / 1.4 同步 bg-primary 新值与 bg-panel token（bg-secondary 归属改为工具栏 / 层次色，bg-elevated 去除 inspector）

## 测试

- vitest：`remoteShortName`（5 例）+ `filterRemoteBranches`（5 例）+ `resolveLocateIndex`（5 例：命中 / one-shot 已处理 / 跨仓库含 null repoId / 提交不在窗口 / 无请求）
- 手动冒烟：
  - 需求 1：本地与远程同名分支并存时 REMOTE 区不再出现该分支；删除本地分支后远程分支重新出现；全部远程被隐藏时 REMOTE 分组整组消失
  - 需求 2：点击本地 / 远程分支行 → History 滚动至该分支 tip 提交并居中 + 高亮 + DiffViewer 显示该提交；重复点击同一分支可重新居中；提交 / pull / 分支操作刷新后不再回拽滚动位置（one-shot）；切换仓库后点击不误滚（repoId 守卫）
  - 需求 3：light 下主窗口 / 侧栏 / WorkingCopyBar / Topbar #ececee，中间与右侧面板 #f8f8f8，图节点描边无色差晕；各区域滚动条 track 与所在底色一致；dark / tide 下 bg-secondary 归属面不变，chrome 面随 bg-primary 语义走
  - 需求 4：diff 项四周留白收紧、文件间隔变小；标题栏 / 行号列与 topbar 同色；行号列变窄；hunk 代码块方角；Split 模式行号列同步变窄

## 验收

- [x] `npm run typecheck` / `test`（49 通过）/ `lint` / `format:check` / `build` 全绿（需求 4 后复验）
- [x] 需求 3 产物级验证：构建 CSS 含 `.bg-bg-panel` 工具类、四个画布类的 track 重声明规则 + 6 处 `--color-bg-panel`（light / dark×2 / tide light / tide dark×2）；src/docs 无 bg-history 残留
- [ ] 手动冒烟（dev）：需求 1 三个场景 + 需求 2 定位 / 居中 / 选中 / 重复点击 / 刷新不回拽 / 跨仓库守卫 + 需求 3 配色与滚动条 + 需求 4 diff 密度
- [x] code-reviewer 审查通过（需求 1 无 🔴、2 项 🟡 已修复；需求 2 一项 🔴 与两项 🟡 已修复；需求 3 机械验证，见 review.md 三段），报告见本目录 `review.md`
