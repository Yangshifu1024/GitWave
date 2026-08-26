# GitWave · 架构决策记录（ADR）

> 项目级架构决策汇总。每项决策遵循 **上下文 / 备选 / 决策 / 后果** 四段式。

## 当前决策

| 编号 | 主题 | 状态 |
|---|---|---|
| 0001 | 框架选型：Tauri 2 + Rust core | 已采纳 |
| 0002 | Workspace 抽象：无 FS 实体 | 已采纳 |
| 0003 | 凭证策略：混合（Keychain + git helper） | 已采纳 |
| 0004 | AI 双轨：写 = 确定性，建议 = AI | 已采纳 |
| 0005 | 前端 UI 库栈：Tailwind v4 + Radix + Lucide + Shiki | 已采纳 |

---

## 0001 · 框架选型：Tauri 2 + Rust core

### 上下文

- v0.1 仅 macOS；v0.2 扩 Windows；v0.3 加 Linux（见 `docs/pm/core/03-roadmap.md`）
- 性能基线对标 Fork / Sublime Merge：history 图数万 commit 必须流畅，diff 大文件不卡 UI
- 包体积与冷启动需优于 Electron 系
- 生态资产：React + Monaco / CodeMirror 等需可复用

### 备选

- **SwiftUI 原生**：macOS 体验极限，但 Windows / Linux 需整套 UI 重写
- **Electron**：生态成熟，但包体积（>100MB）与冷启动偏重；内存占用高
- **Flutter Desktop**：自绘 UI 跨平台一致，但 WebView / Monaco / diff viewer 等生态无法复用
- **Tauri 2**：跨平台路径 + Rust core 性能 + 较小包体积；WebView 三端一致较好（macOS WebKit / Win WebView2 / Linux WebKitGTK）

### 决策

采用 **Tauri 2 + Rust core**，前端 React + TypeScript。

### 后果

- **正面**：v0.2/v0.3 扩平台时 UI 复用率高；性能关键路径（diff 计算、history 渲染、git 后台操作）可下沉 Rust
- **负面**：WebView 内核差异需做适配与回归矩阵（CSS、JS 行为、系统集成）；Monaco 等大型组件需评估 WebKit 下的启动开销
- **风险**：必须早期建立 macOS / Windows / Linux 三平台 CI 矩阵；前端需避免使用 WebView 不一致的最新 Web API

---

## 0002 · Workspace 抽象：无 FS 实体

### 上下文

- Workspace = 多个 Git 仓库 + 共享配置的集合
- 任何时刻只有一个 active repo
- 可同时打开多个 Workspace
- 必须跨平台、跨用户目录、跨仓库迁移
- 用户已有本地仓不可被强制重定位

### 备选

- **每个 repo 内嵌 `.gitwave/`**：随仓迁移；但跨 repo Workspace 不能跨仓共享，且每次 clone 都要决定写不写；语义污染
- **集中根目录**（如 `~/Gitwave/workspaces/<id>/`）：要求 repo 物理上能被收编，与用户现有目录习惯冲突
- **workspace 文件**（VSCode `.code-workspace` 风格）：需要某个目录拥有该文件，违反"无 FS 实体"
- **纯数据库存储**：Workspace 仅是数据库中的一行 + Repo 引用列表，零文件系统依赖

### 决策

Workspace = 数据库中的一行；零 FS 实体。

### 后果

- **正面**：可指向任意路径的 repo（含已存在的本地仓），不影响用户目录结构；Workspace 导入/导出仅是 JSON 文件（`.gitwave-workspace.json`）；跨平台一致
- **负面**：用户重装系统需备份数据库；多 Workspace 同开依赖进程内多 workspace 上下文
- **风险**：数据库损坏需提供自动备份 + 导出机制；启动时检测迁移

---

## 0003 · 凭证策略：混合（Keychain + git helper）

### 上下文

- AI provider API key 必须本地存储（BYOK 原则，见 P1）
- Git 凭证（HTTPS）必须与系统 git 生态一致——用户已有的 helper 配置不可绕过
- SSH 必须走 ssh-agent + 配置 key
- 任何方案都不能"明文凭证落本地"

### 备选

- **OS Keychain 全包**：安全、跨平台差异（macOS Keychain / Win Credential Manager / Linux Secret Service）；但与 git helper 隔离，用户在 git 已配的 helper 不被复用
- **加密本地配置**：每次启动解锁繁琐，体验差
- **仅 git helper + SSH agent**：BYOK 的 AI API key 没有兜底，会被迫落盘
- **混合**：BYOK 走 Keychain，git 凭证走 git helper，SSH 走 ssh-agent

### 决策

采用 **混合方案**：

- **AI provider API key** → OS Keychain
- **HTTPS git 凭证** → 系统 `git credential helper`（用户在系统 git 已配置的自动复用）
- **SSH** → ssh-agent + 用户配置的 key

### 后果

