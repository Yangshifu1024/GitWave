# feat-ui-visual-polish · plan

三项 UI 调整，分支 `feature/ui-visual-polish`。

## 1. 左边栏 section 卡片化（纯 HeroUI 组合）

- `src/components/ui/SidebarSection.tsx` 重写为 `Card`（HeroUI）⊃ `Disclosure`（HeroUI）的薄组合：
  - 手写 chevron 切换（lucide ChevronDown/Right + render-prop）→ `Disclosure.Indicator`，加
    `data-[expanded=false]:-rotate-90 data-[expanded=true]:rotate-0` 保持「收起 ▶ / 展开 ▼」
  - 手写条件渲染内容 → `Disclosure.Content`（收起时 hidden；其高度动画变量无人设置，不影响布局）
  - 类覆盖依据：HeroUI slot 输出 BEM 类、样式在 `@layer base`，工具类（utilities 层）必胜
  - 背景显式 `bg-bg-elevated`：theme bridge 未别名 `--surface`，Card 默认底色不跟主题
- 布局机制：aside 由整体滚动改为固定高度 flex 列（`overflow-hidden` + `gap-1.5 px-2 py-2`）。
  每张卡高度上限 = flex 分配的剩余空间：内容短时自然高度，超高在卡内滚动（`Disclosure.Content`
  内 `overflow-y-auto` 滚动区、表头钉住），不再把其他卡推出可视区
- 行为变化：收起的 section 由卸载改为 hidden 挂载（列表 hooks 在组件级运行，查询行为不变）

## 2. History 鲜艳配色

参考 Fork 与 HeroUI color-swatch 示例色
（https://heroui.com/en/docs/react/components/color-swatch ）：

- `src/styles/tokens.css` 六个主题块 lane 调色板（去灰，五色全鲜艳）：
  - native-blue light `#007aff / #06b7db / #7828c8 / #17c964 / #f5a524`
  - native-blue dark `#0a84ff / #64d2ff / #bf5af2 / #3ecf8e / #f5a524`
  - tide light `#12a594 / #0090ff / #8e4ec6 / #f76b15 / #d6409f`
  - tide dark `#2dd4bf / #60a5fa / #c084fc / #fb923c / #f472b6`
- `--color-branch-remote` 激活为鲜艳 cyan（light `#06b7db` / dark `#64d2ff`）
- `src/lib/palette.ts` `swatch.lanes` 设置页预览同步 lane-1..3
- `src/components/CommitGraph.tsx` RefBadge：
  - remote 分支徽章固定用 `var(--color-branch-remote)`（不再跟随 lane），本地分支继续随 lane
  - 徽章浓度：底色 14%→18%、描边 45%→60%；tag `warning /15→/20`、描边 `/40→/60`

## 3. ActionBar 去组标题

- 删除自定义 `ActionBarGroup`（组标题 span）与 `GroupDivider`，三组按钮平铺为普通 div，
  组间保留 HeroUI `Separator`（2 处）
- `ActionBarButton`（HeroUI Button+Tooltip wrapper）保留

## 验证

- `npx tsc --noEmit`
- `npm run tauri dev` 目检 light/dark × native-blue/tide：卡片布局与卡内滚动、History
  分支线/徽章颜色、ActionBar 单行

## 修复轮（用户反馈后）

1. **卡内无法滚动**：滚动样式原放在 Content 内层 div 上，但被 flex 压缩高度的是
   `Disclosure.Content` 本身（内层 div 高度恒等于内容高度、自身永不溢出，溢出在
   Content 的 `overflow: clip` 处被裁掉）。修复 = 删掉内层包装 div，把
   `overflow-y-auto` 直接放到 `Disclosure.Content`。已用 WebView2 CDP
   （`--remote-debugging-port` + `Runtime.evaluate`）实证：Workspaces 70/117、
   Repos 53/93、Branches 340/522 均可滚，`scrollTop` 驱动后读回真实值并复原。
2. **Recovery / Reflog 重复**：两 section 渲染同一个 `ReflogPanel`（v0.1 的
   Recovery 与 v0.3 的 Reflog 两次合入的产物；Reflog 原有 locate-in-history
   `onSelect` 已在 v0.3 review 中移除，二者自此完全相同）。修复 = 删除 Reflog
   section，保留 Recovery（与面板现有恢复动作功能匹配）。

## 第二批：TopBar 按钮移除 + 双层 Workspace/Repo Tab

