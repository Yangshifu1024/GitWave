# GitWave · Layout Spec

> 3-pane 布局详细规格：toolbar + sidebar + history graph + inspector。

## 1. 全局 shell

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Toolbar (h: 40px)                                                      │
│  Client Work - gitwave - main（居中）                             ⌘K  ☀ │
├────────────┬──────────────────────────────────┬─────────────────────────┤
│            │                                  │                         │
│ Sidebar    │  History graph (flex)            │  Inspector (~500px)     │
│ (w: 320px) │                                  │                         │
│            │                                  │                         │
│            │                                  │                         │
│            │                                  │                         │
├────────────┴──────────────────────────────────┴─────────────────────────┤
│ ActionBar (workspace / repository / branch ops + Local Changes) 详见 §6 │
└─────────────────────────────────────────────────────────────────────────┘
```

最小窗口尺寸：960 × 600。
推荐：1280 × 800+。

## 2. Toolbar

### 2.1 布局

```
┌─────────────────────────────────────────────────────────────────────────┐
│              Client Work - gitwave - main                          ⌘K  ☀ │
└─────────────────────────────────────────────────────────────────────────┘
```

高度：40px。背景：`bg-bg-secondary`（Mist / Abyss）。底边：1px `border-subtle`。
不要居中字标、不要版本号、不要 Help 图标。

### 2.2 元素

| 元素 | 位置 | 交互 |
|---|---|---|
| **Workspace - Repository - branch** | **居中** | 只读上下文标题；` - ` 分隔三段；切换在侧栏 |
| `⌘K` hint | 右 | 点击 → 打开 CommandPalette（v0.2 Sprint 6） |
| Theme toggle | 右 | 点击循环：light → dark → system |
| 溢出菜单 | 右 | SSH Keys |

**Workspace 不在 Toolbar**——在侧栏 WORKSPACES 列表切换，见 §3.1。  
**Sync 不在 Toolbar**——Fetch / Pull / Push 在侧栏 section 标题栏，见 §3.4。

### 2.3 Branch 同步状态（侧栏 BRANCHES 标题栏）

ahead/behind 数字显示在 **BRANCHES section 标题栏** 的 Pull / Push 按钮旁，不在 Toolbar。

格式：`Pull ↓N` / `Push ↑N`

示例：
- `Pull` / `Push`（无数字，behind/ahead = 0 时按钮灰显）
- `Pull ↓3`（behind 3，可 Pull）
- `Push ↑2`（ahead 2，可 Push）
- detached HEAD 时 Pull/Push 均灰显

当 branch 改变（如 commit / checkout / merge）时实时更新。ahead/behind 在每次 fetch 后刷新。

### 2.4 快捷键

- `⌘K` / `Ctrl+K`：打开 Command Palette
- `⌘1` / `⌘2` / `⌘3` / `⌘4`：切换 sidebar / history graph / inspector / working copy bar focus
- `⌘⇧F`：Fetch
- `⌘⇧P`：Pull
- `⌘⇧U`：Push（U = upstream；与 Tower 一致）
- `⌘,`：打开 Preferences（v0.2）

详见 `04-working-copy.md` §8 完整快捷键表。

### 2.5 Sync 全局进度条

Fetch / Pull / Push 进行时，Toolbar **底边**显示 2px Tide 进度条（`SyncProgressBar`）：

- 有传输量时 determinate（`receivedObjects / totalObjects`）
- 否则 indeterminate shimmer
- 同步期间居中标题可临时显示 `Fetching from origin…` 等操作文案
- 完成后 150ms fade-out

## 3. Sidebar

### 3.1 布局

```
┌──────────────────────────┐
│ WORKSPACES             +  │
│   Client Work    [active] │
│   Personal                │
│ REPOS          Fetch  +  │
│   gitwave      [active]   │
│   notes                   │
│ BRANCHES   Pull↓1 Push↑2 +│
│   main             HEAD   │
│   feature/tide-lanes      │
│ STASH / TAGS / REMOTES …  │
└──────────────────────────┘
```

宽度：320px（可拖拽 320-480）。背景 Mist，与 Foam 画布区分。**Workspaces 列表置顶**，其下为 Repos / Branches 等 sections。

### 3.2 元素

| 元素 | 类型 | 备注 |
|---|---|---|
| WORKSPACES 标题 | 静态 + `+` | uppercase label；`+` → 新建 workspace |
| Workspace 行 | ListItem | 点击 → selectWorkspace；hover 显示 AI / Rename / Delete |
| REPOS 标题 | 静态 + actions | uppercase label；右侧 `Fetch`（repo 级）+ `+` |
| Repo 行 | ListItem | 点击 → setActiveRepo；右键菜单：relink / remove |
| BRANCHES 标题 | 静态 + actions | 右侧 `Pull ↓N` / `Push ↑N`（branch 级）+ `+` |
| STASH / TAGS / … | SidebarSection | 见 §4 |

### 3.4 Sync 操作（侧栏标题栏）

| 操作 | Section | 作用域 | 禁用 |
|---|---|---|---|
| Fetch | REPOS | 当前 active repo 的全部 remote | 无 active repo |
| Pull | BRANCHES | 当前 HEAD branch | behind = 0 或 detached |
| Push | BRANCHES | 当前 HEAD branch | ahead = 0 或 detached |

按钮为 10px 文字链，badge 颜色：ahead 绿 / behind 橙。进行中显示 spinner 替换文字。

### 3.3 状态标记

| 状态 | Badge | 位置 |
|---|---|---|
| active（= workspace.last_active_repo_id） | 默认高亮（左侧 3px accent border） | repo 行 |
| missing | `StatusBadge variant="missing"` | repo 行右侧 |
| ahead / behind | `StatusBadge variant="ahead"/"behind"` | repo 行右侧（v0.1 fetch 后展示）|

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

中栏永远是 History graph（Tide Lanes，行高 28px）。右侧 Inspector（~500px）显示选中 commit 或 working-copy 文件的 diff。

```
┌──────────────────────────────┬─────────────────────┐
│ Commit Graph (flex)          │ Inspector (~500px)  │
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

