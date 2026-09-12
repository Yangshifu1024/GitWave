# Review · fix-merge-rebase-dirty-guard（8558f48 + 语义收尾提交）

> 最终态审查记录。本文件为分支内初审报告的刷新版：初审的 🔴 及多数 🟡 已在
> 8558f48 最终代码修复；收尾提交修复剩余语义问题并登记本轮遗留。

## ✅ 优点

- `merge.rs:152` guard 在 `behind == 0` 早退之后，三路（FF/no-ff/3-way）全覆盖；测试断言 HEAD 未动、工作区原样、无 MERGE_HEAD 残留这一关键性质。
- `revert.rs` 改调共享 `ensure_clean`，行为逐行一致；路径守卫「语法关 + 词法关」双层，`../`、`..\`、绝对路径、盘符相对路径（收尾提交起在语法关直接拒绝）全覆盖。
- `continue` 三态（staged→amend / unstaged+untracked→拒并保留 pause）比 plan 1.2 更保守；`PauseState.original_head` 经 `#[serde(default)]` 兼容旧 pause 文件。
- interactive_rebase +429 行无夹带：全部对应 plan 1.2/1.3 与「review 驱动追加修复」声明项，含约 19 个新后端测试。
- 前端 run 路径均做 fresh 复检、按钮禁用；en/zh `dirtyWarning_one/_other` 复数对齐。

## 已修复记录

| 初审发现 | 修复方式 |
|---|---|
| 🔴 Edit-last 过期 pause 致 Continue 重放已落地提交 | 8558f48 已修：Edit 分支恒写 pause（含空 remaining），`edit_last_todo_pauses_without_stale_state` 测试背书 |
| 🟡 冲突后返回 `Ok(Conflicts)` 但盘上无冲突（语义矛盾） | 收尾提交：`restore_original` 后改返 `Kind::AutoAborted`（serde 序列化为 `auto_aborted`），前端 BranchList/InteractiveRebaseDialog 增加 autoAborted 分支与 en/zh 文案，不再展示假冲突文件 |
| 🟡 squash 首项校验发生在 reset 之后 | 8558f48 已修：预检移到 reset 之前 |
| 🟡 MergeConfirmDialog/InteractiveRebaseDialog queryKey 与正典不一致 | 8558f48 已修：queryKey 对齐三段键 |
| 🟡 plan 1.4 `PATH_TRAVERSAL` 新码未做 | 按偏差自述维持：复用 `PATH_ESCAPES_WORKTREE`（文案贴合、零 i18n 改动），plan 已注明 |
| 🟢 amend 覆盖 author | 收尾提交：`amend_head_with_index` 保留原 author（≡ `git commit --amend`），committer 用当前身份 |
| 🟢 Clean 路径 pause 删除失败静默（可致全量重放） | 收尾提交：非 NotFound 删除失败冒泡为 `git.pause_cleanup` 错误 |
| 🟢 `restore_original` 内 reset/cleanup_state 失败静默 | 收尾提交：改为 `tracing::warn`（不中断恢复流程但留痕） |
| 🟢 `unstage_paths` 是唯一未过守卫的路径数组入口 | 收尾提交：循环内补 `reject_escaping_syntax` |
| 🟢 symlink/TOCTOU 局限未声明 | 收尾提交：`worktree_guard.rs` 模块注释显式写明两项假设与局限 |
| 🟢 盘符相对路径 `C:evil` 未固化测试 | 收尾提交：`reject_escaping_syntax` 增加盘符前缀拒绝（不依赖下游 join 兜底），conflict.rs/working_copy.rs 各补 `C:evil.txt` 变体 |

## 遗留（不阻断，可后续处理）

- 🟡 plan 1.5 承诺的 vitest 门禁测试缺失（后端是最后防线、风险可控；UI 胶水靠 typecheck + lint + 手动）。
- 🟡 CommandPalette `gateCheckout` 硬编码 `isCurrent:false` / `occupiedWorktree:null`，未传真实状态。
- 🟢 symlink 纵深防护（canonicalize 二次校验）——模块注释已声明局限，如需对抗恶意仓库再做。
- 🟢 `i18n` `pauseCleared` key 名与文案语义漂移（文案已改为「已中止」，key 名仍叫 Cleared）；en/zh 成对、parity 无虞。
- 🟢 `classify_dirty` 无直接单测（经 continue 路径间接覆盖）。

## 📝 总体评价

核心安全性质达成并有测试背书：脏工作树不丢数据、路径不出工作区、拒绝后无中间态残留、AI 不自动 commit 的铁律未被触碰。收尾提交修正了冲突自动回滚的返回值语义（前端不再误报冲突）与失败冒泡路径。`cargo test --all-targets`（316 passed）、clippy `-D warnings`、fmt、`pnpm test`（163 passed）、tsc、lint 全绿。
