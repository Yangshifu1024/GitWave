# GitWave · v0.1 Sprint Plan

> 把 v0.1 的 14 个 must-have 按依赖顺序排成 sprint。本文档是 `02-scope.md` v0.1 范围内的执行层分解，不改变产品范围。

## 状态

草案（脚手架 Sprint 0 已落 `docs/tasks/feat-bootstrap-tau-app/plan.md`）。

## 目标

- 把 14 个 must-have 拆成可独立交付的 sprint
- 每个 sprint 有明确退出标准
- 暴露依赖图，让并行工作可见
- 风险前置

## Sprint 总览

| Sprint | 主题 | Must-haves | 退出标准 |
|---|---|---|---|
| **0** | 脚手架 | （工程任务：feat-bootstrap-tau-app） | `pnpm tauri dev` 起 UI，工具链 + CI 就绪 |
| **1** | Workspace 基础 | W1, W4（partial） | 创建 / 重命名 / 删除 Workspace；单 Workspace 内 active repo 切换；重启状态恢复 |
| **2** | Repo 收录 + SSH | W2, W3, 10 | 全 W2（init / HTTPS / SSH / 本地）+ W3 移除 / 重链接 + SSH key add/test/delete |
| **3** | Git 读 + history | 5（读子集）, 7 | 可视化 history 图 / 文件 diff / blame；branch / merge / 普通 rebase |
| **4** | Git 写 + AI commit | 5（写子集）, 11, 12 | 端到端 commit → push；BYOK + Ollama AI commit message；AI provider UI |
| **5** | Worktree + stash + interactive rebase | 8, 9, 6 | 高级 git 工作流可用 |
| **6** | Conflict + command palette | 13, 14 | v0.1 完成定义达成：3 核心场景端到端 + AI 行为符合 P1 |

## 依赖图

```
Sprint 0（脚手架）
  └─> Sprint 1（W1, W4 partial）
        └─> Sprint 2（W2, W3, 10）
              └─> Sprint 3（5 读, 7）
                    ├─> Sprint 4（5 写, 11, 12）
                    └─> Sprint 5（8, 9, 6）
                          └─> Sprint 6（13, 14）
```

## Sprint 详细

### Sprint 0：脚手架

参考 `docs/tasks/feat-bootstrap-tau-app/plan.md`。

- Tauri 2 + Vite + React + TS 初始化
- 工具链与工程门禁就位
- CI 矩阵：lint / test / build 三 job
- 不交付任何用户可见功能

### Sprint 1：Workspace 基础

**范围**：W1 Workspace CRUD + W4 单 Workspace 内 active repo 切换

**子任务**：

- SQLite schema：`workspaces` / `repos` 表 + migration
- libgit2 接入：`open_local_repo(path)` → `RepoView`
- tauri-specta typed command：`create_workspace` / `rename_workspace` / `delete_workspace` / `list_repos` / `set_active_repo`
- React 侧 Workspace switcher UI
- W4：active repo 切换事件 + UI 重新渲染 + state snapshot

**退出标准**：

- 创建 Workspace 后 SQLite 有对应行
- 重启应用后状态恢复
- 添加本地仓库后可切换 active
- 单元测试覆盖 SQLite 适配 + libgit2 适配
- E2E：创建 Workspace → 添加本地仓 → 切换 active

### Sprint 2：Repo 收录 + SSH

**范围**：W2 init / clone HTTPS / clone SSH / 本地 + W3 移除 / 重链接 + 10 SSH key 管理

**子任务**：

- `init` 用例：`RepositoryInitOptions`，**不自动 commit**
- `clone_https`：`git credential helper` 桥接（libgit2 `CredentialCallback`）
- `clone_ssh`：前置依赖 SSH key 已加载（10）
- W3 remove / relink：confirm 对话框 + missing repo 检测
- SSH key 管理 UI：list 加载的 key、`ssh-add` 测试、`ssh -T git@github.com` 连通性测试
- clone 进度 UI（W2）

**退出标准**：

- init 不自动 commit
- clone HTTPS 走系统 helper，不本地存凭证
- clone SSH 走 ssh-agent
- SSH key add / test / delete 完整闭环
- W3：缺失 repo 自动检测 + 重链接

### Sprint 3：Git 读 + history

**范围**：5 读子集（branch / merge / 普通 rebase）+ 7 history 图 / 文件 diff / blame

**子任务**：

