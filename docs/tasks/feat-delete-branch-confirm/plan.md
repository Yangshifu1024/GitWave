# feat-delete-branch-confirm · 删除分支确认弹框（含可选删除远程分支）

> 状态：实施完成（待手动冒烟；未提交）
> 需求（用户，2026-08-28）：删除分支时需要弹框确认（截图参考 Fork 的 Delete Branch 对话框）。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 触发路径 | Branches 右键菜单 Delete → 确认弹框 | 原直删路径移除；弹框沿用 GitWave Modal + danger Delete 按钮约定（与 Workspace 删除一致） |
| 弹框要素（照搬 Fork） | 标题 "Delete Branch" / 副标题 "Delete local branch from your repository" / Branch 行（图标 + mono 名）/ "Also delete corresponding remote branch" 勾选 / Cancel + Delete | Fork 的勾选在其截图中呈灰色禁用（无对应远程分支时），本实现同样按数据动态禁用 |
| 远程勾选可用性 | `branches` 中存在短名匹配的远程分支（任一 remote）时可用 | 对应 remote 由远程分支名首段解析（`origin/fix/x` → `origin`）；无对应时禁用 + title 说明 |
| 远程删除语义 | 勾选后对每个匹配 remote 推送裸 refspec `:refs/heads/{name}`（网络操作，走既有凭证链），并 best-effort 清理本地 remote-tracking 引用 | 删除远端默认分支会被服务端拒绝，错误经 ErrorAlert 呈现；本地分支已删、远程删除失败时以错误提示为准 |
| 后端 | 新增 `cmd_delete_remote_branch(workspace_id, remote, branch)` 六步接线 | 远程删除属 push 类操作，归 `remote.rs`（非 branch.rs） |

## 改动清单

### Rust
- `infrastructure/git/remote.rs`：新增 `delete_remote_branch(repo, remote, branch)`（push 裸 refspec + prune 本地跟踪引用）
- `application/use_cases.rs` / `application/mod.rs`：`delete_remote_branch` 用例与再导出
- `lib.rs`：`cmd_delete_remote_branch` + generate_handler 注册

### 前端
- `src/lib/api.ts`：`deleteRemoteBranch(workspaceId, remote, branch)`
- `src/components/BranchList.tsx`：
  - 右键菜单 Delete 项改为打开 `deleteDialog {name, deleteRemote}`（不再直删）
  - `handleConfirmDelete`：删本地分支 → 勾选时逐 remote 删远程对应分支 → notice
  - `deleteCounterparts`：由 `branches`（getBranches 数据）解析匹配的 remote 集合
  - 确认弹框（照搬 Fork 布局；勾选框按对应远程分支存在性禁用/置灰）

## 测试

- cargo fmt / clippy（0 警告）/ test 111 通过；npm typecheck / test（49）/ lint / format / build 全绿
- 手动冒烟：右键 Delete → 弹框；无对应远程分支时勾选禁用置灰；勾选删除后本地与远程分支均消失；未勾选仅删本地；删除当前分支仍被拒（已有后端守卫）
