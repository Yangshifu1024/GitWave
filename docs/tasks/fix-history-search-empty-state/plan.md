# fix-history-search-empty-state · 历史搜索零匹配时中栏被空态整体替换

## 需求（缺陷报告）

用户在提交历史搜索框输入字符后，中栏完全消失，只剩 "No commits yet"，且搜索框随之消失、过滤器无法清除。真机截图 2026-08-30（分支 feature/macos-native-menu 测试期间发现）。

## 根因（tester 分析）

- `src/components/CommitGraph.tsx:537-543`：`commits.length === 0` 时早返回裸空态，**替换整个面板（含搜索框本身）**。
- 触发链：输入 → 300ms 防抖 → `setFilter` → `getCommitLog(ws, limit, filter)` → 后端 `commit_log`（history.rs:194，扫描 1 万条、message/author 大小写不敏感）**正确**返回 `[]`（查询无匹配，如中文）→ 前端进入零命中空态 → 搜索框随面板消失 → 过滤器卡死。
- 次要问题：文案误导（"No commits yet" vs 实际「无匹配」）。
- **与 F007 菜单改动无关**：`git diff main` 确认 CommitGraph.tsx / history.rs 零改动，main 上的既有缺陷。用户确认在当前 feature 分支直接修（工作区有大量未提交菜单改动，不宜切分支）。

## 方案

`CommitGraph.tsx` 重构渲染结构（行为最小变更）：

1. 保留 `!activeWorkspaceId` / `!activeRepoId` 两个早返回（此时无搜索框合理）。
2. 其余状态不再早返回：新增 `showGraph` 布尔 + `stateContent`（loading / error / 零命中）。
3. 面板与搜索栏**常驻渲染**；内容区按状态切换：`showGraph ? 图区 : stateContent`。
4. 零命中区分两种：有 filter → "No commits match “{filter}”" + **Clear search** 按钮（清空 searchInput 与 filter）；无 filter → 原 "No commits yet"。

## 验证清单

- [x] `npm run typecheck && npm run lint && npm test && npm run format:check`
- [x] tauri dev 热更新后人工验证：搜索框输入无匹配字符（如中文）→ 显示 "No commits match" + Clear 按钮，搜索框仍在；点 Clear → 恢复全量历史；输入有匹配字符 → 正常过滤
- [ ] 非 macOS（win/linux CI）：纯渲染结构重构，无平台分支
