# GitWave · Layout Spec

> 3-pane 布局详细规格：toolbar + sidebar + history graph + inspector。

## 1. 全局 shell

```
┌─────────────────────────────────────────────────────────────────────────┐
│ TitleBar (h: 40px)                                                      │
│  File Workspace Repository Branch │ …操作按钮… │ 状态区（窗口居中）      │
├────────────┬──────────────────────────────────┬─────────────────────────┤
│            │                                  │                         │
│ Sidebar    │  History graph (flex)            │  Inspector (~360px)     │
│ (w: 320px) │                                  │                         │
│            │                                  │                         │
│            │                                  │                         │
│            │                                  │                         │
│            │                                  │                         │
└────────────┴──────────────────────────────────┴─────────────────────────┘
```

最小窗口尺寸：1024 × 768。
推荐：1280 × 800+。

## 2. TitleBar（自绘标题栏，唯一顶栏）

> 2026-09 重构：`Toolbar`（菜单/拖拽）与 `ActionBar`（操作按钮）合并为一行
> 40px 标题栏（`TitleBar` 提供外壳，`ActionBar` 提供内容与对话框）。

### 2.1 布局

```
┌─────────────────────────────────────────────────────────────────────────┐
│ File Workspace Repository Branch  [Workspace▾][📁][⌨][</>]  …  [Changes] [Stash] [Fetch] │ [Pull] [Push] │ 状态区 │
└─────────────────────────────────────────────────────────────────────────┘
```

高度：40px。背景：`bg-bg-primary`。底边：1px `border-subtle`。
菜单靠左；操作按钮靠右；同步状态区绝对居中于**整个窗口**（不受两侧留白影响）。

**平台约定（关键）**：标题栏内容必须避开系统窗口控件——

- macOS：原生红绿灯悬浮在栏上，`.app-toolbar--macos` 用 `padding-left` 预留
  `max(78px, --tauri-plugin-decoration-left-clearance)`；菜单走原生系统菜单，
  栏内不再渲染 AppMenuBar。
- Windows / Linux：插件把窗口控制按钮以 HTML 覆盖在栏尾，`.app-toolbar` 用
  `padding-right` 预留 `--tauri-plugin-decoration-right-clearance`；菜单为栏内
  AppMenuBar。
- 全屏时两个 clearance 归 0，由插件负责隐藏控件。

拖拽区位于内容**下方**（`z-0`），内容层 `pointer-events-none`，只有交互分组
`pointer-events-auto`，因此空白处可拖动、按钮可点击。双击缩放走
`useMacTitlebarWindow`（macOS）。

### 2.2 元素

| 元素 | 位置 | 交互 |
|---|---|---|
| App 菜单（File / Workspace / Repository / Branch） | 左 | Windows / Linux 栏内；macOS 为原生菜单 |
| Workspace 下拉 | 左 | 切换工作区（恢复其 last active repo） |
| 外部工具（文件管理器 / 终端 / 编辑器） | 左 | macOS 无菜单，空间充足；窄栏时文字折叠为图标 |
| Local Changes / Stash / Fetch | 右 | 见 §6.1 |
| Pull / Push | 右 | 带 ahead/behind 计数 |
| 同步状态区 | 窗口居中 | Fetch / Pull / Push 进度与最近一次操作结果 |

**Sync 不在侧栏标题栏**（与旧稿相反）：Fetch / Pull / Push 已收进标题栏右侧，
ahead/behind 数字显示在 Pull / Push 按钮上。

### 2.3 Branch 同步状态（标题栏 Pull / Push）

ahead/behind 数字显示在**标题栏右侧 Pull / Push 按钮**上，形如 `Pull (3)` / `Push (2)`：按钮内有一个固定宽度的 `(N)` 计数槽位，计数为 0 时留空（`ActionBarButton`，`src/components/ActionBar.tsx`）。

示例：
- `Pull` / `Push`（计数 0，按钮灰显）
- `Pull (3)`（behind 3，可 Pull）
- `Push (2)`（ahead 2，可 Push）
- detached HEAD 时 Pull/Push 均灰显

