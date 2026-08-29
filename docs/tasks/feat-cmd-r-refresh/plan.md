# feat-cmd-r-refresh · ⌘R / Ctrl+R 刷新仓库数据

对应需求：设置 Auto Refresh（[feat-auto-refresh](../feat-auto-refresh/plan.md)）落地后，用户要求手动刷新入口，且**两种触发都要在状态指示区显示动作**。分支：`feature/macos-native-menu`。

## 方案

1. **`src/hooks/useAutoRefresh.ts` 新增 `useRefreshRepo()`**：共享刷新动作——`setStatus("Refreshing…", "info")` → 本地 `bumpHistoryEpoch()` + `queryClient.invalidateQueries()` → **`fetchRemote`（复用既有 fetch 管线：credential helper、startOp/endOp、busy 时跳过）** → 成功后再本地重读一次让远端新提交立现 → `setStatus("Refreshed")`；fetch 失败降级为本地刷新结果并 danger 报错。绝不 pull / push。
2. **`src/components/Toolbar.tsx`**：既有全局 keydown 扩展 ⌘R / Ctrl+R 分支（`e.repeat` 守卫），调 `useRefreshRepo()`。默认菜单已替换，⌘R 无 webview reload 冲突；三平台一致。
3. **`useAutoRefreshLoop`** 改为复用 `useRefreshRepo()`，定时器每 60s 触发时同样走状态区显示。

## 状态区显示

- 触发即显示 info 态 `Refreshing…`；全部 refetch 结束后 success 态 `Refreshed`（失败 danger）。状态常驻至下一操作覆盖（statusAreaStore 语义）。

## 验证清单

- [x] `npm run typecheck / lint / test / format:check`
- [ ] 手动（HMR）：⌘R / Ctrl+R → 状态区出现 Refreshing… → Refreshed，图区 / 分支树 / 侧栏同步刷新；按住不重复触发；Auto Refresh 每分钟触发时状态区同样显示
