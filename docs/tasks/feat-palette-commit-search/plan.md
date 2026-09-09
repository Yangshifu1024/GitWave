# feat: 将 History 搜索合并进 Ctrl+K Command Palette

## 背景

History（CommitGraph）面板原有一个顶部搜索框，通过 `getCommitLog(workspaceId, limit, filter)` 做后端过滤，把整个 commit 列表替换为匹配项。Command Palette（Ctrl+K）已有静态命令 + Ask AI 两层结构，且 `requestLocate(sha)` → `handlePaletteLocate` → CommitGraph 滚动定位链路已存在。

## 需求

将 History 顶部搜索框的搜索能力合并到 Ctrl+K 弹窗中：

- **完全移除** History 面板顶部搜索框（用户已确认）。
- Palette 中搜索 commit 后**选择并跳转**：显示 top N 匹配（sha / message / author），点击或回车滚动定位到该 commit，History 列表本身不过滤（用户已确认）。

## 方案

### CommandPalette.tsx

- 输入非空时 300ms debounce 调 `getCommitLog(workspaceId, 10, query)`（复用后端 `cmd_get_commit_log` 的 filter 参数，message/author 匹配，scan 上限 10k，无需后端改动）。
- 结果区在静态命令之后新增 "Commits" 段：每行短 sha（mono）、message（截断）、author；加载中 / 出错 / 零命中（隐藏该段，Ask AI 兜底）三态。
- 键盘导航：↑/↓ 在「命令 + commit 结果」统一列表中移动，Enter 执行选中项；未选中时 Enter 仍提交 Ask AI（原行为不变）。
- 选中 commit → 关闭 palette + `requestLocate(sha)`。

### CommitGraph.tsx

- 删除 `searchInput` / `filter` 状态、debounce effect、`fetchKey` 中的 filter、零命中 "No commits match / Clear search" 分支、顶部搜索条 UI 及相关 import。
- `getCommitLog` 恢复无 filter 调用。

### i18n

- `palette.json`（en / zh-CN）新增 `commitsTitle`、`searchingCommits`。
- `branches.json`（en / zh-CN）删除不再引用的 `graph.noMatch / clearSearch / searchPlaceholder`。

### 文档

- `docs/design/02-components.md`：CommandPalette 描述补充 commit 搜索能力。

## 不做的事

- 不新增后端命令。
- 不保留 History 列表过滤语义（搜索只做跳转定位）。
- AI 不自动 commit / push / merge（P1 不变，本需求不涉及）。

## 验证

- `npm run build`（tsc --noEmit + vite build）。
- 手动路径：Ctrl+K → 输入关键词 → Commits 段出现匹配 → 点击/回车 → palette 关闭且 History 滚动定位到该 commit；History 顶部无搜索框，列表正常加载与分页。
- 键盘路径：↑/↓ 跨「命令 + commit 结果」移动、Enter 定位选中的 commit；未选中时 Enter 仍触发 Ask AI。
- 窗口外 commit：palette 命中初始 200 条窗口之外的老 commit 时，locate 未命中会自动扩窗（每次 +300）重试，直至命中或到达历史末尾。