当 branch 改变（如 commit / checkout / merge）时实时更新。ahead/behind 在每次 fetch 后刷新。

### 2.4 快捷键

- `⌘K` / `Ctrl+K`：打开 Command Palette
- `⌘1` / `⌘2` / `⌘3` / `⌘4`：切换 sidebar / history graph / inspector / working copy bar focus
- `⌘⇧F`：Fetch
- `⌘⇧P`：Pull
- `⌘⇧U`：Push（U = upstream；与 Tower 一致）
- `⌘,` / `Ctrl+,`：打开 Settings（已实现；macOS 系统菜单加速键 `CmdOrCtrl+,`，网页层另有兜底监听）

详见 `04-working-copy.md` §8 完整快捷键表。

### 2.5 Sync 全局进度条

窗口居中状态区（`SyncStatusArea`）无卡片容器（透明、无边框、宽度贴合文字），
纯文字融入标题栏背景；仅在有内容时底部显示 2px 状态线：

- 有传输量时 determinate（`receivedObjects / totalObjects`）
- 否则 indeterminate shimmer
- 同步期间状态区临时显示 `Fetching from origin…` 等操作文案（可取消）
- 完成后 150ms fade-out；idle（仅显示分支名）不显示状态线

## 3. Sidebar

### 3.1 布局

```
┌──────────────────────────┐
│ HEALTH                 ▸ │
│ BRANCHES              +  │
│   main             HEAD   │
│   feature/tide-lanes      │
│ STASH                     │
│ TAGS                      │
│ REMOTES                   │
│ WORKTREES                 │
│ SUBMODULES                │
│ RECOVERY (reflog)      ▸ │
└──────────────────────────┘
```

宽度：320px（可拖拽 320-480）。背景 Mist，与 Foam 画布区分。**同步按钮不在侧栏**：Fetch / Pull / Push 位于标题栏右侧（§2.2 / §3.4）。

**工作区与仓库的切换同样不在侧栏**（2026-09 起）：工作区在标题栏的工作区下拉里切换（`components/WorkspaceDropdown.tsx`），仓库在标题栏下方的仓库标签栏里切换（`components/WorkspaceRepoTabs.tsx`）。

### 3.2 元素

| 元素 | 类型 | 备注 |
|---|---|---|
| HEALTH 标题 | SidebarSection | uppercase label，默认折叠；内容见 §4 |
| BRANCHES 标题 | 静态 + actions | 右侧 `+`（同步按钮已移至标题栏，见 §3.4）|
| Branch 行 | ListItem | 见 `components/BranchList.tsx` |
| STASH / TAGS / REMOTES / WORKTREES / SUBMODULES | SidebarSection | 见 §4 |
| RECOVERY（reflog） | SidebarSection | uppercase label，默认折叠 |

### 3.4 Sync 操作（标题栏右侧，不在侧栏）

> 2026-09 起 Fetch / Pull / Push 收进标题栏右侧（§2.2 / §2.3）；侧栏 section 标题栏不再有同步按钮。

| 操作 | 作用域 | 禁用 |
|---|---|---|
| Fetch | 当前 active repo 的全部 remote | 无 active repo |
| Pull | 当前 HEAD branch | behind = 0 或 detached |
| Push | 当前 HEAD branch | ahead = 0 或 detached |

同步进行中按钮 disabled，进度与取消入口在标题栏状态区（§2.5）。

### 3.3 状态标记

| 状态 | Badge | 位置 |
|---|---|---|
| active（= workspace.last_active_repo_id） | 默认高亮（左侧 3px accent border） | repo 行 |
| missing | 仓库标签上无徽标：整行降透明度（`opacity-60`）+ 一个 warning 圆点，并带仅供读屏的 missing 文案（`StatusBadge` 的 `missing` 变体已定义，但当前没有调用点） | repo 标签 |
| ahead / behind | `StatusBadge variant="ahead"/"behind"` | branch 行右侧（fetch 后展示）|

## 4. Feature 入口（侧栏 sections，无浏览器 Tab）

History 永远占中栏。其余功能收进 Sidebar 的 `SidebarSection`，不再使用横向文字 Tab。

