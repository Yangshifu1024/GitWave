# GitWave · UI/UX 设计总览

> 完整界面设计文档。配合 `01-tokens.md` / `02-components.md` / `03-layout.md` 一起阅读。
> 状态：已落地（对照 v0.9.3）。IA 与 3-pane 仍有效；组件栈见下方「库选择」修订。

## 目标

把现在的"single-column feed"（WorkspaceSwitcher + RepoList + SshKeyManager 顺次堆叠）升级为可承载 **history 图 / diff viewer / interactive rebase / AI command palette** 的 workspace-grade UI。

## IA 决策（已与 PM 确认，2026-08-26）

| 决策 | 选择 | 理由 |
|---|---|---|
| 布局 | **3-pane** | 信息密度高、层级清晰；VS Code / Fork / Tower 都是这个 pattern |
| 平台 feel | **macOS 原生优先，其他平台保持与 macOS 一致** | PM 文档 `docs/pm/core/01-features.md` §1.10 明确 macOS 优先；用 GitWave 自有 theme 模拟 macOS 跨平台 |
| 组件策略 | **引入 UI 库** | 自建 primitives 工作量过大；选用 headless 库 + 自定义样式 |
| 主题 | **跟随系统 + 手动切换都要** | 系统默认跟随，留显式 toggle 给偏好用户 |
| 配色 palette | **Native Blue（默认）+ Tide Studio 可选**，Settings 内切换 | 2026-08-27 与 PM 确认：默认贴近 macOS 原生观感；青绿签名 Tide 保留为选项。见 `06-color-palettes.md` |

## 库选择（决策见 `docs/tech/decisions/00-overview.md` ADR 0005）

原选择是 Radix Primitives。2026-08-28 起 `src/components/ui/` 换成 **HeroUI v3**（见 `docs/tasks/feat-heroui-migration/plan.md`）。截至 v0.9.3：

| 用途 | 库 | 理由 |
|---|---|---|
| Utility CSS | **Tailwind CSS v4** | 纯函数 CSS、token-driven；v4 CSS-first |
| 交互组件 | **HeroUI v3**（底层 React Aria Components） | 取代 Radix；文件名 / 导出 / props 保持兼容 |
| 变体管理 | **cva** + **tailwind-merge** | Button / StatusBadge；其余走 HeroUI |
| 图标 | **Lucide React** | MIT、tree-shakeable |
| 语法高亮 | **Shiki**（依赖已入，**DiffViewer 未接线**） | 规划仍是 TextMate 引擎；当前 diff 为自绘 character-level |
| 虚拟滚动 | **@tanstack/react-virtual** | history graph |
| 动效 | HeroUI CSS | **未**引入 Framer Motion |
| 3-pane / Split | 自研 | HeroUI 无 splitter |

**不引入**：

- shadcn/ui（copy-paste 模型与"headless + 自定义样式"目标冲突）
- Material UI / Ant Design（视觉风格锁定 macOS feel）
- Monaco / CodeMirror（diff viewer 自建，不引编辑器）

## 视觉风格基调（原生桌面壳 + GitWave 身份）

桌面 Git 客户端，不是网站。签名只有一处：**Tide Lanes**（history 图的青绿→靛蓝贝塞尔 lane）。其余 chrome 保持安静。

校对稿：[`mockups/README.md`](./mockups/README.md)。

| 维度 | 选择 |
|---|---|
| 色板 | 双 palette 共享中性语义色（见 `06-color-palettes.md`）：**Native Blue 默认**（macOS 系统灰阶 + systemBlue `#007AFF`）；Tide Studio（Foam / Mist / Ink / Tide `#1A8F8A` / Abyss / Coral）可选。「不用纯白、不用系统蓝」原则适用于 Tide 及品牌签名色，native-blue 为显式例外（用户 2026-08-27 拍板） |
| 字体（UI）| SF Pro Text / Segoe UI / Cantarell |
| 字体（SHA / 路径 / diff）| IBM Plex Mono |
| 行高 | chrome 1.2–1.3；列表 / graph 行 28px |
| 圆角 | 6 (控件) / 8 (卡片) / 12 (模态) |
| 阴影 | subtle / modal 两档；不用黑色硬阴影 |
| 焦点态 | Accent 2px outline（native-blue 为系统蓝、tide 为青绿），无 web 式 `ring-offset` |
| 动效 | 160–200ms ease-out；模态用 spring (300ms) |
| 间距 | 4 / 8 / 12 / 16 / 24 / 32 |

具体值见 `01-tokens.md`。

## 3-pane 布局概览

```
┌─────────────────────────────────────────────────────────────────┐
│  TitleBar  菜单 · 工作区 · 外部工具 · 操作按钮 · 状态区（窗口居中）        │
├────────────┬───────────────────────────────┬────────────────────┤
│ Sidebar    │  History graph（主角）         │  Inspector         │
│ (320px)    │  flex                         │  (~360px)          │
│ workspaces │  Tide Lanes + commit 行       │  详情 / diff       │
│ repos      │                               │                    │
│ branches   │                               │                    │
│ stash/tags │                               │                    │
│ remotes /  │                               │                    │
│ worktrees  │                               │                    │
├────────────┴───────────────────────────────┴────────────────────┤
│ Working Copy Bar  clean 32px / dirty 展开文件列表 + commit     │
└─────────────────────────────────────────────────────────────────┘
```

