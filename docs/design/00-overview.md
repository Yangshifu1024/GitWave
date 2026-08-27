# GitWave · UI/UX 设计总览

> 完整界面设计文档。配合 `01-tokens.md` / `02-components.md` / `03-layout.md` 一起阅读。
> 状态：草案（IA 已与 PM 确认；token / 组件 / 布局待 review）。

## 目标

把现在的"single-column feed"（WorkspaceSwitcher + RepoList + SshKeyManager 顺次堆叠）升级为可承载 **history 图 / diff viewer / interactive rebase / AI command palette** 的 workspace-grade UI。

## IA 决策（已与 PM 确认，2026-08-26）

| 决策 | 选择 | 理由 |
|---|---|---|
| 布局 | **3-pane** | 信息密度高、层级清晰；VS Code / Fork / Tower 都是这个 pattern |
| 平台 feel | **macOS 原生优先，其他平台保持与 macOS 一致** | PM 文档 `docs/pm/core/01-features.md` §1.10 明确 macOS 优先；用 GitWave 自有 theme 模拟 macOS 跨平台 |
| 组件策略 | **引入 UI 库** | 自建 primitives 工作量过大；选用 headless 库 + 自定义样式 |
| 主题 | **跟随系统 + 手动切换都要** | 系统默认跟随，留显式 toggle 给偏好用户 |

## 库选择（决策见 `docs/tech/decisions/0005-ui-library-stack.md`）

| 用途 | 库 | 理由 |
|---|---|---|
| Utility CSS | **Tailwind CSS v4** | 纯函数 CSS、token-driven；v4 CSS-first 配置无需 JS config |
| 原子组件（headless）| **Radix UI Primitives** | 完整 a11y；不锁定样式；体积小（按需引入）|
| 变体管理 | **class-variance-authority (cva)** + **tailwind-merge** | 标准 Tailwind 配套 |
| 图标 | **Lucide React** | MIT、tree-shakeable、stroke 风格契合 macOS 风格 |
| 语法高亮 | **Shiki** | VS Code TextMate 引擎，准确度高；预编译 token 一次性 |
| 虚拟滚动 | **@tanstack/react-virtual** | 与现有 TanStack Query 生态一致；Sprint 3 history graph 需要 |
| 动效 | **Framer Motion** | 体积略大但声明式 API 干净；v0.1 可选 |

**不引入**：

- shadcn/ui（copy-paste 模型与"headless + 自定义样式"目标冲突）
- Material UI / Ant Design（视觉风格锁定 macOS feel）
- Monaco / CodeMirror（diff viewer 选 Shiki + 自建 viewer，不引编辑器）

## 视觉风格基调（原生桌面壳 + GitWave 身份）

桌面 Git 客户端，不是网站。签名只有一处：**Tide Lanes**（history 图的青绿→靛蓝贝塞尔 lane）。其余 chrome 保持安静。

校对稿：[`mockups/README.md`](./mockups/README.md)。

| 维度 | 选择 |
|---|---|
| 色板 | Foam / Mist / Ink / Tide `#1A8F8A` / Abyss / Coral。不用纯白、不用系统蓝 `#007aff` |
| 字体（UI）| SF Pro Text / Segoe UI / Cantarell |
| 字体（SHA / 路径 / diff）| IBM Plex Mono |
| 行高 | chrome 1.2–1.3；列表 / graph 行 28px |
| 圆角 | 6 (控件) / 8 (卡片) / 12 (模态) |
| 阴影 | subtle / modal 两档；不用黑色硬阴影 |
| 焦点态 | Tide 2px outline，无 web 式 `ring-offset` |
| 动效 | 160–200ms ease-out；模态用 spring (300ms) |
| 间距 | 4 / 8 / 12 / 16 / 24 / 32 |

具体值见 `01-tokens.md`。

## 3-pane 布局概览

```
┌─────────────────────────────────────────────────────────────────┐
│  Toolbar (~40px)  动作工具栏，不要品牌站                         │
│  [WS▾]  repo  main ↑2 ↓1     Fetch Pull Push         ⌘K  ☀/☾ │
├────────────┬───────────────────────────────┬────────────────────┤
│ Sidebar    │  History graph（主角）         │  Inspector         │
│ (220px)    │  flex                         │  (~360px)          │
│ repos      │  Tide Lanes + commit 行       │  详情 / diff       │
│ branches   │                               │                    │
│ stash/tags │                               │                    │
│ remotes /  │                               │                    │
│ worktrees  │                               │                    │
├────────────┴───────────────────────────────┴────────────────────┤
│ Working Copy Bar  clean 32px / dirty 展开文件列表 + commit     │
└─────────────────────────────────────────────────────────────────┘
```

