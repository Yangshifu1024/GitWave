# ADR 0007 · 仓库 PR 统一使用 Squash and merge

已采纳 · 2026-11-27

## 上下文

GitWave 以 PR 为单位审查变更，发布说明从 Git 历史生成。一次 PR 中可能包含实现、修正和回应评审意见的多次提交。

## 备选

GitHub 的 **Create a merge commit** 会保留分支提交并新增一次合并提交。**Rebase and merge** 将各个提交逐一写入目标分支。**Squash and merge** 将整个 PR 写为一次提交。

## 决策

本仓库的 PR 统一使用 **Squash and merge**。在 GitHub 的 **Settings → General → Pull Requests** 中，仅勾选 **Allow squash merging**。合并时使用符合 Conventional Commits 的英文提交说明，概括整个 PR 的变更。

## 后果

`main` 上每个新合入的 PR 对应一个提交，便于按 PR 生成发布说明或整体回退。需要追查分支内的修订过程时，可以查看原 PR 的提交列表与评审讨论。
