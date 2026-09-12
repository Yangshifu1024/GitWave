## ✅ 优点

- requestId 全链路闭环完整：starter 生成 → invoke options → cmd → use_cases → remote 打标 → 事件回传 → store 按实例匹配，auth 重试经 `{...variables, auth}` 保持同一 id；`lib.rs` 以 `move` 闭包带走 `String` 所有权，无 `&str` 悬垂。
- 交错 / 序号单测覆盖到位（syncStore 双实例、updater epoch 单调 + fail 清理 + markReady 开窗）。
- `platform.ts` 守卫 `typeof navigator === "undefined"`，SSR / 测试默认 POSIX 行为，安全。
- conflictMarkers 锚定 `7 + (空白|行尾)`：`<<<<<<< HEAD` 命中、`>>>>>>>>>>` 拒绝、CRLF（`\s` 含 `\r`）兼容；`lineStartOffsets` 一次预计算 + 越界回退 `?? resolved.length`。

## 🔴 严重问题（必须修复）

无

## 🟡 一般问题（建议修复）

- **位置**：`src/lib/api.ts` formatAppError
- **描述**：blocklist 本身完备（`ns/lng/keySeparator/interpolation/context` 等 24 项，`count` 依约保留），但新增 `typeof value === "string"` 过滤把数值型 `count` 一并丢掉，复数模板（`_one/_other`）收不到 `count`，与偏差节"count 保留给复数模板"的承诺矛盾；其它数值 params 行为亦静默变更。
- **建议**：放行 `string | number`（或至少特判 `count` 为 number 时保留）。

- **位置**：`src/components/PrDescriptionModal.tsx` / `CommitExplainModal.tsx` 会话守卫
- **描述**：`if (context && context.session !== sessionRef.current) return` 为 fail-open；`context` 缺失（理论上 `onMutate` 未执行即回调）时陈旧响应直接写穿。
- **建议**：改为 `if (!context || context.session !== sessionRef.current) return`，或注明 `onMutate` 必执行的假设。

- **位置**：`src/hooks/useRemoteSync.ts` syncUnlisten
- **描述**：`syncUnlisten` 仅用于第二通道失败回滚，无 unmount / HMR 清理路径；HMR 重载后旧监听泄漏且模块级变量重置。
- **建议**：导出清理函数或在 effect 返回中调用；生产单实例影响小，可注明为 dev-only。

- **位置**：`src/stores/syncStore.ts` nextSyncRequestId
- **描述**：模块级自增序列在 HMR 重载后归零，飞行中操作可能出现 id 复用（生产无 HMR，仅开发期风险）。
- **建议**：加随机后缀（如 `${op}-${seq}-${Math.random().toString(36).slice(2)}`）或在注释声明 dev-only 约束。

- **位置**：`src/lib/ignorePattern.ts` deriveIgnorePatterns
- **描述**：`dir` 由 `trimmed` 推导，`full` 却保留原始 `path`（含尾部斜杠/空白），`full: "foo/"` 与 `dir: undefined` 自相矛盾。
- **建议**：`full` 同样用 `trimmed`，或注释说明保留原串是有意为之。

- **位置**：`docs/tasks/fix-frontend-sync-races/plan.md` 实施偏差节
- **描述**："面板 open 变化时自增失效旧请求"与实现不符——`ConflictPanel` 仅在 `openFile` 调用时递增 `seqRef`，open 变化只靠 `openRef` 拦截，无自增。
- **建议**：修正措辞为"close 后响应经 openRef 丢弃；重开同文件经新 seq 丢弃"，其余偏差描述与 diff 一致。

## 🟢 优化建议（可选）

- **位置**：`src/components/SshKeyManager.tsx` startAgent 轮询
- **描述**：卸载后靠下一次 tick（最长 2s）才 `clearInterval`，timer 句柄未存 ref 直接清理。
- **建议**：将 timer 存入 ref，卸载 effect 中直接 `clearInterval`。

- **位置**：`src/stores/syncStore.ts` endOp fade 定时器
- **描述**：超时回调只比对 `activeOp === op` 不比对 `requestId`，今日靠 `fading` 标志已安全，但同名 op 背靠背结束时旧 timer 可能清掉新 occupant 的残留态。
- **建议**：闭包捕获 `requestId` 并比对 `activeRequestId`，加固。

- **位置**：`src/lib/commitMenu.ts` copyCommitInfoText
- **描述**：1970 年前提交（负时间戳）显示 "(unknown date)"，属装饰性误伤。
- **建议**：仅对 `NaN/Infinity` 回退，负值照常格式化；不改亦可。

## 📝 总体评价

改动围绕"按实例而非按名匹配"主线，链路、守卫、测试三者对齐，3.6 打包项均有独立注释与单测，无 PJ 级风险。优先修复 `formatAppError` 数值 `count` 被过滤一处（唯一与注释承诺相悖的行为变更），其余为文档措辞与 dev-only 加固。