| Section | 内容 | 默认 |
|---|---|---|
| Repos | 当前 workspace 的仓库 | 展开 |
| Branches | local / remote | 展开 |
| Stash | stash 列表（compact，无内嵌 diff 栏）| 折叠 |
| Tags | tag 列表 | 折叠 |
| Remotes | remote + tracking | 折叠 |
| Worktrees | worktree 列表 | 折叠 |

变更文件列表在 WorkingCopyModal 内（工具条 Local Changes 打开），不占用三栏。

## 5. Main

### 5.1 布局

中栏永远是 History graph（Tide Lanes，行高 28px）。右侧 Inspector（~360px）显示选中 commit 或 working-copy 文件的 diff。

```
┌──────────────────────────────┬─────────────────────┐
│ Commit Graph (flex)          │ Inspector (~360px)  │
│   Tide Lanes + commit 行     │  sha · author · date│
│                              │  message            │
│                              │  unified / split    │
└──────────────────────────────┴─────────────────────┘
```

### 5.2 Inspector 内容

| 选择 | Inspector |
|---|---|
| History 中的 commit | commit details + diff |
| WorkingCopyModal 中的 Unstaged 文件 | 仅 worktree vs index（unstaged）diff |
| WorkingCopyModal 中的 Staged 文件 | 仅 index vs HEAD（staged）diff |
| 无选择 | 空状态提示（选 commit 看 diff） |

### 5.3 空状态

除 Inspector 的选择提示外，空状态一律给出下一步动作（`EmptyState` 的 `action` 参数），按钮只发动作请求，对话框仍由 `ActionBar` 独占：

| 场景 | 显示 | 动作 |
|---|---|---|
| 无 Workspace（侧边栏） | 「未选择工作区」+ 说明 | 新建工作区 |
| 无 Workspace（Main） | 「选择工作区」+ 说明 | 新建工作区 |
| 有 Workspace、无 repo（Main） | 「未选择仓库」+ 说明 | 克隆远程仓库（主）/ 初始化新仓库 / 添加本地仓库 |
| 有 repo、无 commit（empty repo，commit graph） | 空状态文案 + 「创建第一个提交」按钮 | 打开说明弹窗（空仓库要先有文件才能提交，或改用克隆） |
| 有 repo、无 commit 选择（Inspector） | 空状态提示（选 commit 看 diff） | — |

首次启动（没有任何 Workspace）时侧边栏与 Main 会同时显示各自的空状态，两处都要能直接新建 Workspace——2026-09 修复了一个死胡同：原先只提示「请在侧边栏中选择或创建工作区」，而侧边栏里并没有该入口，三个添加仓库动作又都被门控禁用。详见 `docs/pm/features/F018-getting-started.md`。

## 6. ActionBar + Working Copy Modal（原 Working Copy Bar，已重构）

> 2026-08 重构：底部常驻 WorkingCopyBar 移除，变更操作通过 Local Changes 打开的
> WorkingCopyModal 完成。2026-09：ActionBar 不再独占一行，作为内容渲染在 §2 的
> 自绘标题栏内（`TitleBar` 外壳 + `ActionBar` 内容/对话框）。

### 6.1 ActionBar（标题栏右侧操作组）

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ [Workspace▾] [📁][⌨][</>]  ······  [Changes(n)] [Stash] [Fetch] │ [Pull] [Push] │
└──────────────────────────────────────────────────────────────────────────────┘
```

- 左起：Workspace 下拉（切换工作区），外部工具（文件管理器 / 终端 / 编辑器）
- 右端：Local Changes（打开 WorkingCopyModal）——干净时绿色 `Changes`，有变更时黄色 `Changes(n)`（n 为变更文件数）；无活动仓库时禁用
- Stash（保存贮藏）/ Fetch / Pull（Fork 式对话框：Remote / Branch / Into + rebase + stash）/ Push
- Workspace 新建 / 重命名 / AI Provider / 删除、Repository Init / Clone / Add Local 等低频操作在 App 菜单（F 组）
- 窄栏时外部工具文字折叠为图标（`@6xl` 容器查询）；同步状态区绝对居中于窗口
- 背景：`bg-bg-primary`。fetch / pull / push 错误经标题栏下方 ErrorAlert 呈现

### 6.2 WorkingCopyModal

点击 Local Changes（n ≠ 0）弹出的模态（size xl）：

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ Local Changes                                                            ✕   │
│ main · 5 unstaged · 2 staged                                                │
├──────────────────────┬───────────────────────────────────────────────────────┤
│ Unstaged (5)         │ diff（选中文件的 workdir diff，unified / split）        │
│ Staged (2)           │                                                       │
│ ┌──────────────────┐ │                                                       │
│ │ Commit message   │ │                                                       │
│ │ [AI] [Commit]    │ │                                                       │
│ └──────────────────┘ │                                                       │
└──────────────────────┴───────────────────────────────────────────────────────┘
```