- **TitleBar**：自绘标题栏（40px，唯一顶栏）。左侧应用菜单 + 工作区选择器 + 外部工具快捷入口；右侧 Local Changes / Stash / Fetch / Pull / Push；同步状态区绝对居中于窗口。详见 `03-layout.md` §2。
- **Sidebar（Source List）**：branches + stash / tags / remotes / worktrees / submodules 等 sections（同步按钮不在侧栏）。Mist 底，与 Foam 画布有材质差。
- **History graph**：永远是中栏主角，不是 Tab。
- **Inspector**：选中 commit 或 working-copy 文件的详情 / diff。
- **Working Copy Bar**：clean 32px / dirty 展开至 ~220px（文件列表从 Changes Tab 收回）。详见 `04-working-copy.md`。

详见 `03-layout.md` / `04-working-copy.md`。

## Primitive 清单

> 清单源自 2026-08-26 的待建计划，「状态」列已按当前实现（v0.9.3）更新。通用 primitive 位于 `src/components/ui/`（HeroUI v3 wrapper），Working Copy 相关同样在其中。

### Core primitives

| Primitive | 用途 | 状态 |
|---|---|---|
| `Button` | primary / secondary / danger / ghost | 已落地 |
| `Input` | text / search | 已落地 |
| `Modal` | 居中模态 | 已落地（HeroUI Dialog，取代 HTMLDialogElement） |
| `Tooltip` | hover/focus 提示 | 已落地 |
| `Toast` | 非阻塞反馈 | 未落地（无 Toast 组件；反馈走 `ErrorAlert` 与标题栏状态区） |
| `Tabs` | 二级导航 | 已落地 |
| `Split` / `Pane` | 3-pane 布局 + 可拖拽 handle | 已落地（`Split` + `ThreePaneLayout`，自研） |
| `ListItem` | hover / selected / actions slot / status badge slot | 已落地 |
| `StatusBadge` | active / missing / ahead / behind / conflict | 已落地 |
| `ContextMenu` | 右键菜单（interactive rebase 操作入口） | 已落地 |
| `KeyHint` | Cmd+K hint 等快捷键提示 | 未落地（`src/components/ui/` 无此组件） |
| `EmptyState` | 引导文案 | 已落地 |

### Working Copy primitives

| Primitive | 用途 | 状态 |
|---|---|---|
| `WorkingCopyBar` | 底部复合组件 | 已被 `WorkingCopyModal` 取代（2026-08 重构，见 `03-layout.md` §6） |
| `BranchIndicator` | 当前 branch + ahead/behind chip | 已落地 |
| `FileListItem` | 单个文件变更行（M/A/D/? + +/-） | 已落地 |
| `StatusIcon` | 文件 status 字符 + 颜色 | 已落地 |
| `CommitMessageBox` | 多行 message 输入 + AI 按钮 | 已落地 |
| `SyncButtons` | 同步按钮组 | 已落地但当前未被引用；Fetch / Pull / Push 在标题栏内实现（见 `03-layout.md` §2.2） |

详见 `02-components.md` §3 + `04-working-copy.md`。

## 迁移路径

> 历史排期（Sprint 3 设计落地）。Radix 阶段已完成并在后续被 HeroUI v3 替换。不是当前待办。

| 阶段 | 内容 | 工作量估算 |
|---|---|---|
| **0** | 安装依赖 + Tailwind 配置 + 暗色跟随系统 | 0.5 天 |
| **1** | tokens.ts + Tailwind theme 扩展 + 引入 Radix | 0.5 天 |
| **2** | Core primitives 实现（Button / Input / Tooltip / Toast / Tabs / Pane / ListItem / StatusBadge / ContextMenu / KeyHint / Modal 替换）| 1.5 天 |
| **3** | Working Copy primitives + Bottom Bar + Topbar sync 按钮 | 1.5 天 |
| **4** | App.tsx 重构为 3-pane + Working Copy Bar + 现有 Workspace/Repo/SSH 迁移到新 primitives | 1 天 |
| **5** | 验证（lint / typecheck / build / 手动 / 全平台暗色）| 0.5 天 |
| 合计 | | **~5 天**（原 ~4 + 1 增量 for Working Copy）|

## 验证

- npm run typecheck / lint / build 全过
- `pre-commit run --all-files` 全过
- 手动 pnpm tauri dev 跑通 + 3-pane 可拖拽 resize
- 主题：跟随系统切换 + 手动 override 都生效
- 交互组件走 HeroUI / React Aria 的键盘与 aria 行为（原计划写的是 Radix 内置 a11y）

## 关联

- `01-tokens.md`：颜色 / 间距 / 字体 / 圆角 / 阴影 / 动效 token
- `02-components.md`：组件清单 + API（含 Working Copy primitives §3）
- `03-layout.md`：3-pane 详细规格（含 Working Copy Bar 占位 §6）
- `04-working-copy.md`：Working Copy Bar 完整规范（Sprint 4 实施依据）
- `05-visual-redesign.md`：主界面视觉重设计（v2，chrome 层叠与组件形态）
- `06-color-palettes.md`：配色方案（native-blue 默认 / tide 可选）
- `07-theme.md`：主题设计（颜色 / 字体 / 动效，已实施）
- `docs/tech/decisions/00-overview.md` ADR 0005：库选择（含 HeroUI 修订）
- `docs/pm/core/01-features.md` §1.10：平台与 UX 约束
- `docs/tech/architecture/00-overview.md`：前端架构