- **正面**：与系统 git 完全兼容；API key 走 OS 保险箱；用户已有 git 凭证自动复用；与 P1（本地优先 + 隐私可控）完全一致
- **负面**：BYOK key 在 Keychain 需做三平台测试；用户切换 OS 时 key 不会自动迁移
- **风险**：Linux Secret Service 在桌面环境未启用时（headless 服务器）需降级到加密文件存储（需用户密码）；降级路径要明确提示

---

## 0004 · AI 双轨：写 = 确定性，建议 = AI

### 上下文

- P1 原则：AI 是协作者不是替代者；永不自动 commit / push / merge（见 `docs/pm/core/01-features.md` §2.1）
- 三个核心 AI 能力：commit message 生成、conflict 解释、PR 描述
- 风险：AI 误生成可执行的 git 命令可能改坏仓库；用户信任一旦破坏难恢复

### 备选

- **AI 直接执行一切**：直接违反 P1
- **AI 完全不参与写**：但 commit message / PR 描述 / conflict 解释需要 AI 协助
- **双轨**：所有写操作由确定性引擎（libgit2 + Rust 用例）执行；AI 仅生成文本建议

### 决策

采用 **写 = 确定性，建议 = AI**：

- 所有 git 写操作（commit / push / merge / rebase / 任何改 reflog 的动作）必须由确定性引擎执行
- AI 仅生成：commit message 候选、conflict 语义解释、PR 描述、history 解释、repo health、误操作恢复建议
- AI 输出永远以"建议"形式呈现，用户必须显式确认（编辑、点击接受、忽略）

### 后果

- **正面**：与 P1 完全一致；AI 误生成不会直接改仓库；用户保留完全控制权；审计路径清晰
- **负面**：commit message 多 commit 累积理解需在 UI 层做"上下文拼装"再交给 AI；AI 能力被刻意收窄
- **风险**：conflict 解释面板必须明确"AI 解释仅供参考，冲突解决仍由确定性引擎执行"——UI 措辞是关键
---

## 0005 · 前端 UI 库栈

### 上下文

Sprint 1+2 的前端用纯 HTML + 少量 CSS 实现（来自 Sprint 0 的 Tauri 模板默认）。这种实现对 WorkspaceSwitcher / RepoList / SshKeyManager 这类简单 UI 够用，但要承载 Sprint 3+ 的 history 图、diff viewer、interactive rebase、command palette 远远不够：

- **a11y**：HTMLDialogElement 直接使用 + 自己写 focus trap → 出错风险高，screen reader 体验差
- **变体管理**：Button / Input 各处用 className 字符串拼装 → 重复 + 难维护
- **主题**：light / dark mode 用 CSS @media + 重复样式 → 难扩展
- **虚拟滚动**：history graph 10k commit 流畅目标需要专门的库
- **动效**：模态进出 / toast 现在没有 motion → 体验僵硬

自建 primitives 工作量过大；引入"开箱即用"的库（如 Material UI）会锁定 macOS feel。

### 备选

- **库组合 A：自建 primitives + 纯 CSS**：完全控制 / 最小体积；但 Button / Input / Tooltip / Tabs / ContextMenu / ListItem / StatusBadge / Split 全自建，估计 1500 行 + 大量 a11y 测试。否决。
- **库组合 B：shadcn/ui**：组件全 + a11y；但复制后属于我们的代码需自己维护，v0.1 时间紧引入价值有限。保留为 v0.2 备选。
- **库组合 C：Radix Primitives + Tailwind v4 + cva（推荐）**：覆盖最常用的交互原语，a11y 完整；Tailwind + cva 配变体管理；每个库单一职责可按需替换；体积可控。
- **库组合 D：Mantine / Chakra / Radix Themes**：完整组件库；但主题锁定与 macOS feel 目标冲突。否决。

### 决策

采用**库组合 C**：

| 用途 | 库 |
|---|---|
| Utility CSS | Tailwind CSS v4 |
| 交互原语 | Radix UI Primitives |
| 变体管理 | class-variance-authority (cva) + tailwind-merge |
| 图标 | Lucide React |
| 语法高亮（Sprint 3）| Shiki |
| 虚拟滚动（Sprint 3）| @tanstack/react-virtual |
| 动效（v0.1 可选 / Sprint 6）| Framer Motion |

**总开销**（除 Shiki 外）：约 50KB gzip。Shiki 在 Sprint 3 才引入。

### 后果

- **正面**：a11y 完整；主题切换干净；变体管理标准化；Sprint 3 引入 Shiki / react-virtual 风险低
- **负面**：学习成本（Radix API + cva 模式）；自定义样式需熟悉 Tailwind v4 CSS-first
- **风险**：Tailwind v4 较新，生态适配可能滞后；Radix 各 Primitives 独立发布，需手动同步版本。缓解：固定 package.json 版本范围，季度升级

### 关联

- `docs/design/00-overview.md`：设计总览
- `docs/design/01-tokens.md`：tokens
- `docs/design/02-components.md`：组件 API
- `docs/design/03-layout.md`：3-pane 布局
