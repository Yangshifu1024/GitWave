# fix: 仓库类弹框（init / clone / add local）布局与交互

状态：已实现

## 需求来源

用户 2026-08-28 截图反馈 Initialize new repo 弹框布局需优化，方案确认后落地。分支：`feature/theme-design`（沿用）。

## 根因

1. **🔴 PathInput props 断线（bug）**：声明的 `autoFocus` / `onKeyDown` / `id` 未在函数解构，也没透传给内部 `Input` → 弹框打开不聚焦、Enter 提交无效（影响全部 5 处使用：init / clone dest / add local / relink / ssh key）
2. 单字段表单无 description / label，内容悬在弹框里显空
3. placeholder 过长被截断；Browse… 按钮占宽

## 修复

- `src/components/ui/PathInput.tsx`：解构并透传 `autoFocus` / `onKeyDown` / `id`；Browse 改纯图标（aria-label + title 保留）
- `src/components/ActionBar.tsx`：
  - init：`description="Create a fresh Git repository in an empty folder."` + Location label + placeholder 缩短为 `/Users/me/projects/new`
  - clone：补 `description="Copy a remote repository into a local folder."`
  - add local：`description="Register an existing Git working tree with this workspace."` + Location label + placeholder `/Users/me/projects/existing`

## 验证

- typecheck / prettier / vitest / build
- 真机：弹框打开即聚焦；Enter 提交；init/clone/add 三弹框文案层级一致