1. **TopBar 右侧三按钮移除**（`Toolbar.tsx`）：删 ThemeToggle / Settings / About
   按钮及 `ThemeToggle.tsx`；保留 SettingsModal 挂载、Ctrl+, 快捷键、aboutOpen
   （File 菜单 About 走 onAbout）。入口：Settings=Ctrl+, / File 菜单 / Cmd+K；
   主题=Settings 外观区；About=File 菜单。
2. **双层 Tab**（新组件 `WorkspaceRepoTabs.tsx`，插在 ActionBar 之后）：
   - 第一层 = Workspaces（`selectWorkspace` 恢复 lastActiveRepo）；第二层 = 当前
     workspace 的 repos（`setActiveRepo` DB 持久化 + `setActiveRepoId` + invalidate）
   - repo 标签 = `nickname ?? basename(path)`；missing 仓库禁用 + warning 点
   - 右键菜单（Relink…/Remove）= 受控 HeroUI Popover，定位到右键点，删除 active
     repo 后清 `activeRepoId`
   - 无 workspace → 整条隐藏；无 repo → "No repositories" 提示
3. **侧边栏移除 Workspaces / Repos section**（App.tsx）+ 删除
   `WorkspaceList.tsx` / `RepoList.tsx`（逻辑已迁入 Tab 组件）
4. **Tab 渲染坑（首个 ui/Tabs 使用方）**：
   - TabsTrigger 必须 forward ref 到 `HeroTabs.Tab`（React Aria Pressable 依赖）
   - TabList（RAC collection）会丢弃非 Tab 的包装元素，per-tab 的
     title/onContextMenu 必须走 TabsTrigger 的 DOM 透传
   - Overlay（Popover）放进 tablist 子树会在挂载期崩溃
     （createTextNode is not a function），菜单内容必须在 collection 之外
   - `.tabs__tab` 默认 `w-full`（均分拉伸），需 `w-auto grow-0` 覆盖为内容宽度

## 第二批验证

- tsc / eslint / vite build 干净
- CDP 实证：切 Tests workspace → 第二层变为 Test1*/Test2（lastActiveRepo 恢复）；
  右键 repo tab → 菜单出现 Remove（missing 时含 Relink…）；无异常抛出
- 目检 light/dark：TopBar 无右侧按钮、双层 Tab 紧凑渲染、侧边栏自 Branches 起

## 第三批（用户反馈）：Tab 风格对齐 ActionBar + 全宽 + "+"

1. **风格对齐 ActionBar**：Tab 条背景 `bg-bg-panel → bg-bg-primary`；TabsList 覆盖
   HeroUI 的 `bg-default`/圆角为透明直角；Tab = ghost 按钮式
   （`data-[hovered]:bg-bg-elevated text-primary`、选中 `text-text-primary` +
   accent 下划线替代原白色胶囊 Indicator：`top-auto bottom-0 h-0.5 bg-accent`）
2. **全宽**：Tab 栏整体占满行宽，所有 Tab 子项共同撑满——`.tabs__tab` 默认
   `w-full` 会让每个 Tab 各占一行宽，改为 `w-auto grow`（按内容比例分享整行）；
   HeroUI scroller 默认内容宽导致 `min-w-full` 失效，补 `[&>div]:w-full`
3. **"+" 按钮**：两行右端各一个（Plus 图标，ghost 样式），经
   `requestMenuAction` 接既有对话框——workspace:new / repo:add；布局为
   `flex` 行：Tabs（flex-1 min-w-0）+ 按钮，"+" 在 collection 之外不受 RAC 限制

## 第四批（用户反馈）：去 Workspace Tab 行，ActionBar 左侧加下拉

1. **ActionBar**：最左侧加 `WorkspaceDropdown`（下拉列出全部 workspace、勾选
   当前项，选择走 `selectWorkspace` 恢复 lastActiveRepo）；原有
   Changes/Fetch/Hooks/Pull/Push 组移到右侧，中间 `flex-1` 留白预留
2. **WorkspaceRepoTabs**：删除 Workspace Tab 行与其 "+"；组件只保留
   Repository Tab 行（原右键菜单/missing 语义不变）
3. 入口变化：workspace 切换 = ActionBar 下拉；workspace CRUD = Workspace 菜单；
   repo CRUD = Repository 菜单 + repo Tab 右键

## 第五批（用户反馈）：repo Tab 选中态 + 移除 "+"

1. 移除 Repository Tab 行右端的 "+" 按钮（含 requestMenuAction/useUiStore/Plus
   等无用引用）
2. 选中 Tab 高亮改为 hover 样式：`data-[selected=true]:bg-bg-elevated
   text-text-primary`，并删除 accent 下划线 Indicator（ui/Tabs.tsx）

