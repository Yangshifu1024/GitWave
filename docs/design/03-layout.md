# GitWave · Layout Spec

> 3-pane 布局详细规格：toolbar + sidebar + history graph + inspector。

## 1. 全局 shell

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Toolbar (h: 40px)                                                      │
│  [WS▾]  repo › branch                                        ⌘K  ☀/☾ │
├────────────┬──────────────────────────────────┬─────────────────────────┤
│            │                                  │                         │
│ Sidebar    │  History graph (flex)            │  Inspector (~360px)     │
│ (w: 220px) │                                  │                         │
│            │                                  │                         │
│            │                                  │                         │
│            │                                  │                         │
├────────────┴──────────────────────────────────┴─────────────────────────┤
│ Working Copy Bar (collapsible, ~32-220px) 详见 §6                  │
└─────────────────────────────────────────────────────────────────────────┘
```

最小窗口尺寸：960 × 600。
推荐：1280 × 800+。

## 2. Toolbar

### 2.1 布局

```
┌─────────────────────────────────────────────────────────────────────────┐
│ [Workspace▾]  repo › branch                                      ⌘K  ☀ │
└─────────────────────────────────────────────────────────────────────────┘
```

高度：40px。背景：`bg-bg-secondary`（Mist / Abyss）。底边：1px `border-subtle`。
不要居中字标、不要版本号、不要 Help 图标。

### 2.2 元素

| 元素 | 位置 | 交互 |
|---|---|---|
| Workspace Switcher | 左 | 点击 → 下拉，列出 workspaces + New |
| 当前 repo › branch | 左 | 只读路径（切换在侧栏）；`›` 分隔 |
| `⌘K` hint | 右 | 点击 → 打开 CommandPalette（v0.2 Sprint 6） |
| Theme toggle | 右 | 点击循环：light → dark → system |
| 溢出菜单 | 右 | SSH Keys |

**Sync 不在 Toolbar**。Fetch / Pull / Push 按作用域放在侧栏 section 标题栏，见 §3.4。

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

## 3. Sidebar

### 3.1 布局

```
┌──────────────────────────┐
│ REPOS          Fetch  +  │
│   gitwave      [active]   │
│   notes                   │
│ BRANCHES   Pull↓1 Push↑2 +│
│   main             HEAD   │
│   feature/tide-lanes      │
│ STASH / TAGS / REMOTES …  │
└──────────────────────────┘
```

宽度：220px（可拖拽 180-360）。背景 Mist，与 Foam 画布区分。Workspace 切换在 Toolbar，侧栏从 Repos 起。

### 3.2 元素

| 元素 | 类型 | 备注 |
|---|---|---|
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

Changes 文件列表在 Working Copy Bar（dirty 时展开），不占用中栏。

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
| Working Copy Bar 中的 Unstaged 文件 | 仅 worktree vs index（unstaged）diff |
| Working Copy Bar 中的 Staged 文件 | 仅 index vs HEAD（staged）diff |
| 无选择 | 当前 working-copy 两侧总览 |

### 5.3 空状态

无 active repo：Main 显示空状态文案 + "Select a repository from the sidebar"。

无 commit（empty repo）：commit graph 显示空状态 + "Create your first commit" 按钮。

## 6. Working Copy Bar（新增）

> 详见 `04-working-copy.md` 完整规范。本节给布局总览。

### 6.1 布局

**Clean 状态**（无 unstaged / staged 改动）：

```
┌────────────────────────────────────────────────────────────────────────┐
│ main · clean · 0 ↑ 0 ↓                                          ⌥⇧C  │
└────────────────────────────────────────────────────────────────────────┘
```

高度：32px。背景：`bg-bg-secondary`。

**Dirty 状态**（有改动，展开）：

```
┌────────────────────────────────────────────────────────────────────────┐
│ feature/foo · 5 unstaged · 2 staged                            ⌥⇧C  │
├────────────────────────────────────────────────────────────────────────┤
│ ┌─────────────────────────┬─────────────────────────┬─────────────────┐ │
│ │ Unstaged (5)            │ Staged (2)             │ Commit message  │ │
│ │ ┌─────────────────────┐ │ │ ┌─────────────────┐ │ │ ┌─────────────┐ │ │
│ │ │ M src/api.ts    +3 -1│ │ │ │ A new-file.tsx  │ │ │ │ feat: ...   │ │ │
│ │ │ M README.md     +2 -0│ │ │ │ M test.ts  +5-2│ │ │ │             │ │ │
│ │ │ ? new.txt           │ │ │ └─────────────────┘ │ │ │             │ │ │
│ │ └─────────────────────┘ │ │                      │ │ └─────────────┘ │ │
│ │                          │ │                      │ │                 │ │
│ │ [Stage All]              │ │ [Unstage All]       │ │ [Commit] [Amend]│ │
│ └─────────────────────────┴─────────────────────────┴─────────────────┘ │
└────────────────────────────────────────────────────────────────────────┘
```

高度：可拖拽，范围 32px（clean）— 80px（最小 dirty）— 280px（最大）。记忆用户上次高度（per-repo）。

### 6.2 状态切换

| 当前 | 触发 | 下一态 |
|---|---|---|
| Clean | 文件变化（外部编辑 / 命令行） | Dirty，bar 展开 |
| Dirty | commit 后 | Dirty（如果还有未 commit 的），或 Clean |
| Dirty | `git reset --hard` 等 | Clean |

文件变化检测：Sprint 4 实现时考虑 `fsnotify`（macOS FSEvents / Linux inotify / Windows ReadDirectoryChangesW）vs polling。**v0.1 倾向 polling**（间隔 2s），降低跨平台复杂度。

### 6.3 组件结构

| 子组件 | 用途 | 文档 |
|---|---|---|
| `WorkingCopyBar` | 顶部复合组件 | `04-working-copy.md` |
| `BranchIndicator` | 当前 branch 名 + ahead/behind | `02-components.md` §1.13 |
| `FileListItem` | 单个文件行（M/A/D/?/R/C + 路径 + +/-） | `02-components.md` §1.14 |
| `StatusIcon` | 文件 status 字符 + 颜色 | `02-components.md` §1.15 |
| `CommitMessageBox` | 多行输入 + AI placeholder + Amend prefill | `02-components.md` §1.16 |
| `SyncButtons` | REPOS / BRANCHES 标题栏的 Fetch / Pull / Push | `02-components.md` §1.17 |

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