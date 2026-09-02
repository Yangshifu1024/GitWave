# F004 · Safe branch switch (double-click + dirty-work dialog)

## 背景

分支列表单击即 checkout，且后端 `checkout` 曾默认 `force` + 删除 untracked，未提交改动会被静默丢掉。选中分支（看 ahead/behind、同步）与切换 HEAD 也不应是同一手势。

## 提议方案

- **单击**：只选中（高亮、Pull/Push 目标）
- **双击**：请求切换
- 切换前检查，按优先级：
  1. 已是当前分支 → 无操作
  2. remote-tracking → 拦截（不能直接 checkout）
  3. merge 进行中 → 拦截
  4. interactive rebase paused → 拦截
  5. 目标分支已被其他 worktree checkout → 拦截
  6. 工作区有未提交文件 → 弹窗：**Cancel** / **Discard**（确认后 force checkout）/ **Stash & switch**（stash → checkout → pop）
  7. 干净工作区 → 直接 safe checkout
- 默认 checkout **不再 force**；Discard 才 force。
- 不提供自动 commit（符合 P1）。

## 影响

- 涉及模块：BranchList、ListItem、checkout_branch、stash pop
- 影响版本：v0.1
- 是否破坏向后兼容：否（手势变更，无 IPC 破坏；`cmd_checkout_branch` 增加可选 `force`）

## 决策

- 状态：接受
- 决策人：杨师傅
- 决策日期：2026-08-27
- 关联：F003 branch ops
- 关联决策：F012 修订了本提案第 2 条"remote → 拦截"——双击远程分支改为创建同名本地分支并切换（安全门与三选弹窗沿用）
