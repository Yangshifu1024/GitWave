# feat: 冲突面板布局调整 + Merge 提示条

分支：`feat/conflict-panel-layout`（自 main）。
批准计划见会话记录；本文为执行摘要。

## 需求

1. 左侧文件列表只显示文件名，hover tooltip 全路径；样式对齐 commit modal 的
   unstaged 列表（`ui/FileListItem.tsx`：`px-3 py-1.5 rounded-md text-xs` sans、
   hover `bg-bg-secondary`、选中 `bg-accent/10`、focus ring、原生 `title`）。
2. 三列对比顺序改为 OURS | BASE | THEIRS，内容 mono 字体。
3. 右侧头部拆两行：第一行 = 路径 + 上一个冲突（跳转）+ 冲突数（随内容实时刷新）
   + 下一个冲突（跳转）；第二行 = Use ours / Use theirs / Explain / Mark resolved。
4. 底部预览框保持可编辑，高亮每个冲突 region（含 `<<<<<<<`/`=======`/`>>>>>>>` 标记行）。
5. Merge in progress 提示条移入 `App.tsx` 的 `<Toolbar />` 与 `<ActionBar />` 之间
   （常规文档流），右侧加 Resolve 按钮打开冲突面板；面板不再自动全屏接管，且可关闭
   （✕ / Escape）。

## 实现

- `src/lib/conflictMarkers.ts`：`findConflictRegions(text)` 逐行状态机 → `{start,end}`
  0 基行号闭区间；兼容 diff3 `|||||||`；未闭合 region 算到末行；标记须行首。
  单测 `conflictMarkers.test.ts`。
- `src/hooks/useMergeConflicts.ts`：状态提升 hook（3s 轮询 merge 状态 + 冲突列表，
  暴露 `{active, files, refresh, abort}`），App 持有并下发，ConflictPanel 改受控
  （`open/onClose`），移除内部轮询与旧 fixed banner。
- `src/components/MergeBanner.tsx`：提示条（警示图标 + Merge in progress + 冲突数 /
  全解决文案；右侧 Resolve（0 冲突 disabled）+ Abort merge）。
- `ConflictPanel.tsx` 重构：上述 1–4 项 + overlay 高亮编辑器
  （透明文字 `<textarea wrap="off">` 叠同规格 `<pre>` 背板，`onScroll` 同步；
  hunk 跳转 = `scrollTop = start×20px − 8` + `setSelectionRange` + focus）。
- `tokens.css`：`--color-conflict-region-bg` / `--color-conflict-marker-bg`
  （亮色 @theme + 两处 dark 覆盖），组件用 `bg-conflict-region-bg` 等工具类。

## 验收

- vitest 全量（含新增解析器测试）、typecheck、build 通过。
- 不改变 resolve / abort / explain 后端流程与 3s 轮询节奏（移入 hook）。
