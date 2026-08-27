# feat-diffview-toolbar-and-contextmenu · 右键菜单抑制 + DiffViewer 工具栏改造

> 状态：实施完成（待手动冒烟；未提交）
> 需求（用户，2026-08-28，4 项）：
> 1. 界面右键不再出现 WebView 默认菜单；
> 2. 右边栏展开按钮改为文字：未展开显示 Expand，展开后显示 Restore；
> 3. Unified / Split 改为段控件；
> 4. 新增一段控件（折叠 / 展开），作用于 diff view 中的所有文件。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 右键菜单抑制 | main.tsx 全局 `contextmenu` preventDefault，`input / textarea / [contenteditable]` 例外 | 例外保留输入框原生复制/粘贴；radix ContextMenu 自带 handler 不受影响（分支/文件/仓库右键菜单照常） |
| Expand / Restore | 图标按钮改为文字按钮（secondary） | 状态语义直接可见；`hideMaximize`（WorkingCopyModal 内）逻辑不变 |
| Unified / Split | 段控件（内联 `SegmentedControl`：bordered 容器 + 选中段 accent 反白） | 替代两个独立按钮 |
| 文件折叠 | DiffViewer 持有 `collapsedFiles: Set<key>`；段控件 Collapse = 全部 key 加入 / Expand = 清空；每个文件标题栏 chevron 单独折叠/展开 | key = `s:/u:{path}`（与列表 key 同源）；折叠后仅显示文件标题栏（含 sha / 增删 / Blame）；段控件选中态反映 all / none / mixed（mixed 两段均不选中） |

## 改动清单

- `src/main.tsx`：全局 contextmenu 抑制（editable 例外）
- `src/components/DiffViewer.tsx`：
  - 移除 Maximize2 / Minimize2 / Tooltip 导入；新增 ChevronDown / ChevronRight
  - `SegmentedControl` + `fileChangeKey` 模块级辅助
  - 工具栏：Collapse/Expand 段控件 + Expand/Restore 文字按钮 + Unified/Split 段控件
  - `collapsedFiles` 状态与派生（allCollapsed / anyCollapsed）
  - `FileDiffView` 增加 `collapsed` / `onToggleCollapsed`：标题栏 chevron 切换，折叠时隐藏 hunks 区

## 验证

- [x] `npm run typecheck` / `test`（43）/ `lint` / `format:check` / `build` 全绿
- [ ] 手动冒烟：空白处右键无系统菜单；输入框右键保留原生菜单；分支/文件右键菜单正常；Expand/Restore 切换；Unified/Split 段控件；Collapse 全折叠 / Expand 全展开 / 单文件 chevron；WorkingCopyModal 内无 Expand 按钮且 Collapse 段可用