- **Toolbar**：Workspace 切换 + 当前 repo / branch + Sync（Fetch/Pull/Push）+ ⌘K + 主题。无居中字标、无版本号、无 Help 图标。SSH 进溢出菜单。
- **Sidebar（Source List）**：当前 workspace 的 repos + branches + stash / tags / remotes / worktrees。Mist 底，与 Foam 画布有材质差。
- **History graph**：永远是中栏主角，不是 Tab。
- **Inspector**：选中 commit 或 working-copy 文件的详情 / diff。
- **Working Copy Bar**：clean 32px / dirty 展开至 ~220px（文件列表从 Changes Tab 收回）。详见 `04-working-copy.md`。

详见 `03-layout.md` / `04-working-copy.md`。

## Primitive 清单

### Core primitives

| Primitive | 用途 | 状态 |
|---|---|---|
| `Button` | primary / secondary / danger / ghost | 待建 |
| `Input` | text / search | 待建 |
| `Modal` | 居中模态（已有 HTMLDialogElement，待 Radix 替换以增强 a11y） | 部分已有 |
| `Tooltip` | hover/focus 提示 | 待建 |
| `Toast` | 非阻塞反馈 | 待建 |
| `Tabs` | 二级导航 | 待建 |
| `Split` / `Pane` | 3-pane 布局 + 可拖拽 handle | 待建 |
| `ListItem` | hover / selected / actions slot / status badge slot | 待建 |
| `StatusBadge` | active / missing / ahead / behind / conflict | 待建 |
| `ContextMenu` | 右键菜单（interactive rebase 操作入口） | 待建 |
| `KeyHint` | Cmd+K hint 等快捷键提示 | 待建 |
| `EmptyState` | 引导文案 | 已有（待统一） |

### Working Copy primitives（Sprint 4）

| Primitive | 用途 | 状态 |
|---|---|---|
| `WorkingCopyBar` | 底部复合组件（bar 自身） | 待建 |
| `BranchIndicator` | 当前 branch + ahead/behind chip | 待建 |
| `FileListItem` | 单个文件变更行（M/A/D/? + +/-） | 待建 |
| `StatusIcon` | 文件 status 字符 + 颜色 | 待建 |
| `CommitMessageBox` | 多行 message 输入 + AI 按钮 | 待建 |
| `SyncButtons` | 顶部 Fetch / Pull / Push 三按钮 | 待建 |

详见 `02-components.md` §3 + `04-working-copy.md`。

## 迁移路径

| 阶段 | 内容 | 工作量估算 |
|---|---|---|
| **0** | 安装依赖 + Tailwind 配置 + 暗色跟随系统 | 0.5 天 |
| **1** | tokens.ts + Tailwind theme 扩展 + 引入 Radix | 0.5 天 |
| **2** | Core primitives 实现（Button / Input / Tooltip / Toast / Tabs / Pane / ListItem / StatusBadge / ContextMenu / KeyHint / Modal 替换）| 1.5 天 |
| **3** | Working Copy primitives + Bottom Bar + Topbar sync 按钮 | 1.5 天 |
| **4** | App.tsx 重构为 3-pane + Working Copy Bar + 现有 Workspace/Repo/SSH 迁移到新 primitives | 1 天 |
| **5** | 验证（lint / typecheck / build / 手动 / 全平台暗色）| 0.5 天 |
| 合计 | | **~5 天**（原 ~4 + 1 增量 for Working Copy）|
| **3** | App.tsx 重构为 3-pane 布局 + 现有 WorkspaceSwitcher / RepoList / SshKeyManager 迁移到新 primitives | 1 天 |
| **4** | 验证（lint / typecheck / build / 手动 / 全平台暗色）| 0.5 天 |
| 合计 | | **~5 天**（原 ~4 + 1 增量 for Working Copy）|

## 验证

- npm run typecheck / lint / build 全过
- `pre-commit run --all-files` 全过
- 手动 pnpm tauri dev 跑通 + 3-pane 可拖拽 resize
- 主题：跟随系统切换 + 手动 override 都生效
- 所有 Primitive 通过 Radix 内置 a11y 测试（键盘 Tab 序、焦点环、aria-属性）

## 关联

- `01-tokens.md`：颜色 / 间距 / 字体 / 圆角 / 阴影 / 动效 token
- `02-components.md`：组件清单 + API（含 Working Copy primitives §3）
- `03-layout.md`：3-pane 详细规格（含 Working Copy Bar 占位 §6）
- `04-working-copy.md`：Working Copy Bar 完整规范（Sprint 4 实施依据）
- `docs/tech/decisions/0005-ui-library-stack.md`：库选择 ADR
- `docs/pm/core/01-features.md` §1.10：平台与 UX 约束
- `docs/tech/architecture/00-overview.md`：前端架构