无 active repo：Main 显示空状态文案 + "Select a repository from the sidebar"。

无 commit（empty repo）：commit graph 显示空状态 + "Create your first commit" 按钮。

## 6. Action Bar + Working Copy Modal（原 Working Copy Bar，已重构）

> 2026-08 重构：底部常驻 WorkingCopyBar 移除，操作收敛到 ToolBar 下方的 ActionBar，
> 变更操作通过 Local Changes 打开的 WorkingCopyModal 完成。

### 6.1 ActionBar（TopBar 下方）

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  LOCAL      WORKSPACE    REPOSITORY    BRANCH                                │
│ [Changes] [⇱New][✎Rename][✦AI][⌫Del] [⩚Init][⤓Clone]… [⎇New][⇣][⇡]          │
└──────────────────────────────────────────────────────────────────────────────┘
```

- 四组操作水平居中；每组两行：第一行组头（居中大写），第二行操作按钮（图标在前、文字在后）
- Local 组：Changes（打开 WorkingCopyModal）——干净时绿色 `Changes`，有变更时黄色 `Changes(n)`（n 为变更文件数）；无活动仓库时禁用
- Workspace：New / Rename / AI Provider / Delete（作用于活动 workspace）
- Repository：Init / Clone / Add Local / Fetch
- Branch：New Branch（当前 tip）/ Pull（Fork 式对话框：Remote / Branch / Into + rebase + stash）/ Push
- 背景：`bg-bg-primary`。fetch / pull / push 错误经条下方 ErrorAlert 呈现

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
| `BranchIndicator` | 当前 branch 名 + ahead/behind | `02-components.md` §1.13 |
| `FileListItem` | 单个文件行（M/A/D/?/R/C + 路径 + +/-） | `02-components.md` §1.14 |
| `StatusIcon` | 文件 status 字符 + 颜色 | `02-components.md` §1.15 |
| `CommitMessageBox` | 多行输入 + AI placeholder + Amend prefill | `02-components.md` §1.16 |


## 7. 响应式

| 窗口宽度 | 行为 |
|---|---|
| ≥ 1280px | 3-pane 全显示 |
| 960-1279px | 3-pane 全显示，sidebar / nav 取 minSize |
| 768-959px | sidebar 折叠成图标列；feature nav 折叠为下拉 |
| < 768px | 单 pane（mobile 暂未支持，建议 PWA / Tauri mobile 后续） |

v0.1 桌面端 only，响应式为 v0.2+。

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
| `⌘Z` / `⌘⇧Z` | undo / redo（v0.2）|

完整工作副本快捷键见 `04-working-copy.md` §8。

## 9. 关联

- `00-overview.md`：设计目标 + IA 决策
- `01-tokens.md`：颜色 / 间距 / 字体
- `02-components.md`：组件 API + 样式
- `04-working-copy.md`：Working Copy Bar 详细规格（Sprint 4 实施依据）
- `docs/tech/architecture/00-overview.md`：前端架构