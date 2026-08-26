# GitWave · Working Copy Bar 详细规格

> Sprint 4 的实施依据：覆盖 working copy 状态、文件 stage/unstage、commit message、push/pull/fetch。

## 1. 目标

为 GitWave 用户补齐"改 → commit → push"完整工作流：

- 查看 working copy 当前状态（modified / staged / untracked）
- Stage / unstage 文件
- 写 commit message（含 AI 生成）
- Commit
- Push / Pull / Fetch

## 2. 数据模型（Sprint 4 后端）

```rust
// src-tauri/src/domain/working_copy.rs

/// 文件状态枚举（与 `git status --porcelain` 一致）
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum FileStatusKind {
    Modified,    // M
    Added,       // A
    Deleted,     // D
    Untracked,   // ?
    Renamed,     // R
    Copied,      // C
}

/// 单个文件变更
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FileChange {
    pub path: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub old_path: Option<String>,  // for renamed
    pub kind: FileStatusKind,
    pub staged: bool,
    pub additions: u32,
    pub deletions: u32,
}

/// 当前 working copy 状态
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkingCopy {
    pub repo_id: String,
    pub branch: String,             // current branch or "(detached)"
    pub upstream: Option<String>,    // upstream branch name
    pub sha: String,                 // current HEAD sha
    pub ahead: u32,                  // ahead of upstream
    pub behind: u32,                 // behind upstream
    pub files: Vec<FileChange>,
}
```

## 3. 状态机

```
[Clean] ── file changed ──→ [Dirty]
[Dirty] ── all files committed ──→ [Clean]
[Dirty] ── git reset --hard / stash pop ──→ [Clean]
[Clean] ── detached HEAD checkout ──→ [Clean (detached)]
```

UI 上：

- **Clean 高度**：32px（compact）
- **Dirty 高度**：80-280px（用户可拖拽）
- 切换：smooth animation 200ms

## 4. Sprint 4 use cases（后端）

```rust
// application/use_cases.rs（新增）

/// 读取 active repo 的 working copy 状态
pub fn get_working_copy(ctx: &AppContext, repo_id: String) -> Result<WorkingCopy>;

/// Stage 文件路径列表（幂等）
pub fn stage_files(ctx: &AppContext, repo_id: String, paths: Vec<String>) -> Result<()>;

/// Unstage 文件路径列表（幂等）
pub fn unstage_files(ctx: &AppContext, repo_id: String, paths: Vec<String>) -> Result<()>;

/// Stage 所有 untracked / modified
pub fn stage_all(ctx: &AppContext, repo_id: String) -> Result<()>;

/// Commit 当前 staged；返回新 commit SHA
pub fn commit(ctx: &AppContext, repo_id: String, message: String) -> Result<String>;

/// Amend HEAD（如果用户已显式确认）
pub fn amend_commit(ctx: &AppContext, repo_id: String, message: String) -> Result<String>;

/// Fetch 远端 refs（不动 working tree）
pub fn fetch(ctx: &AppContext, repo_id: String) -> Result<()>;

/// Pull = fetch + merge（默认 fast-forward only）
pub fn pull(ctx: &AppContext, repo_id: String) -> Result<()>;

/// Push 当前 branch 到 upstream
pub fn push(ctx: &AppContext, repo_id: String) -> Result<()>;
```

### 4.1 凭证复用

- **HTTPS** → 已配置 `git credential helper`（see decisions/0003）
- **SSH** → ssh-agent 中已加载 key
- **AI commit message** → BYOK key 从 OS Keychain 取

## 5. Sprint 4 typed commands（前端 IPC）

```rust
#[tauri::command]
fn cmd_get_working_copy(repo_id: String) -> Result<WorkingCopy, AppError>;

#[tauri::command]
fn cmd_stage_files(repo_id: String, paths: Vec<String>) -> Result<(), AppError>;

#[tauri::command]
fn cmd_unstage_files(repo_id: String, paths: Vec<String>) -> Result<(), AppError>;

#[tauri::command]
fn cmd_stage_all(repo_id: String) -> Result<(), AppError>;

#[tauri::command]
fn cmd_commit(repo_id: String, message: String) -> Result<String, AppError>;

#[tauri::command]
fn cmd_amend_commit(repo_id: String, message: String) -> Result<String, AppError>;

#[tauri::command]
fn cmd_fetch(repo_id: String) -> Result<(), AppError>;

#[tauri::command]
fn cmd_pull(repo_id: String) -> Result<(), AppError>;

#[tauri::command]
fn cmd_push(repo_id: String) -> Result<(), AppError>;
```

## 6. 前端实现

### 6.1 文件结构

```
src/
  components/
    WorkingCopyBar.tsx       # 复合组件
    BranchIndicator.tsx      # primitive
    FileListItem.tsx          # primitive
    StatusIcon.tsx            # primitive
    CommitMessageBox.tsx      # primitive
    SyncButtons.tsx           # primitive
  stores/
    workingCopyStore.ts       # Zustand: 当前 WC 状态 + height
  hooks/
    useWorkingCopy.ts         # TanStack Query wrapper
```

### 6.2 轮询 vs fsnotify

| 方案 | 优势 | 劣势 |
|---|---|---|
| `fsnotify` 实时通知 | 文件改动立即反映 | 跨平台复杂度；macOS FSEvents / Linux inotify / Win ReadDirectoryChangesW |
| Polling 2s | 简单、跨平台 | 改动反映有 2s 延迟 |

