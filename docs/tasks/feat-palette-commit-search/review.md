# review: feat-palette-commit-search

审查人：code-reviewer 代理（2026-09-09，分支 feature/palette-commit-search）

## ✅ 优点

- commit 搜索 effect 用 `cancelled` 旗标 + `clearTimeout` 双保险防竞态，300ms debounce 与原 History 逻辑一致；palette 打开时完整重置状态。
- 键盘导航将命令与 commit 结果统一为单一列表，`clampSelection` 处理空列表与越界，Enter 有存在性守卫，未选中时 Enter 回落 Ask AI（原行为不变）。
- CommitGraph 搜索移除干净，无残留引用；i18n en/zh-CN 双语对称增删。

## 🔴 严重问题（已修复）

- **CommitGraph locate 窗口外静默失败**：palette 搜索可命中初始 200 条窗口外的老 commit，`resolveLocateIndex` 返回 null 后无任何动作。已修复：locate 未命中且日志空闲、页面可能被截断（`commits.length >= limit`）时自动 `setLimit(+PAGE_SIZE)` 扩窗重试，短页（到历史末尾）时放弃。

## 🟡 一般问题（已修复）

- workspace 切换等异步清空 `commitResults` 时 `selectedIndex` 可能残留：已在搜索 effect 清空分支重置为 -1。
- 点击行为经 `rows` 索引间接触发，与渲染顺序隐式耦合：已改为命令行直接 `c.run`、commit 行直接 `locateCommit(c.sha)`，`rows` 仅服务键盘导航。

## 🟢 优化建议（未采纳，留待后续）

- commit 搜索可改用 @tanstack/react-query `useQuery`，自动获得缓存与取消。
- commit 搜索错误态可附重试提示。
- 可为 locate 扩窗逻辑补充 vitest 用例（现有 `commitLocate.test.ts` 覆盖 `resolveLocateIndex`，未覆盖扩窗分支）。

## 📝 总体评价

实现质量较高，hooks 竞态防护与清理干净；唯一的严重问题是窗口外 commit 定位静默失败，已通过扩窗重试修复。建议在可运行 Node 的环境补跑 `npm run build` 与手动验证（本机无 Node.js，未执行构建）。
