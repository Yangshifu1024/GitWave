# feat-graph-fork-style-edges · History 分支曲线对齐 Fork 绘制约定

> 状态：实施完成（待手动冒烟；未提交）
> 需求（用户，2026-08-28）：同一仓库 Fork 与应用显示的分支曲线不一致，要求按 Fork 约定重画。

## 根因

拓扑一致（同提交 / 同父子关系 / 同顺序），差异在绘制实现：

- 旧实现每行 SVG 只画自己行内的内容：子→父边仅在子提交自己的行内画肘弯（立即拐到父
  lane），中间行靠 `activeLanes` 竖线续接；并行单提交分支（如 renovate/* 连排）呈现为
  一行一个短肘弯，而非长并行曲线
- 分支 tip 行 `laneContinuesFromAbove` 为 false 时跳过上半段线，行间出现碎片（虚线感）

## 新绘制模型（lane 分配算法不变）

- 每条 child→parent 边跨行绘制：
  - **第一父边**：沿**子 lane** 垂直向下穿越中间行，在父行弯入父节点（同 lane 则直落）
  - **额外父边**（merge 第二父）：在子行内从节点横向弯到**父 lane**，沿父 lane 下行，父行直落进节点
- tip 恒有向下实心 stub（不再跳上半段）——并行分支为多条连续长线在父节点处汇入
- 超出已加载窗口的父：画下垂 stub 提示还有更早历史（配合分页）
- 实现：`computeRowArt(commits, shaToIndex) -> RowArt[]`（每行 verticals / incoming /
  hasStub / outCurves），GraphRow 按 RowArt 画线；删除 `laneContinuesFromAbove` /
  `computeActiveLanes` / activeLanes 透传链
- 颜色：线与曲线取所在 lane 的 lane 色；节点高亮 / merge 双圈 / HEAD 环不变

## 追加：lane 分配对齐 Fork（用户截图反馈，2026-08-28）

边画法修复后仍有差异：renovate/* 是**链式堆叠分支**（每个提交带不同分支引用、父为上一个
提交），旧 lane 分配按纯拓扑把整条链放进同一条 lane（一条竖线），Fork 则给每个分支引用
单独开新 lane（阶梯状）。

- `history.rs` lane 分配改为**带 tag 的预留匹配**：每个 lane 预留携带分支 lineage tag；
  提交携带分支引用时只消费同 branch 的预留，否则**新开 lane**（阶梯）；无引用提交沿用
  所在 lane 的 lineage（链式普通提交不换道）
- 第一父预留 tag = 子提交 lineage；额外父（merge 第二父）预留 tag = 父提交自身分支
  （dd6befd 在 trunk lane 被正确认领，renovate 曲线汇入 trunk——与 Fork 截图一致）
- 新增测试 `commit_log_stacked_branch_tips_get_staircase_lanes`：三个堆叠分支 tip 的
  lane 应为 [0,1,2]
- cargo fmt / clippy（0 警告）/ test 112 通过

## 追加 2：lane 复用 + 兄弟扇出（用户截图反馈 2/3，2026-08-28）

两轮迭代后的最终语义：

- renovate/* 实为**兄弟分支**（各自一个提交、父都是 dev 同一提交）。最初"第一个空闲列"
  回退会让兄弟全挤同一条 lane（父已有预留 → 跳过放置 → 空闲列无人占用 → 下个兄弟复用）
- **最终规则**：分支 tip 取第一个空闲 lane，并**无条件把第一父预留钉在该 lane 上**
  （占住 lane，兄弟各拿各的、空闲 lane 可回收复用——Fork 的 fix/ios-27 复用了 renovate
  腾出的 lane）；堆叠链（预留属于其他分支）仍开新 lane 阶梯；额外父开新 lane 标记为父
  自身分支，已预留则留给曲线汇入
- 测试：`commit_log_stacked_branch_tips_get_staircase_lanes`（堆叠阶梯 [0,1,2]）+
  `commit_log_sibling_branch_tips_get_distinct_lanes`（同父兄弟不共 lane）
- cargo fmt / clippy（0 警告）/ test 113 通过

## 追加 3：RefBadge 样式对齐 Fork（用户截图反馈 4，2026-08-28）

- 分支徽章（local / remote）背景 / 边框 / 文字改用**所在 lane 的线条色**（`color-mix`
  低透明度底 + 中透明度边），remote 徽章带小圆圈前缀图标、local 带 branch 前缀图标
- tag 徽章保留 warning 配色，**前缀 Tag 小图标**（Fork 式）
- HEAD 与当前分支（emphasize）保持既有 accent 处理
- `RefBadge` 增加 `lane` prop，CommitRow 传入 `commit.lane`

## 追加 3：主链提交被误甩侧 lane（用户截图反馈 5，2026-08-28）

GitWave 自身仓库出现怪异曲线：`fix: ci` 等主链提交被甩到 lane 1/2 再折回。根因：阶梯
规则对**任何**带分支引用且预留 tag 不匹配的提交生效——但仓库里大量主链提交背着已合并 /
落后的分支引用（如 fix/window-controls-active-repo 指在 main 链上），它们的预留 tag 是
主干 lineage（"main"），与自身分支引用不匹配 → 误开新 lane，主干折线。

- 修复：引入 `is_tip`（加载列表中没有任何提交以它为父）——**只有真 tip 才允许阶梯开新
  lane**；有子提交的提交必然处于连续线上，一律沿子提交的预留走
- 堆叠链测试语义修正：链式提交（每个都有子节点）应保持连续 lane [0,0,0]（原来的
  [0,1,2] 预期本身就是错误约定），测试更名为 `commit_log_branch_ref_chain_stays_continuous`
- 兄弟扇出测试不受影响（兄弟 tip 无子节点，仍各开新 lane）
- cargo fmt / clippy（0 警告）/ test 113 通过

## 验证

- [x] `npm run typecheck` / `lint` / `format:check` / `test`（43）/ `build` 全绿
- [ ] 手动冒烟：该仓库与 Fork 对照——renovate/* 六条并行单提交分支应各占一条连续长线并在
      父提交处汇入；merge 第二父曲线从节点出发；线性历史为一条直线；tip 无断线