**v0.1 选 Polling**。Sprint 4 实装时 polling interval 默认 2s，可在 Preferences 调。

### 6.3 TanStack Query 配置

```ts
useQuery({
  queryKey: ["working-copy", activeRepoId],
  queryFn: () => getWorkingCopy(activeRepoId),
  refetchInterval: 2000,
  enabled: !!activeRepoId,
});
```

mutation（stage / commit / push）成功后 `invalidateQueries(["working-copy", repoId])` 触发刷新。

### 6.4 Toast 反馈

| 操作 | Toast 内容 |
|---|---|
| Stage 5 files | "5 files staged" (info) |
| Unstage 2 files | "2 files unstaged" (info) |
| Commit 成功 | "Committed abc1234" + 点击跳转 commit (success) |
| Commit 失败 | error 详情 (danger) |
| Fetch 成功 | "Fetched 5 commits from origin" (info) |
| Pull 成功 | "Pulled 3 commits, no conflicts" (success) |
| Pull 失败 | "Pull failed: conflict in src/api.ts" (danger) → 触发 3-way merge UI |
| Push 成功 | "Pushed 2 commits to origin/main" (success) |
| Push 失败 | "Push rejected: ..." (danger) |

## 7. 交互细节

### 7.1 文件点击

- 单击文件行 → 在 Main 显示该文件的 diff（替换当前 diff 或新 pane，看设计）
- 双击文件行 → 触发 stage / unstage toggle
- 右键文件行 → context menu：Stage / Unstage / Open in editor (v0.2)

### 7.2 拖拽

- Unstaged 文件 → 拖到 Staged 列 → stage
- Staged 文件 → 拖到 Unstaged 列 → unstage
- v0.1 简化：HTML5 drag-drop API 实现；v0.2 评估 react-dnd

### 7.3 Commit message 校验

- 空 message → Commit 按钮 disabled
- 仅空白 → 弹 confirm modal "Commit with empty message?"
- > 72 字符首行警告（不改色，只 marker）
- Conventional Commits 格式**不强制**（v0.1 接受自由格式；v0.2 可选 strict）

### 7.4 Amend

- 默认隐藏 Amend 按钮
- 触发条件：HEAD 存在 + 当前 branch 与 detached 都不是
- 行为：弹 confirm modal "Amend the most recent commit? This rewrites history."（明确提示 rebase 影响）
- Amend 模式时 Commit 按钮文案改为 "Amend commit"

### 7.5 Conflict 时

- Pull / merge 失败 → Working Copy Bar 显示 conflict 标记 + Toast 提示
- 点击 conflict 文件 → Sprint 6 的 `ConflictResolver` UI（3-way merge）
- Sprint 4 只展示 conflict 标记 + 文件列表高亮；解析放 Sprint 6

## 8. 完整快捷键

| 键 | 动作 |
|---|---|
| `⌘⇧F` | Fetch |
| `⌘⇧P` | Pull |
| `⌘⇧U` | Push |
| `⌥⇧C` | focus commit message box |
| `⌘Enter` | commit（commit message 框聚焦时） |
| `Space` | toggle stage 当前选中行 |
| `⌘S` | stage current file（Sprint 4 后期可加） |
| `⌥S` | unstage current file |

完整全局快捷键见 `03-layout.md` §8。

## 9. 状态 / 错误处理

| 场景 | UI 行为 |
|---|---|
| 无 active repo | Working Copy Bar 隐藏 |
| Active repo missing（路径无效） | Bar 显示 "Repo path invalid" + Relink 按钮 |
| Repo 是 bare | Bar 显示 "Bare repository — no working copy" |
| Detached HEAD | BranchIndicator 显示 `(detached @ sha)` + ahead/behind 不显示 |
| network 失败 | Toast danger + sync 按钮恢复 enabled |
| 凭证被拒（pull / push） | Toast danger + "Configure credentials" 按钮（弹凭证设置 modal，Sprint 4 末或 v0.2） |

## 10. Sprint 4 实施 checklist

- [ ] domain/working_copy.rs：FileChange / WorkingCopy / FileStatusKind
- [ ] infrastructure/git/working_copy.rs：基于 libgit2 的 status 实现
- [ ] use cases：get_working_copy / stage_files / unstage_files / stage_all / commit / amend_commit / fetch / pull / push
- [ ] typed commands × 9
- [ ] 前端 api wrappers
- [ ] 6 个 primitive（BranchIndicator / FileListItem / StatusIcon / CommitMessageBox / SyncButtons / WorkingCopyBar）
- [ ] App.tsx 接入（bar 在底部、sync 在 topbar）
- [ ] Polling (2s) 或 fsnotify
- [ ] Toast 反馈
- [ ] 单元测试：libgit2 status / commit / push mocks
- [ ] E2E happy path：本地 init → 改文件 → stage → commit → push (mock remote)
- [ ] 手动验证：真实 push 到测试 repo

## 11. 关联

- `00-overview.md`：决策背景
- `02-components.md`：Working Copy 相关 primitive API
- `03-layout.md`：bar 在布局中的位置
- `docs/tasks/feat-workspace-crud/plan.md`：Sprint 1 use cases 模式参考
- `docs/tech/decisions/0004-ai-dual-track-boundary.md`：commit 写操作由确定性引擎执行（AI 仅生成 message）
- `docs/pm/core/02-scope.md` §1.1：commit / push / pull 是 v0.1 must-have
- `docs/pm/core/04-sprint-v0.1.md` Sprint 4：本次实施范围