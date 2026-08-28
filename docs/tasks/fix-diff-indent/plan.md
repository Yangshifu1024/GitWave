# fix: diff / blame 代码行不保留原始缩进

## 现象

WorkingCopyModal（commit modal）右侧 diff 与 Inspector 的 DiffViewer 中，代码行没有按原始文件的缩进显示；BlameView 同样受影响。

## 根因

`DiffViewer.tsx` 与 `BlameView.tsx` 把 `line.content` 直接渲染进普通 `<span>`。HTML 默认 `white-space: normal` 会把行首连续空白折叠为一个空格，导致源码缩进丢失。后端（libgit2）返回的 diff line content 本身带完整缩进，问题纯在前端展示层。

WorkingCopyModal 内嵌 DiffViewer，因此 modal 与 diff view 是同一处根因。

## 修复方案

给代码内容 span 增加 `whitespace-pre-wrap`：

- 保留行首空白与 tab → 缩进恢复
- `pre-wrap` 而非 `pre`，保持现有"长行折行"行为不变，不引入横向溢出

### 修改点（均为加一个 class）

1. `src/components/DiffViewer.tsx`
   - `DiffLineView` unified 模式内容 span（`{prefix} {line.content}`）
   - `DiffLineView` split 模式 left / right 内容 span
   - `DiffHunkView` word-diff 成对行的 `-` / `+` 内容 span（`WordDiffSpans` 输出裸文本，跟随父级 white-space）
2. `src/components/BlameView.tsx`
   - `BlameLineRow` 内容 span（保留 `break-all`）

## 验证

- `npm run typecheck`、`npm run lint`
- 手动：查看含缩进的 diff（unified / split / word-diff 行）与 blame，行首缩进应与源文件一致
