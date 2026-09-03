# feat-collapse-commit-message · 右侧栏提交消息点击展开/折叠

> 状态：已完成（实现 + 自动验证通过，待人工冒烟）
> 需求（用户）：右侧栏顶部的提交消息标题可点击展开/折叠；折叠时只显示提交消息标题（第一行），且不换行。

## 方案要点

- 折叠范围仅限提交消息本身（subject + body）：作者/日期、分支/标签徽章、Explain / Tag / Cherry-pick / Revert 按钮行始终可见
- 折叠态 subject 单行截断（Tailwind `truncate`，省略号结尾）；展开态与现状一致（`break-words` 多行 + body 完整显示）
- 默认展开；折叠状态为会话级 `useState`——`CommitInfoHeader` 切换 sha 不重挂载，状态天然跨提交保持，重启后复位；不持久化到 localStorage
- 复用既有折叠交互模式（DiffViewer `FileDiffView` / ChangesPanel `FileSection`）：共享 `Button`（ghost）整行可点 + `ChevronDown` / `ChevronRight` 14px 按状态切换 + `aria-expanded`；hash `Chip` 不在点击区域内

## 改动

- `src/components/CommitInfoHeader.tsx`：新增 `messageExpanded` 状态；标题行由 `<p>` 改为整行 ghost `Button`（chevron + subject），hover 有 `bg-bg-primary/60` 背景反馈；body 改为 `messageExpanded && body` 条件渲染；新增 `cn` 导入
- `src/i18n/locales/en/commits.json` / `src/i18n/locales/zh-CN/commits.json`：`commits.header` 新增 `expandMessage` / `collapseMessage`（用作 aria-label 与 title 提示），两份语言文件同步（parity 测试强制）

## 测试

- `npm run typecheck` ✅；`npm test` 22 文件 / 148 用例全过（含 i18n en/zh-CN parity）✅；`npm run lint` ✅；改动文件 prettier ✅
- 未新增单测：纯展示性 toggle、无可抽取纯逻辑，仓库亦无组件挂载测试设施（vitest 为 node 环境）
- 待人工冒烟：长消息折叠后标题单行省略号、点击标题展开/折叠、作者/徽章/按钮行不受影响、无 body 的提交切换正常、hash Chip 点击不触发折叠
