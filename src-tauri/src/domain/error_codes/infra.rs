//! Error codes raised by the rest of `infrastructure/*` (ai, persistence,
//! ssh) — everything outside `infrastructure/git`.

// ─── AI provider（ai/provider.rs） ────────────────────────────────────

/// Provider 返回 HTTP 错误响应（401/403 为凭证问题，其余可重试）。
pub const PROVIDER_HTTP: &str = "ai.provider_http";

/// 所有 AI provider 尝试均失败且未捕获到错误。
pub const ALL_ATTEMPTS_FAILED: &str = "ai.all_attempts_failed";

/// 不支持的 AI provider。
pub const UNSUPPORTED_PROVIDER: &str = "ai.unsupported_provider";

/// OpenAI API key 未配置。
pub const OPENAI_KEY_MISSING: &str = "ai.openai.key_missing";

/// OpenAI 请求失败。
pub const OPENAI_REQUEST: &str = "ai.openai.request_failed";

/// OpenAI 响应解析失败。
pub const OPENAI_JSON: &str = "ai.openai.json_failed";

/// OpenAI 返回了空内容。
pub const OPENAI_EMPTY_CONTENT: &str = "ai.openai.empty_content";

/// Anthropic API key 未配置。
pub const ANTHROPIC_KEY_MISSING: &str = "ai.anthropic.key_missing";

/// Anthropic 请求失败。
pub const ANTHROPIC_REQUEST: &str = "ai.anthropic.request_failed";

/// Anthropic 响应解析失败。
pub const ANTHROPIC_JSON: &str = "ai.anthropic.json_failed";

/// Anthropic 模型因 max_tokens 仍在推理而未产出文本。
pub const ANTHROPIC_MAX_TOKENS: &str = "ai.anthropic.max_tokens";

/// Anthropic 响应中没有文本内容。
pub const ANTHROPIC_NO_TEXT: &str = "ai.anthropic.no_text";

/// 本地 Ollama 无法连接。
pub const OLLAMA_UNREACHABLE: &str = "ai.ollama.unreachable";

/// Ollama 探测失败。
pub const OLLAMA_PROBE_FAILED: &str = "ai.ollama.probe_failed";

/// Ollama 响应解析失败。
pub const OLLAMA_JSON: &str = "ai.ollama.json_failed";

/// Ollama 请求失败。
pub const OLLAMA_REQUEST: &str = "ai.ollama.request_failed";

/// Ollama 返回了空内容。
pub const OLLAMA_EMPTY_CONTENT: &str = "ai.ollama.empty_content";

// ─── AI keychain（ai/secrets.rs） ─────────────────────────────────────

/// 系统钥匙串访问失败。
pub const KEYCHAIN_ERROR: &str = "ai.keychain.error";

/// API key 不能为空。
pub const API_KEY_EMPTY: &str = "ai.key.empty";

/// API key 写入钥匙串失败。
pub const KEYCHAIN_SET: &str = "ai.keychain.set_failed";

/// API key 从钥匙串读取失败。
pub const KEYCHAIN_GET: &str = "ai.keychain.get_failed";

/// API key 从钥匙串删除失败。
pub const KEYCHAIN_DELETE: &str = "ai.keychain.delete_failed";

// ─── persistence（persistence/*） ─────────────────────────────────────

/// 工作区不存在。
pub const WORKSPACE_NOT_FOUND: &str = "persist.workspace.not_found";

/// 仓库记录不存在。
pub const REPO_NOT_FOUND: &str = "persist.repo.not_found";

/// 指定工作区下仓库记录不存在。
pub const REPO_NOT_FOUND_IN_WS: &str = "persist.repo.not_found_in_workspace";

/// 排序 id 与工作区现有仓库不匹配。
pub const REORDER_MISMATCH: &str = "persist.repo.reorder_mismatch";

/// SQLite 数据库错误。
pub const SQLITE_ERROR: &str = "persist.sqlite.error";

/// 数据序列化 / 反序列化失败。
pub const SERDE_ERROR: &str = "persist.serde.error";

/// 无法确定用户数据目录。
pub const DATA_DIR_RESOLVE: &str = "persist.data_dir.resolve";

/// 用户数据目录创建失败。
pub const DATA_DIR_CREATE: &str = "persist.data_dir.create";

/// 数据库迁移失败。
pub const MIGRATION_FAILED: &str = "persist.migration.failed";

// ─── ssh（ssh/keys.rs） ───────────────────────────────────────────────

/// ssh-add 命令启动失败。
pub const SSH_ADD_FAILED: &str = "ssh.add_failed";

/// ssh-add 执行失败（非零退出码）。
pub const SSH_ADD_EXIT: &str = "ssh.add_exit_failed";

/// 私钥带 passphrase，无法在 GUI 中加载。
pub const KEY_PASSPHRASE: &str = "ssh.key.passphrase";

/// ssh-agent 未运行。
pub const AGENT_NOT_RUNNING: &str = "ssh.agent.not_running";

/// ssh-add -d 移除密钥失败。
pub const SSH_DELETE_FAILED: &str = "ssh.delete_failed";

/// ssh 命令启动失败。
pub const SSH_SPAWN_FAILED: &str = "ssh.spawn_failed";
