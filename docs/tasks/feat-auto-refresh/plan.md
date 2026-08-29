# feat-auto-refresh · 设置新增 General → Auto Refresh（每分钟自动刷新）

## 需求

设置界面新增 General 分区与 Auto Refresh 复选框；勾选后每分钟自动刷新一次，刷新逻辑复用既有原语（等价于原 ⌘R 方案的刷新体，⌘R 快捷键本身被否决不做）。分支：`feature/macos-native-menu`。

## 方案

1. **`src/hooks/useAutoRefresh.ts`**（新增，镜像 usePalette 的 hook + localStorage 惯例）：
   - `STORAGE_KEY = "gitwave-auto-refresh"`，默认 `false`，try/catch 守卫
   - `useAutoRefresh()`：偏好读写（持久化 + state 同步）
   - `useAutoRefreshLoop()`：全局唯一定时器，enabled 时每 60s 执行 `bumpHistoryEpoch()` + `queryClient.invalidateQueries()`
2. **`src/components/SettingsModal.tsx`**：`SettingsSection` 加 `"general"` 置于 SECTIONS 首位（icon `Settings2`），默认分区改为 general；新增 `GeneralSection`——`ui/Checkbox`（text-sm）+ 说明文案（明确不含网络操作）
3. **`src/App.tsx`**：挂 `useAutoRefreshLoop()`（App 级全局单例；无 active workspace/repo 时无害空转）
4. **三个手动 effect 面板**补 `historyEpoch` 依赖（invalidateQueries 触不到它们）：`RemotesPanel` / `WorktreePanel` / `SubmodulesPanel` 的 `refresh` useCallback deps 各加 `historyEpoch`
5. **（增补）状态区显示**：刷新体抽为 `useRefreshRepo()` 共享动作，触发时在状态指示区显示 `Refreshing…`（info）→ `Refreshed`（success）；⌘R / Ctrl+R 手动触发见 [feat-cmd-r-refresh](../feat-cmd-r-refresh/plan.md)
6. **（增补，用户决策）刷新含 fetch**：refresh 动作含 `fetchRemote`（busy 时跳过、失败降级本地），设置文案同步更新；仍绝不 pull / push

## 验证清单

- [x] `npm run typecheck / lint / test / format:check`
- [ ] 手动（HMR）：Settings → General → 勾选 Auto refresh → 终端 `git commit` 外部变更 → 1 分钟内图区 / 分支树 / Remotes / Worktrees / Submodules 反映；取消勾选不再刷新；重启应用勾选状态保持
