//! Reflog reading (M0 prerequisite for M2 "AI 误操作恢复").
//!
//! Read-only: this module never writes reflog entries — libgit2 appends them
//! automatically as refs move.

use git2::Repository;
use serde::Serialize;

use crate::domain::error::{AppError, Result};

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

/// One reflog entry. Lists come back newest-first.
#[derive(Debug, Clone, Serialize)]
pub struct ReflogEntry {
    pub old_oid: String,
    pub new_oid: String,
    /// Raw reflog message, e.g. `commit: fix x`, `reset: moving to v1.0`.
    pub message: String,
    /// Semantic action class derived from the message (see [`classify`]).
    pub action: String,
    pub committer: String,
    /// Unix epoch seconds.
    pub time: i64,
}

/// Classify a raw reflog message into a stable action id for the recovery
/// UI and AI prompts.
pub fn classify(message: &str) -> &'static str {
    let m = message.trim_start();
    let lower = m.to_lowercase();
    if lower.starts_with("commit (initial)") {
        "initial_commit"
    } else if lower.starts_with("commit (amend)") {
        "amend"
    } else if lower.starts_with("commit") {
        "commit"
    } else if lower.starts_with("checkout") {
        "checkout"
    } else if lower.starts_with("reset") {
        "reset"
    } else if lower.starts_with("merge") {
        "merge"
    } else if lower.starts_with("rebase") {
        "rebase"
    } else if lower.starts_with("pull") {
        "pull"
    } else if lower.starts_with("push") {
        "push"
    } else if lower.starts_with("branch") {
        "branch"
    } else if lower.starts_with("revert") {
        "revert"
    } else if lower.starts_with("cherry-pick") {
        "cherry_pick"
    } else if lower.starts_with("stash") {
        "stash"
    } else if lower.starts_with("clone") {
        "clone"
    } else {
        "other"
    }
}

/// Accept `HEAD` or a branch shorthand; anything containing `refs/` passes
/// through unchanged.
fn normalize_ref(reference: &str) -> String {
    let r = reference.trim();
    if r == "HEAD" || r.starts_with("refs/") {
        r.to_string()
    } else {
        format!("refs/heads/{r}")
    }
}

/// Full reflog of `reference` (HEAD or a branch), newest first — libgit2
/// stores and iterates reflog entries newest-first already.
pub fn list_reflog(repo: &Repository, reference: &str) -> Result<Vec<ReflogEntry>> {
    let name = normalize_ref(reference);
    if repo.find_reference(&name).is_err() {
        return Err(AppError::Protocol(format!(
            "reference not found: {reference}"
        )));
    }
    let log = repo.reflog(&name).map_err(map_git_err)?;
    if log.len() == 0 {
        return Err(AppError::Protocol(format!(
            "no reflog for reference: {reference}"
        )));
    }

    Ok(log
        .iter()
        .map(|entry| ReflogEntry {
            old_oid: entry.id_old().to_string(),
            new_oid: entry.id_new().to_string(),
            action: classify(entry.message().unwrap_or("")).to_string(),
            message: entry.message().unwrap_or("").to_string(),
            committer: entry.committer().name().unwrap_or("").to_string(),
            time: entry.committer().when().seconds(),
        })
        .collect())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::build_linear_repo;
    use std::fs;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    #[test]
    fn head_reflog_lists_commits_newest_first() {
        let (path, repo) = build_linear_repo(3);
        let log = list_reflog(&repo, "HEAD").unwrap();
        // build_linear_repo makes an initial commit + 2 more (3 entries); the
        // fixture may add exactly one reflog entry per commit.
        assert!(log.len() >= 3, "expected >=3 entries, got {}", log.len());
        assert!(log[0].message.starts_with("commit:"), "newest is a commit");
        assert_eq!(
            log[0].new_oid,
            repo.head().unwrap().target().unwrap().to_string()
        );
        // Newest-first: entry 0 is at least as new as the last one.
        assert!(log[0].time >= log[log.len() - 1].time);
        cleanup(&path);
    }

    #[test]
    fn branch_shorthand_resolves() {
        let (path, repo) = build_linear_repo(2);
        let base = repo.head().unwrap().peel_to_commit().unwrap().id();
        repo.branch("side", &repo.find_commit(base).unwrap(), false)
            .unwrap();
        // New branch reflog has one entry (branch creation).
        let log = list_reflog(&repo, "side").unwrap();
        assert_eq!(log.len(), 1);
        cleanup(&path);
    }

    #[test]
    fn classify_covers_common_git_operations() {
        assert_eq!(classify("commit: fix login"), "commit");
        assert_eq!(classify("commit (initial): bootstrap"), "initial_commit");
        assert_eq!(classify("commit (amend): fix"), "amend");
        assert_eq!(classify("checkout: moving from main to side"), "checkout");
        assert_eq!(classify("reset: moving to v1.0"), "reset");
        assert_eq!(classify("merge feat: Merge made by ort"), "merge");
        assert_eq!(
            classify("rebase (finish): refs/heads/main onto 9c3"),
            "rebase"
        );
        assert_eq!(classify("pull: fast-forward"), "pull");
        assert_eq!(classify("branch: Created from HEAD"), "branch");
        assert_eq!(classify("revert: Revert \"x\""), "revert");
        assert_eq!(classify("cherry-pick: onto main"), "cherry_pick");
        assert_eq!(classify("mystery operation"), "other");
    }

    #[test]
    fn missing_reference_errors_protocol() {
        let (path, repo) = build_linear_repo(1);
        let err = list_reflog(&repo, "no-such-branch").unwrap_err();
        assert_eq!(err.category(), "Protocol");
        cleanup(&path);
    }

    #[test]
    fn reflog_tracks_checkouts_and_resets() {
        let (path, repo) = build_linear_repo(2);
        let base = repo.head().unwrap().peel_to_commit().unwrap().id();
        repo.branch("side", &repo.find_commit(base).unwrap(), false)
            .unwrap();
        repo.set_head("refs/heads/side").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();
        let log = list_reflog(&repo, "HEAD").unwrap();
        assert!(
            log.iter().any(|e| e.message.starts_with("checkout")),
            "checkout should be recorded, got: {:?}",
            log.iter().map(|e| &e.message).collect::<Vec<_>>()
        );
        cleanup(&path);
    }
}
