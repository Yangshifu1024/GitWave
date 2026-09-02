# review: diff view 不折行 + 横向滚动(缩进保持)

审查对象:分支 `fix/diff-wrap-mode` 未提交改动(`src/components/DiffViewer.tsx`、`src/styles/tokens.css`、`docs/tasks/fix-diff-wrap-mode/plan.md`)。背景见 [plan.md](./plan.md)。

## 审查结论(code-reviewer,7 维度)

布局核心机制逐项推演通过:

- `w-max min-w-full` 在 `overflow-x-auto` 容器内宽度解析正确;文件头与工具栏在滚动容器外保持固定
- split 模式两栏 flex-fraction 内在宽度算法下行 max-content = 72px + 2×max(左,右),渲染时左右各 (W−72)/2 严格对齐(与 GitHub 一致);空侧 `&nbsp;` 占位不受影响;内容 span 保留的 `min-w-0` 在 w-max 父级下永不触发收缩,无害
- word-diff 的 `WordDiffSpans` 切片文本在 `whitespace-pre` 下表现正确
- 移除 hunk 根 `overflow-hidden` 对圆角/裁剪无影响(无边框圆角,滚动容器接管溢出)
- 超长行(minified 单行)布局一次 O(n),横向滚动走合成器,性能优于原 pre-wrap 逐行折行
- 回归点(BlameView 不在范围、WorkingCopyModal 共用组件、空态/折叠/最大化)均安全
- `npm run typecheck`、`npm run lint` 通过;`npm run build` 确认 `bg-diff-*-bg-solid` 与 `.isolate` 工具类生成

## 🔴 问题(2 项,均已修复)

1. **暗色主题 word-diff 行号列遮不住滑过的正文** — 暗色 `--color-diff-del/add-bg` 为 `color-mix(...14%, transparent)`(半透明),sticky 行号用它做背景时正文以约 86% 可见度从下方透出。
   **修复**:tokens.css 新增不透明 token `--color-diff-add-bg-solid` / `--color-diff-del-bg-solid`(亮色混入 white、暗色混入 `--color-bg-panel` #2a2a2d),4 处 word-diff 行号 span 改用 `bg-diff-*-bg-solid`。unified/split 的 `bg-bg-elevated` 本就不透明,不受影响。

2. **行号 z-10 与 sticky 工具栏同层,垂直滚动时数字浮在工具栏上** — 行号 span(z-10,DOM 靠后)与工具栏(`sticky top-0 z-10`,DOM 靠前)同一层叠上下文,行号绘制在工具栏之上。
   **修复**:hunks 滚动容器加 `isolate`,把行号 z-10 封闭在内层层叠上下文;容器整体按 z:auto 绘制、低于工具栏 z-10,容器内行号仍盖得住正文,sticky 行为不变。

## 🟡 采纳(1 项)

- **同文件多 hunk 宽度不一致**:各 hunk 独立 `w-max` 时,滚动到最宽 hunk 远端,窄 hunk 右边框中断。已改为 hunks 列表外包一层 `w-max min-w-full` div,hunk 根退回普通块级,全部拉伸至最宽 hunk 同宽,边框贯通。

## 🟢 可选项(本次不做)

- 行号 span 长类名重复 8 次、`left-9` 与 `w-9` 隐式耦合:可抽 `LineNumberSpan` 小组件 + 注释
- 滚动容器不可聚焦,纯键盘用户无法触达溢出内容(GitHub 同样如此):可加 `tabIndex={0}`
- split 亮色行号列 `bg-bg-elevated`(#f4f4f5)与面板底(#f8f8f8)微弱色差:透明改不透明的必然结果,可接受
- 中期可补 Playwright e2e:打开含长行 diff → 横向滚动截图对比

## 验证

- `npm run typecheck` ✓ `npm run lint` ✓ `npm run build` ✓(生成 `bg-diff-*-bg-solid`、`.isolate`)
- 手动:用户已在 dev 实例验证不折行、缩进原样、行号固定;审查修复项(暗色主题 / 工具栏重叠 / 多 hunk)按 plan.md 补充清单复核
