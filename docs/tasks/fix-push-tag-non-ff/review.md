# fix-push-tag-non-ff · review（PR #24）

> 审查对象：分支 fix/push-tag-non-ff（e6c5955 tag 修复 + b7f21af F012 + 本修复提交）
> 审查方式：code-reviewer 代理，7 维度；libgit2 1.9.7 上游源码核对
> 结论：初判 **NEEDS_FIXES**（🔴2 / 🟡5 / 🟢6）→ 🔴 全部修复，🟡 修 4 项、
> 1 项（组件级测试）立后续任务 → 复核 **CLEAN**

## 🔴 已修复

1. **多远端 fetch 凭证作用域泄漏**：fetch-all 曾把用户输入的凭证发给每一个
   挑战认证的远端（跨主机泄漏面），且弹窗兜底显示 "origin"。修复：
   - `use_cases::fetch` 多远端循环撞 `FETCH_AUTH_FAILED` 立即中止整批并返回
     （错误 params 携带 `remote` 名，不再继续喂给其他远端）；
   - `remote.rs::fetch` 的 auth 错误 params 增加 `("remote", remote_name)`；
   - 前端 fetch onError 用 `errorParam(e, "remote")` 提取失败远端，重试改为
     **单远端** fetch + auth；弹窗显示真实挑战的远端名。
2. **useRemoteSync 三操作违反「至多提示一次」**：retry 带着输入的凭证再次
   认证失败时 `isAuthError` 仍为真 → 重新弹窗循环。修复：`handleError` 增
   `canPrompt` 参数，三个 onError 传 `variables?.auth === undefined`——
   重试失败只走状态区错误，与 BranchList / RemotesPanel 行为对齐。

## 🟡 已修 4 项

- api.ts `PushOptions` 双重声明（依赖 declaration merging）→ 删除重复。
- AuthPromptDialog 关闭后输入残留（密码滞留组件状态）→ 打开时重置。
- `InlineAuth` 派生 `Debug` 可打印密码 → 手写 Debug 掩码 password。
- skippedTags 名单 force 时带 `+` 前缀 → trim 后再剥 `refs/tags/`。

## 🟡 遗留（后续任务，不阻塞）

- 组件/基建级测试缺口：push 重试梯子（file:// 远端可构造 NotFastForward）、
  InlineCredentialProvider 生命周期、isAuthError 识别——与 PR #23 遗留的
  组件测试基建任务合并跟进。

## 🟢 可选（记录）

- cmd_delete_remote_branch 的 auth 链路暂无前端触发点（保留，后续接弹窗）。
- push_once 每次成功尝试都 approve（幂等，最多 N+2 次 helper 子进程）。
- authRetried key 不含 workspaceId 且仅成功清除（触发面窄）。
- `starts_with("http")` 也会放行明文 http（与 git CLI 一致，文档知悉）。
- 重试梯子最坏 1+1+1+N 次协商，受 run_sync_op 总超时覆盖。
- ActionBar 过期注释已随本轮更新。

## Verdict

🔴 全部修复、🟡4/5 修复、1 项测试债务立后续任务 → **CLEAN**，允许合入。
