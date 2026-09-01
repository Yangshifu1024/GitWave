# feat-commit-context-menu · 代码审查报告

- 审查人：code-reviewer 代理（按 `.opencode/agents/code-reviewer.md` 7 维度规范）
- 审查日期：2026-09-01
- 审查范围：`feature/commit-context-menu` 分支全部改动（后端 5 文件、前端 12 文件、文档 3 文件）
- 审查时实测：`cargo test` 通过、`npm run test` 144/144、`tsc --noEmit` 与 `eslint` 无告警

## ✅ 优点

- **后端实现与既有模式高度一致**：`checkout_commit`（src-tauri/src/infrastructure/git/branch.rs）完全复刻同文件 `checkout_branch` 的错误映射、peel-to-tree、`checkout_tree` + `set_head*` 顺序；复用既有错误码 `INVALID_OID` / `COMMIT_NOT_FOUND` / `DIRTY_WORKTREE`，无新增码。libgit2 SAFE 语义 workaround 有清晰 doc 注释与 plan.md 记录。
- **测试充分且有效**：Rust 单测覆盖 detached 切换（含工作树内容交换与「删除已跟踪文件」这一 SAFE 难点）、非法 oid 不动 HEAD、脏工作区拒绝 + force 恢复；`gateCommitCheckout` 等纯函数提取使前端逻辑可测（11 个单测，含 clipboard 降级、嵌套分支名解析）。
- **P1 合规到位**：删除本地分支 / 远程分支 / 标签、hard reset、revert、cherry-pick、脏工作区 discard 检出，全部有显式确认弹窗；禁用态设计合理。
- **架构选点正确**：行菜单项按行渲染、确认/输入弹窗经单一 `useCommitMenuActions` 实例挂在列表根部，避免虚拟列表中每行一份弹窗；`RefBadgeContextMenu` hooks 全部在条件 return 之前，rules-of-hooks 无违规。
- **TagManagerModal 提取为等价搬迁**，CommitInfoHeader 行为无回归；`useActiveRepoState` 复用 `["working-copy"]` query key 共享缓存。
- **useBranchCheckout 忠实镜像 BranchList**：gate 纯函数、Promise.all 预检、stash 降级路径、`startOp("checkout")`/`endOp` 一条不缺。
- **i18n 质量**：en / zh-CN 键完全同步（parity 测试保障），右键选中行、head 徽章穿透、徽章菜单 stopPropagation 均与声明一致。

## 🔴 严重问题（必须修复）

无。未发现数据丢失、权限缺失或逻辑错误：所有破坏性路径均有确认门禁；后端非 force 路径在改动 HEAD / 工作树之前完成全部校验，拒绝路径不影响 HEAD（有测试锁定）。

## 🟡 一般问题（建议修复）— 均已在本分支修复

1. **RefBadgeContextMenu 删除后 invalidate 时序**：`invalidateQueries` 在删除请求发起后同步执行，与删除并发竞争；tags query 无 epoch 兜底，竞态失败会让 TagsPanel 持续显示已删标签。→ **已修复**：invalidate 移入 `run()` 成功回调，与删除落库后执行。
2. **CommitContextMenu 检出未注册 syncStore**：`checkoutOnto` 缺 `startOp("checkout")`/`endOp`，图谱菜单检出期间侧栏可并发检出、忙态不感知。→ **已修复**：补 startOp/endOp，与 useBranchCheckout 对齐。
3. **afterMutation 遗漏 health / reflog 失效**：reset / discard 检出后 HealthPanel 与 ReflogPanel 显示过期数据。→ **已修复**：补 `["health"]`、`["reflog"]` invalidateQueries（对齐 ReflogPanel.afterMutation）。
4. **gateCommitCheckout blocked 文案硬编码英文**：zh-CN 用户会看到英文阻断提示。→ **已修复**：gate 返回 `reason`（不再携带 message），弹窗按 reason 渲染 `commits.menu.blockedMerge / blockedRebase` 双语文案；`checkoutGate.ts` 的同类既有问题记 follow-up。
5. **TagManagerModal 删除标签无确认**：与 P1 有张力，且 F011 扩大了暴露面（应用内 TagsPanel / 徽章删标签均有确认，此处是孤例）。→ **已修复**：补轻量确认弹窗（复用 `repo.tags.deleteDialog.*` 键），行为与 TagsPanel 对齐。

## 🟢 优化建议（可选）

1. checkout_commit 把任何 untracked 文件都判脏，比 `git checkout <commit>` 更保守——属刻意设计（与 F004 门禁粒度一致、保证 stash-and-switch 流程正确），→ **已补 doc 注释 + untracked 拒绝单测锁定**；同 commit detach 防御性测试也已补。
2. 每个可见行挂 1-3 个 ContextMenu 的性能：被虚拟化约束在可接受范围；将来若 memo CommitRow，先把 `state` / 回调稳定化。
3. 异步 setState after unmount：React 18 为无害 no-op，状态提示走全局 store，无需改动。
4. 本地分支删除的禁用提示曾复用 `branches.deleteRemote.currentGuard`（键名误导）→ **已迁移为中性键 `branches.guard.currentBranch`**。
5. `copyCommitInfoText` 的 "Author:" / "Date:" 标签为英文固定格式：属剪贴板数据载荷（非 UI 文案）且日期走 `toLocaleString()`，维持现状。

## 📝 总体评价

整体质量高：后端实现贴合既有抽象且测试到位，前端以「纯函数门禁 + 单实例弹窗控制器 + 共享 hook」的干净分层落地了 Fork 式两级右键菜单，P1 确认门禁与 i18n 纪律执行良好，文档与实现相互印证。无 🔴 问题；5 个 🟡 问题全部在本分支修复并回归验证（`cargo test` 240 通过、`npm run test` 144 通过、clippy / eslint / tsc / prettier 全绿）。剩余 follow-up：`checkoutGate.ts` 既有英文文案 i18n 化、BranchList 迁移到 `useBranchCheckout`。

## 结论

**通过审查，可合入**（所有 🟡 已修复，无阻塞项）。
