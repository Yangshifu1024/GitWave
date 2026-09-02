# fix: diff view 窄栏下长行折行导致缩进"看起来丢失"

## 现象

用户反馈右侧栏(Inspector)diff view 代码缩进"又没了"。排查确认**不是回归**:前一次修复(`e17316f`,为 6 处代码内容 span 加 `whitespace-pre-wrap`)在当前源码、仓库 dist 构建产物、以及用户运行的安装版 v0.7.3 中均完整生效(exe 内嵌 CSS 与 dist 的 Vite 内容哈希一致 `index-Bmo9A0md`;运行中 App 截图放大核实,行首缩进实际渲染正常)。

## 根因

`whitespace-pre-wrap` 保留了行首空白,但**软折行后的续行从内容盒第 0 列开始**。右侧栏宽约 270px(≈35 字符/行),长行(如 Java 70+ 字符)必然折行,续行视觉上与"没有缩进的独立代码行"无法区分。长行占比高的 diff,大部分视觉行都是续行,整体看起来缩进丢失。

前次修复方案([fix-diff-indent/plan.md](../fix-diff-indent/plan.md))明确选择 `pre-wrap` 而非 `pre`,理由是"保持长行折行行为,不引入横向溢出"——该取舍在长行场景下暴露了上述视觉缺陷。

## 修复方案

改为 GitHub 风格的**不折行 + 横向滚动**:代码行 `white-space: pre`,缩进永远原样显示;超宽行在所属文件块内横向滚动查看。不引入折行开关(保持最小改动,后续有需求再评估)。

### 修改点(仅 `src/components/DiffViewer.tsx`)

1. `FileDiffView` hunks 容器:加 `overflow-x-auto`,作为每个文件块的横向滚动容器(文件头与 diff 工具栏保持固定)
2. `DiffHunkView` 根 div:移除 `overflow-hidden`(阻断 sticky 行号定位;w-max 布局下也不再需要裁剪),改为 `w-max min-w-full`——hunk 盒随最宽行扩展,短行内容时仍占满容器全宽,边框完整
3. 5 处内容 span(unified / split 左右 / word-diff 成对行):`whitespace-pre-wrap` → `whitespace-pre`
4. 行号列 sticky:各行的两个行号 span 加 `sticky left-0`(第 1 列)/ `sticky left-9`(第 2 列,`w-9`=2.25rem)+ `z-10`;split 与 word-diff 行号原本无背景,补不透明背景类(`bg-bg-elevated` / 对应 diff 底色),保证横向滚动时行号固定可见、正文从其下方穿过

BlameView 不在本次范围(已有 `break-all` 折行,行为独立)。

## 验证

- `npm run typecheck`、`npm run lint`
- 手动(dev 模式,用含长行的真实仓库 OCPP `D:\code\project` commit `174d580`):
  - 行首缩进原样显示,不再折行
  - 每个文件块底部出现横向滚动条;滚动时行号列固定,正文不覆盖行号
  - 短行文件无横向滚动条,渲染与改动前一致
  - unified / split / word-diff 三种模式;WorkingCopyModal(与右侧栏共用组件);面板最大化按钮
  - 暗色主题下 word-diff 横向滚动:行号列为不透明底(`--color-diff-*-bg-solid`),正文不得从行号下透出
  - 垂直滚动长 diff 至文件行与 sticky 工具栏重叠:行号数字不得浮在工具栏文字上(滚动容器 `isolate` 封闭 z-index)
  - 同文件多个宽窄不一的 hunk:横向滚动时各 hunk 右边框贯通一致(w-max 包裹层统一宽度)
