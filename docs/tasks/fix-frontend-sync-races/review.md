# Review · fix-frontend-sync-races（7d4eddf + 生命周期收尾提交）

> 最终态审查记录。初审报告的发现已逐项核对最终代码；本收尾提交修复了
> 初审遗留的监听器生命周期/守卫方向问题，并同步了 plan 偏差节措辞。

## ✅ 优点

- requestId 全链路闭环完整：starter 生成 → invoke options → cmd → use_cases → remote 打标 → 事件回传 → store 按实例匹配，auth 重试经 `{...variables, auth}` 保持同一 id；调用侧普查 8 处 starter 全部带 id，无串台残留。
- 交错/序号单测覆盖真实回归路径：superseded 迟到事件被弃、superseded endOp 被忽略、updater epoch 单调 + fail 清理 + markReady 开窗、无 id 旧事件放行（向后兼容有测试）。
- `platform.ts` 守卫 `typeof navigator === "undefined"`，SSR / 测试默认 POSIX 行为，安全。
- conflictMarkers 锚定 `7 + (空白|行尾)`：`<<<<<<< HEAD` 命中、`>>>>>>>>>>` 拒绝、CRLF 兼容；`lineStartOffsets` 一次预计算。

## 已修复记录

| 初审发现 | 修复方式 |
|---|---|
| 🟡 `formatAppError` 数值 `count` 被过滤 | 初审即误报：最终代码（`api.ts:186-197`）放行 `string \| number` 并有注释与单测，无需处理 |
| 🟡 Modal 会话守卫 fail-open | 本提交：两处改 `!context \|\|` 短路，context 缺失即放弃写入（fail-closed） |
| 🟡 useRemoteSync 监听器无清理、注册可重复 | 本提交：注册改为 async/await + 模块级 promise 去重（StrictMode 双挂载共享一次注册）；双通道任一失败回滚已挂通道后指数退避重试；导出 `teardownSyncProgressListener`，unmount/HMR 释放全部监听 |
| 🟡 ConflictPanel「open 变化自增 seqRef」未实现 | 本提交：`useEffect([open])` 关闭分支自增 `seqRef`，在途响应不再写穿重开后的面板；plan 偏差节措辞已同步 |
| 🟡 `nextSyncRequestId` HMR 后 id 复用 | 本提交：wall clock 种子 + 随机盐后缀 |
| 🟢 SshKeyManager 轮询 timer 卸载延迟清理 | 本提交：timer 句柄入 ref，卸载 effect 直接 `clearInterval` |
| 🟢 endOp fade 定时器同名 op 背靠背隐患 | 本提交：timer 闭包捕获 `endedRequestId`，触发时校验 `activeRequestId` 未变 |
| 🟢 ignorePattern `full` 与 `dir` 推导不自洽 | 本提交：`full` 统一用 `trimmed`（既有测试无空白/尾斜杠输入，语义不变），加注释 |
| 🟢 commitMenu 1970 前提交显示 unknown date | 本提交：仅对 `NaN/Infinity` 回退，负时间戳照常格式化 |

## 遗留（不阻断，可后续处理）

- 🟢 `WorkspaceRepoTabs` 守卫仅覆盖 workspace 维度，同 workspace 内快速连点两个 repo 的两次 IPC 交错仍是后者先落地被前者覆盖——plan 范围外既有行为，记录备查。
- 🟢 `useRemoteSync` 重试逻辑、ConflictPanel/WorkspaceRepoTabs 守卫无组件级测试（项目无组件测试设施，靠 typecheck + lint + 手动验证）。
- 🟢 多 remote fetch-all 共享一个 id 的正确性靠推理（后端顺序发射、store 单槽），未做专项测试。

## 📝 总体评价

核心竞态修复（按实例而非按名匹配）链路、守卫、测试三者对齐；本收尾提交补齐了 plan 承诺的监听器清理与守卫方向修正。`pnpm test`、`tsc --noEmit`、`pnpm lint` 全绿。
