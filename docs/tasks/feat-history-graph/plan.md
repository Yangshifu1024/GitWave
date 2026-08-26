# feat-history-graph

> Sprint 3：history 图 + 文件 diff + blame + branch CRUD + merge + 普通 rebase。
> 提案：`docs/pm/features/F003-history-diff-blame.md`。

## 状态

草案。

## 目标

1. libgit2 `Revwalk` 实现 commit log + lane assignment 算法（10k commit 流畅）
2. libgit2 branch CRUD（list / create / delete / switch）+ merge + non-interactive rebase
3. libgit2 commit-vs-parent diff + word-level diff
4. libgit2 blame / annotate
5. Use cases + typed commands 暴露给前端
6. 前端 CommitGraph（virtual scroll）+ CommitDetails + DiffViewer（Shiki syntax highlight）+ BlameView + BranchList + MergeConflictView（Sprint 3 先列冲突文件）
7. 单元测试 + 手动验证（10k commit 仓流畅）

## 范围

### In Scope

- domain types：`CommitSummary` / `CommitDetails` / `FileDiff` / `BlameLine` / `BranchInfo`
- infrastructure/git/history.rs：`commit_log` + `lane_assignment`（homegrown 算法）
- infrastructure/git/branch.rs：`list_branches` + `create_branch` + `delete_branch` + `checkout_branch`
- infrastructure/git/merge.rs：`merge_branch`（fast-forward + three-way + 冲突检测）
- infrastructure/git/rebase.rs：`rebase_branch`（non-interactive；interactive 留 Sprint 5）
- infrastructure/git/diff.rs：`diff_commits` + `diff_working_tree`（word-level 通过 `git2::Patch` + 二次解析）
- infrastructure/git/blame.rs：`blame_file`
- application/use_cases.rs：12 个 use case
- lib.rs：12 个 typed command
- 前端：
  - `CommitGraph.tsx`（virtual scroll + lane DAG）
  - `CommitDetails.tsx`（metadata + file list）
  - `DiffViewer.tsx`（split / unified + Shiki + word diff）
  - `BlameView.tsx`（inline 注释 + hover）
  - `BranchList.tsx`（branch tree + 操作 menu）
  - `MergeConflictView.tsx`（仅列冲突文件；resolve 留 Sprint 6）
- 依赖：新增 `shiki` + `@tanstack/react-virtual`

### Out of Scope

- Interactive rebase 拖拽（Sprint 5）
- Stash / Tags / Worktrees UI（Sprint 5）
- Submodule diff（Sprint 5+）
- Cherry-pick（v0.2）
- AI conflict resolve / AI commit message（Sprint 4）
- 3-way merge resolve UI（Sprint 6）

## 依赖 / 前置

- Sprint 0+1+2 已合 main：Tauri scaffold、DDD 骨架、SQLite + libgit2 + tracing、Workspace / Repo CRUD
- 设计系统 Day 1-4 已合：Tailwind v4 + Radix + 18 primitives + 3-pane layout + theme toggle（commit `90587fd`）
- 系统 OpenSSL（git2 crate 通过 `vendored-openssl` feature 已自带）

## 步骤（按顺序）

### 1. Domain types

- `domain/history.rs`：
  - `CommitSummary { sha, author, author_email, time, message_summary, lane: u32 }`
  - `CommitDetails { sha, author, author_email, time, message_full, parents: Vec<String>, files: Vec<FileSummary> }`
  - `FileSummary { path, kind: FileKind, additions, deletions }`
- `domain/diff.rs`：
  - `FileDiff { path, old_sha, new_sha, hunks: Vec<DiffHunk>, additions, deletions }`
  - `DiffHunk { old_start, old_lines, new_start, new_lines, lines: Vec<DiffLine> }`
  - `DiffLine { kind: Added | Removed | Context, content, old_line_no, new_line_no }`
- `domain/blame.rs`：`BlameLine { line_no, sha, author, time, content }`
- `domain/branch.rs`：`BranchInfo { name, kind: Local | Remote, current, upstream, ahead, behind, last_commit }`
- mod.rs re-export

