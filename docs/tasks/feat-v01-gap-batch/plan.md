# feat: v0.1 计划缺口统一开发批次

状态：进行中

## 审计结论（2026-08-28，对照 docs/pm/core/*）

14 项 must 中 13 项已实现；项 14（AI command palette）按计划 may-slip 顺延 v0.2。
计划文本中明确列出但未实现的需求（本批次范围）：

| # | 需求 | 计划出处 | 状态→动作 |
|---|---|---|---|
| 1 | revert / cherry-pick | 1.1 Core Git Operations（"全集是基本盘，不做简化版"） | ❌ → 新增命令 + 右栏 commit 操作入口 |
| 2 | tag + annotated tag 创建/删除 | 1.1 + S3 Should | ❌ 仅只读展示 → 新增 |
| 3 | submodule init/update | roadmap v0.1 范围点名 + S1 | ❌ → 新增 |
| 4 | history 筛选/搜索 | 1.2 Visualization | ❌ → Rust filter 参数 + 前端搜索框 |
| 5 | 重启后状态完整恢复 | 1.4 Workspace 存储 | 🟡 仅切换时恢复 → 启动恢复 lastActiveWorkspace/Repo |
| 6 | .gitignore 编辑器 | S2 Should | ❌ 仅逐条追加 → 模态编辑器 |
| 7 | Workspace 导入/导出 | S6 Should（.gitwave-workspace.json） | ❌ → 新增 |
| 8 | Prompt 模板管理（三套 UI） | 1.6（类型已有 pr/commit/conflict，pr 无消费方） | 🟡 → AiProviderSettings 三 textarea |
| 9 | 完全离线模式开关 | 1.6（"一键禁用所有云端调用"） | ❌（注释宣称无实现）→ ai_offline 字段 + 拦截 + UI |

## 明确不做（本轮）

- AI command palette（may-slip → v0.2）、provider 故障转移、per-repo AI rules（.gitwave/）—— v0.2 范围
- file tree / diff syntax highlight（1.2 列出但 shiki/树组件成本高，单独评估）
- PR 创建（S4，依赖 provider API 与 v0.3 协作方向）
- Git LFS / reflog explorer / hooks 编辑器（N1-N3 Could）

## 实现要点

1. revert/cherry-pick：`git2::revert` + 复用 interactive_rebase 内 cherry-pick 逻辑，暴露
   `cmd_revert_commit` / `cmd_cherry_pick`；入口 = 右栏 CommitInfoHeader 操作区（P1：产生
   未提交变更或直接提交？revert 直接生成 commit（git 默认），cherry-pick 应用到 index+worktree；
   冲突时走既有冲突面板路径）。
2. tag：`tag.rs`（list 已有只读 refs；create lightweight/annotated + delete）+ CommitInfoHeader
   入口 + tag 列表管理（BranchList 右键或右栏）。
3. submodule：`submodule.rs`（list/init/update，git2 Submodule API）+ RepoList 工具菜单入口。
4. history 搜索：`commit_log` 加 `filter: Option<String>`（message/author contains，大小写不敏感），
   `cmd_get_commit_log` 透传，CommitGraph 顶部搜索框（输入防抖重查）。
5. 启动恢复：workspaceStore bootstrap（读 list_workspaces 取 last_active 字段/持久化的
   activeWorkspaceId）。
6-9 见各提交。

## 验证

- cargo test --lib 全绿、fmt/clippy 干净、npm build 通过；每项带单元测试（Rust 侧）。
