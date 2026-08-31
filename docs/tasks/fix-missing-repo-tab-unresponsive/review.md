# fix-missing-repo-tab-unresponsive · code review

> 审查人：code-reviewer（按 `.opencode/agents/code-reviewer.md` 七维度）
> 日期：2026-09-01
> 对象：worktree `D:/Code/GitWave-missing-repo-tab-fix` 未提交改动（分支
> `fix/missing-repo-tab-unresponsive`，基于 main@4f47971，+89/−11 起审）
> 审查实测：`tsc --noEmit` 通过；`vitest` i18n parity 5 用例通过；`cargo check`
> 通过；并对照 `node_modules/@heroui/styles` 产物 CSS 核实了根因主张
> （`data-disabled="true"` → `status-disabled` → `pointer-events: none`，且
> `aria-disabled="true"` 同样触发）。

## ✅ 优点

- 根因诊断准确、修复分层互补：交互可达性（去 disabled）、启动恢复校验
  （fetchQuery 预热）、运行中兜底（守卫 effect）、后端线程模型
  （spawn_blocking）四层各自成立且互相兜底，改动面克制（Rust 侧未动
  use-case 签名）。
- Rust 侧严格复用既有模式：`cmd_list_repos` 与 `cmd_clone_repo` /
  `cmd_list_remotes` 完全同构（`Arc::clone(&ctx.workspaces)` →
  `spawn_blocking` → 闭包内 `AppContext::new` → TASK_JOIN 错误码）。
  `State<'_, AppContext>` 借用在首个 `.await` 前结束，`AppError` / `RepoRef`
  全字段 `Send`，无生命周期问题。
- 错误码 ↔ i18n 一致性有静态守护：`LIST_REPOS_TASK_JOIN` 与 zh/en
  `errors-cmds.json` 由 `parity.test.ts` 双向校验；`{{error}}` 占位符与 Rust
  params 对齐。
- 守卫 effect 终止条件完备：`isSuccess` 门闸避免加载中误清空；TanStack
  structural sharing 保证数据未变的 60s refetch 不产生新 `repos` 引用，
  effect 不会空转——未发现无限循环路径。
- `App.tsx` 恢复链健壮：`fetchQuery` 与 tab strip 共享 `["repos", ws.id]`
  同 key 缓存，校验零额外往返；async then 内任意异常均被外层 `.catch` 兜住。
- `removeMut` 补 `onError` 消除静默失败；注释均解释动机，可读性高。

## 🔴 严重问题（必须修复）

未发现。

## 🟡 一般问题（建议修复）→ 处理记录

| # | 审查项 | 处理 |
|---|---|---|
| 1 | 守卫 effect stale-closure 竞态：IPC 往返期间用户手动切 tab，守卫 `.then` 会覆盖用户选择 | **已修复**：`.then` / `.catch` 内先读 `useWorkspaceUiStore.getState().activeRepoId`，与发起时不一致则放弃写入 |
| 2 | 删除 active repo 后置 null，守卫对 null 早退 → 空态而非「切到剩余可用 repo」，与 plan 冒烟第 5 条不符 | **已修复（审查建议方案 a）**：守卫放开 null 分支，无选中且存在非 missing repo 时自动选第一个，与启动恢复 fallback 语义对齐 |
| 3 | 键盘/读屏：Enter/Space 触发的选择被门控静默吞掉；警示圆点 aria-label 挂在无 role 的 span 上不可靠 | **已修复 SR 部分**：dot 改 `aria-hidden` + `<span className="sr-only">` 状态文本。刻意不用 `aria-disabled`（HeroUI 对其同样应用 `pointer-events:none`，会复现本 bug）。门控静默无反馈保留现状（tooltip 已引导），键盘 Menu 键打开右键菜单列为后续候选 |
| 4 | 运行中切换 workspace 只靠异步兜底，lastActive 失效时可能闪现一次错误弹窗 | **已修复（缓存路径）**：新增 `useValidatedWorkspaceSwitch`（`pickRestoredRepo` + `getQueryData(["repos", id])` 同步校验），Switcher / Dropdown / ActionBar import 路径统一接入。未缓存（从未访问过的 workspace）仍由守卫 effect 异步纠正 |
| 5 | `fetchQuery` 沿用默认 retry(3)+退避，后端故障时启动恢复最长阻塞约 7s | **已修复**：预热传 `retry: 0`，快速失败落到空态；加载错误仍由 tab strip 自己的 query 呈现 |
| 6 | fallback 挑选逻辑内联在组件/effect 中，核心边界无单测 | **已修复**：提取 `src/lib/repoSelection.ts` 纯函数，`src/lib/repoSelection.test.ts` 覆盖 missing target / stale id / null / 全 missing / 空表（+6 用例） |

## 🟢 优化建议（可选）→ 不做（记录为后续候选）

- 预热 `staleTime`：避免恢复落地后 tab strip 挂载即后台 refetch 做第二次
  presence sweep（本地无感，量级小）。
- `activateRepo` 与守卫 effect 的切换序列参数化合并。
- missing tab 聚焦时支持 Menu 键 / Shift+F10 打开右键菜单（纯键盘用户目前
  仍无法触达 Relink/Remove——既有缺口，非本次引入）。

## 📝 总体评价

分层修复思路清晰且与既有代码模式高度一致，三层根因（disabled 剥夺指针事件、
启动恢复不校验、sync 命令主线程探测）均有对应改动并互相兜底；无必须修复的
严重问题；🟡 6 项已全部处理（5 项代码修复 + 1 项部分修复并记录后续候选），
🟢 3 项按改动面控制原则记录不做。

复审验证（🟡 修复后重跑）：`npm run typecheck` / `npm test`（18 files,
127 tests，含新增 `repoSelection.test.ts`）/ `npm run lint` / `npm run build`
全绿；后端本轮无改动，`cargo check` / `cargo test`（208 passed）结论仍有效。
