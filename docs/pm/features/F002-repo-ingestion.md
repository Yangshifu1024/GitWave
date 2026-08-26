# F002 · Repo Ingestion + SSH Key Management

## 背景

Sprint 1（F001）实现了 Workspace CRUD，但 Workspace 是空壳——还没有任何 repo 被添加进去。v0.1 必须有 must-have W2（添加 repo 四种方式）、W3（移除 / 重新链接）、10（SSH key 管理），否则用户拿不到能用的 Git 客户端。

不做 W2 + W3 + 10：

- Workspace 永远没有仓库，history / diff / commit 等后续能力无用武之地
- 无法 clone SSH 仓库（10 是前置依赖）
- clone HTTPS 无法走系统 `git credential helper`，体验比 CLI 差

## 提议方案

### 范围（In Scope）

- **添加 repo 四种方式**：
  - **init**：在指定路径用 libgit2 `RepositoryInitOptions` 初始化本地仓；**不自动 commit**（P1：永不自动 commit）
  - **clone HTTPS**：libgit2 clone + `CredentialCallback` 桥接系统 `git credential helper`；不在本地存任何明文凭证
  - **clone SSH**：前置依赖 SSH key 已加载到 ssh-agent（10）；libgit2 用 key SSH 认证
  - **本地已有仓库**：用户选定路径，验证是合法 git 仓，添加为引用
- **移除 repo**：从 Workspace 移除 RepoRef；**不删除本地仓库**；confirm 对话框明示
- **重新链接 missing repo**：启动时检测 Workspace 中 RepoRef 的 path 是否还指向有效 git 仓；缺失标记为 `missing`，提供"重新链接"操作（重新指向新路径）
- **SSH key 管理**：
  - 列出 `~/.ssh` 已加载 key（`ssh-add -l`）
  - 添加：`ssh-add <key>`，可选 passphrase
  - 删除：`ssh-add -d <key>`（从 agent 删除，不删文件）
  - 测试连通性：`ssh -T git@github.com` / `ssh -T git@gitlab.com` 等
- **错误分类**：
  - clone 失败 → 区分网络 / 凭证 / 权限 / 协议
  - 非合法 git 仓 → Protocol
  - 凭证被拒 → Credential

### 范围外（Out of Scope）

- Worktree 创建 / 切换（W8）—— Sprint 5
- Submodule（W2 的子任务，但 v0.1 仅 W2 范围内的"添加"，submodule init 是后续）
- SSH key 生成（`ssh-keygen`）—— 用户自行用 `ssh-keygen` 生成，GitWave 只负责 add / test / delete
- HTTPS 个人凭证本地缓存 —— 明确不做（`docs/tech/decisions/0003`）

## 影响

- **涉及模块**：
  - `src-tauri/src/infrastructure/git/repo_adapter.rs` 新增：`init` / `open_local` / `clone_https` / `clone_ssh` / `head_summary`
  - `src-tauri/src/infrastructure/git/credentials.rs` 新增：`CredentialCallback` 桥接 `git credential helper`
  - `src-tauri/src/infrastructure/ssh/` 新增 `keys.rs`：包 `ssh-add` / `ssh -T` 调用 + `~/.ssh` 文件列举
  - `src-tauri/src/application/use_cases.rs` 扩展：新增 9 个 use case
  - `src-tauri/src/domain/workspace.rs` 扩展：`RepoRef` 加 `missing` 状态 + 加 `RepoSummary` 投影
  - `src-tauri/src/infrastructure/persistence/workspace_repo.rs` 扩展：repos 表 CRUD（add / remove / list / mark_missing / relink）
  - `src-tauri/migrations/0002-repos-and-status.sql`：补 repos 表当前缺的状态列 + missing_at 字段；migration runner 已支持
  - `src-tauri/src/lib.rs` 新增 typed commands：
    - `cmd_init_repo` / `cmd_clone_https` / `cmd_clone_ssh` / `cmd_add_local_repo`
    - `cmd_remove_repo` / `cmd_relink_repo`
    - `cmd_list_ssh_keys` / `cmd_add_ssh_key` / `cmd_delete_ssh_key` / `cmd_test_ssh_connection`
  - `src/components/RepoList.tsx` 新增
  - `src/components/CloneRepoDialog.tsx` 新增（含 URL / 协议自动识别）
  - `src/components/SshKeyManager.tsx` 新增
- **影响版本**：v0.1
- **是否破坏向后兼容**：否（Sprint 1 Workspace 不含 repos；新增 repos 后保持 Vec<RepoRef> 兼容）

## 验收

- 创建一个新 Workspace → 选本地路径 init → Workspace 出现新 RepoRef，无自动 commit
- 同一 Workspace 内 clone 一个 HTTPS 公共仓 → 凭证走 helper；Workspace 出现新 RepoRef
- 列出 SSH 已加载 key；add 一个新 key；test 连接 GitHub 返回成功信息
- 删除 repo → 确认对话框 → RepoRef 消失；本地仓保留
- 重启应用 → 所有 repos 状态恢复
- 把某 repo 的本地路径改名 / 删除 → 重启后该 repo 标记为 `missing`；用户可点击"重新链接"指定新路径
- 单元测试覆盖：libgit2 init / open / clone（HTTP mock 或 fixture）/ SSH credential / SSH key 列表 / Workspace repos CRUD
- E2E：创建 WS → init 本地仓 → 删除 repo → 验证本地仓仍在

## 决策

- **状态**：接受
- **决策人**：PM
- **决策日期**：2026-08-26
- **关联决策**：`docs/tech/decisions/0003`（凭证策略：BYOK Keychain + git helper / ssh-agent）
- **关联计划**：`docs/tasks/feat-repo-ingestion/plan.md`
- **关联 Sprint**：`docs/pm/core/04-sprint-v0.1.md` Sprint 2