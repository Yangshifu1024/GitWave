# fix-branches-pull-disabled · Branches 区 Pull 按钮始终禁用

> 状态：已修复（待提交）
> 问题（用户，2026-08-27）：Branches 的 pull 按钮似乎始终是 disabled。

## 根因

非数据 bug，是前端过度门控。`BranchList` 的 `pullDisabled` 含 `(behind === 0)` 条件，而
`behind` 只有在 fetch 更新远程跟踪引用后才可能 >0：

- Branches 区自身无 Fetch 入口（Fetch 在 Repos 区头部），用户不主动 Fetch 则 behind 恒 0
- upstream 未设置的仓库（本地 init 后添加）`ahead_behind` 恒返回 (0, 0)，永远禁用

后端 `pull`（remote.rs:127）本身安全：fetch → `merge_analysis`，up-to-date 时静默
`Ok(())`；无 upstream 报 "no upstream configured"（ErrorAlert 可见）；分歧报
VersionConflict 引导 Merge UI。

## 修复

`src/components/BranchList.tsx`：`pullDisabled` 去掉 `(behind === 0)`，仅保留
`!activeRepoId || isSyncBusy || "(detached)"`——与主流客户端（VS Code / GitHub
Desktop）一致，Pull 始终可点，无新提交时为 no-op，`↓N` 徽标逻辑不变。
Push 的 `ahead === 0` 门控保留（commit 后自然满足，无同类陷阱）。

## 追加决策（用户，2026-08-27）：pull 操作前需要确认 → 完全照搬 Fork 对话框

- 第一版：点击 Pull 先弹简单确认 Modal。**用户随后要求"完全照搬" Fork 的 Pull 对话框**，按新要求实施
- 对话框要素（照搬 Fork）：标题 "Pull" + 副标题 "Pull remote branches and merge them into your local branch"；Remote 下拉；Branch 下拉（远程分支，显示 `{remote}/{name}`）；Into 行（当前本地分支，只读）；"Rebase instead of merge" / "Stash and reapply local changes" 两个勾选框；Cancel / Pull
- 后端新增：
  - `remote.rs`：`list_remotes`；`PullOptions {branch, rebase, stash}` + `pull_with_options`（fetch → 指定 `{remote}/{branch}` 或 upstream → merge_analysis：up-to-date 静默 / ff fast-forward / 分歧时 rebase 模式走 `rebase_branch`（Conflicts 报错且 HEAD 已还原）、否则 VersionConflict；stash 模式仅在工作区脏（含 untracked）时先 `save_stash`，pull 失败 best-effort pop 恢复、成功后 pop 失败报 "stash was kept"）
  - `use_cases.rs`：`pull` 签名扩展 + `list_remotes`；`lib.rs`：`cmd_pull` 可选参数 + `cmd_list_remotes` 注册
- 前端：`api.ts` `listRemotes` / `pullRemote(options)`；`useRemoteSync` / `useWorkingCopy` `pull(options?)` 透传；`BranchList` 完整对话框（remotes 查询仅打开时 enabled；Branch 选项来自 `getBranches` 远程分支按 `{remote}/` 前缀过滤，默认值兜底；打开时按 upstream 预填 remote/branch）
- 边界：Into 固定当前分支；后端无 rebase-pull 冲突续行 UI（冲突时本地提交原样保留并报错）；Fetch / Push 不加确认
- 顺带修复（环境问题，与本功能无关的既有失败）：`working_copy.rs` 3 个 discard 测试在 Windows `core.autocrlf=true` 下因 CRLF 断言失败——`test_helpers.rs` 夹具显式 `core.autocrlf=false` + 断言前 normalize 换行（已验证 HEAD 上同样失败，非本功能引入）
- 冒烟清单：ff pull / up-to-date / 分歧 merge 模式报冲突 / 分歧 rebase 模式 / stash 模式（脏→stash→pull→pop；pop 失败保留 stash）/ 无 upstream 指定 branch / 不存在的远程分支报错

## 验证

- [x] `cargo fmt` / `clippy --all-targets`（0 警告）/ `test`（111 通过，0 失败——含修复的 3 个 Windows autocrlf 既有失败）
- [x] `npm run typecheck` / `test`（49 通过）/ `lint` / `format:check` / `build` 全绿
- [ ] 手动冒烟：ff pull；up-to-date；分歧 + merge 模式报冲突；分歧 + rebase 模式；stash 模式（脏工作区 → stash → pull → pop，pop 失败保留 stash）；无 upstream 指定 branch；不存在的远程分支报错
