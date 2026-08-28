# GitWave · 版本路线

> 本文档定义 GitWave 的版本发布计划与迭代节奏。项目采用敏捷快速迭代，无固定阶段。

## 1. 版本总览

| 版本 | 范围 | 平台 | 状态 |
|---|---|---|---|
| **v0.1** | 14 个核心场景跑通 | macOS | 计划 |
| **v0.2** | v0.1 全量 + Windows + AI 进阶 + 公开发布 | macOS + Win | 计划 |
| **v0.3** | v0.2 全量 + 三平台 + 协作 + AI 智能 | macOS + Win + Linux | 计划 |

## 2. 版本详细

### 2.1 v0.1（macOS）

**范围**：

- Workspace 管理（第一入口）：CRUD、添加/移除 repo、单 active 切换、多 Workspace 同时开
- Git 核心：clone（HTTPS / SSH）/ init / commit / branch / merge / rebase / push / pull / fetch / history / diff / blame / interactive rebase 拖拽 / worktree / stash / submodule init
- SSH key 管理
- 3-way merge + AI conflict explain
- AI commit message（BYOK + Ollama）
- AI provider 配置 UI（OpenAI / Anthropic / Ollama 三选，Workspace-scoped）
- 仅 macOS

**完成定义**：

- 14 个 must-have 全部可用
- 三个核心场景（commit → push、conflict 解决、Workspace 切换）端到端可走通
- 内部 dogfooding 时间合理，主要场景无崩溃
- AI 行为符合 P1

### 2.2 v0.2（macOS + Win）

**新增范围**：

- Windows 适配
- Git LFS / 完整 submodule / reflog 浏览器 / hooks 编辑器
- AI PR 描述 / AI history 解释 / AI command palette（Cmd+K）
- 自定义 prompt template UI（commit / conflict / PR 三套）
- Per-repo AI rules（`.gitwave/` 目录）
- Provider 故障转移

**完成定义**：

- macOS + Windows 同时可用
- v0.1 全量功能在 Win 上跑通
- AI 进阶能力端到端可用
- 用户能从 v0.1 平滑升级到 v0.2

### 2.3 v0.3（三平台）

**新增范围**：

- AI repo health dashboard
- AI 误操作恢复（reflog 语义化 + 恢复建议）
- 协作（remote / PR / lightweight review）
- Linux 稳定版

**完成定义**：

- 三平台可用
- AI 智能能力端到端可用
- 协作能力走通

---

## 3. 决策节奏

持续事件驱动，不固定时间周期：

- 持续：sprint review 更新进度状态
- 持续：审视不做清单与版本节奏
- 重大方向调整时：重新审视平台优先级、新功能纳入/排除
- 版本整体回看：重设下一版本