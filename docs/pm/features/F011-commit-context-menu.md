# F011 · History 提交右键菜单

## 背景

当前 history 图中的提交行只支持单击选中（查看 diff）与搜索定位，所有提交级操作（建分支 / 建标签 / cherry-pick / revert / reset）都散落在 Inspector 头部按钮、侧栏 BranchList / TagsPanel 或 Reflog 恢复面板里，入口深、路径长。同类 Git 客户端（Fork）在提交行右键即提供完整的操作菜单，交互效率高，用户已习惯该模式。

## 提议方案

参考 Fork，为 history 列表提供两级右键菜单：

**提交行右键**（自上而下）：

- New Branch…（在此提交创建分支，输入弹窗）
- New Tag…（在此提交创建 / 管理标签，复用 TagManagerModal）
- Checkout Commit（detached 检出该提交；已是当前 HEAD 时禁用；脏工作区走 F004 三选一弹窗）
- Reset '当前分支' to Here…（hard reset + 确认弹窗；detached 时禁用）
- Cherry-pick Commit… / Revert Commit…（各带确认弹窗，与 Inspector 头部行为一致）
- Copy Commit SHA / Copy Commit Info（纯前端 clipboard，Fork 风格多行文本）

右键同时选中该行（Fork 行为）。

**行内 ref 徽章右键**：

- 本地分支徽章：Checkout（复用 F004 安全门）、Delete Branch（确认，当前分支禁用）、Copy Branch Name
- 远端分支徽章（`origin/xxx`）：Delete Remote Branch（确认）、Copy Branch Name
- 标签徽章：Delete Tag（确认）、Copy Tag Name
- `head` 徽章不弹徽章菜单，右键穿透到提交行菜单

**侧栏分支列表右键**（Fork 对齐扩展，同 PR 实现）：

- Checkout（F004 安全门；当前分支禁用）
- Push to '<remote>'…（确认后 plain push 该分支——后端扩展 `cmd_push` 支持推送任意本地分支，非当前分支亦可；上游远端优先，缺省 `origin`）
- New Branch…（从分支顶端建分支，原「New」更名）
- New Tag…（在分支顶端提交建 / 管理标签）
- Tracking…（设置 / 清除上游 tracking——新命令 `cmd_set_branch_upstream`；以对话框列出远端分支，替代 Fork 的子菜单）
- Rename…（重命名本地分支——新命令 `cmd_rename_branch`；HEAD 跟随、上游关系保留）
- Delete（确认，当前分支禁用）
- Copy Branch Name
- 保留 GitWave 特有的 Merge into current / Rebase current onto this / Interactive rebase（非当前分支）

**明确不做**：Fork 的「Push and Create Pull Request on 'origin'」（产品无 GitHub 集成）；其余同上（reset soft/mixed、save as patch 等）。

与产品原则不冲突：所有操作均由用户手动发起并逐项确认，符合 P1（AI 协作约束不涉及）；全部动作属于核心功能清单 §1.1（revert / branch / cherry-pick / tag / checkout / reset）。

## 影响

- 涉及模块：history 图与侧栏分支列表（前端）、git 基础层（新增 detached checkout、推送任意分支、重命名分支、设置上游共 4 组命令）、i18n
- 影响版本：v0.7.x
- 是否破坏向后兼容：否（纯新增入口；`cmd_push` 新增可选 `branch` 参数，默认行为不变）

## 决策

- 状态：接受
- 决策人：用户（2026-09-01 需求提出时确认范围）
- 决策日期：2026-09-01
- 关联决策：F003（history / branch ops）、F004（safe branch switch 的安全门与三选一弹窗）
