# GitWave · 版本路线

> v0.11.0 交付：[F018](../features/F018-workbench-redesign.md) Tide 工作台第一阶段——双工作区导航、History / Changes 同级工作区、可调分栏、仓库级会话与写操作隔离、虚拟化 diff、受控弹窗确定性关闭。保留现有 Git 能力与确认保护；冲突、stash、工具、AI/设置的整体容器迁移属于后续阶段。见 [任务](../../tasks/feat-workbench-redesign/plan.md) 与 [设计](../../design/09-workbench-redesign.md)。

> v0.10.4 交付：修复历史图节点与 merge 曲线几何、对齐 refs / SHA / 作者 / 时间列，隐藏横向滚动条并保留 graph panning；更新 Tauri、React Aria、Shiki、Lucide 与开发依赖。见 [发布任务](../../tasks/chore-release-v0.10.4/plan.md)。

> v0.10.3 交付：历史优先 UI，见 [任务](../../tasks/feat-history-first-ui/plan.md) 与 [设计](../../design/08-history-first.md)。

> 本文档定义 GitWave 的版本发布计划与迭代节奏。项目采用敏捷快速迭代，无固定阶段。
> 当前发布版本：**v0.11.0**（三平台）。§2.1–§2.3 是已交付的规划基线，不是待办。

## 1. 版本总览

| 版本 | 范围 | 平台 | 状态 |
|---|---|---|---|
| **v0.1** | 14 个核心场景跑通 | macOS 起步 | **已交付** |
| **v0.2** | v0.1 全量 + Windows + AI 进阶 | macOS + Win | **已交付** |
| **v0.3** | v0.2 全量 + Linux + remote 协作 + AI 智能 | macOS + Win + Linux | **已交付**（PR/MR / Issue 未做，见 §2.3） |
| **v0.4–v0.11** | 官网、自动更新、i18n、代理、凭证弹窗、图片 diff、工作台重设计等持续迭代 | 三平台 | **进行中**（当前 v0.11.0） |

工程拆解见 `docs/tech/planning/roadmap-v0.2.md`、`docs/tech/planning/roadmap-v0.3.md`（均为已执行计划，不是开工文档）。

## 2. 版本详细

### 2.1 v0.1（macOS）

**范围**（已交付）：

- Workspace 管理（第一入口）：CRUD、添加/移除 repo、单 active 切换、多 Workspace 同时开
- Git 核心：clone（HTTPS / SSH）/ init / commit / branch / merge / rebase / push / pull / fetch / history / diff / blame / interactive rebase 拖拽 / worktree / stash / submodule init
- SSH key 管理
- 3-way merge + AI conflict explain
- AI commit message（BYOK + Ollama）
- AI provider 配置 UI（OpenAI / Anthropic / Ollama 三选，Workspace-scoped）
- 仅 macOS（后续版本扩到 Win / Linux）

**完成定义**（已满足）：

- 14 个 must-have 全部可用（项 14 AI command palette 按 may-slip 在 v0.2 交付）
- 三个核心场景（commit → push、conflict 解决、Workspace 切换）端到端可走通
- 内部 dogfooding 时间合理，主要场景无崩溃
- AI 行为符合 P1

### 2.2 v0.2（macOS + Win）

**新增范围**（已交付）：

- Windows 适配
- Git LFS / 完整 submodule / reflog 浏览器 / hooks 编辑器
- AI PR 描述 / AI history 解释 / AI command palette（Cmd+K）
- 自定义 prompt template UI（commit / conflict / PR 三套）
- Per-repo AI rules（`.gitwave/` 目录）
- Provider 故障转移

**完成定义**（已满足）：

- macOS + Windows 同时可用
- v0.1 全量功能在 Win 上跑通
- AI 进阶能力端到端可用
- 用户能从 v0.1 平滑升级到 v0.2

### 2.3 v0.3（三平台）

**新增范围**：

- AI repo health dashboard — **已交付**
- AI 误操作恢复（reflog 语义化 + 恢复建议，用户确认后执行）— **已交付**
- 协作：remote 增删改 + URL / 状态可视化 — **已交付**
- 协作：PR/MR 创建、lightweight review、Issue 链接 — **未做**（v0.3 计划明确砍到「仅 remote 管理」；AI PR 描述只生成文本，不调平台 API）
- Linux 稳定版（deb / rpm / AppImage，tag 触发 CI）— **已交付**

**完成定义**：

- 三平台可用 — **已满足**
- AI 智能能力端到端可用 — **已满足**
- 协作能力走通 — **按当时决策：remote 管理闭环已满足**；PR/MR 仍属后续产品范围

### 2.4 v0.4–v0.10（持续迭代）

v0.3 之后不再按「大版本一次性范围」排期，按功能提案合入、tag 发版。已合入产品的提案见 `docs/pm/features/README.md`。按版本号大致对应：

