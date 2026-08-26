# GitWave · Layout Spec

> 3-pane 布局详细规格：topbar + sidebar + feature nav + main。

## 1. 全局 shell

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Topbar (h: 48px)                                                       │
│ ┌──────────┬─────────────────────────┬───────────────────────────────┐ │
│ │ Workspace│  GitWave                │  ⌘K │ ☀/☾ │ SSH │ ? │ v0.1 │ │
│ │ Switcher │                         │      └─ Theme toggle        │ │
│ └──────────┴─────────────────────────┴───────────────────────────────┘ │
├──────────────┬───────────────────────┬──────────────────────────────────┤
│              │                       │                                  │
│ Sidebar      │  Feature Nav         │  Main                            │
│ (w: 240px)   │  (w: 280px)          │  (flex: fill)                   │
│              │                       │                                  │
│              │                       │                                  │
│              │                       │                                  │
│              │                       │                                  │
│              │                       │                                  │
│              │                       │                                  │
│              │                       │                                  │
│              │                       │                                  │
│              │                       │                                  │
│              │                       │                                  │
│              │                       │                                  │
├──────────────┴───────────────────────┴──────────────────────────────────┤
│ Statusbar (h: 24px, 预留)                                            │
└─────────────────────────────────────────────────────────────────────────┘
```

最小窗口尺寸：960 × 600。
推荐：1280 × 800+。

## 2. Topbar

### 2.1 布局

```
┌─────────────────────────────────────────────────────────────────────┐
│ [Workspace▼]  GitWave                       ⌘K  ☀  SSH  ?  v0.1  │
└─────────────────────────────────────────────────────────────────────┘
```

高度：48px。背景：`bg-bg-secondary`。底边：1px `border-subtle`。

### 2.2 元素

| 元素 | 位置 | 交互 |
|---|---|---|
| Workspace Switcher | 左 | 点击 → 下拉，列出 workspaces + New |
| Logo / 名称 | 左中 | GitWave 字样；点击无动作（或跳首页）|
| `⌘K` hint | 右 | 点击 → 打开 CommandPalette（v0.2 Sprint 6）|
| Theme toggle | 右 | 点击循环：light → dark → system |
| SSH 入口 | 右 | 点击 → 打开 SshKeyManager popover / 抽屉 |
| Help (`?`) | 右 | 点击 → 打开快捷键 /文档链接 |
| 版本号 | 右最 | 静态文本（GitWave v0.1.x） |

### 2.3 快捷键

- `⌘K` / `Ctrl+K`：打开 Command Palette
- `⌘1` / `⌘2` / `⌘3`：切换 sidebar / feature nav / main focus
- `⌘,`：打开 Preferences（v0.2）

## 3. Sidebar

### 3.1 布局

```
┌──────────────────────────┐
│ WORKSPACES            [+] │
│                          │
│ ▾ Workspace A         ⋯ │   ← workspace 行（可折叠）
│   ▸ repo-1   [active]    │   ← repo 行（高亮 = active repo）
│   ▸ repo-2   [missing]   │
│   ▸ repo-3               │
│ ▾ Workspace B         ⋯ │
│   ▸ repo-4   [active]    │
│                          │
│ [+ New Workspace]       │
└──────────────────────────┘
```

宽度：240px（可拖拽 180-360）。

### 3.2 元素

| 元素 | 类型 | 备注 |
|---|---|---|
| WORKSPACES 标题 | 静态 | uppercase, text-xs, text-muted |
| `[+]` | Button (ghost) | 新建 workspace 弹 Modal |
| Workspace 行 | ListItem | 折叠 / 展开（▸ / ▾）+ 操作菜单（⋯: rename / delete）|
| Repo 行 | ListItem | 点击 → setActiveRepo；右键菜单：relink / remove |
| `[+ New Workspace]` | Button (ghost) | 底部，全宽 |

### 3.3 状态标记

| 状态 | Badge | 位置 |
|---|---|---|
| active（= workspace.last_active_repo_id） | 默认高亮（左侧 3px accent border） | repo 行 |
| missing | `StatusBadge variant="missing"` | repo 行右侧 |
| ahead / behind | `StatusBadge variant="ahead"/"behind"` | repo 行右侧（v0.1 fetch 后展示）|

## 4. Feature Nav

### 4.1 布局

```
┌────────────────────────────┐
│ [History] Branches Stash    │  ← tabs
│ Tags Remotes Worktrees       │
├────────────────────────────┤
│                            │
│   (active tab content)      │
│   - 文件列表                │
│   - branch 列表             │
│   - stash 列表              │
│                            │
└────────────────────────────┘
```

宽度：280px（可拖拽 200-400）。

### 4.2 Tabs

| Tab | 内容（Sprint） | Sprint |
|---|---|---|
| History | commit 列表 + 当前 commit 信息 | 3 |
| Branches | local / remote branches 列表 | 3 |
| Stash | stash 列表 | 5 |
| Tags | tag 列表 | 5 |
| Remotes | remote + branch tracking | 3 |
| Worktrees | worktree 列表 | 5 |

Tabs 横向排（≥ 6 个时滚动）；active tab 下边框 2px accent。

### 4.3 空状态

无 active repo 时整个 Feature Nav 隐藏（与 sidebar 的 active 状态联动）。

## 5. Main

### 5.1 布局

Main 本身不是单一组件 — 它根据 Feature Nav 的 active tab 切换内容。例如 History tab 时：

```
┌────────────────────────────────────────────────────┐
│ Commit Graph (h: ~50%)                            │
│   ●─ ●─ ●── main                                 │
│   │ ↘                                          │
│   ●─ ●─ ●  ← feature/foo                       │
│                                                    │
│ ─────────── (resize handle, vertical) ─────────── │
│                                                    │
│ Selected Commit Details + Diff (h: ~50%)            │
│   sha · author · date                            │
│   message                                         │
│   ── unified │ split ──                          │
│   @@ -1 +1 @@                                    │
│   -old                                           │
│   +new                                           │
│                                                    │
└────────────────────────────────────────────────────┘
```

也可横向切：commit 详情在左、diff 在右（看个人偏好，v0.1 先纵向）。

### 5.2 各 Tab 对应内容

| Tab | Main 内容 |
|---|---|
| History | 上：commit graph（virtual scroll）；下：commit details + diff |
| Branches | branch 列表 + 选中 branch 的 commit 链 |
| Stash | stash 列表 + 选中 stash 的 diff |
| Tags | tag 列表 + 选中 tag 的 commit details |
| Remotes | remote 列表 + branch tracking 状态 |
| Worktrees | worktree 列表 + 当前 worktree 信息 |

### 5.3 空状态

无 active repo：Main 显示空状态文案 + "Select a repository from the sidebar"。

无 commit（empty repo）：commit graph 显示空状态 + "Create your first commit" 按钮。

## 6. Statusbar（v0.1 占位）

```
┌────────────────────────────────────────────────────┐
│ main · +0 -0  ·  Ahead 0 / Behind 0  ·  SSH ready  │
└────────────────────────────────────────────────────┘
```

高度：24px。文本：`text-xs text-muted`。显示当前 branch、uncommitted changes、ahead/behind、SSH 状态。Sprint 3+ 接入实际数据。

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
| `↑` / `↓` | 在 sidebar / nav 列表项间移动 |
| `←` / `→` | 折叠 / 展开 sidebar workspace |
| `Enter` | 激活当前焦点项 |
| `Space` | 切换 sidebar 当前 workspace 的 active repo |
| `Tab` / `Shift+Tab` | 在 main 内表单字段间移动 |
| `Esc` | 关闭 Modal / popover / palette |
| `⌘Z` / `⌘⇧Z` | undo / redo（v0.2）|

## 9. 关联

- `00-overview.md`：设计目标 + IA 决策
- `01-tokens.md`：颜色 / 间距 / 字体
- `02-components.md`：组件 API + 样式
- `docs/tech/architecture/00-overview.md`：前端架构