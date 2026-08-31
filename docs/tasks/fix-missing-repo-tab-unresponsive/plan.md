# fix-missing-repo-tab-unresponsive · 仓库被移动后 repo tab 卡死、无法操作和删除

> 状态：已修复（待冒烟 / review）
> 问题（用户，2026-09-01）：workspace 中仓库被移动的情况下，tab 页卡死，无法响应左右键点击，无法操作和删除。

## 根因

三层叠加，前两层为确定原因，第三层为可能原因（网络盘场景）：

1. **missing tab 被完全剥夺指针事件（直接根因）**：`WorkspaceRepoTabs.tsx` 对
   `status === "missing"` 的 tab 传 `disabled` → HeroUI `isDisabled` 渲染
   `data-disabled="true"` 并应用 `status-disabled`，该 utility 含
   `pointer-events: none`（`@heroui/styles` tabs.css + utilities/index.css）。
   于是 missing tab 左键（切换）、右键（`onContextMenu`）、拖拽 pointerdown
   全部收不到事件——而 Move Left/Right / Relink / Remove 的**唯一 UI 入口**就是
   右键菜单，导致「无法操作、无法删除」。`feat-repo-tab-drag-reorder/review.md`
   曾记载该 disabled 样式行为，但当时误以为右键菜单仍可用。
2. **启动恢复不校验 repo 有效性（放大器）**：`App.tsx` 恢复时直接
   `selectWorkspace(ws.id, repoId ?? ws.last_active_repo_id)`。lastActive 指向已
   失效 repo 时，activeRepoId 停留在 missing repo 上 → BranchList / WorktreePanel
   等面板查询持续失败 → `ErrorAlert`（阻塞式全屏 Modal）弹出；其 `message` 归零
   会重置 dismissed，每次 historyEpoch bump / 60s 自动刷新后**重弹**，弹窗期间
   整窗不可点。
3. **`cmd_list_repos` 在主线程做文件系统探测（可能原因，网络盘）**：它是 sync
   命令，Tauri 2 下在 UI 线程执行；内部 `refresh_repo_presence` 逐 repo
   `git2::Repository::open` 探测。本地路径快速失败无碍；repo 位于断连的网络盘 /
   NAS 时 stat 可挂起数十秒 → 整窗冻结，同为 sync 的 `cmd_delete_workspace` 也
   无法执行。60s 自动刷新会周期性重打该命令。

产品预期对照（`docs/pm/features/F002-repo-ingestion.md`）：路径失效应「标记
missing + 可 relink / 可 remove」，绝不能卡死。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| missing tab 不用 `disabled` | 移除 prop，改为纯视觉降级（`opacity-60`） | HeroUI disabled = `pointer-events:none`，连 contextmenu/pointerdown 一起杀死；右键菜单是 relink/remove 唯一入口，必须可达 |
| 左键点击 missing tab | 不激活、不跳选 | `onValueChange` 门控：目标是 missing 则 return（Tabs 受控，选择停留）；tooltip 已引导「右键 relink」 |
| active repo 失效时行为（用户确认） | 自动切到 workspace 内第一个可用 repo，全无则置空 | 防止面板查询持续失败 + ErrorAlert 反复弹出；同时兜住「persisted id 已不存在」的 stale 场景（原代码注释声称 skip stale ids 但未实现） |
| 启动恢复校验位置 | `App.tsx` 恢复时先 `fetchQuery` 预热 `["repos", ws]` 缓存再选目标 | 避免首帧先落在 missing repo 上再被纠正的弹窗闪烁；缓存被 tab strip 复用，无额外往返 |
| 运行中失效的兜底 | `WorkspaceRepoTabs` 守卫 effect（query `isSuccess` 后判定） | 60s presence sweep 标记 missing 后自动切走；`isSuccess` 门闸避免加载中误清空 activeRepo |
| `cmd_list_repos` 移出主线程（用户确认） | 改 async + `spawn_blocking`，闭包内 `AppContext::new(workspaces)` 调 use case | 复用 clone/fetch 既有模式；**不改 `list_repos` use-case 签名**（它有 9 个调用点含测试），比原计划的小 |
| removeMut 失败静默 | 补 `onError` → 关确认弹窗 + `setActionError` | 删除失败不再无声；避免确认弹窗与 ErrorAlert 叠两层 |
| 守卫 effect 与手动切换的竞态（review 🟡） | `.then`/`.catch` 内先读 store 快照，`activeRepoId` 已被用户改走则放弃写入 | IPC 往返期间用户点击了别的 tab 时，守卫不再覆盖用户选择 |
| remove 后空选中（review 🟡） | 守卫放开 `!activeRepoId` 早退：无选中且存在可用 repo 时自动选第一个 | 与启动恢复 fallback 语义对齐；「Remove 后切到剩余可用 repo」从靠运气变成保证 |
| workspace 切换的同步校验（review 🟡） | 抽 `pickRestoredRepo` 纯函数 + `useValidatedWorkspaceSwitch` hook，Switcher / Dropdown / ActionBar import 路径统一接入 | 已缓存 repos 的 workspace 切换不再闪现一次错误弹窗；未缓存的仍由守卫 effect 异步纠正 |
| SR 可达性（review 🟡） | missing 徽标 dot 改 `aria-hidden` + `sr-only` 文本 | 显式不用 `aria-disabled`：HeroUI 对 `aria-disabled` 同样应用 `pointer-events:none`，会复现本 bug |
| 启动预热快速失败（review 🟡） | `fetchQuery` 传 `retry: 0` | 默认 retry(3) 在后端故障时会把启动恢复拖住约 7s；加载错误仍由 tab strip 自己的 query 呈现 |
| 网络盘瞬时抖动 → sweep 误标 missing → active 被切走 | 接受此权衡 | 路径恢复后 sweep 自动 flip 回 Active，用户手动点回即可；换取永远不落入「active 指向失效 repo」的错误循环 |

