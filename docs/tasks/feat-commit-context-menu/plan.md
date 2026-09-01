# feat-commit-context-menu · 实施计划与记录

关联提案：[F011](../../pm/features/F011-commit-context-menu.md) · 分支：`feature/commit-context-menu`

## 需求

History 列表提交行右键弹出操作菜单（参考 Fork），同时在行内 branch / tag 徽章上支持右键菜单。范围经用户确认：核心集 + Checkout Commit（新增后端 detached checkout）；徽章右键一并实现。

## 菜单结构（最终实现）

**提交行**：New Branch… / New Tag… / ―― / Checkout Commit（HEAD 处禁用）/ Reset '分支' to Here…（detached 禁用，hard reset + 确认）/ ―― / Cherry-pick… / Revert… / ―― / Copy Commit SHA / Copy Commit Info。右键同时选中该行。

**徽章**（head 徽章穿透到行菜单）：本地分支 Checkout（F004 门）+ Delete（确认）+ Copy Name；远端分支 Delete Remote Branch（确认）+ Copy Name；标签 Delete Tag（确认）+ Copy Name。

## 改动清单

### 后端（1 个新命令）

- `infrastructure/git/branch.rs`：`checkout_commit(repo, oid, force)`。实现要点：libgit2 的 SAFE checkout 策略对「删除已跟踪文件」的干净切换也会报 Conflict（实测 `1 conflict prevents checkout`，即使 workdir 干净），因此语义定为——**非 force 先用 statuses 检查工作区干净（含 untracked），脏则报 `git.dirty_worktree`；干净后以 force checkout_tree + `set_head_detached` 执行**。与 `git checkout <commit>` 行为一致，复用既有错误码 `INVALID_OID` / `COMMIT_NOT_FOUND` / `DIRTY_WORKTREE`。
- `application/use_cases.rs` + `application/mod.rs` + `lib.rs`：use case `checkout_commit`、re-export、`cmd_checkout_commit(workspace_id, oid, force)` 注册。
- 单测：detached 切换（HEAD 非分支、指向目标 oid、工作树内容交换）、非法 oid、脏工作区拒绝 + force 成功。
- 不改 AI palette 白名单（`PALETTE_ACTIONS_CONFIRM` 等）：那是 AI 提案动作体系，与用户手动菜单无关。

### 前端

| 文件 | 内容 |
|---|---|
| `src/lib/api.ts` | `checkoutCommit(workspaceId, oid, force)` |
| `src/lib/commitMenu.ts`（新） | `parseRemoteBranchName` / `copyCommitInfoText` / `copyToClipboard` / `gateCommitCheckout`（纯函数） |
| `src/lib/commitMenu.test.ts`（新） | 11 个单测 |
| `src/hooks/useActiveRepoState.ts`（新） | 轻量读取当前分支 + HEAD sha（共享 `["working-copy"]` query key） |
| `src/hooks/useBranchCheckout.tsx`（新） | F004 安全门 + 三选一弹窗流程（供徽章菜单用），镜像 BranchList 的 `handleCheckout` / `checkoutOnto` |
| `src/components/TagManagerModal.tsx`（新） | 从 CommitInfoHeader 原样提取，两处复用 |
| `src/components/CommitContextMenu.tsx`（新） | `CommitMenuItems`（行菜单项）+ `useCommitMenuActions`（动作 + 单实例弹窗） |
| `src/components/RefBadgeContextMenu.tsx`（新） | 徽章级菜单（trigger 自带 stopPropagation，不冒泡到行菜单） |
| `src/components/CommitGraph.tsx` | 行 / 徽章接线；`menu.renderModals()` 单实例挂在列表根部 |
| `src/components/CommitInfoHeader.tsx` | 改用提取后的 TagManagerModal |
| i18n `commits.json` ×2 | 新增 `commits.menu.*` |
| i18n `branches.json` ×2 | 新增 `branches.menu.checkout/copyName/copyTagName`、`branches.deleteRemote.*` |

复用的既有文案与流程（不重复造轮子）：`branches.switch.*`（三选一 / blocked 弹窗）、`branches.checkout.*`（状态条反馈）、`branches.newBranch.*`（建分支弹窗）、`repo.reflog.resetTitle/resetDescription/resetConfirm/branchReset`（reset 确认）、`repo.tags.deleteDialog.*`（删标签确认）、`commits.revert.*` / `commits.cherryPick.*`（revert / cherry-pick 确认，行为对齐 CommitInfoHeader）。

### 关键实现决策

1. **useBranchCheckout 不重构 BranchList**：BranchList 的 checkout 流程与其本地状态（busy / selectedName / irebasePaused / run() 包装）深度耦合，本 PR 内提取会让侧栏承担回归风险。徽章菜单使用独立 hook（复用同一 `gateCheckout` 纯函数、同一 API 序列与 i18n 文案），BranchList 迁移记 follow-up。
2. **行菜单挂在 Surface 上**：HeroUI `Surface` 以 `...rest` 透传 DOM props（已核实实现），`ContextMenuTrigger asChild` 直接挂 Surface；Surface 自身 `onContextMenu` 先选中该行（右键即选中），trigger 随后 preventDefault + stopPropagation 并打开菜单。
3. **徽章菜单的行选中**：徽章 trigger 的 stopPropagation 会拦住 Surface 的选中 handler，故在徽章外层 span 的 `onContextMenu` 里先调 `onSelect(sha)` 再由 trigger 打开菜单。
4. **detached HEAD 的 head 标记无需后端改动**：`collect_commit_refs` 用 `repo.head()` 打 `head` ref（`history.rs:485`），detached 时同样生效，history 高亮自动正确。
5. **refs 命名格式**：本地分支短名、远端 `origin/xxx` 全短名、tag shorthand（`history.rs:418-461`）；`parseRemoteBranchName` 按第一个 `/` 拆分，解析失败时隐藏 Delete 项。