### 2. libgit2 history walker

- `infrastructure/git/history.rs`：
  - `commit_log(repo: &Repository, from_ref: &str, max: u32) -> Result<Vec<CommitSummary>>`
  - 用 `git2::Revwalk` 从 HEAD（或指定 ref）回溯
  - 走 max 个 commits，构造 `CommitSummary`
  - **lane assignment**（核心）：
    - 输入：commit list（拓扑序：parent 1 在前）
    - 输出：每个 commit 的 lane index + 可视化位置
    - 算法（homegrown）：
      1. 维护 lane 栈（每条 lane 当前 commit sha）
      2. 遍历每个 commit：找未占用的 lane（空 / 与父 commit 重合）
      3. 该 commit 占据 lane；其余 lane 在该 commit 位置绘制 "passing through"
      4. 多个 parent 时，parent 2+ 分配新 lane
  - 复杂度 O(N * lanes)；lanes 数通常 < 50

### 3. libgit2 branch operations

- `infrastructure/git/branch.rs`：
  - `list_branches(repo) -> Vec<BranchInfo>`（local + remote）
  - `create_branch(repo, name, from_sha)` — `RepoBuilder` 不适用，用 `repo.branch(name, commit, force)?`
  - `delete_branch(repo, name, is_remote)`
  - `checkout_branch(repo, name)` — `git2::Repository::set_head` + checkout 工作树
- 单元测试：init repo → commit → create branch → checkout → 操作成功

### 4. libgit2 merge / rebase

- `infrastructure/git/merge.rs`：
  - `merge_branch(repo, branch_name) -> MergeResult { kind, conflicts }`
  - `kind`: `FastForward | ThreeWay | AlreadyUpToDate | Conflicts`
  - 冲突时返回冲突文件列表（不 resolve；Sprint 6）
- `infrastructure/git/rebase.rs`：
  - `rebase_branch(repo, upstream) -> Result<RebaseResult>`
  - `git2::Rebase::new` 包裹 + 逐 commit apply；冲突时中止返回
  - 非 interactive（无 todo list 编辑）；interactive 留 Sprint 5

### 5. libgit2 diff / blame

- `infrastructure/git/diff.rs`：
  - `diff_commits(repo, base_sha, head_sha) -> Vec<FileDiff>`
  - `diff_working_tree(repo, path) -> FileDiff`（HEAD vs 当前未提交）
  - 使用 `git2::Diff::tree_to_tree` + `git2::Patch` 解析 hunks + lines
- `infrastructure/git/blame.rs`：
  - `blame_file(repo, path) -> Vec<BlameLine>`
  - `git2::Blame::for_file` + 逐 hunk 解析

### 6. Use cases + commands

- `application/use_cases.rs`（新增）：
  - `commit_log(ctx, repo_id, from_ref, max)` / `commit_details(ctx, repo_id, sha)` / `search_commits(ctx, repo_id, query, max)`
  - `list_branches(ctx, repo_id)` / `create_branch(ctx, repo_id, name, from_sha)` / `delete_branch(ctx, repo_id, name)` / `checkout_branch(ctx, repo_id, name)`
  - `merge_branch(ctx, repo_id, branch_name)` / `rebase_branch(ctx, repo_id, upstream)`
  - `diff_commits(ctx, repo_id, base, head)` / `diff_working_tree(ctx, repo_id, path)`
  - `blame_file(ctx, repo_id, path)`
- `lib.rs` typed commands：12 个（每个 use case 一个）
- 全部通过 `cmd_*` 前缀

### 7. 前端基础设施

- 安装依赖：`pnpm add shiki @tanstack/react-virtual`
- `src/lib/diff.ts`：diff 后端 wrapper + 类型
- `src/lib/shiki.ts`：Shiki 单例 + 语法加载（lazy）

### 8. CommitGraph 组件

- `src/components/CommitGraph.tsx`：
  - 用 `@tanstack/react-virtual` virtual scroll
  - 每行：lane DAG（SVG）+ commit message + author + 时间
  - 点击 → 选中 → 触发 `commit_details` 查询 → 高亮