## 第六批（用户反馈）：切窗口回来的蓝色焦点环

- 现象：alt-tab 回来后选中的 repo Tab 套着蓝色圆角环——HeroUI `status-focused`
  （= `ring-2 ring-focus`，bridge 后 focus=accent 蓝）+ React Aria 在窗口
  refocus 时给 Tab 重新标 `data-focus-visible` 所致
- 修复（ui/Tabs.tsx）：`focus-visible` 与 `data-[focus-visible=true]` 一律
  `outline-none ring-0`，键盘聚焦改为淡背景提示（`bg-bg-elevated text-primary`）
- CDP 实证：手动置 `data-focus-visible` 后 computed boxShadow 全透明、outline
  none；移除属性后恢复正常

## 第十四批（用户反馈）：标题改为应用名+版本

- `ToolbarContextTitle.tsx`（workspace - repo - branch 联动标题）删除，替换为
  `ToolbarAppTitle.tsx`：恒显 "GitWave v{getAppVersion()}"（版本经 Tauri API
  拉取，失败时只显示 GitWave）；不再订阅任何 context

## 第十五批（用户反馈）：移除 TopBar 与 ActionBar 之间的分割线

- Toolbar header 的 `border-b border-border-subtle` 移除（ActionBar 自身
  border-b 保留，继续分隔 ActionBar 与 repo Tab 行）；CDP 实测
  headerBorderBottom=0px



## 第八批（用户反馈）：切 repo 不再出现分支 Loading

- 根因：BranchList 在切 repo 的 effect 里 `setBranches([]) + setLoading(true)`，
  同步清空列表 → 渲染 "Loading branches..." → 数据落地才恢复。桌面本地读仅毫秒级，
  清空+转圈是纯噪音
- 修复：切换时保留旧列表渲染（毫秒级被新数据原地替换），删除 Loading 文案（仅冷
  启动无数据时留 8px 空占位）；加载窗口内行操作统一禁点（`busy={busy || loading}`，
  覆盖单击/双击/右键各项），保住"reload 期间不能点到旧仓库分支"的原守卫意图
- CDP 实证：点 repo tab 后 41 帧（~15ms 间隔）采样，全程无 Loading 文案、分支行
  始终可见，切换完成选中正确


## 第七批（用户反馈）：选中 Tab 加回底部 indicator

- `ui/Tabs.tsx`：Indicator 恢复并改为底部 accent 细线
  （`top-auto bottom-0 h-0.5 rounded-none bg-accent shadow-none`），RAC 自动
  随选中项移动；与第五批的 `bg-elevated` 选中底色叠加
- CDP 实证：仅选中 Tab 有 indicator（全宽 × 2px、贴 tab 底、accent 蓝），
  未选中为 null

## 第八批补充：加 repo 后 indicator 不同步

- 现象：Add repo 后新 Tab 已有选中底色，但下划线仍留在旧 Tab——HeroUI
  Indicator 的可见性走内部测量/缓存状态，与 `data-selected` 属性两套信号，
  collection 变化时会分叉
- 修复：弃用 `HeroTabs.Indicator`，Tab 内自绘下划线 span，由 `data-selected`
  纯 CSS 驱动（`group` + `invisible group-data-[selected=true]:visible`）——
  与底色高亮同一信号源，结构上不可能再分叉；顺带移除 HeroUI Indicator
- CDP 实证：computed visibility 选中=visible、未选中=hidden，切 repo 正确跟随

## 第九批（用户反馈）：ActionBar 中部状态指示区（唯一状态显示）

- 新 `statusAreaStore`（zustand）：`{ text, variant } | null`，`setStatus` 覆盖写入、
  不自动清除——保留最后一次操作信息
- 新 `SyncStatusArea` 组件（w-72 居中，aria-live）：渲染优先级 =
  1) 同步中（syncStore.activeOp）：`syncOperationLabel` + isIndeterminate shimmer
  进度条，文字 accent 蓝；2) 最后结果：success 绿 / danger 红；3) 空态：当前分支名
  （muted）
- 写入点：`useRemoteSync` 三 mutation onSuccess/onError（Fetched/Pulled/Pushed
  from origin / * failed）；BranchList checkoutOnto 三处 checkout toast 改
  setStatus（merge/rebase/delete 等 toast 保留）
- **状态显示唯一化**：删除 SyncProgressBar.tsx（Toolbar 底边进度条）、
  ToolbarContextTitle 同步标题替换逻辑；`syncOperationLabel` 迁至 syncStore.ts；
  shimmer keyframes 保留复用
