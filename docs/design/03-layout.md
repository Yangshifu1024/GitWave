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
│ (w: 240px)   │  (w: 280px)          │ (flex: fill)                   │
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
│ Working Copy Bar (collapsible, ~32-280px) 详见 §6                  │
└─────────────────────────────────────────────────────────────────────────┘
```

最小窗口尺寸：960 × 600。
推荐：1280 × 800+。

## 2. Topbar

### 2.1 布局

```
┌─────────────────────────────────────────────────────────────────────────┐
│ [Workspace▼]  GitWave                          ⌘K  ☀  [Fetch] [Pull↓3] [Push↑2]  ?  v0.1  │
└─────────────────────────────────────────────────────────────────────────┘
```

高度：48px。背景：`bg-bg-secondary`。底边：1px `border-subtle`。

### 2.2 元素

| 元素 | 位置 | 交互 |
|---|---|---|
| Workspace Switcher | 左 | 点击 → 下拉，列出 workspaces + New |
| 当前 branch 指示 | 左中 | 显示 branch 名（ahead/behind 数字 chip） |
| Logo / 名称 | 左中 | GitWave 字样；点击无动作（或跳首页） |
| `⌘K` hint | 右 | 点击 → 打开 CommandPalette（v0.2 Sprint 6） |
| Theme toggle | 右 | 点击循环：light → dark → system |
| **Fetch 按钮** | 右 | 点击 → `cmd_fetch`，Toast 反馈 |
| **Pull 按钮** | 右 | behind = 0 时灰显；`↓N` badge 显示 ahead |
| **Push 按钮** | 右 | ahead = 0 时灰显；`↑N` badge 显示 ahead |
| SSH 入口 | 右 | 点击 → 打开 SshKeyManager popover / 抽屉 |
| Help (`?`) | 右 | 点击 → 打开快捷键 /文档链接 |
| 版本号 | 右最 | 静态文本（GitWave v0.1.x） |

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
- `⌘1` / `⌘2` / `⌘3` / `⌘4`：切换 sidebar / feature nav / main / working copy bar focus
- `⌘⇧F`：Fetch
- `⌘⇧P`：Pull
- `⌘⇧U`：Push（U = upstream；与 Tower 一致）
- `⌘,`：打开 Preferences（v0.2）

详见 `04-working-copy.md` §8 完整快捷键表。

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