# fix-remove-head-badge

> 移除 commit 图表 / Inspector 中的字面 HEAD 徽章；当前提交与当前分支改由"当前分支高亮 + emphasize 强调色"表达（Fork 惯例）。

## 背景

HEAD 恒存在且每仓库唯一，其位置等价于"当前分支"，单独渲染一个 `HEAD` 徽章不增加信息（终端 `git log --decorate` 显示它只是因为没有高亮手段）。此前 HEAD 徽章还承担了提交图表"当前提交高亮"的唯一数据源（`CommitGraph.tsx` 的 `isHead` 派生），移除时必须一并解决高亮替代来源。

## 方案

- **后端**（`src-tauri`）
  - `domain/history.rs`：`CommitRefKind` 删除 `Head` 变体及文档提及。
  - `infrastructure/git/history.rs`：`collect_commit_refs` 删除 HEAD push 块；排序 rank 删除 `Head => 0` 臂；更新 `commit_log_linear_returns_all_in_lane_zero` 测试断言（tip 只需带 `main` 装饰）。
- **前端**（`src`）
  - `lib/api.ts`：`CommitRef["kind"]` 联合类型去掉 `"head"`。
  - `RefBadge.tsx`：删除 head 分支；文档注释改为"当前分支用 emphasize 表达"。
  - `RefBadgeContextMenu.tsx`：删除 head 早退分支（此后无 head 徽章，菜单覆盖所有徽章）。
  - `CommitGraph.tsx`：`isHead` 数据源从 HEAD 装饰改为 `menu.headSha === commit.sha`——与右键菜单判断"当前 HEAD"同源（working-copy tip），detached 时 `headSha` 仍指向 detached commit，高亮行为不变。
  - `CommitInfoHeader.tsx`：当前分支不再伪造成 head，映射为 `local_branch` + `emphasize={b.is_current}`（复用 RefBadge emphasize 强调色）。
- **不做**
  - 设计 mockup（`docs/design/03-layout.md`、`05-visual-redesign.md`、`mockups/*.html`）中的 "HEAD" 字样属历史设计稿，不改。
  - i18n 无变更（HEAD 文案来自后端 ref name，无独立 key）。
  - 不新增后端 `is_head` 字段（复用 `useActiveRepoState` 的 `headSha`）。

## 验证

- `cargo test`（src-tauri 全量）；前端 typecheck / vitest 通过。
- 手动：图表当前提交仍有节点光圈 / 行高亮 / 当前分支徽章强调，HEAD 徽章不再出现；切分支后高亮跟随；Inspector 头部当前分支徽章仍以强调色区分。
