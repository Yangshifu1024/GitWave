# feat: 点击 commit 时右栏顶部显示提交详情

状态：已实现

## 需求来源

用户 2026-08-28：点击 history 中的 commit 时，右侧栏顶部应显示本次提交的相关信息。分支：`feature/theme-design`（沿用）。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 数据通道 | 新增 `cmd_get_commit_details`（workspace_id + oid） | 前端 `CommitDetails`/`FileSummary` TS 类型早已预置（api.ts:252）而后端从未实现；虚拟列表中 commit 可能不在已加载页，不能只靠前端已有 CommitSummary |
| files 统计 | 独立 tree diff + `Diff::find_similar(None)` | git2 0.20 的 rename 检测是 Diff 上的 find_similar（DiffOptions 无 track_renames/find_renames）；FileSummary 带 kind + old_path（rename），RefCell 累加器复用 diff_to_files 模式 |
| 头部内容 | subject 加粗 + body（pre-wrap）+ 作者/邮箱/日期 + 短 sha chip | 文件数/增删统计不重复展示（DiffViewer 工具栏已有） |
| 挂载 | App.tsx MainContent：header shrink-0 + DiffViewer 包在 `flex-1 min-h-0` | `key={activeRepoId}` 上移到包裹层，切仓仍整体重置 |

## 改动清单

- `src-tauri/src/infrastructure/git/history.rs`：`commit_details(repo, sha)` + 测试（身份/完整 message/文件统计/根提交空树 diff）
- `src-tauri/src/application/use_cases.rs` + `application/mod.rs`：`get_commit_details` 用例 + re-export
- `src-tauri/src/lib.rs`：`cmd_get_commit_details` + 注册 + import
- `src/lib/api.ts`：`getCommitDetails()` 封装（类型已有）
- `src/components/CommitInfoHeader.tsx`：新组件（react-query `["commit-details", workspaceId, sha]`）
- `src/App.tsx`：inspector 顶部接入

## 验证

- `cargo test --lib` 120 通过、clippy 0 warning、fmt 通过
- 前端 typecheck / lint / test / build（见执行记录）
- 真机：点击 graph 中 commit，右栏顶部出现详情卡（含 body、作者、日期、短 sha），切 commit 即刷新
