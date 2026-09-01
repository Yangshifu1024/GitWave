# F012 · 应用内凭证恢复（认证失败自动弹窗）

> 状态：已实现（并入 fix/push-tag-non-ff 分支）
> 关联任务：docs/tasks/fix-push-tag-non-ff/plan.md

## 问题

推送 / 拉取 / 获取因凭证缺失或失效失败时，应用只透出错误
（如「no credentials available」），用户只能去终端跑 git 命令重新输入
凭证——流程断裂，且移动端场景（无终端）完全无解。本会话实际发生：
keychain 条目丢失后，应用内 push 无任何自救途径。

## 目标

- 同步操作（push / pull / fetch）认证失败时，应用内自动弹出认证对话框，
  输入用户名 + 访问令牌（PAT）后**原地重试原操作**。
- 可勾选「保存到系统钥匙串」：通过 `git credential approve` 落盘到用户
  已配置的 credential helper（osxkeychain 等），与 git CLI 共享。

## 非目标

- SSH 远端：继续走 ssh-agent，本特性不覆盖（远端为 git@/ssh:// 时
  输入的凭证被忽略）。
- 凭证管理 UI（列表 / 删除）：后续可做，本次不涉及。
- OAuth / 浏览器跳转流程。

## UX 流程

```
push/pull/fetch 撞到认证失败（*_auth_failed）
        │ 自动弹窗（每个操作至多一次，不循环）
        ▼
「需要认证」对话框：远端名 / 用户名 / 访问令牌 / ☑保存到钥匙串
        │ 保存并重试
        ├─ 成功 → 状态区成功文案（勾选保存时 approve 落盘）
        └─ 再次认证失败 → 状态区普通错误（不再弹窗，可再次手动触发）
```

## 技术要点

- Rust：`InlineCredentialProvider`（credentials.rs）按次操作使用输入凭证；
  `approve` 仅在 `remember=true` 时通知 helper 落盘，`reject` 恒为 no-op
  （会话输入的凭证未落盘，无可抹除，绝不误删系统存储）。
- `cmd_push / cmd_fetch / cmd_pull / cmd_delete_remote_branch` 增加
  `auth` 入参，逐层透传到四个网络操作。
- 前端：`isAuthError` 识别三个认证失败码；`authPromptStore` +
  `AuthPromptDialog`（App 全局单实例）；触发点 = useRemoteSync 三操作、
  BranchList 推送、RemotesPanel 单远端 fetch（均一次操作仅提示一次）。
