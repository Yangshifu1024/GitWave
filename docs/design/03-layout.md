# GitWave · Layout Spec

> 3-pane 布局详细规格：toolbar + sidebar + history graph + inspector。

## 1. 全局 shell

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Toolbar (h: 40px)                                                      │
│  [WS▾]  repo-name  main ↑2 ↓1     Fetch  Pull  Push          ⌘K  ☀/☾ │
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
│ [Workspace▾]  repo  main ↑2 ↓1     Fetch  Pull↓3  Push↑2          ⌘K  ☀ │
└─────────────────────────────────────────────────────────────────────────┘
```

高度：40px。背景：`bg-bg-secondary`（Mist / Abyss）。底边：1px `border-subtle`。
不要居中字标、不要版本号、不要 Help 图标。

### 2.2 元素

| 元素 | 位置 | 交互 |
|---|---|---|
| Workspace Switcher | 左 | 点击 → 下拉，列出 workspaces + New |
| 当前 repo 名 | 左 | 只读标签（切换在侧栏） |
| 当前 branch 指示 | 左中 | 显示 branch 名（ahead/behind 数字 chip） |
| **Fetch / Pull / Push** | 中 | Pull/Push 在 ahead/behind = 0 时灰显 |
| `⌘K` hint | 右 | 点击 → 打开 CommandPalette（v0.2 Sprint 6） |
| Theme toggle | 右 | 点击循环：light → dark → system |
| 溢出菜单 | 右 | SSH Keys |

### 2.3 当前 Branch 指示

格式：`branch_name [↑N] [↓N]`

示例：
- `main`（无 ahead/behind）
- `feature/foo ↑2 ↓3`（本地领先 2，远程领先 3）
- `feature/foo ↑2`（ahead 2，可 Push）
- `feature/foo ↓3`（behind 3，可 Pull）
- `detached @ abc1234`（HEAD 分离状态，灰显）

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
│ REPOS                 [+] │
│   gitwave      [active]   │
│   notes                   │
│ BRANCHES              [+] │
│   main             HEAD   │
│   feature/tide-lanes      │
│ STASH / TAGS / REMOTES …  │
└──────────────────────────┘
```

宽度：220px（可拖拽 180-360）。背景 Mist，与 Foam 画布区分。Workspace 切换在 Toolbar，侧栏从 Repos 起。

### 3.2 元素

| 元素 | 类型 | 备注 |
|---|---|---|
| REPOS 标题 | 静态 | uppercase, text-xs, text-muted |
| Repo 行 | ListItem | 点击 → setActiveRepo；右键菜单：relink / remove |
| BRANCHES / STASH / … | SidebarSection | 见 §4 |

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
| `SyncButtons` | 顶部的 Fetch / Pull / Push 三按钮 | `02-components.md` §1.17 |

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