# feat: v0.2 全量功能开发批次 · 审查报告

状态：已完成（11 个功能 commit + 1 个 review-fix commit，全部验证绿）

计划文档：`docs/tech/planning/roadmap-v0.2.md`（跨任务计划归 tech/planning）。
分支：`feature/v0.2`（worktree `D:/Work/GitWave-v02`），基线 `main` = `b895acd`。

## 提交清单（main..feature/v0.2）

| Commit | 功能 | 对应里程碑 |
|---|---|---|
| 2043608 | provider failover chain with ordered fallbacks | M0 |
| 3f633c9 | per-repo AI rules via .gitwave/AI.md | M0 |
| c6b13d5 | prompt template guidance and blank-template normalization | M0 |
| 4179e8a | PR description generation for the active branch | M1 |
| f7e75d8 | explain a commit from the history inspector | M1 |
| d55d005 | command palette with confirm-gated AI git actions (Cmd+K) | M1 |
| 24e6c70 | Git LFS support — install, track, untrack patterns | M2 |
| 173aa20 | full submodule support — add / recursive update / deinit / sync status | M2 |
| 322d111 | reflog browser in the sidebar with locate-in-history | M2 |
| 50d8ad5 | hooks editor for .git/hooks scripts | M2 |
| 7cc09a5 | enable libgit2 SSH transport, actionable ssh-add errors; bump to 0.2.0 | M3 |
| 0c6ee99 | review fixes — offline filter order, 401 chain stop, submodule add rollback | E1 |

## code-reviewer 审查结论（2026-08-28）

维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践。

- 🔴 严重问题：无。
- 🟡 一般问题：4 项，全部当轮修复（0c6ee99）：
  1. offline 过滤提前到 primary key 硬检查之前——离线 + 云 primary 无 key 时
     不再误导用户去配被禁用的 provider，Ollama fallback 可达（含新测试）；
  2. HTTP 401/403 映射为 `Credential`（停止故障转移链、直报根因），
     链耗尽时错误信息聚合各 provider 的一行失败摘要；
  3. `add_submodule` clone 失败时回滚半成品目录，并明示 `.gitmodules`
     可能需要 discard；
  4. `resolve_ref_oid` / `commits_ahead_of` 补直接单元测试（本地优先、
     revparse 回退、`base..head` 语义、limit 截断）。
- 🟢 采纳：hooks / deinit 文档与实现对齐；ReflogPanel 刷新成功清除旧错误。
  其余（palette base 下拉、prompt 注入防御文案等）记录为后续改进，不阻塞。

## 关键决策记录

- **git2 启用 `ssh` feature**：此前 vendored libgit2 不带 libssh2 传输，
  `Cred::ssh_key_from_agent` 在所有平台都不可用。启用后 libssh2 在 unix 走
  `SSH_AUTH_SOCK`、Windows 走 OpenSSH Agent named pipe，SSH clone/fetch/push 才真正可用。
- **palette 白名单在服务端硬校验**（`parse_palette_intent`）+ 前端全量确认门控，
  commit / push / merge / rebase 永不可由 palette 触发（P1）。
- **修复既有 bug**：`list_submodules` 原以 `sm.init(false)` 探测 initialized——
  该调用本身会写 `.git/config`（deinit 后再列出会静默重新注册）。改为只读
  config 查询 + `in_sync`（HEAD vs index gitlink）标记。
- **PR 描述仅生成**：不接 GitHub API（协作归 v0.3，见 roadmap-v0.3.md 决策 3）。

## 验证记录（本机 Windows 10，全部绿）

- `cargo test --all-targets`：171 passed / 0 failed（新增 32 个测试）
- `cargo fmt --check`、`cargo clippy --all-targets -- -D warnings`：干净
- `npm run typecheck`（tsc --noEmit）、`npm run lint`（eslint）、
  `npx prettier --check .`：干净
- `npm test`（vitest）：46 passed / 46
- `npm run build`（tsc + vite build）：通过
- macOS 侧依赖 CI 三矩阵（lint / test / build 均含 macos-latest）
