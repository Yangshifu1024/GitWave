# feat-workspace-crud

> Sprint 1：W1 Workspace CRUD + W4 partial（单 Workspace 内 active repo 切换）。
> 提案：`docs/pm/features/F001-workspace-crud.md`。

## 状态

草案。

## 目标

1. SQLite schema：`workspaces` / `repos` 表 + 嵌入式 migration
2. `SqliteWorkspaceRepo` 实现 Workspace 持久化（CRUD + active workspace + active repo）
3. Application use cases：Create / List / Rename / Delete / SwitchActiveWorkspace / SwitchActiveRepo
4. Tauri typed commands 暴露 use case
5. 前端 WorkspaceSwitcher 组件 + Zustand store + 对话框
6. 单元测试覆盖持久化与 use case；E2E 覆盖 happy path

## 范围

### In Scope

- SQLite 表：`workspaces(id, name, settings_json, last_active_repo_id, created_at, updated_at)`、`repos(id, workspace_id, path, nickname, settings_override_json)`
- Migration runner：嵌入式 SQL（`include_str!`）+ 顺序应用 + `schema_version` 跟踪
- `WorkspaceRepository` trait + `SqliteWorkspaceRepo` 实现
- Application use cases（不依赖 Tauri，便于单测）
- Tauri typed commands：
  - `list_workspaces() -> Vec<WorkspaceSummary>`（替换现有 stub）
  - `create_workspace(name: String) -> Workspace`
  - `rename_workspace(id: String, new_name: String) -> ()`
  - `delete_workspace(id: String) -> ()`
  - `set_active_workspace(id: String) -> ()`
  - `set_active_repo(workspace_id: String, repo_id: String) -> ()`
  - `list_repos(workspace_id: String) -> Vec<RepoRef>`
- 前端：Zustand store + `WorkspaceSwitcher` + dialogs + React Router 单 Workspace 内视图

### Out of Scope

- 多 Workspace 同时打开（完整 W4）
- 添加 / 移除 repo（W2 / W3）—— 下个 sprint
- Workspace-scoped AI 配置（11 / 12）—— Sprint 4
- Workspace 导入 / 导出（S6）—— v0.2

## 依赖 / 前置

- Sprint 0 已合 main：Tauri scaffold、DDD 骨架、SQLite + libgit2 adapter、tracing init、CI

## 步骤（按顺序）

### 1. SQLite schema migrations

- 新建 `src-tauri/migrations/0001-workspaces-repos.sql`：
  - `workspaces` 表
  - `repos` 表 + 外键到 workspaces
  - `schema_version` 表跟踪
- `infrastructure/persistence/migrations.rs`：嵌入式 SQL + 顺序应用
- `sqlite.rs::open()` 接入 migration runner

### 2. Repository trait + SQLite 实现

- `infrastructure/persistence/workspace_repo.rs`：
  - `WorkspaceRepository` trait（async-friendly，但 Sprint 1 用同步 impl）
  - `SqliteWorkspaceRepo` 实现
- CRUD + 单元测试（in-memory SQLite）

### 3. Domain 增强

- `domain/workspace.rs`：加 `WorkspaceSummary`（列表展示用，不含完整 settings）
- `domain/repo.rs` 新文件：`RepoRef`（已在 workspace.rs 里，复用）

### 4. Application use cases

- `application/use_cases.rs`（新文件）或 `application/workspace/`：
  - `CreateWorkspace { name } -> Workspace`
  - `ListWorkspaces -> Vec<WorkspaceSummary>`
  - `RenameWorkspace { id, new_name }`
  - `DeleteWorkspace { id }`
  - `SwitchActiveWorkspace { id }`
  - `SwitchActiveRepo { workspace_id, repo_id }`
- `application::AppContext` 持有 `Arc<SqliteWorkspaceRepo>`
- 单元测试覆盖

### 5. Tauri commands

- `lib.rs` 用 `tauri::State<AppContext>` 注入 use cases
- typed commands 暴露 use case
- 替换现有 `list_workspaces` stub 为真实实现

### 6. 前端状态 + UI

- `src/stores/workspaceStore.ts`：Zustand store（activeWorkspaceId / workspaces / repos）
- `src/components/WorkspaceSwitcher.tsx`：侧边栏顶部下拉
- `src/components/CreateWorkspaceDialog.tsx`
- `src/components/RenameWorkspaceDialog.tsx`
- `src/components/DeleteWorkspaceConfirm.tsx`
- `src/App.tsx` 接入 switcher，调用真实 list / create / rename / delete commands

### 7. E2E（Playwright + tauri-driver）

- happy path：创建 → 重命名 → 切 active → 删除
- 状态恢复：创建 Workspace → 关闭 app → 重启 → 验证列表保留

### 8. CI 验证

- 跑 `cargo test --all-targets`、`cargo clippy`、`npm run lint`、`npm run typecheck`
- Playwright E2E（在 CI 上 macOS runner 跑）
- 更新现有 `pre-commit` / CI 矩阵

## 分支

推荐：`feature/workspace-crud`

## 验证（DoD）

1. CI 三 job 全绿
2. `cargo test --all-targets` 全过（含新增 repo / use case 测试）
3. `vitest run` + `playwright test` 全过
4. 端到端：创建 Workspace、重启应用、状态恢复、删除 Workspace、切换 active repo 全跑通
5. pre-commit 全过
6. UI 显示 WorkspaceSwitcher，可创建 / 重命名 / 删除 / 切换
7. SQLite 文件真实落盘；迁移 schema_version 正确递增

## 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| 同步 SQLite 在 tokio runtime 阻塞 | UI 卡 | 短期可接受（Sprint 1 use case 体量小）；v0.2 评估 rusqlite 在 spawn_blocking 包裹 |
| Workspace 名唯一性约束 | 用户期待"允许同名" | 不强制 unique，name 仅展示用，id 唯一 |
| 删除 Workspace 时 active repo 状态 | 删除后 UI 切哪？ | 切到剩余最近更新的 Workspace；若无则 active 置空 |
| React Query 与 Zustand 边界 | 缓存 vs 全局态 | use cases 结果走 TanStack Query；UI 状态走 Zustand；query invalidation 用 mutation 后 refetch |
| E2E tauri-driver 在 CI 上稳定性 | 偶发超时 | macOS runner + 单线程跑 + 重试 |

## 关联

- `docs/pm/features/F001-workspace-crud.md`：PM 提案
- `docs/pm/core/04-sprint-v0.1.md` Sprint 1
- `docs/tech/architecture/00-overview.md`：分层与 IPC 边界
- `docs/tech/tech-selection/00-overview.md`：SQLite + WAL 配置
- `docs/tech/decisions/0002`：Workspace 无 FS 实体
- `docs/tasks/feat-bootstrap-tau-app/plan.md`：Sprint 0 的脚手架