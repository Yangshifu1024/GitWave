//! Error codes raised by `infrastructure/git/*`.

/// Raw libgit2 error text (untranslatable — the UI localizes the framing).
pub const RAW: &str = "git.raw";

/// Git 操作失败（libgit2 通用错误）。
pub const GIT_ERROR: &str = "git.error";

// ─── fetch / push / pull（remote.rs） ─────────────────────────────────

/// 认证失败。
pub const AUTH_FAILED: &str = "git.auth_failed";

/// 获取（fetch）认证失败。
pub const FETCH_AUTH_FAILED: &str = "git.fetch_auth_failed";

/// 获取（fetch）失败。
pub const FETCH_FAILED: &str = "git.fetch_failed";

/// 推送（push）认证失败。
pub const PUSH_AUTH_FAILED: &str = "git.push_auth_failed";

/// 推送（push）失败。
pub const PUSH_FAILED: &str = "git.push_failed";

/// 推送（push）被拒：远端引用已存在且不可快进（常见于同名 tag 指向不同提交）。
pub const PUSH_NON_FAST_FORWARD: &str = "git.push_non_fast_forward";

/// 网络同步操作（fetch/pull/push/删远端分支）超过总时限被中止。
pub const SYNC_TIMEOUT: &str = "git.sync_timeout";

/// 网络同步操作被用户主动取消。
pub const SYNC_CANCELLED: &str = "git.sync_cancelled";

/// 删除远程分支失败。
pub const DELETE_REMOTE_BRANCH_FAILED: &str = "git.delete_remote_branch_failed";

/// 远程没有配置 URL。
pub const REMOTE_NO_URL: &str = "git.remote_no_url";

/// 远程名不存在。
pub const REMOTE_NOT_FOUND: &str = "git.remote.not_found";

/// 远程已存在（libgit2 报错）。
pub const REMOTE_EXISTS: &str = "git.remote_exists";

/// 远程已存在（本地预检）。
pub const REMOTE_DUPLICATE: &str = "git.remote_duplicate";

/// 远程名不能为空。
pub const REMOTE_NAME_EMPTY: &str = "git.remote_name_empty";

/// 远程 URL 不能为空。
pub const REMOTE_URL_EMPTY: &str = "git.remote_url_empty";

/// 远程操作失败。
pub const REMOTE_OP_FAILED: &str = "git.remote_op_failed";

/// HEAD 处于 detached 状态，无法推送。
pub const PUSH_DETACHED_HEAD: &str = "git.push_detached_head";

/// HEAD 处于 detached 状态，无法拉取。
pub const PULL_DETACHED_HEAD: &str = "git.pull_detached_head";

/// 拉取完成但贮藏未能重新应用（已保留）。
pub const STASH_REAPPLY_FAILED: &str = "git.stash_reapply_failed";

/// 拉取失败且贮藏恢复也失败（已保留）。
pub const STASH_RESTORE_FAILED: &str = "git.stash_restore_failed";

/// 无法解析目标引用。
pub const CANNOT_RESOLVE_REF: &str = "git.cannot_resolve_ref";

/// 拉取需要干净的工作区。
pub const PULL_DIRTY_WORKTREE: &str = "git.pull_dirty_worktree";

/// 拉取（rebase 方式）遇到冲突，本地提交未被改动。
pub const PULL_REBASE_CONFLICTS: &str = "git.pull_rebase_conflicts";

/// 拉取需要合并，请改用 Rebase 或分支合并。
pub const PULL_NEEDS_MERGE: &str = "git.pull_needs_merge";

/// 拉取的合并分析结果异常。
pub const PULL_UNEXPECTED: &str = "git.pull_unexpected";

/// 变基结束但没有产生新 HEAD。
pub const REBASE_NO_NEW_HEAD: &str = "git.rebase_no_new_head";

// ─── working copy（working_copy.rs） ──────────────────────────────────

/// 提交信息不能为空。
pub const COMMIT_MESSAGE_EMPTY: &str = "git.commit_message_empty";

