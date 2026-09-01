# fix-auto-refresh-toggle · 关闭自动刷新后仍每分钟刷新

> 状态：已修复（待冒烟 / review）
> 需求（用户，bug 报告）：关闭自动刷新似乎无效。

## 根因

1. `src/hooks/useAutoRefresh.ts` 的 `useAutoRefresh()` 用**组件级 `useState`** 存偏好，
   仅在 hook 首次挂载时从 localStorage（key `gitwave-auto-refresh`）惰性初始化一次。
2. 该 hook 有两个互不相干的调用方，各持一份独立 state 实例：
   - `App.tsx` → `useAutoRefreshLoop()`（拥有全局 60s 定时器，读实例 A）
   - `SettingsModal.tsx` `GeneralSection`（开关 UI，读写实例 B）
3. 用户在设置弹窗关闭开关时：localStorage 正确写入 `"false"`、实例 B 重渲染，但
   App 中实例 A 的 `autoRefresh` 仍是挂载时的值 → `useAutoRefreshLoop` 的
   `useEffect`（依赖 `[autoRefresh, refreshRepo]`）不重跑 → 定时器永不清除，且回调
   `() => refreshRepo()` 内无运行时开关检查，无条件执行刷新（bump epoch + 全量
   invalidate + fetch）。设置直到应用完全重启才生效。

这是「初始化时读一次 + 状态不跨组件共享」问题（自 e56d440 引入该功能起即存在），
而非 interval 回调的经典 stale closure——门禁 `if (!autoRefresh)` 只在 effect 建立
时求值一次，而它读到的值被冻结在 App 挂载时刻。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 修复方式 | 偏好移入 zustand store `src/stores/autoRefreshStore.ts`，`useAutoRefresh()` 对外接口不变，内部改订阅 store | 对齐既有 store 模式（`uiStore.ts`）；App 与设置弹窗共享同一份状态，关闭开关立即触发 `useAutoRefreshLoop` effect 重跑并清除定时器 |
| localStorage key 不变（`gitwave-auto-refresh`） | 用户既有偏好无缝迁移 | store 初始值复用 `readStoredAutoRefresh()`；setter 保持 best-effort 持久化语义 |
| interval 回调内实时检查开关 | 防御性双保险 | 每次 tick 用 `useAutoRefreshStore.getState()` 复核，杜绝回调持有过期值类回归 |
| 修复范围 | 仅修 60s 自动刷新定时器 | 已与用户确认：工作副本 2s 轮询（`useWorkingCopy.ts`）、合并冲突 3s 轮询（`useMergeConflicts.ts`）、react-query 窗口聚焦重取**不受该开关控制**，保持现状——它们服务于实时文件变更检测与冲突提示，且设置文案「每分钟刷新仓库数据」描述的就是定时器行为 |
| 单测 | 新增 store 级 `getState()/setState()` 测试；因 store 在模块加载期读 localStorage，需 `vi.stubGlobal` + `vi.resetModules()` + 动态 import（区别于 `uiStore.test.ts` 的静态 import） | 测试栈无 testing-library，hook 渲染级测试做不了；覆盖：初始值读 localStorage、setAutoRefresh 双写、localStorage 异常时内存值仍生效 |

## 改动清单

- 新增 `src/stores/autoRefreshStore.ts`：`autoRefresh` 偏好 + `setAutoRefresh` action
- `src/hooks/useAutoRefresh.ts`：`useAutoRefresh()` 内部从 `useState` 换为订阅 store
  （接口 `{ autoRefresh, setAutoRefresh }` 不变）；`useAutoRefreshLoop` 回调内实时
  复核开关；更新文件头注释
- 新增 `src/stores/autoRefreshStore.test.ts`
- `SettingsModal.tsx` / `App.tsx`：无需改动（接口未变）

## 测试

- `npm run typecheck` / `npm run lint` / `npm test` 全绿
- 手动冒烟要点：
  - 设置弹窗关闭「自动刷新」→ 状态栏不再每分钟出现「刷新中」、不再自动 fetch（等待 > 1 分钟验证）
  - 关闭后 ⌘R / Ctrl+R 手动刷新仍可用（有意不受开关控制）
  - 重新开启 → 等待 1 分钟后自动刷新恢复
  - 重启应用 → 开关状态与关闭前一致（localStorage 持久化未变）
