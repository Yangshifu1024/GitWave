# fix-status-area-progress-toast · 审查报告

审查对象：分支 `fix/status-area-progress-toast` 全部未提交改动（23 个文件 + 3 个新增，删除 `Toast.tsx`）。
审查维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践。

## 审查确认的优点

- Toast 迁移完整：全仓无任何 `toast` / `Toast` / `ToastVariant` 残留；variant 映射逐点核对无语义错误（AI fallback 4 处 info，冲突/失败路径保持 danger，其余成功消息 success）。
- hooks deps 同步更新：`CommandPalette` useMemo 与 `GitignoreEditor` useEffect 的 `toast` 依赖均替换为稳定的 zustand action。
- CSS 根因修复到位：`ProgressBar.Fill` 去掉 `w-full` 与自定义 shimmer，HeroUI 内建 indeterminate 动画恢复；`tokens.css` 死代码清除。
- fade 时序有测试兜底；消息进入 `aria-live="polite"` 区域，a11y 与原 Toast 持平。

## 发现的问题与处置

| 级别 | 问题 | 处置 |
|---|---|---|
| 🔴 | CommandPalette 两条 fetch 路径（静态命令 + AI intent `fetch_remotes`）直接调 `fetchRemote` 而后端会 emit `sync-progress`，全局 listener 会把 `activeOp` 置为 `fetch` 且无人 `endOp` → 状态区永久卡在 "Fetching…"、同步按钮被 `isSyncBusy` 永久禁用 | ✅ 已修复：两处接 `startOp("fetch")` / `endOp("fetch")`，入口加 `isBusy()` 门控 |
| 🟡 | `updateProgress` 仅判 `fading`，本地 op 进行中会被后端进度事件覆盖标签；150ms 清理完成后迟到事件仍可复活 op | ✅ 已修复：三重守卫（`fading` / `activeOp === null` / `activeOp` 为 UiOperation 时忽略） |
| 🟡 | `endOp()` 无归属：跨 op 时旧 op 的清理会误清新 op 的指示（或误设 fading 使新指示淡出） | ✅ 已修复：`endOp(op)` 归属校验，`activeOp !== op` 时为 no-op，清理定时器同样校验 |
| 🟡 | store 为模块级单例，测试间状态泄漏；缺双 `endOp`、跨 op 场景用例 | ✅ 已修复：`beforeEach` 重置；新增 op 不匹配 endOp、双 endOp、UI op 下进度事件被忽略、清理后迟到事件被忽略等用例 |
| 🟢 | 150ms 魔数在 store 与 CSS 双写 | ✅ 导出 `OP_FADE_MS`，`SyncStatusArea` 加互指注释 |
| 🟢 | `LocalOperation` 命名与 `remote-op`（含网络操作）不符 | ✅ 更名 `UiOperation` 并修正注释 |
| 🟢 | `isBusy()` 无调用点 | ✅ 已被 CommandPalette fetch 门控使用 |
| 🟢 | plan.md 的枚举与实现不一致 | ✅ 已回写 |
| 🟢 | fade 结束后结果文本直接出现（无 fade-in）；前序清理 timer 极端时序截断下一 op 的 fade（纯视觉，已验证无状态错误） | 接受 |
| 🟢 | palette 的 checkout/tag/stash、MergeBanner abort、SubmodulesPanel 操作暂无进行中指示（不发进度事件，无卡死风险），与 BranchList 不一致 | 留作后续统一任务 |

## 结论

🔴 已清零，🟡 全部修复，残余均为 🟢 视觉/一致性问题且已记录。`typecheck`、`lint`、`test`（92 用例）全部通过。**可合入**（squash merge，commit 由用户确认执行）。