- 分支 CRUD（libgit2）
- merge：libgit2 index 操作；冲突标记
- rebase：libgit2 rebase API（非 interactive）
- history DAG 渲染：virtual scroll + 分片
- 文件 diff viewer：split + unified，syntax highlight
- blame view

**退出标准**：

- 大仓（10k commit）history 流畅（性能预算见 `engineering/00-overview.md`）
- 单文件 diff < 200ms（1MB 文件基线）
- merge 后冲突标记正确
- 单元 + 集成测试覆盖关键算法

### Sprint 4：Git 写 + AI commit

**范围**：5 写子集（push / pull）+ 11 AI commit + 12 AI provider UI

**子任务**：

- push / pull（libgit2）
- AI commit message 用例：多 commit 累积理解 + provider 调用
- 自研 `Provider` trait（OpenAI / Anthropic / Ollama，OpenAI 兼容协议）
- BYOK Keychain 存储（macOS Keychain / Win Credential Manager / Linux Secret Service）
- AI provider 配置 UI（Workspace-scoped）
- diff scrubber（regex + token 扫描）
- 离线模式 toggle

**退出标准**：

- commit → push 端到端
- AI message 可编辑 + 多 commit 累积理解
- BYOK / Ollama 三 provider 都可用
- API key 仅存于 Keychain
- scrubber 在 diff 发送前跑
- 离线模式禁用云端

### Sprint 5：Worktree + stash + interactive rebase

**范围**：8 + 9 + 6

**子任务**：

- worktree UI（libgit2 worktree API）
- stash 全套（save / apply / drop / diff）
- interactive rebase 拖拽：UI 层驱动（pick / reword / edit / squash / fixup / drop），不依赖 `git rebase -i`
- 拖拽交互的 keyboard + mouse 支持

**退出标准**：

- worktree 创建 / 切换
- stash 完整流程
- interactive rebase 拖拽操作正确生成 todo list 并执行
- 单元测试覆盖 rebase todo → libgit2 rebase sequence 映射

### Sprint 6：Conflict + command palette

**范围**：13 3-way merge + AI conflict explain + 14 AI command palette（MVP）

**子任务**：

- 3-way merge 面板（libgit2 merge + 冲突文件三栏视图）
- AI conflict explain：UI 层拼装冲突 + 提示词 + AI 调用
- Cmd+K MVP：自然语言 → 命令解析（**may-slip** 标记）
- v0.1 完成定义三场景全跑通

**退出标准**：

- conflict 解决端到端
- AI 解释面板明确"仅供参考，确定性引擎执行"
- Cmd+K 可执行常见命令（add / commit / branch / switch 等基础集）
- v0.1 完成定义三条全部达成：
  1. 14 must-have 全部可用
  2. 三核心场景（commit → push、conflict 解决、workspace 切换）端到端
  3. AI 行为符合 P1（永不自动 commit / push / merge）

## 风险与依赖

| 风险 | 影响 | 缓解 |
|---|---|---|
| WebView 三端差异 | Sprint 0/1 CI 不全绿 | v0.1 CI 跑 macOS + Linux，Windows runner v0.2 加 |
| libgit2 与系统 git 行为偏差 | 5 / 8 / 9 边界 | Sprint 3 起跑"真实 git CLI 对比测试"作为参考实现 |
| AI provider 协议变更 | Sprint 4 维护成本 | Provider trait 抽象 + 协议适配层独立 |
| tauri-specta 不稳 | Sprint 1 起 typed IPC | 准备手工 typed wrapper 降级方案 |
| v0.2 Windows 适配 | Sprint 0 后 CI 加 Windows runner | Sprint 0 后追加 runner |
| large repo 性能不达标 | Sprint 3 history / diff 卡 | 引入 virtual scroll + worker thread；性能预算见 engineering |
| Sprint 6 command palette 延期 | v0.1 完成定义不全 | 14 标注 may-slip，允许 v0.1.1 补丁再合 |

## 关联

- `01-features.md`：功能定义源头
- `02-scope.md` §1.1：14 个 must-have 列表
- `03-roadmap.md` v0.1：版本完成定义
- `docs/tasks/feat-bootstrap-tau-app/plan.md`：Sprint 0 的执行细节
- `docs/tech/engineering/00-overview.md`：工程门禁与性能预算
- `docs/tech/decisions/0004`：AI 双轨边界（影响 Sprint 4 / 6）