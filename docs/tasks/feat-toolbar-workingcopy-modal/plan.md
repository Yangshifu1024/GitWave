# feat-toolbar-workingcopy-modal · ActionBar + WorkingCopyModal 重构

> 状态：实施完成（待手动冒烟；未提交）
> 需求（用户，2026-08-28，5 项）：
> 1. TopBar 下方新增 ToolBar；
> 2. ToolBar 从左到右为 workspace / repository / branch 操作（各配图标），最右 Local Changes(n)（n=变更文件数）；
> 3. 点击 Local Changes：n≠0 时 WorkingCopyBar 以 Modal 弹出，n=0 不可点击；
> 4. WorkingCopyBar 改双列布局：左列 unstaged / staged / message box / AI generate / commit，右列点击文件的 diff；文件选择不再联动右边栏；
> 5. WorkingCopyBar 更名 WorkingCopyModal。

## 已确认决策（AskUserQuestion）

| 决策点 | 结论 |
|---|---|
| 底部常驻条 | 彻底移除（clean 状态行也不保留）；ConflictPanel 原位保留 |
| 左侧栏 | 操作全部移入 ToolBar（组头分组），左侧栏变纯导航列表；行级操作（Relink、右键 Merge/Rebase/Delete）保留 |
| 操作清单 | Workspace：New/Rename/AI/Delete；Repo：Init/Clone/Add Local/Fetch；Branch：New Branch/Pull(Fork 对话框)/Push |
| 组件命名 | **`ActionBar.tsx`**——用户口径的 "ToolBar" 与既有 TopBar 组件 `Toolbar.tsx` 仅大小写之差，Windows 文件系统大小写不敏感会冲突，故改名（需求口径不变） |

## 改动清单

### 新增
- `src/components/ActionBar.tsx`：三组操作（组头 + 图标按钮 + Tooltip）+ Local Changes(n)（FileDiff 图标）；迁入 workspace create/rename/delete 弹窗与 mutations、AI Provider 设置、repo init/clone/add-local 弹窗与 mutations（含 clone 进度监听）、branch create 弹窗、Fork 式 pull 对话框（remotes/branches 查询）；fetch/pull/push 直接走 `useWorkingCopy`
- `src/components/ui/WorkingCopyModal.tsx`：size xl Modal，双列 `grid-cols-[minmax(340px,420px)_1fr]`；左列 `ChangesPanel layout="modal"`，右列选中文件 `DiffViewer workdir path staged hideMaximize`；选择为 Modal 本地 state（repo 切换/文件消失时清空），不联动 Inspector；头部 BranchIndicator

### 修改
- `ChangesPanel.tsx`：`layout` 新增 `"modal"`（单列：unstaged 上 / staged 下 / commit box 底，bar 式计数头不可折叠，border-b 分隔）
- `DiffViewer.tsx`：新增 `hideMaximize`（Modal 内隐藏扩展 Inspector 按钮）
- `Modal.tsx`：新增 `xl` 尺寸（92vw / max 1200px）
- `WorkspaceList.tsx`：重写为纯列表（移除行内按钮 / 3 弹窗 / mutations / AiProviderSettings）
- `RepoList.tsx`：重写（移除区头四操作、init/clone/add 弹窗与 mutations、clone 监听、wc 依赖；保留激活、Relink、右键 Remove）
- `BranchList.tsx`：重写（移除区头 Pull/Push/New、Fork pull 对话框、create-branch 弹窗、wc 依赖；保留 checkout/merge/rebase/删除确认框/interactive rebase）
- `App.tsx`：挂载 `<ActionBar />`；移除底部 WorkingCopyBar、workdirSelection 联动、layoutStore wcBar 重置；MainContent 简化（无 workdir 视图，无 commit 选中时 EmptyState）
- `layoutStore.ts`：移除 wcBar 四字段；`layoutStore.test.ts` 删除（全部用例仅覆盖 wcBar）
- `ui/index.ts`：导出移除 WorkingCopyBar；`tokens.css` 注释更新

## 验证

- [x] `npm run typecheck` / `test`（49 通过）/ `lint`（0 问题）/ `format:check` / `build` 全绿
- [ ] 手动冒烟：ToolBar 三组操作全链路（含无活动 workspace/repo 的禁用态）；Local Changes 门控（n=0 禁用）；Modal 双列（点文件出 diff、stage/unstage 往返、AI 生成、commit 后 n 归零）；右栏不再随 Modal 选文件变化；pull/push 从 ToolBar 生效

## 边界

- 右侧 Inspector 不再有 workdir 总览视图（原文件联动移除）；diff 入口=History 点 commit 或 Modal 内
- 顶栏（`Toolbar.tsx`）与上下文标题不变；SyncProgressBar、ConflictPanel、Relink 不受影响
