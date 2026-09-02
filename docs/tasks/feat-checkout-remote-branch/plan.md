# feat-checkout-remote-branch

> 双击远程分支（如 `origin/feat/x`）→ 自动创建同名本地分支 `feat/x`、设置 upstream、切换（DWIM）。取代 F004 的"remote → 拦截"行为。提案见 [F012](../../pm/features/F012-checkout-remote-branch.md)。

## 行为

双击远程分支（入口：侧栏 BranchList、commit graph 远程 ref 徽章等所有 F004 流程入口）：

| 状态 | UI |
|---|---|
| 同名本地分支是当前分支 | 无操作 |
| 同名本地分支已存在 | 直接切换到该本地分支（不动其 upstream） |
| merge / rebase paused | 拦截弹窗 |
| 目标本地分支被其他 worktree 占用 | 拦截弹窗 |
| dirty | Cancel / Discard / Stash & switch（沿用 F004 三选弹窗） |
| clean | 后端单命令：建分支 + set_upstream + checkout，提示"已创建并切换" |

远程分支行新增最小右键菜单：切换到此分支（同双击流程）、复制名称。

## 实现

- 后端 `checkout_remote_branch(repo, remote_name, force) -> CheckoutRemoteOutcome { created, already_current, local_name }`：`src-tauri/src/infrastructure/git/branch.rs`
  - 遍历 `repo.remotes()` 最长前缀匹配解析本地名（`origin/feat/x` → `feat/x`）
  - 已存在本地分支：HEAD → `already_current`（不动工作区）；否则复用 `checkout_branch`
  - 创建路径：`force=false` 先做脏工作区检查（含 untracked），拒绝时**不写任何 ref**；然后 `repo.branch` + `set_upstream` + `checkout_branch`
- 用例 + 命令：`use_cases.rs::checkout_remote_branch`、`lib.rs::cmd_checkout_remote_branch`（`{ workspaceId, name, force }`）
- 前端 gate：`checkoutGate.ts` 移除 remote 拦截分支与 `branchKind` 输入（远程 → 目标本地名的解析移到调用方）
- 前端流程：`useBranchCheckout.tsx`——remote 时 `target = remoteShortName(name)`；worktree 占用、弹窗标题、提示均用 target；isCurrent 从 `["branches", workspaceId]` query 缓存解析（兜底 false，后端 `already_current` 二次兜底）；switch 改调 `checkoutRemoteBranch`，成功 invalidate `["branches", workspaceId]`
- `BranchList.tsx` 迁移到 `useBranchCheckout`（消除 F004 复制品，hook 注释明确期待）；hook 增加可选 `onSwitched(target)` 供侧边栏 setSelectedName + refresh；远程行渲染最小右键菜单
- i18n：`branches.checkout.createdFromRemote`（en / zh-CN）

## 验证

- Rust：创建+跟踪+切换；已存在本地分支复用且保留 upstream；目标为当前分支不动工作区；脏工作区拒绝且无半成品分支
- 前端：`checkoutGate.test.ts` 更新；vitest / lint / build 通过
- 手动：双击 `origin/feat/ally-optimizations-port` 全流程；脏工作区三选弹窗；右键菜单；ref 徽章入口一致性