| 版本带 | 已合入（代表性） |
|---|---|
| v0.4.x | 官网（GitHub Pages + gitwave.work） |
| v0.5.x | 应用内自动更新（macOS / Windows / AppImage；deb/rpm 引导到 Releases） |
| v0.6.x | UI 中英双语 + AI 回复语言 |
| v0.7.x | macOS 原生菜单、提交/分支右键菜单、远程分支 DWIM 检出、系统代理、应用内 HTTPS 凭证弹窗、外部工具 / 编辑器打开、图片 diff |
| v0.8.x | 标题栏与操作栏合并、stash 按仓库隔离、未暂存图片 diff 读工作区、macOS dmg 公证票据、检出后侧栏揭示分支、自动刷新全部仓库 |
| v0.9.x | 一次添加多个本地仓库、仓库标签栏滚轮横滚、stash 面板重做、提交列表滚动性能、时间格式统一、孤立 index 冲突可见与 pull 前置检查、可靠性优化、新版图标、按需语法资源与更小前端分包 |
| v0.10.x | 新增仓库与 worktree 的激活选择保持正确，批量添加激活最后一个新增仓库；修复 macOS 原生菜单无法打开设置、添加仓库等弹框，以及更新后应用未重新打开；重启失败保留已安装状态并允许重试；历史优先 UI（紧凑侧栏与健康摘要、有界图预览及完整拓扑、refs 按需展开、紧凑文件列表、键盘调宽与底栏状态）；依赖与 lockfile 维护 |

### 2.5 后续功能优先级

2026-11-27 根据 `main`（`004c9f4`）的功能核查确定以下顺序。近期目标是改善多仓库开发者日常审阅与提交的体验，并让用户能控制哪些内容会发送给云端 AI。

| 顺位 | 优先级 | 功能 | 计划交付范围 |
|---:|---|---|---|
| 1 | P0 | 本地搜索与 Ask AI | 已合入 main（#257），待发布。默认选中首条本地匹配结果，↑ / ↓ 可改选，Enter 执行当前项；无匹配时保留查询。只有明确选择 Ask AI 才发送内容。 |
| 2 | P0 | 云端 AI 脱敏 | 已实现，待发布。七个入口共用凭证、邮箱和电话脱敏；可在本机预览系统提示、用户内容与替换计数，再发送所见内容，或直接生成。见 [F019](../features/F019-ai-redaction-preview.md)。 |
| 3 | P1 | History 变更文件树与独立搜索 | 先在提交详情中按目录组织本次变更的文件，支持展开和折叠；随后在 History 内复用现有分页和定位能力搜索提交。 |
| 4 | P1 | Changes 提交流程 | 提交成功后可直接跳到 History 查看新提交；stage 后文件消失时选中邻项；允许调整提交编辑区的高度。 |
| 5 | P1 | 冲突与交互式 rebase 工作区 | 在主画布区分未保存编辑、已标记解决的文件和待完成的 Git 操作；为 rebase 待办列表补齐键盘排序。 |
| 6 | P2 | 按 hunk、按行暂存 | 先支持按 hunk 暂存，再支持按行选择。用真实仓库验证部分暂存、取消暂存和文件被外部修改时的行为。 |
| 7 | P2 | 两提交对比与路径历史 | 先复用已有 diff 接口，让用户能选取任意两个提交进行比较；再添加按路径筛选历史，明确重命名和分页行为。 |
| 8 | P2 | 会话操作记录与统一命令入口 | 先支持回看操作的目标仓库和结果，再让工具栏、菜单、命令面板复用相同的动作。 |
| 9 | P2 | Workspace 与仓库配置覆盖、快捷键重映射 | 先明确设置作用域、继承顺序和生效方式，再提供仓库级覆盖与快捷键编辑。vim mode 单独评估。 |
| 10 | P3 | 平台协作 | 开发版可用已登录的 GitHub CLI 将已核对的草稿发布为同仓库 PR。[GitHub PR 任务](../../tasks/feat-github-pr/plan.md)。GitLab MR、Issue 关联和轻量评审分别排期。 |

**近期排期**

- 下一版优先交付两项 P0 和第 3 项的[提交详情变更文件树](../../tasks/feat-history-file-tree/plan.md)。[History 本地提交搜索与定位](../../tasks/feat-history-search/plan.md)复用现有分页，第 4 项可拆成小改动穿插交付。
- 第 5 项单独安排一个版本，以真实仓库回归冲突、暂停、继续与中止流程。
- GitHub PR 创建已进入开发版，其他平台集成单独排期。

**待评估与可穿插项**：`.gitignore` 语法高亮可随小改动交付；更多 AI 提供商预设按接入需求安排。统一 AI 助手、stash 与工具面板迁移、Workspace 模板、跨 Workspace 搜索和父目录仓库扫描留待后续评估。

---

## 3. 决策节奏

持续事件驱动，不固定时间周期：

- 持续：发版后更新本表状态与 `docs/pm/features/` 提案状态
- 持续：审视不做清单与版本节奏
- 重大方向调整时：重新审视平台优先级、新功能纳入/排除
- 版本整体回看：重设下一版本

各补丁版本的具体交付见 `02-scope.md` §3.5 版本对照表。