- CDP 实证（真实 fetch 流转）：idle=分支名(muted) → in-flight "Fetching from
  origin…"(accent #007aff)+shimmer → settled "Fetched from origin"(success
  #2f9e6b) 保留；setStatus 覆盖/danger/clear 均正确；旧 Toolbar 进度条已消失

## 第十批（用户反馈）：无前缀分支排在文件夹之前

- BranchList renderGroup 原先 folders 在前、roots 在最后；调整为
  `renderRows(roots)` 先渲染——无前缀分支（main、origin/main 等）置顶，
  前缀文件夹（feat/feature/fix…）在其下，Fork 风格
- CDP 实证：LOCAL 组顺序 main → origin/main → feat(2) → feature(7) → fix(6)

## 第十一批（用户反馈）：分支行缩进统一

- 现象：无前缀分支用 ListItem 默认 `px-3`（图标 x=23），文件夹子行 `pl-8`
  （图标 x=43），缩进不一致
- 修复：`renderRows(roots, display, true)`——无前缀分支与文件夹子行共用
  `pl-8`，所有分支行前导图标对齐同一竖线；文件夹头保持 `pl-9`（chevron 层级）
- CDP 实测：main/origin/main 图标 x=43（pl 32px）= 文件夹子行

## 第十二批（用户反馈）：文件夹子项再加一级缩进

- 上一批把所有分支行拉到同一缩进后，文件夹头与子项层级仍不清
- 修复：BranchRow `indented` 语义改为"文件夹子项深一级"——顶层行 `pl-8`
  （32px）、文件夹头 `pl-9`（36px chevron）、文件夹子项 `pl-12`（48px）
- CDP 实测：顶层行图标 x=43（32px），文件夹子项 x=59（48px），层级清晰

## 第十三批（用户反馈）：选中 Tab 贯通下方内容区

- Fork 式效果：选中的 repo Tab 底色（bg-bg-panel）向下融入内容面板——Tab 加高
  （h-8→h-9，items-end 底对齐）、只留上圆角（rounded-t-md rounded-b-none）、
  底边框颜色与自身同色（border-bg-panel）
- 关键：条带容器自身的 border-b 移除，改为**每个 Tab 自带底部分隔线**
  （border-border-subtle）——Tab 等分撑满整行，相邻底线连续成行；选中 Tab 的
  底边段与背景同色 → 视觉上断开，形成"贯通"；无 overflow 裁切问题（不走
  负 margin/越界覆盖方案，HeroUI ListContainer 的 overflow-x-auto 会裁掉
  越界 1px）
- 选中底色从 bg-bg-elevated 改为 bg-bg-panel（与下方 History 面板同色）
- CDP 实测：选中 Tab h=36/bg+底边=panel 色(248,248,248)；未选中 h=32/透明底/
  底边 subtle 发丝线；全部底边平齐；截图目检贯通效果成立

### 补丁：选中 Tab 下仍有灰线

- 根因：`ui/Tabs.tsx` TabsList 封装基类残留 `border-b border-border-subtle`，
  调用方的 `border-b-0` 与其同层、层叠顺序不可控 → 容器级细线仍画在选中 Tab
  下方（正好在加高的选中 Tab 底边处）
- 修复：封装基类直接移除容器 border-b（每个 Tab 自带底线已足够）；CDP 实测
  ListContainer borderBottomWidth=0px，截图确认选中 Tab 下方无任何线

- CDP 实测：选中 Tab h=36/bg+底边=panel 色(248,248,248)；未选中 h=32/透明底/
  底边 subtle 发丝线；全部底边平齐；截图目检贯通效果成立

## 第十三批补丁二（用户反馈）：repo Tab 三项微调

1. 选中 Tab 文字 `font-semibold` 加粗
2. 底部灰色区域根因：`.tabs__list` 自带 `p-1`（4px 内边距），Tab 下方 4px 露出
   容器灰底（采样实测：tabs 底 121 / list 底 121 / 容器底 129 但中间为列表
   padding 区）。修复 = TabsList 上 `[&_[role=tablist]]:p-0` 清零列表内边距，
   Tab 底边与条带底边完全平齐（贯通无缝）
3. 选中 Tab 加阴影 `shadow-[0_2px_8px_rgba(0,0,0,0.15)]`
4. Tab 栏降高：TabsList `h-9 → h-7`、TabsTrigger `h-8 → h-7`（选中态由
   `data-[selected=true]:h-9` 一并下调为随容器 h-7）

