//! Error codes raised by the Tauri command layer (`lib.rs`).

/// 等待克隆后台任务结束失败。
pub const CLONE_TASK_JOIN: &str = "cmds.clone_task_join";

/// 等待获取（fetch）后台任务结束失败。
pub const FETCH_TASK_JOIN: &str = "cmds.fetch_task_join";

/// 等待拉取（pull）后台任务结束失败。
pub const PULL_TASK_JOIN: &str = "cmds.pull_task_join";

/// 等待列出远程仓库后台任务结束失败。
pub const LIST_REMOTES_TASK_JOIN: &str = "cmds.list_remotes_task_join";

/// 等待列出仓库后台任务结束失败。
pub const LIST_REPOS_TASK_JOIN: &str = "cmds.list_repos_task_join";

/// 等待推送（push）后台任务结束失败。
pub const PUSH_TASK_JOIN: &str = "cmds.push_task_join";

// Populated by the i18n error-key migration (docs/tasks/feat-i18n/plan.md).