## 扩展：侧栏分支右键菜单（Fork 对齐，同 PR 第二批）

用户追加需求：侧栏 BranchList 也要 Fork 的分支右键菜单。范围（用户未答复范围问题时按推荐「完整对齐」执行）：

**菜单结构（本地分支行，自上而下）**：Checkout（当前分支禁用）· Push to '<remote>'…（确认弹窗，plain push）· New Branch…（原「New」更名）· New Tag…（分支顶端）· Tracking…（设置 / 清除上游）· Rename… · Delete（当前分支禁用）· Copy Branch Name · ―― · Merge / Rebase / Interactive rebase（GitWave 特有，非当前分支保留）。

### 后端新增

| 能力 | 位置 | 要点 |
|---|---|---|
| 推送任意本地分支 | `remote.rs` `PushRequest.branch: Option<String>` | Some(name) 时校验本地分支存在并以其构造 refspec（不要求 HEAD 在该分支）；None 走原 HEAD 路径，默认行为不变。`cmd_push` 加可选 `branch` 参数 |
| 重命名分支 | `branch.rs` `rename_branch` | `Branch::rename`；**重命名当前分支后需 `set_head` 回新名**（libgit2 不会自动改 HEAD symref，测试锁定）；若有上游，rename 后重新 `set_upstream` 保存跟踪关系 |
| 设置 / 清除上游 | `branch.rs` `set_branch_upstream` | `Branch::set_upstream("origin/xxx")`；测试中需先 `repo.remote("origin", url)`（set_upstream 会从 config 解析远端，仅建 refs/remotes 引用不够） |

单测新增 5 个：rename 移动 ref、rename 当前分支 HEAD 跟随、重名非 force 报错 / force 覆盖、rename 保留上游、set/clear upstream。push 传输层无测试（仓库现状如此，需网络/远端夹具）。

### 前端

- `api.ts`：`pushRemote` 加 `branch?: string`；新增 `renameBranch` / `setBranchUpstream`
- `BranchList.tsx`：BranchRow 菜单按 Fork 顺序重构；新增 push 确认弹窗（复用 `commits.sync.pushTitle`，plain push 非 force）、TagManagerModal（分支顶端 sha）、rename 输入弹窗（错误回显）、tracking 上游选择弹窗（Select 列出远端分支 + 「无上游」）；Copy 用 `copyToClipboard`
- 推送远端取值：`branch.upstream` 首段，否则 `origin`（`upstreamRemote()` 辅助函数）
- Tracking 用对话框而非 Fork 的子菜单：自研 ContextMenu 组件无子菜单能力，对话框同时能展示当前上游、天然容纳「清除」动作

### 测试与验证（增量）

`cargo test` 245 通过 · `npm run test` 144 通过 · tsc / eslint / prettier / clippy 全绿。

## 明确不做（follow-up 候选）

- 徽章 Push（侧栏分支菜单已支持推送任意分支，徽章级入口留后续）
- Reset soft/mixed、Interactive Rebase to Here、Save as Patch、Compare to Local Changes（后端缺失）
- BranchList 的 checkout 流程迁移到 `useBranchCheckout`（本批直接复用 BranchList 原有 handleCheckout，保持行为不变）
- `checkoutGate.ts`（分支侧 gate）既有英文阻断文案 i18n 化——本次提交侧 gate 已改为 reason + 调用方 i18n，分支侧照此模式迁移即可

## 审查与修复记录

code-reviewer 审查结论：无 🔴 问题，可合入；5 个 🟡 均已修复（invalidate 时序、syncStore 注册、health/reflog 失效、blocked 文案 i18n、TagManagerModal 删除确认），详见 [review.md](./review.md)。

### 增量审查（侧栏分支菜单批次）

无 🔴 问题；3 个 🟡 均已修复：submitPush 加 syncStore 忙碌守卫、rename 删除冗余上游重挂（libgit2 配置节搬运已覆盖，避免半成功报错）、rename 增加链接 worktree 占用守卫（新错误码 `git.branch.rename_in_worktree` + en/zh 文案 + 单测）。🟢 采纳：取消错误区分展示、push 弹窗 in-flight 禁关、Checkout 禁用文案、rename 未改名禁用确认。详见 [review.md](./review.md) 增量附录。

## 验证记录

- `cargo test`：238 passed / 0 failed（含 3 个新增 checkout_commit 测试）
- `cargo clippy --all-targets`：0 告警
- `npm run test`：144 passed（含 commitMenu 11 个新测试 + i18n parity）
- `npm run typecheck` / `lint` / `format:check`：通过
- 手动验证清单：行右键菜单各项与禁用态、右键选中行、徽章右键不冒泡触发行菜单、detached checkout 后 history HEAD 标记移动且工作区切换、reset/revert/cherry-pick 后列表刷新与状态条反馈、中英文文案