- 性能：仅渲染可见 rows（~30）；lane 路径用 SVG path 拼接
- 集成到 Main pane 的 History tab（替换当前的 EmptyState）

### 9. DiffViewer 组件

- `src/components/DiffViewer.tsx`：
  - split / unified toggle
  - Shiki 高亮（按文件扩展名）
  - word-level diff：在行内再切；新增绿 / 删除红
  - virtual scroll（@tanstack/react-virtual）
- 集成到 CommitDetails 的 file click 事件

### 10. BlameView 组件

- `src/components/BlameView.tsx`：
  - inline 注释（每行左侧：author + sha 短 + 日期）
  - hover 整行高亮
  - 单击 commit → 跳到该 commit
- 通过 Main 的某个特殊 view（或 file tree 选中文件的右键菜单）

### 11. BranchList + MergeConflictView

- `src/components/BranchList.tsx`：在 Feature Nav 的 Branches tab 替换 EmptyState
- `src/components/MergeConflictView.tsx`：merge 冲突时显示冲突文件列表 + 提示（Sprint 6 再做 3-way resolve）
- 操作 menu：create / delete / checkout

### 12. Tests + 验证

- 单元测试（libgit2 + use cases）：
  - commit_log：init repo + 5 commit → log returns 5 items 顺序正确
  - lane_assignment：merge commit（2 parents） → lanes 正确
  - branch CRUD：create + checkout + delete
  - merge：fast-forward + conflict 检测
  - rebase：non-interactive apply
  - diff：commit-vs-parent
  - blame：blame 一个文件
- 手动验证：clone `https://github.com/git/git` （~50k commit）→ 浏览 → 性能预算
- E2E（v0.1 跳过；手动）

## 分支

推荐：`feature/history-graph-and-diff`

## 验证（DoD）

1. CI 三 job 全绿
2. 单元测试全过（含 lane assignment + merge conflict + blame）
3. 手动：在 `git/git` 大仓（10k+ commit）上流畅滚动 + diff 大文件
4. 性能预算：`commit_log` 10k commits < 500ms；`diff_commits` 1MB < 200ms
5. pre-commit 全过

## 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| lane assignment 复杂 merge 拓扑出错 | 视觉错位 | 大量 fixture 测试（chain / fork / octopus merge） |
| Shiki bundle 200KB gzip | 启动时间 | 预加载 vs lazy；只加载 project 需要的高频语言 |
| libgit2 rebase 交互需求 | 无法用 `Rebase::new` API 表达 | 非 interactive（无 todo list）即可；interactive 留 Sprint 5 |
| 10k commit 滚动掉帧 | 主线程压力 | virtual scroll + 最小化每行渲染 |
| merge conflict 解析缺 | 用户卡住 | v0.1 仅列冲突文件；resolve 留 Sprint 6 |
| SSH 凭证意外触发（clone 公共仓） | clone 大仓慢 | 公共仓走 https + helper；SSH 仅对 push/pull |

## 关联

- `docs/pm/features/F003-history-diff-blame.md`：PM 提案
- `docs/pm/core/04-sprint-v0.1.md` Sprint 3
- `docs/tech/architecture/00-overview.md`：分层 + 性能预算
- `docs/tech/tech-selection/00-overview.md`：libgit2 全量 + 钩子不自动跑
- `docs/design/04-working-copy.md`：commit / push / pull 留 Sprint 4；本 sprint 专注读路径
- `docs/design/02-components.md`：virtual scroll 需 `@tanstack/react-virtual`
- `docs/design/03-layout.md`：CommitGraph 在 Main 的 History tab
- `docs/tasks/feat-bootstrap-tau-app/plan.md`：Sprint 0 工程基座
- `docs/tasks/feat-workspace-crud/plan.md`：Sprint 1 workspace CRUD 模式
- `docs/tasks/feat-repo-ingestion/plan.md`：Sprint 2 repo 模式