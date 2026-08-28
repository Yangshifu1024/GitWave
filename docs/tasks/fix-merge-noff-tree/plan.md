# fix: No fast-forward 合并产生空 merge commit,被合并分支全部改动未进入主线

状态：已实现

## 事故（2026-08-28，用户真机）

用户在 main(9a85a29) 上通过分支右键菜单 "Merge into current" 合并
feature/heroui-migration,生成 commit `fe9dd2a`(双亲:9a85a29 + f534a3b):

- **没有文件改动**:`fe9dd2a` 的 tree 与第一父 9a85a29 完全相同 ——
  HeroUI 迁移的 42 个文件(+1929/−1727)没有进入 main;
- **提交者不是用户**:author/committer 均为硬编码的 `GitWave <noreply@gitwave.local>`。

## 根因

`merge.rs::merge_branch` 的 `--no-ff` 分支(ahead == 0 && no_ff):

- 创建 merge commit 时用 `our_commit.tree()`(HEAD 自己的树),注释假设
  "The target adds nothing to the trees" —— 这是把"可快进(ahead == 0)"误当成
  "目标无新增内容"。实际上 a head==0 意味着 HEAD 是目标 tip 的祖先,no-ff
  合并结果 tree 应为**目标的 tree**;用 ours 导致空 merge,分支改动全部丢失。
- 旧回归测试 `merge_no_ff_creates_merge_commit_when_ff_possible` 将错误语义
  断言为期望值("keeps our tree"),使 bug 有测试护航。
- 未刷新工作区:merge commit 创建后工作区仍是旧内容(与空 tree 一致,故未暴露)。

签名:`merge.rs` 及 rebase/交互 rebase 等多处用
`git2::Signature::now("GitWave", "noreply@gitwave.local")` 硬编码,
不读仓库/全局 `user.name` / `user.email`(working_copy/stash 已用
`repo.signature()`,是正确模式)。

## 修复

1. **no-ff tree**:`ahead == 0 && no_ff` 时 merge commit 用
   `their_commit.tree()`(合并结果 = 目标 tree,HEAD 是其祖先);创建后
   `checkout_head(force)` 刷新工作区/索引(与 FastForward 分支一致)。
2. **签名**:`git2_adapter.rs` 新增 `commit_signature(repo)`:优先
   `repo.signature()`(读仓库+全局 config),失败回退占位签名;
   merge.rs / rebase.rs / interactive_rebase.rs ×2 / working_copy.rs /
   stash.rs 全部改用,去除硬编码。
3. **测试**:修正 no-ff 断言(tree == 目标 tip tree、工作区刷新、author 为
   配置身份);新增 `commit_signature_prefers_repo_config`。

## 验证

- `cargo test --lib`:125 passed, 0 failed(含 no-ff 回归 + 签名测试)
- `cargo fmt --check` 通过;`cargo clippy --lib` 0 warning
- 后续:main 重新合并 feature/heroui-migration 恢复 42 文件改动
