# feat-push-confirm · Push 确认弹框（照搬 Fork，含 tags / force 选项）

> 状态：实施完成（待手动冒烟；未提交）
> 需求（用户，2026-08-28）：push 增加确认弹框（截图参考 Fork 的 Push 对话框）。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 弹框要素（照搬 Fork） | 标题 "Push" / 副标题 "Push your local changes to remote repository" / Branch 行（当前分支，只读 mono）/ To 行（`default (origin/main)`，取 upstream，无则 `origin/{branch}`）/ "Push all tags" / "Force push" 勾选 / Cancel + Push | 与 Pull 对话框同款布局语言 |
| Branch / To 不做下拉 | 只读展示 | 后端 push 语义固定为"当前分支推到同名远端分支"；拉非当前分支不在范围（与 Pull 的 Into 只读同理） |
| Push all tags | 枚举 `refs/tags/*` 逐个构造 `refs/tags/x:refs/tags/x` refspec 一并推送 | libgit2 push 通配 refspec 行为不可靠，显式枚举 |
| Force push | refspec 前缀 `+`；服务端保护（如拒绝强推默认分支）由远端返回错误呈现 | 勾选即用户明示意图；应用层不做额外限制 |
| 按钮门控 | 移除 `ahead === 0` 禁用，仅保留无仓库 / 同步中 / detached | 勾选 tags 后即使 ahead=0 也可能要推标签；与 Pull 常亮语义一致，误触由弹框确认兜底 |

## 改动清单

### Rust
- `infrastructure/git/remote.rs`：`PushRequest { tags, force }` + `push_with_options`（替代 `push`；force 加 `+` 前缀，tags 枚举引用构造 refspecs）
- `application/use_cases.rs`：`push` 签名扩展 `tags/force`
- `lib.rs`：`cmd_push` 增加可选 `tags/force` 透传

### 前端
- `src/lib/api.ts`：`PushOptions { remote?; tags?; force? }`，`pushRemote(workspaceId, options?)`
- `src/hooks/useRemoteSync.ts` / `useWorkingCopy.ts`：`push(options?)` 透传
- `src/components/ActionBar.tsx`：Push 按钮打开 `pushDialog`；确认弹框（Branch / To / 两勾选 / Cancel / Push）

## 验证

- [x] `cargo fmt` / `clippy --all-targets`（0 警告）/ `test`（111 通过）
- [x] `npm run typecheck` / `test`（43）/ `lint` / `format:check` / `build` 全绿
- [ ] 手动冒烟：普通 push（ahead>0）；up-to-date push（no-op）；勾选 Push all tags（标签到达远端）；Force push（分歧分支强推覆盖）；远端拒绝（受保护分支强推报错）