## 改动清单

- `src/components/WorkspaceRepoTabs.tsx`
  - 去掉 missing tab 的 `disabled`，改 `opacity-60` 视觉降级（右键菜单 / 拖拽恢复可达）
  - `onValueChange` 增加 missing 门控（不激活）
  - 新增守卫 effect：active 选中 missing / 不存在 / 为空时自动切到可用 repo（含竞态防护）
  - missing 徽标 dot 改 `aria-hidden` + `sr-only` 状态文本
  - `removeMut` 补 `onError`
- `src/App.tsx`：启动恢复先预热 `["repos", ws]`（`retry: 0`）并经 `pickRestoredRepo` 校验，只恢复到仍存在的 repo
- `src/lib/repoSelection.ts`（新增）：`pickRestoredRepo` 纯函数（target 有效保留 / 否则第一个非 missing / 否则 null）
- `src/lib/repoSelection.test.ts`（新增）：上述边界（missing target / stale id / null / 全 missing / 空表）单测
- `src/hooks/useValidatedWorkspaceSwitch.ts`（新增）：workspace 切换时按缓存同步校验 lastActiveRepoId
- `src/components/WorkspaceSwitcher.tsx` / `WorkspaceDropdown.tsx` / `ActionBar.tsx`（import 路径）：切换统一走 `useValidatedWorkspaceSwitch`
- `src/components/ui/Tabs.tsx`：`disabled` prop 注释警告 HeroUI disabled 含
  `pointer-events:none`（交互型 tab 勿用）
- `src-tauri/src/lib.rs`：`cmd_list_repos` 改 async + `spawn_blocking`（网络盘
  探测不再冻结主线程 / 整窗）
- `src-tauri/src/domain/error_codes/cmds.rs`：新增 `LIST_REPOS_TASK_JOIN`
- `src/i18n/locales/{en,zh-CN}/errors-cmds.json`：新错误码文案

## 测试 / 验证

- `cargo check` / `cargo test` 全绿
- `npm run typecheck` / `npm test` / `npm run lint` / `npm run build` 全绿
- 手动冒烟要点：
  - 添加仓库 → 移动 / 重命名其目录 → 重启 app：tab 带 missing 徽标，**右键菜单可
    打开**，Move Left/Right、Relink、Remove 全部可达
  - 左键点击 missing tab：不激活、不跳选，tooltip 引导 relink
  - Relink 指回新路径 → 状态恢复 active
  - Remove → 确认弹窗 → repo 被移除，active 切到剩余可用 repo（或空态）
  - lastActive 指向 missing repo 时重启 → 直接恢复到可用 repo（或空态），无错误
    弹窗风暴
  - app 运行中移走 active repo 目录 → 60s sweep 后 active 自动切走，无弹窗循环
  - missing tab 可拖拽重排（F005 恢复完整行为）