/// 没有可提交的内容。
pub const NOTHING_TO_COMMIT: &str = "git.nothing_to_commit";

/// 路径越出工作区。
pub const PATH_ESCAPES_WORKTREE: &str = "git.path_escapes_worktree";

/// 存在冲突，需先解决再丢弃。
pub const DISCARD_CONFLICTED: &str = "git.discard_conflicted";

/// 忽略规则（.gitignore）条目无效。
pub const INVALID_IGNORE_PATTERN: &str = "git.invalid_ignore_pattern";

/// 读取 .gitignore 失败。
pub const READ_GITIGNORE: &str = "git.read_gitignore";

/// 写入 .gitignore 失败。
pub const WRITE_GITIGNORE: &str = "git.write_gitignore";

/// HEAD 处于 detached 状态，无法重置。
pub const RESET_DETACHED_HEAD: &str = "git.reset_detached_head";

// ─── git 层共用 ───────────────────────────────────────────────────────

/// 裸仓库没有工作区。
pub const BARE_REPO: &str = "git.bare_repo";

/// 提交 id（oid）无效。
pub const INVALID_OID: &str = "git.invalid_oid";

/// 提交不存在。
pub const COMMIT_NOT_FOUND: &str = "git.commit_not_found";

/// 文件系统错误。
pub const FS_ERROR: &str = "git.fs_error";

/// HEAD 还没有任何提交（unborn）。
pub const HEAD_UNBORN: &str = "git.head_unborn";

/// 工作区不干净，请先提交或贮藏。
pub const DIRTY_WORKTREE: &str = "git.dirty_worktree";

// ─── LFS（lfs.rs） ────────────────────────────────────────────────────

/// LFS 跟踪模式无效。
pub const INVALID_LFS_PATTERN: &str = "git.lfs.invalid_pattern";

/// LFS 模式不能包含空格。
pub const LFS_PATTERN_SPACES: &str = "git.lfs.pattern_spaces";

/// LFS 模式不能以「#」开头。
pub const LFS_PATTERN_COMMENT: &str = "git.lfs.pattern_comment";

/// git lfs 命令执行失败。
pub const LFS_CLI_FAILED: &str = "git.lfs.cli_failed";

/// git lfs install 失败。
pub const LFS_INSTALL_FAILED: &str = "git.lfs.install_failed";

/// 读取 .gitattributes 失败。
pub const READ_GITATTRIBUTES: &str = "git.read_gitattributes";

/// 写入 .gitattributes 失败。
pub const WRITE_GITATTRIBUTES: &str = "git.write_gitattributes";

// ─── revert / cherry-pick（revert.rs） ────────────────────────────────

/// 不支持 revert merge commit。
pub const REVERT_MERGE_COMMIT: &str = "git.revert_merge_commit";

/// revert 遇到冲突。
pub const REVERT_CONFLICTS: &str = "git.revert_conflicts";

/// cherry-pick 遇到冲突。
pub const CHERRY_PICK_CONFLICTS: &str = "git.cherry_pick_conflicts";

/// cherry-pick 没有产生变更（提交已包含在当前分支）。
pub const CHERRY_PICK_NO_CHANGES: &str = "git.cherry_pick_no_changes";

// ─── 交互式变基（interactive_rebase.rs） ──────────────────────────────

/// 提交 id（oid）格式错误。
pub const BAD_OID: &str = "git.bad_oid";

/// 应用提交时发生冲突。
pub const APPLY_CONFLICT: &str = "git.apply_conflict";

/// 暂停状态序列化失败。
pub const SERIALIZE_PAUSE: &str = "git.pause_serialize";

/// 暂停状态写入失败。
pub const WRITE_PAUSE: &str = "git.pause_write";

/// 暂停状态解析失败。
pub const PARSE_PAUSE: &str = "git.pause_parse";

/// 没有处于 edit 暂停中的交互式变基。
pub const NOT_PAUSED: &str = "git.not_paused";

/// 不能对 todo 列表的第一个提交执行 squash/fixup。
pub const SQUASH_FIRST_COMMIT: &str = "git.squash_first_commit";