- 左列：Unstaged / Staged 列表 + CommitMessageBox（AI generate + Commit）
- 右列：点击文件的 diff（复用 DiffViewer workdir 模式，隐藏最大化按钮）
- 文件选择**仅作用于 Modal 内部**，不再联动右侧 Inspector

### 6.3 组件结构

| 子组件 | 用途 | 文档 |
|---|---|---|
| `ActionBar` | TopBar 下方操作条 | 本节 |
| `WorkingCopyModal` | 变更模态（双列） | 本节 |
| `ChangesPanel` | unstaged/staged 列表 + commit box（layout: stacked/bar/modal） | `04-working-copy.md` |
| `BranchIndicator` | 当前 branch 名 + ahead/behind | `02-components.md` §3.1 |
| `FileListItem` | 单个文件行（M/A/D/?/R/C + 路径 + +/-） | `02-components.md` §3.2 |
| `StatusIcon` | 文件 status 字符 + 颜色 | `02-components.md` §3.3 |
| `CommitMessageBox` | 多行输入 + AI placeholder + Amend prefill | `02-components.md` §3.4 |


## 7. 响应式

| 窗口宽度 | 行为 |
|---|---|
| ≥ 1280px | 3-pane 全显示 |
| 960-1279px | 3-pane 全显示，sidebar / nav 取 minSize |
| 768-959px | sidebar 折叠成图标列；feature nav 折叠为下拉 |
| < 768px | 单 pane（mobile 暂未支持，建议 PWA / Tauri mobile 后续） |

桌面端 only（当前仍是桌面应用）；窗口最小尺寸 1024 × 768（`src-tauri/tauri.conf.json`），表中更窄的断点当前不可达。

## 8. 键盘导航

| 键 | 动作 |
|---|---|
| `⌘K` / `Ctrl+K` | Command Palette |
| `⌘1` | focus sidebar |
| `⌘2` | focus feature nav |
| `⌘3` | focus main |
| `⌘4` | focus working copy bar |
| `⌘⇧F` | Fetch |
| `⌘⇧P` | Pull |
| `⌘⇧U` | Push |
| `⌥⇧C` | focus commit message box |
| `⌘Enter` | commit（commit message 框聚焦时） |
| `Space` | 在 working copy 文件列表上 stage / unstage 当前选中行 |
| `↑` / `↓` | 在 sidebar / nav / 文件列表间移动 |
| `←` / `→` | 折叠 / 展开 sidebar workspace |
| `Enter` | 激活当前焦点项（sidebar 选中 repo / file 选中查看 diff） |
| `Tab` / `Shift+Tab` | 在 main / 表单字段间移动 |
| `Esc` | 关闭 Modal / popover / palette；取消 commit 框 focus |
| `⌘Z` / `⌘⇧Z` | 文本输入框内的 undo / redo（无 git 操作级撤销）|

完整工作副本快捷键见 `04-working-copy.md` §8。

## 9. 关联

- `00-overview.md`：设计目标 + IA 决策
- `01-tokens.md`：颜色 / 间距 / 字体
- `02-components.md`：组件 API + 样式
- `04-working-copy.md`：Working Copy Bar 详细规格（Sprint 4 实施依据）
- `docs/tech/architecture/00-overview.md`：前端架构