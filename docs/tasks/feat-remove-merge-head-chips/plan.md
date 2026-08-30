# feat-remove-merge-head-chips · 移除提交行的 merge/HEAD 文字徽标

> 分支：`feat/remove-merge-head-chips`（自 main f8937f2）。用户需求：右侧提交
> 历史列表行尾的 "merge" / "HEAD" 文字显示去掉。

## 现象

提交历史每行 "作者 · 时间" 后面跟着蓝色 "merge"（合并提交）/"HEAD"（当前
提交）文字，信息冗余——行首已有 HEAD 高亮（accent 背景 + 左侧竖条）与消息
加粗，图上合并提交本就画了分叉边。

## 改动

`src/components/CommitGraph.tsx` 行尾时间戳 span 内删掉两个条件渲染的 span：

- `{commit.parents.length > 1 ? <span>merge</span> : null}`
- `{isHead ? <span>HEAD</span> : null}`

保留不变：HEAD 行高亮/加粗（视觉标识仍在）、RefBadge 分支/tag 徽标、
GraphRow 的分叉图形（`commit.parents` 仍被图渲染使用）。

## 验证

- `tsc --noEmit`、`prettier --check`、vitest 106 全绿
- GUI：提交列表行尾只剩 "作者 · 时间"；当前提交行仍有高亮标识