// ─── submodule（submodule.rs） ────────────────────────────────────────

/// 子模块不存在。
pub const SUBMODULE_NOT_FOUND: &str = "git.submodule.not_found";

/// 子模块 URL 与路径均为必填。
pub const SUBMODULE_ARGS_REQUIRED: &str = "git.submodule.args_required";

/// 子模块路径必须是仓库内的相对路径。
pub const SUBMODULE_PATH_INVALID: &str = "git.submodule.path_invalid";

/// 添加子模块失败。
pub const SUBMODULE_ADD_FAILED: &str = "git.submodule.add_failed";

/// 子模块克隆失败。
pub const SUBMODULE_CLONE_FAILED: &str = "git.submodule.clone_failed";

/// 子模块未初始化。
pub const SUBMODULE_NOT_INITIALIZED: &str = "git.submodule.not_initialized";

// ─── hooks（hooks.rs） ────────────────────────────────────────────────

/// hook 名称无效。
pub const INVALID_HOOK_NAME: &str = "git.hook.invalid_name";

/// 读取 hook 失败。
pub const READ_HOOK: &str = "git.hook.read_failed";

/// 写入 hook 失败。
pub const WRITE_HOOK: &str = "git.hook.write_failed";

/// 设置 hook 执行权限失败。
pub const HOOK_CHMOD: &str = "git.hook.chmod_failed";

// ─── init / clone（repo_adapter.rs） ──────────────────────────────────

/// 路径已经是仓库。
pub const INIT_EXISTS: &str = "git.init.exists";

/// 仓库初始化失败。
pub const INIT_FAILED: &str = "git.init.failed";

/// 克隆认证失败。
pub const CLONE_AUTH_FAILED: &str = "git.clone.auth_failed";

/// 克隆目标不存在。
pub const CLONE_NOT_FOUND: &str = "git.clone.not_found";

/// 克隆网络错误。
pub const CLONE_NETWORK: &str = "git.clone.network";

// ─── rebase（rebase.rs） ──────────────────────────────────────────────

/// HEAD 处于 detached 状态，无法完成变基。
pub const FINALIZE_DETACHED_HEAD: &str = "git.finalize_detached_head";

/// 分支引用没有名称。
pub const REF_NO_NAME: &str = "git.ref_no_name";

// ─── reflog（reflog.rs） ──────────────────────────────────────────────

/// 引用不存在。
pub const REF_NOT_FOUND: &str = "git.ref_not_found";

/// 该引用没有 reflog 记录。
pub const REFLOG_EMPTY: &str = "git.reflog_empty";

// ─── merge（merge.rs） ────────────────────────────────────────────────

/// 分支没有指向任何提交。
pub const BRANCH_NO_TARGET: &str = "git.branch_no_target";

// ─── 仓库打开（git2_adapter.rs） ──────────────────────────────────────

/// 路径不是 git 仓库。
pub const NOT_A_REPO: &str = "git.not_a_repo";

/// 打开仓库失败。
pub const OPEN_FAILED: &str = "git.open_failed";

// ─── worktree（worktree.rs） ──────────────────────────────────────────

/// 主 worktree 不能删除。
pub const REMOVE_MAIN_WORKTREE: &str = "git.worktree.remove_main";

// ─── stash（stash.rs） ────────────────────────────────────────────────

/// 贮藏 id（oid）无效。
pub const INVALID_STASH_OID: &str = "git.stash.invalid_oid";

// ─── branch（branch.rs） ──────────────────────────────────────────────

/// 不能删除当前检出的分支。
pub const DELETE_CHECKED_OUT_BRANCH: &str = "git.branch.delete_checked_out";

/// 分支被链接 worktree 占用，不能重命名。
pub const RENAME_IN_WORKTREE: &str = "git.branch.rename_in_worktree";

// ─── tag（tag.rs） ────────────────────────────────────────────────────

/// 标签名不能为空。
pub const TAG_NAME_EMPTY: &str = "git.tag.name_empty";
