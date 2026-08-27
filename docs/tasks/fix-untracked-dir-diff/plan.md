# fix-untracked-dir-diff

> 新目录中的 untracked 文件点开后无 diff；应展示完整文件内容。

## 根因

`working_copy::status` 使用 `recurse_untracked_dirs(true)`，Changes 列表能列出 `src/lib.rs`。

`diff_workdir_to_index` 未设置该选项，libgit2 只产出目录 delta（`src/`），没有文件 hunk。`filterDiffSummary` 按文件路径过滤后为空，界面显示 `No diff for …`。

## 方案

`DiffOptions` 增加 `recurse_untracked_dirs(true)`（已有 `include_untracked` + `show_untracked_content`）。

## 验证

- 新目录下的 untracked 文件：diff 含全部行
- 仓库根目录 untracked / staged added：回归通过
