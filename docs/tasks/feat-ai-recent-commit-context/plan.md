# feat: AI Generate 提示词附最近 3 次完整提交信息

状态：已实现

## 需求来源

用户 2026-08-28：将当前分支的最近 3 次提交信息一并放进 AI Generate 提示词，供 AI 参考。分支：`feature/theme-design`（沿用）。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 数据来源 | 新增 `commit_recent_messages(repo, n)`，HEAD 第一父链 | `CommitSummary` 只有 subject（`commit_log` 丢弃 body）；不往 `CommitSummary` 加字段，避免历史图虚拟列表 IPC 负载变大 |
| "当前分支"语义 | 第一父链（含分支线上的 merge，不含从侧支并入的提交） | 符合"本分支最近 n 次"直觉 |
| 提示词形态 | 现有 8 条 subject 列表保留，追加 `Last 3 commit messages on this branch (style reference):` 分节，`---` 包裹完整 message | 不动 system prompt；分节标题已向模型表意"风格参考" |

## 改动清单

- `src-tauri/src/infrastructure/git/history.rs`：`commit_recent_messages` + 3 个测试（线性仓库 newest-first / merge 夹具验证第一父链与完整 message / 空仓库空列表）
- `src-tauri/src/application/use_cases.rs`：`generate_commit_message` 拼装提示词时追加分节

## 需求 2（2026-08-28 追加）

AI Generate 只基于 staged 文件生成提交消息，unstaged 不进提示词。

- `generate_commit_message` 移除 unstaged diff 的计算与 `Unstaged changes:` 分节
- 顺带加空保护：无 staged 文件时直接返回 `no staged changes — stage files before generating a commit message`，避免模型对着 `(none)` 幻觉生成
- `infra_diff_workdir_to_index` 导入保留（working copy 用例仍在用）

## 验证

- `cargo test --lib` 119 通过（含新增 3 用例）、`cargo clippy --all-targets` 0 warning、`cargo fmt --check` 通过
- 真机：generate 结果应贴近仓库既有提交语言与格式（待用户验证）
