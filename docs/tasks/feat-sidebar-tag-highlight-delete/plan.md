# feat-sidebar-tag-highlight-delete · 左侧栏 Tag 行高亮与右键删除

> 状态：实施中（未提交）
> 需求（用户）：
> 1. 左侧栏中增加 Tag（已有 TagsPanel，卡片级存在性确认）
> 2. 左侧栏 tag 面板增加 hover 高亮 / 选中高亮和右键菜单-删除

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 需求 1「左侧栏中增加 Tag」 | 确认已有 `TagsPanel`，无需新增 | 侧栏已渲染 Tags 卡片（App.tsx sidebar 顺序：Health → Branches → Stash → **Tags** → Remotes → Worktrees → Submodules → Reflog）；本任务聚焦需求 2 的行级交互 |
| 选中高亮语义 | tag 指向的 commit 被选中时高亮该行 | 与 CommitGraph 的选中态同源：`App.tsx` 的 `selectedCommitOid`（repo-scoped）传入 `selectedSha`，点 tag / 点提交图 / 命令面板定位均联动 |
| 行组件 | 复用 `ListItem`（selected 态：左侧 accent 边 + accent/10 底 + 加粗） | 与 BranchList / StashPanel / WorkspaceSwitcher 的侧栏选中态一致，替代原手写 `hover:bg-bg-elevated` button |
| 删除确认 | 右键 Delete → 确认弹框（danger Delete 按钮） | 用户选择；照搬 `feat-delete-branch-confirm` 的 Modal 布局惯例（Cancel + danger 按钮 3:7 分栏）；弹框描述明确「仅移除标签引用，提交保留」 |
| 通知 | 成功后写入 ActionBar 状态区（`useStatusAreaStore`） | BranchList 删除分支同样走状态区，是全局唯一操作状态出口 |
| 后端 | 无改动 | `deleteTag` API（`cmd_delete_tag`）已存在并被 CommitInfoHeader 的标签管理器使用；错误码已有 i18n key |
| i18n | `repo.tags.*` 新增 menu.delete / deleteDialog.{title,description,deleted}；按钮文案复用 `common.cancel` / `common.delete` | en + zh-CN 双语 |

## 改动清单

### 前端
- `src/components/TagsPanel.tsx`：
  - 行组件改为 `ListItem`：selected 高亮（accent 边 + accent/10 底）+ 自带 hover 高亮（bg-bg-primary/70）
  - 每行包 `ContextMenu`：Label（tag 名）+ 分隔线 + destructive「删除标签」项
  - 新增 `selectedSha` prop；新增 `deleteTarget` 状态 + 确认弹框（Modal，danger Delete）
  - 删除成功：invalidate `["tags", workspaceId]`（同步 CommitInfoHeader 的同名 query）+ 状态区 notice；失败走 `ErrorAlert`
- `src/App.tsx`：`<TagsPanel onSelect={...} selectedSha={selectedCommitOid} />`
- `src/i18n/locales/{en,zh-CN}/repo.json`：`repo.tags.menu.delete`、`repo.tags.deleteDialog.*`

## 测试

- `npm run typecheck` / `npm test` / `npm run lint` / `npm run build` 全绿（见 review）
- 手动冒烟要点：
  - hover tag 行出现高亮；点击 tag 行定位到 History 中对应提交，行保持 accent 高亮
  - 点击提交图其他提交后，无 tag 指向该提交时 tag 行高亮消失
  - 右键 tag 行 → 菜单含删除项 → 弹框确认 → 标签消失、CommitInfoHeader 标签同步、状态区出现「已删除标签 xxx」
  - 切换语言 en/zh-CN，菜单与弹框文案正确
