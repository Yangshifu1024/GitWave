# fix-history-graph-diff-ux

> 恢复：History 倒序 + merge 连线 + branch/tag 徽章；diff hunks；commit 选中驱动右侧面板；Unified/Split。

## 状态

已恢复（待手动验证）。

## 内容

1. 后端 `commit_log`：newest-first lane + `refs`（branch/tag/HEAD）
2. 后端 `diff_to_files`：填充 hunks/lines（RefCell）
3. `CommitGraph`：SVG 连线 / ref badges / 不 reverse
4. App：选中 commit → `DiffViewer(commitOid)`；否则 workdir
