# review: fix-merge-noff-tree(提交 fd48a09)

审查方式:code-reviewer 子代理因模型提供者未配置无法调用,由主代理按 7 维度复核;
已对照事故提交 fe9dd2a 逐行审读完整 diff,并运行全量测试。

## 结论

**通过** — 无 🔴 问题。

## 七维度复核

### 正确性(✅)

- no-ff 修复:`head == 目标祖先(ahead == 0)` 时 3-way 合并结果 tree 必然等于目标 tip 的 tree,
  `their_commit.tree()` 语义正确(与 `git merge --no-ff` 行为一致);
- 新增 `checkout_head(force)` 与 FastForward 分支一致,工作区/索引同步到新 HEAD;
- 签名:`commit_signature` 用 git2 `Repository::signature()`(仓库 config 含全局层)取
  user.name/user.email,失败回退占位 —— 与 working_copy/stash 原有模式一致,替换不改变
  有配置时的行为;
- rebase/interactive_rebase 用同一 helper:rebase 新提交的 committer 现为用户配置身份,
  author 仍沿用原提交(未改动 rebase 逻辑本身);
- 全量 `cargo test --lib`:125 passed / 0 failed / 2 ignored(既有 1 个 ignore 与本任务无关)。

### 安全(✅)

- 无新增进程/文件操作;仅 tree 来源与签名来源变化,不扩大攻击面。

### 性能(✅)

- 无差异。

### 可维护性(✅)

- 6 处硬编码签名收敛为单一 `commit_signature`;注释补充事故背景,后续不会重蹈覆辙。

### 可读性(✅)

- no-ff 分支注释明确说明"必须带目标 tree,否则丢整条分支",配合回归测试自解释。

### 测试覆盖(✅)

- `merge_no_ff_creates_merge_commit_when_ff_possible`:断言 tree == 目标 tip tree(原错误断言
  "keeps our tree" 已修正)、工作区刷新(file2.txt 存在)、author == "Merge Tester";
- 新增 `commit_signature_prefers_repo_config`(仓库 config 优先);
- 其余 4 文件签名替换由现有测试覆盖(全量通过)。

### 最佳实践(✅)

- 复用 git2 官方 `Repository::signature()` 而非自造 signature,是首选方案;
- 错误映射 `commit_signature` 直接返回 `Result<AppError>` 与模块内 `map_git_err` 配合得当。

## 风险提示

- 用户在无 git config 环境仍会以占位身份提交(有意保留,避免应用卡死);
- 历史数据恢复(重合并 feature/heroui-migration)是独立的 git 操作,不在本提交内,
  由后续步骤完成。
