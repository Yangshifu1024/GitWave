# feat-safe-branch-switch

> 单击选中分支；双击切换。切换前检查 merge / rebase / worktree / 未提交改动。

## 行为

| 状态 | UI |
|---|---|
| 当前分支 | 双击无操作 |
| remote | 拦截弹窗 |
| merge / rebase paused | 拦截弹窗 |
| 其他 worktree 占用 | 拦截弹窗 |
| dirty | Cancel / Discard / Stash & switch |
| clean | 直接 checkout |

## 实现

- `gateCheckout`：`src/lib/checkoutGate.ts`
- `checkout_branch(..., force)`：先 `checkout_tree` 再 `set_head`；默认不 force
- Stash & switch：`save_stash`（含 untracked）→ checkout → `pop_stash`；pop 失败则保留 stash
