# fix-hide-origin-head · 侧栏远程分支列表混入 origin/HEAD

> 状态：已修复（待提交）
> 问题（用户，2026-09-10）：左侧栏分支列表的远程分支里有一个 "HEAD"，有什么实际意义吗？

## 根因

那个 "HEAD" 是 `refs/remotes/origin/HEAD` —— git clone 时生成的**符号引用**，
指向远程默认分支（如 `refs/remotes/origin/main`），用于 CLI 缺省检出目标，
不是真实分支。

- `list_branches`（`src-tauri/src/infrastructure/git/history.rs:594`）用
  `branches(Some(BranchType::Remote))` 枚举时会把该 symref 一并列出，未过滤
- 前端 `BranchList.tsx` 按名字第一段归入 origin 组，`remoteShortName` 剥掉
  `origin/` 前缀后只剩 "HEAD"
- 该条目对用户无任何可交互意义：symref 无直接 target，`last_commit_sha` /
  `last_commit_time` 为空；checkout 路径早已专门防它（`branch.rs:86-96`
  双击静默 no-op）。`git branch -r` 至少显示 `origin/HEAD -> origin/main`
  带指向语义，本 UI 只剩裸 "HEAD"，纯属噪音

对比：提交图装饰 `collect_commit_refs`（history.rs:444）因 symref 无
`target()`（返回 None → `continue`）已天然跳过它，无需修——该判断只挡
symref 形态。

## 修复

按名字过滤而非 `is_symbolic()`：`name == "HEAD" || name.ends_with("/HEAD")`。
原因：除 clone 生成的 symref 外，`fetch +HEAD:refs/remotes/origin/HEAD`
refspec 会产生**直连 ref** 形态的 origin/HEAD，`is_symbolic()` 挡不住；
真实远程分支以 `/HEAD` 结尾理论上可能但会被隐藏——git 自身把
`refs/remotes/<remote>/HEAD` 保留给默认分支指针语义，且裸 "HEAD" 分支名
被 `git branch` 拒绝（`check-ref-format` 层面虽合法，但分支名校验含
symref 歧义检查），与主流客户端行为一致。

修改点（后端单一数据源，前端不改，所有消费 UI 一致受益）：

1. `history.rs` `list_branches` 远程循环：跳过上述名字
2. `history.rs` `collect_commit_refs` 远程循环：同样跳过（symref 已被
   `target()==None` 排除，此处为直连 ref 形态兜底）
3. `use_cases.rs` AI palette 分支枚举（`repo.branches(None)` 收集
   remote_branches）：同样跳过，避免 AI 上下文混入 origin/HEAD

## 回归测试

`history.rs` tests 新增 `list_branches_hides_origin_head`：

- fixture：`build_linear_repo` + 直连 ref `refs/remotes/origin/main` +
  符号引用 `refs/remotes/origin/HEAD → refs/remotes/origin/main`（模拟
  真实 clone 产物）
- 断言：结果含 `origin/main`（kind=Remote），不含 `origin/HEAD`

## 验证

- [x] `cargo fmt` / `clippy --all-targets`（0 警告）/ `cargo test`（297 通过，含新增 3 个测试）
- [x] code-reviewer 审查通过（无 🔴，🟡 已落实），报告见 `review.md`
- [ ] 手动冒烟：打开含远程仓库的 Workspace，侧栏 origin 组不再出现 "HEAD"
- 按 P1 约定不自动 commit，由用户自行提交
