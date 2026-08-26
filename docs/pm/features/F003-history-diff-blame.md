# F003 · History Graph + File Diff + Blame + Branch Ops

## 背景

v0.1 必须包含 §`docs/pm/core/02-scope.md` 1.1 的 5 / 7 两项 must-have：

- **5 读子集**：branch CRUD、merge、普通（non-interactive）rebase
- **7**：history 图（commit DAG，可视化 + 筛选 / 搜索）、文件 diff（split / unified + syntax highlight）、blame / annotate

不做这两项，用户能用 GitWave 创建仓库、commit、push，但**看不到自己的历史**——等于把仓库存进黑盒。这违背了 PM 文档 §1.2 的核心可视化承诺。

## 提议方案

### 范围（In Scope）

#### History（核心可视化）

- **Commit DAG 可视化**：左到右横向 lane 图，每个 commit 占一个 lane；颜色区分 branch（local / current / remote）
- **virtual scroll**：仅渲染可见 commits；10k+ commits 流畅目标
- **commit 详情**：选中 commit 后右侧显示 metadata（sha / 作者 / 日期 / message）+ 影响的文件列表 + diff 触发
- **筛选 / 搜索**：branch / author / message 关键字（v0.1 简化：branch 切换 + message 搜索）
- **branch CRUD**：list / create / delete / switch（checkout）；分支类型（local / remote）
- **merge**：fast-forward / three-way；冲突标记
- **blame**：inline 注释 + 悬停高亮

#### 文件 diff（详情视图）

- **split / unified 切换**
- **syntax highlight**：Shiki 引擎（预编译 TextMate grammar），按文件扩展名
- **word-level diff**：同一行内的增删高亮
- **virtual scroll**：大文件（>1MB）流畅
- **file tree**：在 commit 详情侧折叠 / 展开

### 范围外（Out of Scope）

- Interactive rebase 拖拽（Sprint 5）
- Stash（v0.1 不做；Sprint 5）
- Tags CRUD（Sprint 5）
- Submodule diff（Sprint 5+）
- Commit message AI 生成 / conflict AI（Sprint 4 已有 plan）
- LFS / reflog explorer（Sprint 5+）
- Cherry 3 / cherry-pick（v0.2）

## 影响

### Backend（Rust）

- `src-tauri/src/domain/history.rs`：`CommitSummary`（sha / author / time / message / parents / lane）
- `src-tauri/src/domain/diff.rs`：`FileDiff`（path / hunks / additions / deletions）
- `src-tauri/src/domain/blame.rs`：`BlameLine`（line_no / sha / author / content）
- `src-tauri/src/infrastructure/git/history.rs`：基于 `git2::Revwalk` 的 commit log；lane assignment（homegrown 算法）
- `src-tauri/src/infrastructure/git/branch.rs`：branch list / create / delete / checkout
- `src-tauri/src/infrastructure/git/merge.rs`：fast-forward / three-way merge
- `src-tauri/src/infrastructure/git/rebase.rs`：non-interactive rebase
- `src-tauri/src/infrastructure/git/diff.rs`：commit-vs-parent diff + word-level
- `src-tauri/src/infrastructure/git/blame.rs`：基于 `git2::Blame`
- 新 use cases ~12 个（`get_commit_log` / `get_commit_details` / `diff_commits` / `diff_working_tree` / `list_branches` / `create_branch` / `delete_branch` / `checkout_branch` / `merge_branch` / `rebase_branch` / `blame_file` / `search_commits`）
- 新 typed commands ~12 个

### Frontend（React）

- `src/components/CommitGraph.tsx`：lane DAG + virtual scroll
- `src/components/CommitDetails.tsx`：metadata + file list + diff trigger
- `src/components/DiffViewer.tsx`：split / unified + syntax highlight + word diff
- `src/components/BranchList.tsx`：branch tree + 操作 menu
- `src/components/MergeConflictView.tsx`：Sprint 3 先列文件，3-way resolve 留 Sprint 6
- `src/components/BlameView.tsx`：inline 注释 + hover highlight
- `src/components/SearchBar.tsx`：commit message 搜索（v0.1 简化）
- 引入 Shiki（~200KB gzip，首次加载）+ `@tanstack/react-virtual`

### 性能预算

| 指标 | 目标 |
|---|---|
| history graph 打开到首次可交互（10k commit 仓） | < 500ms |
| 单文件 diff 渲染（1MB 文件） | < 200ms |
| 后台 git 操作不阻塞 UI | 取消令牌 + tokio task |

来源：`docs/tech/engineering/00-overview.md` 性能预算。

## 验收

- 选中某 commit → 显示 file list → 选某文件 → diff 展示
- branch list 显示 local + remote；create / switch / delete 操作可达
- merge 后冲突文件被标记（具体 resolve 留 Sprint 6）
- blame inline 显示 author + commit sha + hover 整行高亮
- virtual scroll：10k commit 仓流畅滚动 60fps（DevTools 帧率验证）
- 单元测试：libgit2 history walk / lane assignment / diff 计算 / blame parser
- 手动验证：clone 公开大型仓（如 git/git）→ 浏览 history → 选 commit → diff 大文件

## 决策

- **状态**：接受
- **决策人**：PM
- **决策日期**：2026-08-26
- **关联决策**：`docs/tech/architecture/00-overview.md`（分层 + 性能预算）；`docs/tech/tech-selection/00-overview.md`（libgit2 全量 + 钩子不自动跑）
- **关联计划**：`docs/tasks/feat-history-graph/plan.md`
- **关联 Sprint**：`docs/pm/core/04-sprint-v0.1.md` Sprint 3