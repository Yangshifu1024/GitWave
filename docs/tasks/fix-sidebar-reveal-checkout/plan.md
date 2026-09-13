# fix: sidebar reveal on checkout from any entry point

## 缺陷

在 history 提交图右键分支徽章（如 `fix/abc`）执行 checkout 后，左侧分支列表既不展开 `fix` 前缀分组，也不选中该分支——尽管数据已刷新。

## 根因

checkout 有多个入口，但「reveal（展开分组/前缀文件夹 + 选中）」只挂在 `BranchList` 自己的 `onSwitched` 回调上：

- 侧边栏双击/右键 → `BranchList` 的 `useBranchCheckout({ onSwitched })` ✅
- history 右键 → `RefBadgeContextMenu` 的 `useBranchCheckout()` **未传 `onSwitched`**，跨组件也无通知通道 ❌

## 修复（2 文件）

- `src/hooks/useBranchCheckout.tsx`：新增模块级 broadcast —— 导出 `onBranchRevealed(listener)` 订阅（返回退订函数），`switchTo` 成功后 `emitBranchRevealed(req.target)`。所有 hook 消费者自动获得通知；`options.onSwitched` 保留向后兼容。
- `src/components/BranchList.tsx`：`useEffect` 订阅 `onBranchRevealed` → `splitBranchPrefix(target)` 展开 `local` 分组与 `local:<prefix>` 文件夹（显式展开覆盖默认折叠规则）并 `setSelectedName(target)`；自身 `onSwitched` 瘦身为只 `refresh()`，消除重复。

命令面板 `CommandPalette` 直接调 `checkoutBranch` API，走独立路径，本次不扩（后续可统一走 hook）。

## 验证

- `pnpm typecheck` / `pnpm lint` / `prettier --check` 全绿；`pnpm test` 181 passed。
- 手工回归点：history 右键 checkout `fix/abc` → 侧边栏 `fix` 分组自动展开且该分支高亮；侧边栏自身 checkout 不回归；卸载侧边栏（切 workspace）后无泄漏退订。

## 分支与提交

- 分支 `fix/sidebar-reveal-checkout`（基线 main，含 v0.8.1 bump 之后）
- commit：`fix(branches): reveal sidebar selection on any checkout entry point`
