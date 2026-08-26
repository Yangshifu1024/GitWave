# feat-repo-ingestion

> Sprint 2：W2 添加 repo（四种方式）+ W3 移除 / 重新链接 + 10 SSH key 管理。
> 提案：`docs/pm/features/F002-repo-ingestion.md`。

## 状态

草案。

## 目标

1. libgit2 `init` / `clone` 走通四种 repo 添加方式
2. libgit2 `CredentialCallback` 桥接 `git credential helper`（HTTPS）+ ssh-agent（SSH）
3. SSH key add / test / delete + 列表（封装 `ssh-add` / `ssh -T`）
4. Workspace `repos` 表 + 状态字段（active / missing）+ 迁移 0002
5. Application use cases：init / clone_https / clone_ssh / add_local / remove / relink / ssh ops
6. Tauri typed commands 暴露 use case
7. 前端 RepoList + CloneRepoDialog + SshKeyManager UI
8. 单元测试 + E2E

## 范围

### In Scope

- libgit2 adapter：`init` / `open_local` / `clone`（HTTPS via helper / SSH via ssh-agent）
- Credentials bridge：libgit2 `CredentialCallback` 实现，按协议分支
- SSH module：`ssh-add -l` / `ssh-add` / `ssh-add -d` / `ssh -T` 子进程封装
- 持久化：migration 0002 给 `repos` 表加 `status` + `missing_at` 列
- Workspace repos CRUD + 状态切换
- 前端：RepoList（侧栏）、CloneRepoDialog（URL 自动识别协议）、SshKeyManager（key 列表 + add / test / delete）
- 错误反馈：clone 失败按 Network / Credential / Permission / Protocol 分类展示

### Out of Scope

- Worktree UI（W8）—— Sprint 5
- Submodule init —— 后续
- HTTPS 个人凭证本地缓存 —— 明确不做（decisions/0003）
- SSH key 生成 —— 用户自管

## 依赖 / 前置

- Sprint 1 已合 main：Workspace CRUD、SQLite 持久化、typed IPC
- 系统依赖：macOS / Linux 自带 `ssh-add` / `ssh`；Windows 需要 Git for Windows 的 OpenSSH
- libgit2 + ssh-agent 交互：libgit2 用 `GIT_SSH_COMMAND` / 走系统 ssh 客户端

## 步骤（按顺序）

### 1. Domain 扩展

- `domain/workspace.rs`：`RepoRef` 加 `status: RepoStatus`（`Active` / `Missing { since: i64 }`）+ `RepoSummary` 投影
- `domain/mod.rs` re-export

### 2. libgit2 adapter 扩展

- `infrastructure/git/repo_adapter.rs`：
  - `init(path: &Path) -> Result<()>`（`RepositoryInitOptions::new().bare(false).initial_head(HEAD)`，**不自动 commit**）
  - `open_local(path: &Path) -> Result<Repository>`（已有 move 自 `git2_adapter`）
  - `clone_https(url, dest, credential_callback) -> Result<()>`
  - `clone_ssh(url, dest, ssh_key_callback) -> Result<()>`
- 单元测试：init（验证 `.git/` 存在 + HEAD = refs/heads/main 但无 commit）+ open + non-repo 报错

### 3. Credentials bridge

- `infrastructure/git/credentials.rs`：
  - `GitCredentialHelper`：调用 `git credential fill` 子进程，按需 store（用户提示一次后）
  - `SshAgentCredential`：返回 ssh-agent 公钥路径；libgit2 自动走
  - 把 callback 注册到 `git2::FetchOptions`
- 单元测试：helper 拿到期望的 user/pass；无凭证时不阻塞

### 4. SSH module

- `infrastructure/ssh/keys.rs`：
  - `list_loaded() -> Result<Vec<SshKey>>`：parse `ssh-add -l` 输出
  - `add(path: &Path, passphrase: Option<&str>) -> Result<()>`
  - `delete(path: &Path) -> Result<()>`
  - `test_connection(host: &str, user: &str) -> Result<TestResult>`：`ssh -T <user>@<host>`，parse stdout/stderr 判定
- 单元测试：`ssh-add -l` 输出 parser + `ssh -T` 判定

### 5. Migration 0002

- `migrations/0002-repos-status.sql`：
  - `ALTER TABLE repos ADD COLUMN status TEXT NOT NULL DEFAULT 'active'`
  - `ALTER TABLE repos ADD COLUMN missing_at INTEGER`
- migrations.rs 注册新条目

### 6. WorkspaceRepository 扩展

