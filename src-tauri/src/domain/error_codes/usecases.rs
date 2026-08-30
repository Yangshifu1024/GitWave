//! Error codes raised by `application/use_cases.rs`.

// ─── Repo ───────────────────────────────────────────────────────────────────

/// git 仓库打开失败（路径无效或不是仓库）。
pub const REPO_OPEN_FAILED: &str = "usecases.repo.open_failed";

/// 工作区中没有设置活动仓库。
pub const NO_ACTIVE_REPO: &str = "usecases.repo.no_active";

/// 活动仓库在 workspace 中不存在。
pub const REPO_NOT_FOUND: &str = "usecases.repo.not_found";

// ─── Workspace ──────────────────────────────────────────────────────────────

/// Workspace 名称为空。
pub const WORKSPACE_NAME_EMPTY: &str = "usecases.workspace.name_empty";

/// Workspace 不存在。
pub const WORKSPACE_NOT_FOUND: &str = "usecases.workspace.not_found";

// ─── Clone ──────────────────────────────────────────────────────────────────

/// clone 重试前清理目标目录失败。
pub const CLONE_DEST_CLEAR_FAILED: &str = "usecases.clone.dest_clear_failed";

// ─── AI provider chain ──────────────────────────────────────────────────────

/// AI 提供商不受支持。
pub const AI_PROVIDER_UNSUPPORTED: &str = "usecases.ai.provider_unsupported";

/// AI 故障转移列表中的提供商不受支持。
pub const AI_FAILOVER_PROVIDER_UNSUPPORTED: &str = "usecases.ai.failover_provider_unsupported";

/// Workspace 未配置 AI 提供商。
pub const AI_PROVIDER_NOT_CONFIGURED: &str = "usecases.ai.provider_not_configured";

/// 离线模式下云端 AI 调用被禁用。
pub const AI_OFFLINE_MODE: &str = "usecases.ai.offline_mode";

/// AI 提供商的 API key 未配置。
pub const AI_API_KEY_MISSING: &str = "usecases.ai.api_key_missing";

/// 所有 AI 提供商均调用失败。
pub const AI_ALL_PROVIDERS_FAILED: &str = "usecases.ai.all_providers_failed";

/// 健康报告序列化失败。
pub const HEALTH_SERIALIZE_FAILED: &str = "usecases.health.serialize_failed";

// ─── Commit / diff ──────────────────────────────────────────────────────────

/// 生成 commit message 前没有已暂存的变更。
pub const COMMIT_NO_STAGED: &str = "usecases.commit.no_staged";

/// commit OID 无法解析。
pub const COMMIT_OID_INVALID: &str = "usecases.commit.oid_invalid";

/// 文件 diff 的起始 OID 无法解析。
pub const FILE_DIFF_FROM_OID_INVALID: &str = "usecases.file_diff.from_oid_invalid";

/// 文件 diff 的目标 OID 无法解析。
pub const FILE_DIFF_TO_OID_INVALID: &str = "usecases.file_diff.to_oid_invalid";

// ─── Branch / PR ────────────────────────────────────────────────────────────

/// branch 创建后在仓库中未找到。
pub const BRANCH_NOT_FOUND_AFTER_CREATE: &str = "usecases.branch.not_found_after_create";

/// 未找到可用的 base branch。
pub const PR_NO_BASE_BRANCH: &str = "usecases.pr.no_base_branch";

/// 仓库还没有任何 commit，无法生成 PR 描述。
pub const PR_NO_COMMITS_YET: &str = "usecases.pr.no_commits_yet";

/// HEAD 与 base branch 之间没有共同祖先。
pub const PR_NO_COMMON_ANCESTOR: &str = "usecases.pr.no_common_ancestor";

/// 没有领先 base branch 的 commit，无法生成 PR 描述。
pub const PR_NO_COMMITS_AHEAD: &str = "usecases.pr.no_commits_ahead";

// ─── AI palette ─────────────────────────────────────────────────────────────

/// palette 请求内容为空。
pub const PALETTE_EMPTY_REQUEST: &str = "usecases.palette.empty_request";

/// AI 未返回 JSON 格式的 palette action。
pub const PALETTE_NO_JSON: &str = "usecases.palette.no_json";

/// AI 返回的 JSON 无法解析。
pub const PALETTE_MALFORMED_JSON: &str = "usecases.palette.malformed_json";

/// palette action 不在白名单内（commit / push / merge / rebase 永远禁止）。
pub const PALETTE_ACTION_FORBIDDEN: &str = "usecases.palette.action_forbidden";

/// AI action 缺少必需参数。
pub const PALETTE_PARAM_MISSING: &str = "usecases.palette.param_missing";

/// palette 仓库上下文序列化失败。
pub const PALETTE_CONTEXT_SERIALIZE_FAILED: &str = "usecases.palette.context_serialize_failed";

// ─── Rebase ─────────────────────────────────────────────────────────────────

/// rebase 要求干净的 worktree。
pub const REBASE_DIRTY_WORKTREE: &str = "usecases.rebase.dirty_worktree";

/// rebase 结束后未能确定新的 HEAD。
pub const REBASE_NO_NEW_HEAD: &str = "usecases.rebase.no_new_head";

// ─── LFS / gitignore ────────────────────────────────────────────────────────

/// 未安装 git lfs。
pub const LFS_NOT_INSTALLED: &str = "usecases.lfs.not_installed";

/// 读取 .gitignore 失败。
pub const GITIGNORE_READ_FAILED: &str = "usecases.gitignore.read_failed";

/// 写入 .gitignore 失败。
pub const GITIGNORE_WRITE_FAILED: &str = "usecases.gitignore.write_failed";

// ─── Workspace transfer ─────────────────────────────────────────────────────

/// 读取工作区传输文件失败。
pub const TRANSFER_READ_FAILED: &str = "usecases.transfer.read_failed";

/// 工作区传输文件内容无效。
pub const TRANSFER_INVALID: &str = "usecases.transfer.invalid";

/// 工作区传输文件版本不受支持。
pub const TRANSFER_VERSION_UNSUPPORTED: &str = "usecases.transfer.version_unsupported";

/// 工作区序列化为传输文件失败。
pub const TRANSFER_SERIALIZE_FAILED: &str = "usecases.transfer.serialize_failed";

/// 写入工作区传输文件失败。
pub const TRANSFER_WRITE_FAILED: &str = "usecases.transfer.write_failed";

/// 导入后的工作区随即消失（持久化异常）。
pub const TRANSFER_WORKSPACE_VANISHED: &str = "usecases.transfer.workspace_vanished";

// ─── Tests ──────────────────────────────────────────────────────────────────

/// 测试用：网络类错误样本。
pub const TEST_NETWORK: &str = "usecases.test.network";

/// 测试用：协议类错误样本。
pub const TEST_PROTOCOL: &str = "usecases.test.protocol";

/// 测试用：凭证类错误样本。
pub const TEST_CREDENTIAL: &str = "usecases.test.credential";

/// 测试用：未知类错误样本。
pub const TEST_UNKNOWN: &str = "usecases.test.unknown";

/// 测试用：模拟钥匙串不可用。
pub const TEST_KEYCHAIN_UNAVAILABLE: &str = "usecases.test.keychain_unavailable";
