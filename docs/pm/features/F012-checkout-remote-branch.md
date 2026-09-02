# F012 · 双击远程分支：创建同名本地分支并切换（DWIM）

## 背景

F004 规定双击 remote-tracking 分支时拦截（不能直接 checkout），但拦截后没有给出路：远程分支行没有右键菜单，其他入口也不提供"从远程分支创建本地分支"的操作。结果是用户在 UI 上没有任何途径把远程分支拉到本地（如双击 `origin/feat/ally-optimizations-port` 只会得到"无法切换"错误弹窗），与 git switch / VS Code / Fork / GitHub Desktop 的标准行为（DWIM）不一致。

## 提议方案

**双击远程分支**（侧栏分支列表、commit graph 远程 ref 徽章等所有 F004 入口）：

1. 无同名本地分支 → 自动创建同名本地分支（`origin/feat/x` → `feat/x`，指向远程 tip）、设置 upstream 跟踪该远程分支、切换过去
2. 已有同名本地分支 → 直接切换到该本地分支（不动其 upstream）
3. 该本地分支已是当前分支 → 无操作
4. 安全门沿用 F004：merge 进行中 / interactive rebase paused / 目标本地分支被其他 worktree 占用 → 拦截；工作区脏 → Cancel / Discard / Stash & switch 三选弹窗；干净 → 直接执行
5. 创建与切换在后端单命令内完成（创建前先做脏工作区检查，拒绝时不留下半成品分支）

**远程分支行右键菜单**（最小集）：切换到此分支（同双击流程）、复制名称。

**明确不做**：右键菜单的"从 tip 新建分支（自定义名）/ 删除远程分支"等扩展项（后续按需再提）；自动 fetch。

与产品原则不冲突：所有操作由用户手动发起（双击 / 右键确认），符合 P1；属于核心功能清单 §1.1 的 branch / checkout 基本盘。

## 影响

- 涉及模块：checkoutGate、useBranchCheckout（F004 流程权威实现）、BranchList（迁移到 hook + 远程行菜单）、git 基础层（新命令 `cmd_checkout_remote_branch`）、i18n
- 影响版本：v0.7.x
- 是否破坏向后兼容：否（纯新增能力；`cmd_checkout_branch` 行为不变）

## 决策

- 状态：接受
- 决策人：杨师傅
- 决策日期：2026-09-02
- 关联决策：F004（本提案取代其"remote → 拦截"行为，安全门与三选弹窗沿用）