- `infrastructure/persistence/workspace_repo.rs`：
  - `add_repo(workspace_id, repo_ref)` / `remove_repo(workspace_id, repo_id)`
  - `list_repos(workspace_id) -> Vec<RepoRef>`
  - `mark_repo_missing(workspace_id, repo_id)` / `relink_repo(workspace_id, repo_id, new_path)`
  - 启动时 sweep：每个 active workspace 的每个 repo 验证路径；若不存在 → mark_missing
- 单元测试覆盖

### 7. Application use cases

- `application/use_cases.rs`：
  - `init_repo(ctx, workspace_id, path)` / `clone_repo_https(ctx, workspace_id, url, dest_path)`
  - `clone_repo_ssh(ctx, workspace_id, url, dest_path)`
  - `add_local_repo(ctx, workspace_id, path)`
  - `remove_repo(ctx, workspace_id, repo_id)` / `relink_repo(ctx, workspace_id, repo_id, new_path)`
  - `list_repos(ctx, workspace_id) -> Vec<RepoSummary>`
  - SSH：`list_ssh_keys()` / `add_ssh_key(path, passphrase)` / `delete_ssh_key(path)` / `test_ssh_connection(host, user)`
- 单元测试覆盖

### 8. Tauri commands

- `lib.rs` 新增 9 个 typed commands
- `cmd_clone_repo` 合并 https/ssh：根据 URL 协议路由

### 9. 前端

- `src/lib/api.ts` 新增 typed wrappers
- `src/components/RepoList.tsx`：在 active workspace 视图下显示 repos；missing 状态 badge；relink 操作
- `src/components/CloneRepoDialog.tsx`：URL 输入 + 自动识别 `git@github.com:` (SSH) vs `https://` (HTTPS) vs `git://`
- `src/components/SshKeyManager.tsx`：列表 + add 按钮（文件 picker） + test 按钮 + delete 按钮
- `src/App.tsx` 接入

### 10. 启动 sweep

- `run()` 启动时扫描所有 workspaces + repos，mark missing
- 启动完成后日志记录 missing 数

### 11. 测试

- 单元测试：libgit2 init/open/clone（本地 file:// fixture）；credentials parser；ssh parser；use cases；repo persistence
- E2E（Playwright + tauri-driver）：创建 WS → init 本地仓 → 列表显示 → 删除 → 验证本地仓仍在

### 12. CI 验证

- cargo / npm 测试全过
- CI workflow 新增 ssh-add / ssh-keygen 系统包（如需）
- E2E 在 CI 上跑

## 分支

推荐：`feature/repo-ingestion`

## 验证（DoD）

1. CI 三 job 全绿（含 E2E）
2. 单元测试：libgit2 + credentials + ssh + repo persistence + use cases 全过
3. E2E happy path：创建 WS → init 本地仓 → 列表显示 → 删除 → 验证本地仓仍在
4. `pnpm tauri dev` 手动验证四种添加方式 + SSH key add/test/delete + missing 检测
5. pre-commit 全过
6. 真实 HTTPS 公开仓 clone 成功（CI 上用 `https://github.com/octocat/Hello-World` 之类的 fixture）

## 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| libgit2 CredentialCallback 与 git helper 协议差异 | HTTPS clone 体验差 | Sprint 2 先实现 basic；v0v2 评估 git2_credentials crate |
| ssh-agent 在 macOS / Linux / Windows 行为不一致 | SSH clone 失败 | 三个平台各跑一次 E2E；失败给清晰提示 |
| `ssh-add` passphrase 输入需要 TTY | UI 卡顿 | 用 `SSH_ASKPASS` + helper 脚本；或限制为无 passphrase key（v0v1 接受） |
| 用户本地仓路径变更 | 启动 sweep 大量 missing | sweep 仅后台异步执行，不阻塞启动 |
| libgit2 `clone` hooks 不自动跑（设计） | 用户预期差异 | UI clone 完成时明示"未触发 hooks" |
| clone 大仓库阻塞主线程 | UI 卡 | 后续 sprint 评估 tokio::task::spawn_blocking |

## 关联

- `docs/pm/features/F002-repo-ingestion.md`：PM 提案
- `docs/pm/core/04-sprint-v0.1.md` Sprint 2
- `docs/tech/architecture/00-overview.md`：分层 + IPC 边界
- `docs/tech/decisions/0003`：凭证策略（Keychain + helper + ssh-agent）
- `docs/tech/tech-selection/00-overview.md`：libgit2 全量 + 钩子不自动跑
- `docs/tasks/feat-workspace-crud/plan.md`：Sprint 1 的 Workspace CRUD
- `AGENTS.md` P1：永不自动 commit