### 补丁：未选中 Tab hover 样式

- 全宽 Tab 上铺 `bg-bg-elevated` 的 hover 底像一大块白色板，观感怪异
- 修复：hover 只做文字强调（`text-secondary → text-primary`），不再铺背景色
- CDP 实测：hover 后 bg 保持透明、文字变 text-primary

### 补丁：状态区用 Card 包裹

- `SyncStatusArea` 外层换 HeroUI Card（`bg-bg-elevated` + `border-border-subtle`
  + `rounded-md`，与侧边栏卡片同风格），状态文字有了明显背景
- 进度条改为 Card 内绝对定位（bottom-0 inset-x-0），卡片高度恒定 28px 不随
  同步跳动；CDP 实测 bg/border/radius 全部生效

### 补丁：进度条常驻 + 状态取色

- 底部条**常驻显示**，颜色与状态文字一致：idle 灰（bg-border-subtle）/
  同步中 accent（shimmer）/ 成功 success 绿 / 失败 danger 红
- 非同步状态用静态 div 条；同步中仍为 isIndeterminate shimmer 的 ProgressBar

### 补丁：状态卡高度与按钮一致

- 卡片显式 `h-7`（28px，border-box 含 1px 边框），文字行 `h-full`——与右侧
  ActionBarButton（h-7）等高；底部状态条仍绝对定位贴卡底

## 第十六批（用户反馈）：彻查切换窗口后的网页式焦点环

- 现象：alt-tab 切回后，菜单触发器（Branch 等）/列表行等套着蓝色圆角描边
- 机制：Chromium 在窗口 refocus 时重新匹配 `:focus-visible`，React Aria 也会
  重新标记 `data-focus-visible`；tokens.css 原全局规则 `:focus-visible`
  （2px accent outline）+ HeroUI `status-focused`（ring-2 蓝圈）随之复现
- 全局策略（tokens.css）：
  - 原 `:focus-visible { outline: 2px solid accent }` 改为 `outline: none`
  - 新增 `[data-focus-visible="true"] { outline: none; box-shadow: none }`
    （覆盖 HeroUI status-focused 的 ring-2 蓝圈）
  - 表单域豁免：input/textarea/select 不受影响（焦点仍由 field-border-focus
    边框色表达）
- 组件清理：ConflictPanel / ChangesPanel / SettingsModal / ListItem /
  FileListItem 五处 `focus-visible:outline-2 outline-accent` 描边类删除
- 取舍说明：纯键盘 Tab 导航不再显示焦点框（方向 = 选中态/hover 态本身），
  桌面应用指针优先，用户明确要求

## 第十七批（用户反馈）：History 区域与搜索框背景统一 #f8f8f8

- History 窗格已是 `bg-bg-panel`（浅色 = #f8f8f8），无需改动
- 搜索框（CommitGraph 顶部 Input）：背景由 field-background（bg-elevated
  #f4f4f5）改为与窗格同色——`bg-bg-panel` + hover/focus-within/focus-visible
  全部压制为同色，实现完全平坦的融合效果；dark 主题自动跟随（#2a2a2d）

### 补丁：搜索行底色也透明

- History 窗格内唯一的不透明层是搜索行包装（`bg-bg-elevated` #f4f4f5），
  与下方 #f8f8f8 窗格形成色差。修复 = 移除该行 `bg-bg-elevated`（透明），
  整个 History 区域统一 #f8f8f8；搜索输入框保留 bg-panel 同色 + InputGroup
  自身边框仍可辨识

## 第十三批补丁三（用户反馈）：状态区窗口级居中

- 原先卡片在"下拉与右侧按钮之间"的弹性区居中——该区左右不对称（按钮组更宽）
  导致卡片相对窗口偏左
- 修复：ActionBar 根加 `relative`，状态卡片改为 `absolute inset-x-0
  justify-center` 相对整条 ActionBar 居中（与上方 "GitWave v0.2.0" 标题同
  一中轴）；中间恢复 `flex-1` 留白；`pointer-events-none` 保证窄窗口重叠时
  按钮仍可点

## 第十七批补充（用户反馈）：最小窗口 1024x768 + ActionBar 精简

- `tauri.conf.json`：`minWidth/minHeight` 800x600 → **1024x768**
- ActionBar：移除 Hooks 按钮（Hooks 编辑器仍可从 Repository 菜单打开），
  Fetch 移入 Pull/Push 组首位 → 右侧组 = Fetch / Pull / Push，少一组分隔线
