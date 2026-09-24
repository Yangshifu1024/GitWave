# GitWave · 版本路线

> 本文档定义 GitWave 的版本发布计划与迭代节奏。项目采用敏捷快速迭代，无固定阶段。
> 当前发布版本：**v0.9.4**（三平台）。§2.1–§2.3 是已交付的规划基线，不是待办。

## 1. 版本总览

| 版本 | 范围 | 平台 | 状态 |
|---|---|---|---|
| **v0.1** | 14 个核心场景跑通 | macOS 起步 | **已交付** |
| **v0.2** | v0.1 全量 + Windows + AI 进阶 | macOS + Win | **已交付** |
| **v0.3** | v0.2 全量 + Linux + remote 协作 + AI 智能 | macOS + Win + Linux | **已交付**（PR/MR / Issue 未做，见 §2.3） |
| **v0.4–v0.9** | 官网、自动更新、i18n、代理、凭证弹窗、图片 diff 等持续迭代 | 三平台 | **进行中**（当前 v0.9.4） |

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

### 2.4 v0.4–v0.9（持续迭代）

v0.3 之后不再按「大版本一次性范围」排期，按功能提案合入、tag 发版。已合入产品的提案见 `docs/pm/features/README.md`。按版本号大致对应：

| 版本带 | 已合入（代表性） |
|---|---|
| v0.4.x | 官网（GitHub Pages + gitwave.work） |
| v0.5.x | 应用内自动更新（macOS / Windows / AppImage；deb/rpm 引导到 Releases） |
| v0.6.x | UI 中英双语 + AI 回复语言 |
| v0.7.x | macOS 原生菜单、提交/分支右键菜单、远程分支 DWIM 检出、系统代理、应用内 HTTPS 凭证弹窗、外部工具 / 编辑器打开、图片 diff |
| v0.8.x | 标题栏与操作栏合并、stash 按仓库隔离、未暂存图片 diff 读工作区、macOS dmg 公证票据、检出后侧栏揭示分支、自动刷新全部仓库 |
| v0.9.x | 一次添加多个本地仓库、仓库标签栏滚轮横滚、stash 面板重做、提交列表滚动性能、时间格式统一、孤立 index 冲突可见与 pull 前置检查 |

**相对规划仍未做、且仍在产品意图里的项**（不是本路线图的承诺日期）：

- GitHub / GitLab PR/MR 创建与 lightweight review、Issue 链接
- history 侧 file tree
- 可配置快捷键 / vim mode、per-repo settings 覆盖 UI
- Workspace 模板、跨 Workspace 全局搜索

---

## 3. 决策节奏

持续事件驱动，不固定时间周期：

- 持续：发版后更新本表状态与 `docs/pm/features/` 提案状态
- 持续：审视不做清单与版本节奏
- 重大方向调整时：重新审视平台优先级、新功能纳入/排除
- 版本整体回看：重设下一版本
