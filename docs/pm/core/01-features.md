# GitWave · 功能列表

> 本文仅列出 GitWave 应具备的功能与明确不做的事。产品原则与 Workspace 概念作为内联说明融入对应功能小节。
> 当前发布版本 **v0.9.3**。下文是产品意图，不是待办清单；已交付对照见 `02-scope.md` §3 与 `03-roadmap.md`。

## 1. 功能范围

### 1.1 Core Git Operations

GUI友好的 Git 全集是基本盘，不做简化版 Git GUI。

clone / init / open · commit / amend / revert · branch / checkout / switch / create / delete · merge · rebase (interactive) · cherry-pick · push / pull / fetch · stash · tag

stash 面板：每条 stash 常驻一排带标签的操作按钮（查看 / 应用 / 应用并删除 / 删除）；详情窗口把文件列表与 diff 并排；应用前先检查工作区是否有未提交改动；丢弃前需确认；以未跟踪方式保存的文件也出现在 stash 内容里。

### 1.2 Visualization

性能为入场券级要求（基线对标 Fork / Sublime Merge）：打开应用到首个可交互界面不可有可感知等待；仓库 history 图（含数万 commit）渲染必须流畅；提交列表在大仓库中滚动同样不得卡顿；大文件 diff 不卡 UI；后台 git 操作不阻塞用户主操作；视觉化渲染具备 lazy 策略。

History graph（commit DAG，筛选 / 搜索）· 文件 diff（split / unified，character-level 高亮；Shiki 语法高亮已入依赖尚未接线）· blame / annotate · file tree（**未交付**）· branch tree

### 1.3 Advanced Git

- Worktree UI
- Submodule 管理
- Git LFS
- Reflog explorer
- Hooks 编辑器

### 1.4 Workspace 管理

**概念**：Workspace = 一个有名字、持久的 Git 仓库集合 + 共享配置。Workspace 是抽象概念，在文件系统中无实体——不依赖任何 root 目录，也不对应真实文件夹。任何时刻只有一个 active 仓库，但可同时打开多个 Workspace 并一键切换。

**存储**：数据库或 settings 文件，重启后状态完整恢复。

**数据模型**：

```ts
Workspace { id, name, repos: [RepoRef], settings, lastActiveRepoId?, createdAt, updatedAt }
WorkspaceSettings { aiProvider, promptTemplates, commitConvention, themeOverride?, keyBindingProfile }
RepoRef { id, path, nickname?, settingsOverride? }
```

**功能**：

- Workspace CRUD（创建 / 打开 / 重命名 / 删除）
- Repo 添加：init 本地新仓库（不自动初始 commit）/ clone HTTPS（走 `git credential helper`）/ clone SSH（走配置 key）/ 本地已有仓库
- Repo 批量添加：一次选择多个本地文件夹加入，跳过已加入的路径与本批内的重复；不可用的路径会报告，但不中断整批
- Repo 移除 / 重新链接缺失 repo
- Workspace 内切换 repo（侧边栏点击主视图，单 active）
- 空状态引导：无 Workspace / 有 Workspace 但无 repo / 空仓库三种空状态都直接给出下一步动作（新建工作区、克隆 / 初始化 / 添加本地仓库、创建第一个提交），不需要先去应用菜单里找入口
- 仓库标签栏在标签溢出时可用鼠标滚轮横向滚动，并把激活标签带回视野
- 多 Workspace 同时打开，状态并行
- 自动刷新：按可配置间隔刷新全部仓库的状态（一次刷新工作区里的每个仓库，默认 5 分钟，间隔可在设置里配置）
- Workspace-scoped 配置：AI provider / prompt 模板 / commit 规范 / 主题 / key binding profile（**schema 有字段，快捷键配置 UI 未交付**）
- Per-repo 覆盖 Workspace 配置（**repos.settings_override 列存在，无 UI**）

**用户旅程**：

- 首次启动：命名 Workspace（空状态里直接给「新建工作区」按钮，不必先找到应用菜单）+ 添加 repo（克隆 / 初始化 / 添加本地仓库，空状态给三个按钮，与应用菜单同源）
- 可选批量导入：用户指定扫描路径，自动加入 Git 仓库（便利功能，非必须）
- 日常：侧边栏显示所有 repo，点击切换 active
- 切换 Workspace：保存当前状态 + 加载 lastActiveRepo

**边界**：

- ❌ 不做并排双 repo 视图
- ❌ 不做 Workspace 嵌套 / 共享 / 同步
- 🔍 待评估：Workspace 模板、全局搜索、启动快捷键（导入 / 导出已交付）

### 1.5 AI 增强能力

**原则**：AI 是协作者而非替代者。所有 AI 输出可审、可改、可一键拒绝。绝不自动 commit / push / merge。AI 默认「建议而非执行」。

- AI commit message 生成：多 commit 累积理解
- AI conflict 解释：语义级，非 diff 罗列
- AI PR 描述
- AI command palette：Cmd+K / Ctrl+K 自然语言驱动（已随 v0.2 交付）
- AI history 解释
- AI repo health
- AI 误操作恢复

### 1.6 AI Provider 集成

**原则**：本地优先 + 隐私可控 + 可配置。diff 默认不离开本机；云端 provider 走 BYOK（API key 不上传至服务端）；提供 diff scrubber（扫描凭证 / secret / PII 后再发送）；完全离线模式一键禁用所有云端调用；AI provider / prompt 模板 / commit 规范 / 主题 / 快捷键均可自定义。

- BYOK：OpenAI / Anthropic / Gemini / DeepSeek / Qwen / Azure OpenAI / 自定义 endpoint
- 本地：Ollama 一键检测 + LM Studio 兼容 endpoint
- Prompt 模板管理（commit / conflict / PR / reflog / health；后两套随 v0.3 恢复与 health 接入）
- Per-repo AI rules（从 `.gitwave/` 目录读取）
- Provider 故障转移

### 1.7 Repo 添加 UX

- clone / init 进度 UI 可见
- 错误明确（区分网络 / 凭证 / 权限）
- 一键重试

### 1.8 SSH key 管理

- 一键 add / test / 删除
- GUI 列出已加载 key
- 测试与 GitHub / GitLab 连通性

### 1.9 Collaboration

Remote 管理（GitHub / GitLab / Gitea / 自建）— **已交付** · PR / MR 创建 · Lightweight code review · Issue 链接 — **未交付**（AI PR 描述只生成文本）

### 1.10 Platform & UX

**原则**：中文 + 英文一等公民；可配置（Theme + CSS 变量覆盖；快捷键全可配，支持 vim mode）。

- macOS 优先：Apple Silicon native
- Windows
- Linux
- Theme：light / dark / follow system + CSS 变量覆盖
- 快捷键全可配（支持 vim mode）— **未交付**
- 本地化：中文 / English 双语 — **已交付**
- 时间显示统一：各面板的提交时间与操作时间走同一套格式化规则，表现一致 — **已交付**

---

## 2. 不做清单

### 2.1 永远不做

- 自动 commit / push / merge
- 训练用户代码
- 收集用户代码至服务端
- 锁定 AI provider
- 任何形式的商业化（订阅 / 付费 tier / 团队版收费 / 永久授权）
- IDE 插件

### 2.2 近期不做

- VS Code / JetBrains 插件
- 完整 code review 工具
- CI/CD 编排
- Issue 管理
- 自托管 Git 服务
- 移动端