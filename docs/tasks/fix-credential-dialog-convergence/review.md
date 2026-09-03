# fix-credential-dialog-convergence · 审查报告

- 审查人：code-reviewer 代理（7 维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践）
- 审查对象：分支 `fix/credential-dialog-convergence` 全部未提交改动
  （20 个修改文件 + 新增 `src/lib/authRetry.ts` 与本任务目录）
- 审查时已验证：cargo test 289 全过；cargo fmt / clippy 干净；eslint /
  tsc / vitest / prettier 干净

## 审查确认的优点

- 核心修复正确且防漂移：`helper_command` 统一 fill 与 approve/reject 的
  「死提示环境」，交互禁止通道完备（`GCM_INTERACTIVE=never` +
  `-c credential.interactive=never` + `GIT_TERMINAL_PROMPT=0` + 双
  askpass 指向不存在程序），并配单测断言 `-c` 位于子命令之前。
- 凭证安全：`never` 只拒绝交互，已存凭证（含 GCM OAuth token）照常静默
  返回；非 GCM helper 忽略该变量；`InlineAuth` Debug 脱敏、helper 子进程
  stdout/stderr 均 null、`credential_request` 协议防注入保留——凭证不会
  进日志或错误参数。
- clone 收敛复用既有契约：`provider_for_operation` + `cancelled_if_flagged`
  + `run_sync_op`，与 fetch 完全同构；transfer_progress 取消检查点齐备。
- submodule 的 callbacks 与 approve/reject 为同一 provider 实例，F013
  proxy attach 保留。
- 错误码与 i18n 前后端一致；locale parity 测试通过。

## 发现的问题与处置

| 级别 | 位置 | 问题 | 处置 |
|---|---|---|---|
| 🔴 | `AuthPromptDialog.tsx` footer Cancel | 按钮调 `close()` 绕过 `cancel()`，`onDismiss` 永不触发，await 型调用方（withAuthRetry）的 busy 状态永久挂起 | ✅ 已修：改为 `cancel()` |
| 🟡 | `authPromptStore.ts` `show()` | 直接覆盖 `retry`/`onDismiss`，前一个等待方被丢弃后永久 pending | ✅ 已修：`show()` 先 `cancel()` 结算旧注册者再接管 |
| 🟡 | `ActionBar.tsx` clone F012 | `show()` 未传 `onDismiss`，用户取消后无任何反馈 | ✅ 已修：onDismiss 写状态栏 cancelled 提示 |
| 🟡 | `BranchList.tsx` `remoteDeleted` | 多 remote 时部分成功 + 取消会误报「仅本地删除」 | ✅ 已修：循环内每成功删一个置 true |
| 🟡 | clone 取消无 UI 入口 | 后端取消链路就绪但状态区无 clone 取消按钮 | 📌 记录为后续任务（plan.md 验收项已注明超时兜底） |
| 🟡 | submodule 无 transfer 取消检查点 | 180s 超时翻 flag 后传输不会被中止 | ✅ 已修：`update_with_credentials` 挂 transfer_progress 检查点 |
| 🟢 | `sync_ops.rs` / `lib.rs` / `api.ts` 注释 | 操作清单未含 clone / submodule | ✅ 已补 |
| 🟢 | SSH clone 认证失败弹 F012 无意义 | fetch/pull/push 既有行为，clone 继承 | 📌 既有问题，非本任务引入，留待后续统一按 scheme 过滤 |
| 🟢 | 错误 params 的 url 未脱敏 userinfo | 与 remote.rs 既有模式一致 | 📌 防御性改进，留待后续统一 redact 函数 |
| 🟢 | submodule add 重试幂等性 | 未有自动化测试覆盖 | 📌 列入真机手动验收清单 |

## 本轮补齐的测试

- `src/stores/authPromptStore.test.ts`：cancel 通知 onDismiss；close 不通知；
  `show()` 覆盖时结算旧注册者。
- `src/lib/authRetry.test.ts`：成功不弹窗；提交凭证重试一次；取消 reject
  携带 `git.sync_cancelled`；非 auth 错误直接 rethrow；重试再失败不二次
  弹窗；`remoteHost` 三分支。

## 最终检查（修复后全量复验）

- 后端：cargo test 289 passed；cargo fmt --check / clippy 无警告
- 前端：eslint / tsc / prettier 干净；vitest 161 passed

## 结论

🔴 必修项与高价值 🟡 已全部修复并补充单测；遗留项均为既有问题或后续任务
（已在 plan.md 注明）。**审查通过，可交用户提交并发起 PR。**
