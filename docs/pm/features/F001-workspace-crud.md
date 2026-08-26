# F001 · Workspace CRUD

## 背景

Workspace 是 GitWave 的第一入口（见 `AGENTS.md` 核心约束）。Workspace CRUD 是 v0.1 的 must-have（W1，§`docs/pm/core/02-scope.md` 1.1），也是后续所有 use case 的载体：未创建 Workspace 就无法添加 repo、无法启动 AI 配置、无法切 active repo。

不做 Workspace CRUD，用户拿不到一个能用的 Git 客户端最小形态。

## 提议方案

v0.1 内实现 Workspace 的完整 CRUD + 单 Workspace 内 active repo 切换（W1 + W4 子集）。多 Workspace 同时打开（完整 W4）放后续 sprint。

### 范围（In Scope）

- **Create Workspace**：命名 + 落库，UI 提供 dialog；新建后默认无 repo，lastActiveRepoId 为空
- **List Workspaces**：侧边栏列出，按 `updated_at` 倒序；显示当前 active Workspace 高亮
- **Rename Workspace**：双击 / context menu；UI 即时反馈
- **Delete Workspace**：confirm dialog（包含提示"将一并移除 active repo 引用，但不会删本地仓"）
- **Switch Active Workspace**：单 active 模型；切换时保存当前 UI state snapshot，加载目标 Workspace 的 lastActiveRepoId
- **Switch Active Repo（单 Workspace 内）**：侧边栏 repo 列表点击切换，触发 UI 重新渲染
- **数据模型**：见 `docs/pm/core/01-features.md` §1.4 `Workspace { id, name, repos: [RepoRef], settings, lastActiveRepoId?, createdAt, updatedAt }`，对应 SQLite 表
- **持久化**：SQLite（`docs/tech/tech-selection/00-overview.md` §本地存储），单文件 `state.db`，WAL 模式
- **凭据 / 网络**：本期无

### 范围外（Out of Scope）

- 多 Workspace 同时打开（完整 W4）—— v0.1 仅单 Workspace active，状态切换走 SQLite 读取 + UI 重建
- 添加 / 移除 repo（W2 / W3）—— 后续 sprint
- Workspace 导入 / 导出（S6）—— v0.2
- AI 配置（11 / 12）—— Sprint 4
- 工作区模板 / 嵌套 / 共享 / 同步—— 明确不做

## 影响

- **涉及模块**：
  - `src-tauri/src/infrastructure/persistence/` 新增 `workspaces` / `repos` SQLite 表 + `SqliteWorkspaceRepo`
  - `src-tauri/src/application/` 新增 `CreateWorkspace` / `ListWorkspaces` / `RenameWorkspace` / `DeleteWorkspace` / `SwitchActiveRepo` use case
  - `src-tauri/src/lib.rs` 新增 typed commands: `list_workspaces` (已有 stub) / `create_workspace` / `rename_workspace` / `delete_workspace` / `set_active_workspace` / `set_active_repo`
  - `src/` 新增 `WorkspaceSwitcher` 组件 + `useWorkspaces` Zustand store + 对话框组件
- **影响版本**：v0.1
- **是否破坏向后兼容**：否（净增量；现有 `list_workspaces` 返回 `Vec<Workspace>` 的契约保持，只是从"永远空"改为"实际返回"）

## 验收

- 创建 Workspace 后 SQLite `workspaces` 表有一行；重启应用后状态恢复
- 重命名 / 删除反应正确（list 即时刷新）
- 切换 active Workspace 后侧边栏高亮 + lastActiveRepoId 加载
- 单 Workspace 内切换 repo 触发 UI 重新渲染
- 单元测试覆盖 `SqliteWorkspaceRepo` 的所有方法 + use case 编排
- E2E（Playwright + tauri-driver）：创建 → 重命名 → 删除 happy path

## 决策

- **状态**：接受
- **决策人**：PM
- **决策日期**：2026-08-26
- **关联决策**：`docs/tech/decisions/0002`（Workspace 无 FS 实体）、`docs/tech/decisions/0004`（AI 双轨）
- **关联计划**：`docs/tasks/feat-workspace-crud/plan.md`
- **关联 Sprint**：`docs/pm/core/04-sprint-v0.1.md` Sprint 1