# fix-status-area-progress-toast · 实施计划

## 背景

两个问题：

1. 现有所有 Toast（HeroUI 顶部弹窗，16 个文件约 40 个调用点）应统一改用 ActionBar 中央的状态指示区域（`SyncStatusArea`）显示。
2. 状态指示区域"进行中"的蓝色循环进度条不可见：点击操作后区域空白、完成后直接变绿。

## 根因

### 进度条不可见（三个叠加原因）

- **CSS 冲突（主因）**：`SyncStatusArea.tsx` 的 `ProgressBar.Fill` 带 `w-full`（Tailwind utilities 层），覆盖了 HeroUI indeterminate 动画依赖的 `width: 40%`；但 HeroUI 的 `translate(-100% → 350%)` 动画仍作用于 Fill。100% 宽的填充条每个 1.5s 周期约 55% 的时间完全滑出 `overflow-hidden` 轨道，视觉上就是空白/频闪。应用自带的 `.sync-progress-indeterminate::after` shimmer 定位在 Fill 内部，Fill 被甩出轨道时随之消失，无法补位。
- **状态流缺口**：仅 fetch/pull/push 经 `syncStore.startOp` 进入 "sync" 态；checkout / stash / worktree / merge / rebase / remote 管理等操作从不显示进行中，期间底条为 `bg-border-subtle`（视觉≈空白），完成后直接变绿。
- **时序**：`endOp` 立即置 `fading: true` 使 `syncing = false`，蓝条瞬间卸载，快速操作连一帧都渲染不出；150ms 只是清理延时，没有实际淡出过渡。
- **隐患**：`endOp` 之后迟到的 `sync-progress` 事件会把 `activeOp` 重新置为非 null 且 `fading = false`（`updateProgress` 无守卫），可能永久卡在 "sync" 态。

### Toast

`statusAreaStore.StatusVariant` 仅有 `success | danger`，无法表达 Toast 的 info 语义；调用点分散在 16 个文件。

## 方案

### A. Toast → 状态指示区域

1. `statusAreaStore.StatusVariant` 扩为 `"success" | "danger" | "info"`（warning 无调用点，不加）。
2. `SyncStatusArea` 的 `AreaState` / `TEXT_COLOR` / `BAR_COLOR` 增加 `info`（`text-accent` / `bg-accent`）。
3. 全部调用点替换 `toast({ title, variant })` → `setStatus(text, variant)`：
   - 映射：`danger → danger`、显式 `info → info`、默认（语义为成功）→ `success`。
   - 封装点改内部即可：BranchList `showNotice`、RemotesPanel `run()`、CommitInfoHeader 的 TagManagerModal 回调。
4. 删除 `src/components/ui/Toast.tsx`、`ui/index.ts` 导出、`App.tsx` 的 `ToastProvider`。
5. 行为变化：提示持久显示直到被下一次操作覆盖（Toast 的 4s 自动消失取消）；模态框内操作的结果在模态关闭后可见；长文本由状态区域 `truncate` 截断。

### B. 进行中进度条修复

1. **CSS**：`Fill` 去掉 `w-full` 与 `sync-progress-indeterminate`，保留 `h-full rounded-none bg-accent`，完全交给 HeroUI 内建 indeterminate 动画（react-aria 在 indeterminate 时不输出 `aria-valuenow`，动画选择器命中已验证）。删除 `tokens.css` 中 `@keyframes sync-progress-shimmer` 与 `.sync-progress-indeterminate::after` 死代码。
2. **泛化 running 态**：`syncStore.activeOp` 类型扩为 `SyncOperation | UiOperation`（`UiOperation` = checkout / delete / merge / rebase / stash / worktree / remote-op，`lib/api` 不动），`syncOperationLabel` 更名 `operationLabel` 并补文案；接线 `startOp`/`endOp(op)` 到 BranchList（checkout、`run()` 包装的 merge/rebase/delete 等）、ActionBar（stash、worktree）、RemotesPanel（`run()`）、CommandPalette（两条 fetch 路径）。
   - `endOp(op)` 带归属校验：`activeOp !== op` 时为 no-op（防跨 op 误清 / 误淡出）；清理定时器同样校验。
   - `updateProgress` 三重守卫：`fading`、`activeOp === null`、`activeOp` 为 UiOperation 时忽略后端进度事件（防迟到事件复活 op、防覆盖 UI 操作）。
   - 常量 `OP_FADE_MS = 150` 导出，与 `SyncStatusArea` 的 `duration-150` 过渡互指注释。
3. **时序**：`SyncStatusArea` 改 `syncing = activeOp !== null`，fading 期间对内容区加 `opacity-0 transition-opacity duration-150` 实现真正淡出交接；`syncStore.updateProgress` 加 `fading` 守卫忽略迟到事件。

## 验证

- 新增单测：syncStore（迟到事件守卫、endOp 清理）、statusAreaStore（info variant）。
- `npm run typecheck && npm run lint && npm run test`。
- code-reviewer 审查通过后由用户确认 commit（AI 不自动 commit，P1 约束）。
