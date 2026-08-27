# feat-ui-native-studio-v2 · 实施计划

> 状态：执行中  
> 设计依据：[05-visual-redesign.md](../../design/05-visual-redesign.md)、[mockups/v2/index.html](../../design/mockups/v2/index.html)

## 目标

落地 A 方案：Native Studio 布局 + Tide Studio 配色（P0→P1→P2）。

## 批次

### P0 — 骨架 IA + Source List + Sync + Toolbar

- `ListItem` Source List 重写 + pane inset shadow
- `WorkspaceList`（删除 dropdown）
- Sync 下沉至 REPOS/BRANCHES + `SectionAction`
- `ToolbarContextTitle` 居中标题
- `SyncProgressBar` + `syncStore` / `useRemoteSync` + 后端 `sync-progress`

### P1 — Inspector + WC Bar

- `DiffViewer` file bar + gutter
- `WorkingCopyBar` 上阴影 + 列标题

### P2 — 焦点环 + 文档

- 统一 `:focus-visible`
- 更新 `03-layout.md`、`02-components.md`

## 验证

- `pnpm lint` / `pnpm test`
- `pnpm tauri dev` 对照 mockup（palette=tide）
