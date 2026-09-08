//! Working-copy status, stage / unstage, and commit via libgit2.

use std::collections::HashMap;
use std::path::Path;

use git2::{Repository, Status, StatusOptions};

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;
use crate::domain::working_copy::{FileChange, FileStatusKind, WorkingCopy};
use crate::infrastructure::git::git2_adapter::commit_signature;
use crate::infrastructure::git::history::ahead_behind;

fn map_git_err(e: git2::Error) -> AppError {
    AppError::unknown_with(
        codes::git::GIT_ERROR,
        format!("git: {e}"),
        &[("error", e.to_string())],
    )
}

/// Build a `WorkingCopy` snapshot for `repo_id`.
pub fn status(repo: &Repository, repo_id: &str) -> Result<WorkingCopy> {
    let (branch, upstream, sha, head_message) = head_meta(repo)?;
    let (ahead, behind) = if branch != "(detached)" && branch != "(unborn)" && upstream.is_some() {
        ahead_behind(repo, &branch).unwrap_or((0, 0))
    } else {
        (0, 0)
    };

    let mut opts = StatusOptions::new();
    opts.include_untracked(true)
        .recurse_untracked_dirs(true)
        .include_unmodified(false)
        .renames_head_to_index(true)
        .renames_index_to_workdir(true);

    let statuses = repo.statuses(Some(&mut opts)).map_err(map_git_err)?;
    let mut files: Vec<FileChange> = Vec::new();

    for entry in statuses.iter() {
        let path = entry.path().unwrap_or("").to_string();
        if path.is_empty() {
            continue;
        }
        let s = entry.status();
        let old_path = entry.head_to_index().and_then(|d| {
            d.old_file()
                .path()
                .map(|p| p.to_string_lossy().into_owned())
        });

        if let Some(kind) = index_kind(s) {
            files.push(FileChange {
                path: path.clone(),
                old_path: old_path.clone(),
                kind,
                staged: true,
                additions: 0,
                deletions: 0,
            });
        }
        if let Some(kind) = worktree_kind(s) {
            files.push(FileChange {
                path,
                old_path: None,
                kind,
                staged: false,
                additions: 0,
                deletions: 0,
            });
        }
    }

    Ok(WorkingCopy {
        repo_id: repo_id.to_string(),
        branch,
        upstream,
        sha,
        head_message,
        ahead,
        behind,
        files,
    })
}

fn head_meta(repo: &Repository) -> Result<(String, Option<String>, String, Option<String>)> {
    match repo.head() {
        Ok(head) => {
            let commit = head.peel_to_commit().map_err(map_git_err)?;
            let sha = commit.id().to_string();
            // Full message (not just summary) so Amend can prefill the exact
            // text the user wrote last time.
            let message = Some(commit.message().unwrap_or("").trim_end().to_string());

            if head.is_branch() {
                let branch = head.shorthand().unwrap_or("(unknown)").to_string();
                let upstream = repo
                    .find_branch(&branch, git2::BranchType::Local)
                    .ok()
                    .and_then(|b| b.upstream().ok())
                    .and_then(|u| u.name().ok().flatten().map(str::to_string));
                Ok((branch, upstream, sha, message))
            } else {
                // Detached HEAD: amend is hidden by the UI (message stays but
                // branch name signals the state).
                Ok(("(detached)".into(), None, sha, None))
            }
        }
        Err(e) if e.code() == git2::ErrorCode::UnbornBranch => {
            let branch = repo
                .find_reference("HEAD")
                .ok()
                .and_then(|r| {
                    r.symbolic_target()
                        .map(|t| t.trim_start_matches("refs/heads/").to_string())
                })
                .unwrap_or_else(|| "main".into());
            Ok((branch, None, String::new(), None))
        }
        Err(e) => Err(map_git_err(e)),
    }
}

fn index_kind(s: Status) -> Option<FileStatusKind> {
    if s.contains(Status::INDEX_NEW) {
        Some(FileStatusKind::Added)
    } else if s.contains(Status::INDEX_DELETED) {
        Some(FileStatusKind::Deleted)
    } else if s.contains(Status::INDEX_RENAMED) {
        Some(FileStatusKind::Renamed)
    } else if s.contains(Status::INDEX_TYPECHANGE) || s.contains(Status::INDEX_MODIFIED) {
        Some(FileStatusKind::Modified)
    } else {
        None
    }
}

fn worktree_kind(s: Status) -> Option<FileStatusKind> {
    if s.contains(Status::WT_NEW) {
        Some(FileStatusKind::Untracked)
    } else if s.contains(Status::WT_DELETED) {
        Some(FileStatusKind::Deleted)
    } else if s.contains(Status::WT_RENAMED) {
        Some(FileStatusKind::Renamed)
    } else if s.contains(Status::WT_TYPECHANGE) || s.contains(Status::WT_MODIFIED) {
        Some(FileStatusKind::Modified)
    } else {
        None
    }
}

/// Stage paths into the index (idempotent).
pub fn stage_paths(repo: &Repository, paths: &[String]) -> Result<()> {
    let mut index = repo.index().map_err(map_git_err)?;
    for path in paths {
        let p = Path::new(path);
        let abs = repo.workdir().map(|w| w.join(p));
        let deleted = abs.as_ref().is_some_and(|a| !a.exists());
        if deleted {
            // Path relative for index API
            index.remove_path(p).map_err(map_git_err)?;
        } else {
            index.add_path(p).map_err(map_git_err)?;
        }
    }
    index.write().map_err(map_git_err)?;
    Ok(())
}

/// Unstage paths (reset index entries to HEAD). Idempotent for untracked.
pub fn unstage_paths(repo: &Repository, paths: &[String]) -> Result<()> {
    if paths.is_empty() {
        return Ok(());
    }
    match repo.head() {
        Ok(head) => {
            let obj = head.peel(git2::ObjectType::Commit).map_err(map_git_err)?;
            repo.reset_default(Some(&obj), paths).map_err(map_git_err)?;
        }
        Err(e) if e.code() == git2::ErrorCode::UnbornBranch => {
            // No HEAD commit yet — unstage = remove from index.
            let mut index = repo.index().map_err(map_git_err)?;
            for path in paths {
                let _ = index.remove_path(Path::new(path));
            }
            index.write().map_err(map_git_err)?;
        }
        Err(e) => return Err(map_git_err(e)),
    }
    Ok(())
}

/// Stage every dirty path (tracked modifications + untracked).
pub fn stage_all(repo: &Repository) -> Result<()> {
    let snap = status(repo, "")?;
    let mut paths = Vec::new();
    let mut seen = HashMap::new();
    for f in snap.files {
        if !f.staged && seen.insert(f.path.clone(), ()).is_none() {
            paths.push(f.path);
        }
    }
    stage_paths(repo, &paths)
}

/// Resolve MERGE_HEAD to the commit it names (the branch being merged in).
fn read_merge_head(repo: &Repository) -> Result<git2::Commit<'_>> {
    let raw = std::fs::read_to_string(repo.path().join("MERGE_HEAD")).map_err(|e| {
        AppError::unknown_with(
            codes::git::FS_ERROR,
            format!("read MERGE_HEAD: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    // An octopus merge (several heads) leaves one oid per line here;
    // committing it with a single extra parent would silently drop the
    // others from the history, so refuse with an actionable message.
    let (first, rest) = match raw.split_once('\n') {
        Some((first, rest)) => (first, rest.trim()),
        None => (raw.as_str(), ""),
    };
    if !rest.is_empty() {
        return Err(AppError::protocol(
            codes::git::GIT_ERROR,
            "octopus merge (multiple MERGE_HEAD entries) is not supported — finish or abort the merge with git",
        ));
    }
    let oid = git2::Oid::from_str(first.trim()).map_err(map_git_err)?;
    repo.find_commit(oid).map_err(map_git_err)
}

/// Create a commit from the current index. Returns new commit SHA.
/// Never auto-runs: caller must pass an explicit message (P1).
///
/// While a merge is in progress (MERGE_HEAD present) the commit finishes
/// the merge: MERGE_HEAD becomes the second parent and the merge state is
/// cleaned up afterwards — libgit2's `commit`, unlike the git CLI, leaves
/// MERGE_HEAD in place.
pub fn commit(repo: &Repository, message: &str) -> Result<String> {
    let trimmed = message.trim();
    if trimmed.is_empty() {
        return Err(AppError::protocol(
            codes::git::COMMIT_MESSAGE_EMPTY,
            "commit message cannot be empty",
        ));
    }

    let mut index = repo.index().map_err(map_git_err)?;
    let tree_oid = index.write_tree().map_err(map_git_err)?;
    let tree = repo.find_tree(tree_oid).map_err(map_git_err)?;
    let sig = commit_signature(repo)?;

    let parent_commit = match repo.head() {
        Ok(h) => Some(h.peel_to_commit().map_err(map_git_err)?),
        Err(e) if e.code() == git2::ErrorCode::UnbornBranch => None,
        Err(e) => return Err(map_git_err(e)),
    };

    let merge_head = if super::conflict::is_merge_in_progress(repo) {
        Some(read_merge_head(repo)?)
    } else {
        None
    };

    // Refuse empty commits (tree identical to parent) — except when
    // finishing a merge: an all-"ours" resolution yields the HEAD tree but
    // the commit is still required to record the second parent.
    if let Some(ref parent) = parent_commit {
        let parent_tree = parent.tree().map_err(map_git_err)?;
        if merge_head.is_none() && parent_tree.id() == tree.id() {
            return Err(AppError::protocol(
                codes::git::NOTHING_TO_COMMIT,
                "nothing to commit",
            ));
        }
    } else if tree.is_empty() {
        return Err(AppError::protocol(
            codes::git::NOTHING_TO_COMMIT,
            "nothing to commit",
        ));
    }

    let mut parents: Vec<&git2::Commit<'_>> = parent_commit.iter().collect();
    if let Some(ref merge_commit) = merge_head {
        parents.push(merge_commit);
    }
    let oid = repo
        .commit(Some("HEAD"), &sig, &sig, trimmed, &tree, &parents)
        .map_err(map_git_err)?;

    // Finishing a merge: clear MERGE_HEAD / MERGE_MSG / MERGE_MODE so
    // is_merge_in_progress (and the UI banner) settles. Best-effort, like
    // the other cleanup_state call sites (merge / revert / rebase).
    if merge_head.is_some() {
        let _ = repo.cleanup_state();
    }
    Ok(oid.to_string())
}

/// Amend HEAD: replace it with a new commit that has the same parents but
/// the current index as tree and `message` as message (`git commit --amend`).
/// Returns the new commit SHA. Never auto-runs: caller must pass an explicit
/// message after the user confirmed the history rewrite (P1).
///
/// Semantics match git: the original author (name/email/time) is preserved,
/// only the committer identity/timestamp is refreshed. Allowed with an empty
/// staged set — amending message-only is a legal `--amend` use.
pub fn amend_commit(repo: &Repository, message: &str) -> Result<String> {
    let trimmed = message.trim();
    if trimmed.is_empty() {
        return Err(AppError::protocol(
            codes::git::COMMIT_MESSAGE_EMPTY,
            "commit message cannot be empty",
        ));
    }
    if super::conflict::is_merge_in_progress(repo) {
        return Err(AppError::protocol(
            codes::git::AMEND_MERGE_IN_PROGRESS,
            "amend is not allowed while a merge is in progress — finish or abort the merge first",
        ));
    }

    let head = match repo.head() {
        Ok(h) => h,
        Err(e) if e.code() == git2::ErrorCode::UnbornBranch => {
            return Err(AppError::protocol(
                codes::git::HEAD_UNBORN,
                "no commit to amend — HEAD is unborn",
            ));
        }
        Err(e) => return Err(map_git_err(e)),
    };
    let branch_ref = if head.is_branch() {
        Some(head.name().unwrap_or("HEAD").to_string())
    } else {
        // Detached HEAD has no branch to move; refuse like `reset_head_hard`.
        return Err(AppError::protocol(
            codes::git::RESET_DETACHED_HEAD,
            "detached HEAD — checkout a branch before amending",
        ));
    };
    let old_commit = head.peel_to_commit().map_err(map_git_err)?;

    let mut index = repo.index().map_err(map_git_err)?;
    let tree_oid = index.write_tree().map_err(map_git_err)?;
    let tree = repo.find_tree(tree_oid).map_err(map_git_err)?;

    // Preserve the original author (identity + time); refresh only the
    // committer signature, matching `git commit --amend`.
    let original_author = old_commit.author();
    let committer = commit_signature(repo)?;

    let parent_commits: Vec<git2::Commit<'_>> = (0..old_commit.parent_count())
        .filter_map(|i| old_commit.parent(i).ok())
        .collect();
    let parents: Vec<&git2::Commit<'_>> = parent_commits.iter().collect();

    let oid = repo
        .commit(None, &original_author, &committer, trimmed, &tree, &parents)
        .map_err(map_git_err)?;

    // Move the branch to the rewritten commit, recording a dedicated
    // `commit (amend)` reflog entry (what `git commit --amend` writes) so
    // the undo panel classifies and can restore the pre-amend commit.
    // libgit2 mirrors the update into HEAD's reflog too when the branch is
    // checked out (same as git), so the Reflog panel sees it without extra
    // bookkeeping here.
    let reflog_msg = format!("commit (amend): {}", trimmed.lines().next().unwrap_or(""));
    repo.reference(
        branch_ref.as_deref().unwrap_or("HEAD"),
        oid,
        true,
        &reflog_msg,
    )
    .map_err(map_git_err)?;

    Ok(oid.to_string())
}

/// Discard unstaged worktree changes for `paths` (≡ `git restore <path>`).
///
/// Paths tracked in the index are restored from the index (covers modified
/// and deleted-in-worktree files); untracked files are removed from disk.
pub fn discard_worktree_changes(repo: &Repository, paths: &[String]) -> Result<()> {
    if paths.is_empty() {
        return Ok(());
    }
    let mut index = repo.index().map_err(map_git_err)?;
    let workdir = repo.workdir().ok_or_else(|| {
        AppError::protocol(codes::git::BARE_REPO, "bare repository has no worktree")
    })?;

    let mut tracked: Vec<String> = Vec::new();
    for path in paths {
        // Reject anything that could leave the worktree before libgit2 sees
        // it: absolute paths replace the base in `PathBuf::join`, and both
        // it and libgit2's own checks treat `..` differently.
        if Path::new(path).is_absolute() || path.split(['/', '\\']).any(|seg| seg == "..") {
            return Err(AppError::protocol_with(
                codes::git::PATH_ESCAPES_WORKTREE,
                format!("path escapes worktree: {path}"),
                &[("path", path.clone())],
            ));
        }
        let abs = workdir.join(path);
        if !abs.starts_with(workdir) {
            return Err(AppError::protocol_with(
                codes::git::PATH_ESCAPES_WORKTREE,
                format!("path escapes worktree: {path}"),
                &[("path", path.clone())],
            ));
        }

        let status = repo.status_file(Path::new(path)).map_err(map_git_err)?;
        if status.contains(Status::CONFLICTED) {
            // Stage 0 is absent during a conflict — deleting here would
            // destroy one side of it.
            return Err(AppError::protocol_with(
                codes::git::DISCARD_CONFLICTED,
                format!("resolve conflicts before discarding: {path}"),
                &[("path", path.clone())],
            ));
        }
        if status.contains(Status::WT_NEW) {
            match std::fs::remove_file(&abs) {
                Ok(()) => {}
                // Already gone — treat as discarded.
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
                Err(e) => {
                    return Err(AppError::unknown_with(
                        codes::git::FS_ERROR,
                        format!("fs: {e}"),
                        &[("error", e.to_string())],
                    ))
                }
            }
        } else if !status.is_empty() {
            tracked.push(path.clone());
        }
        // Empty status = clean at HEAD, nothing to discard.
    }

    if !tracked.is_empty() {
        // Restore worktree copies from the index without touching it.
        let mut opts = git2::build::CheckoutBuilder::new();
        opts.force()
            .update_index(false)
            .disable_pathspec_match(true);
        for path in &tracked {
            opts.path(path.as_str());
        }
        repo.checkout_index(Some(&mut index), Some(&mut opts))
            .map_err(map_git_err)?;
    }
    Ok(())
}

/// Append `pattern` to the repo-root `.gitignore`. Idempotent per line.
/// Note: `.gitignore` only affects untracked paths (standard Git behavior).
pub fn ignore_path(repo: &Repository, pattern: &str) -> Result<()> {
    if pattern.trim().is_empty() || pattern.contains('\n') {
        return Err(AppError::protocol(
            codes::git::INVALID_IGNORE_PATTERN,
            "invalid ignore pattern",
        ));
    }
    let workdir = repo.workdir().ok_or_else(|| {
        AppError::protocol(codes::git::BARE_REPO, "bare repository has no worktree")
    })?;
    let gitignore = workdir.join(".gitignore");
    // Missing file is the normal first-run case — treat as empty. Any other
    // read error (e.g. non-UTF-8 bytes) must abort rather than overwrite the
    // user's existing rules with an almost-empty file below.
    let existing = match std::fs::read_to_string(&gitignore) {
        Ok(content) => content,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => String::new(),
        Err(e) => {
            return Err(AppError::unknown_with(
                codes::git::READ_GITIGNORE,
                format!("fs: read .gitignore: {e}"),
                &[("error", e.to_string())],
            ))
        }
    };

    if existing.lines().any(|line| line == pattern) {
        return Ok(());
    }
    let mut next = existing;
    if !next.is_empty() && !next.ends_with('\n') {
        next.push('\n');
    }
    next.push_str(pattern);
    next.push('\n');
    std::fs::write(&gitignore, next).map_err(|e| {
        AppError::unknown_with(
            codes::git::WRITE_GITIGNORE,
            format!("fs: write .gitignore: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::conflict::{is_merge_in_progress, resolve_conflict};
    use crate::infrastructure::git::test_helpers::{build_conflicted_merge, build_linear_repo};
    use std::fs;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    /// `core.autocrlf` turns LF into CRLF on checkout on Windows; compare
    /// line-ending-insensitively.
    fn normalize(text: &str) -> String {
        text.replace("\r\n", "\n")
    }

    #[test]
    fn commit_while_merging_records_second_parent_and_cleans_up() {
        let (path, repo, feature_tip) = build_conflicted_merge();

        // Resolving the last conflict does not end the merge by itself.
        resolve_conflict(&repo, "file0.txt", "resolved\n").unwrap();
        assert!(is_merge_in_progress(&repo));

        let sha = commit(&repo, "merge feature").unwrap();
        let done = repo
            .find_commit(git2::Oid::from_str(&sha).unwrap())
            .unwrap();

        // The commit is a real merge commit: HEAD + the merged branch tip.
        assert_eq!(done.parent_count(), 2);
        assert_eq!(done.parent_id(1).unwrap(), feature_tip);
        // Merge state files are gone, so the UI banner can settle.
        assert!(!is_merge_in_progress(&repo));
        assert!(!repo.path().join("MERGE_MSG").exists());

        cleanup(&path);
    }

    #[test]
    fn commit_while_merging_allows_tree_identical_to_head() {
        let (path, repo, feature_tip) = build_conflicted_merge();

        // All-"ours" resolution: the tree ends up identical to HEAD's, and
        // the empty-commit refusal must not block finishing the merge.
        resolve_conflict(&repo, "file0.txt", "main\n").unwrap();
        let sha = commit(&repo, "merge feature").unwrap();
        let done = repo
            .find_commit(git2::Oid::from_str(&sha).unwrap())
            .unwrap();

        assert_eq!(done.parent_count(), 2);
        assert_eq!(done.parent_id(1).unwrap(), feature_tip);
        assert!(!is_merge_in_progress(&repo));

        cleanup(&path);
    }

    #[test]
    fn commit_refuses_empty_tree_without_merge() {
        let (path, repo) = build_linear_repo(1);

        // No MERGE_HEAD: tree == HEAD tree stays a refused empty commit.
        let err = commit(&repo, "empty").unwrap_err();
        assert_eq!(err.code(), codes::git::NOTHING_TO_COMMIT);

        cleanup(&path);
    }

    #[test]
    fn commit_with_unparseable_merge_head_fails_and_keeps_state() {
        let (path, repo, _feature_tip) = build_conflicted_merge();
        resolve_conflict(&repo, "file0.txt", "resolved\n").unwrap();
        fs::write(repo.path().join("MERGE_HEAD"), "not-an-oid\n").unwrap();

        // Fail-safe: committing fails, the merge state is untouched.
        let err = commit(&repo, "merge feature").unwrap_err();
        assert_eq!(err.code(), codes::git::GIT_ERROR);
        assert!(is_merge_in_progress(&repo));

        cleanup(&path);
    }

    #[test]
    fn commit_refuses_octopus_merge_head() {
        let (path, repo, feature_tip) = build_conflicted_merge();
        resolve_conflict(&repo, "file0.txt", "resolved\n").unwrap();
        let tip = feature_tip.to_string();
        fs::write(repo.path().join("MERGE_HEAD"), format!("{tip}\n{tip}\n")).unwrap();

        let err = commit(&repo, "merge feature").unwrap_err();
        assert_eq!(err.code(), codes::git::GIT_ERROR);
        assert!(err.message().contains("octopus"));
        assert!(is_merge_in_progress(&repo));

        cleanup(&path);
    }

    #[test]
    fn amend_replaces_head_and_reflogs_amend() {
        let (path, repo) = build_linear_repo(2);
        let old_head = repo.head().unwrap().peel_to_commit().unwrap();
        let old_sha = old_head.id();
        let old_parent = old_head.parent_id(0).unwrap();

        fs::write(path.join("late.txt"), "forgotten\n").unwrap();
        stage_paths(&repo, &["late.txt".into()]).unwrap();
        let new_sha = amend_commit(&repo, "amended: two files").unwrap();

        let head = repo.head().unwrap().peel_to_commit().unwrap();
        assert_ne!(head.id(), old_sha, "HEAD must move to the rewritten commit");
        assert_eq!(head.id().to_string(), new_sha);
        assert_eq!(
            head.parent_id(0).unwrap(),
            old_parent,
            "parents must be preserved"
        );
        assert_eq!(head.message().unwrap(), "amended: two files");
        // The forgotten file is now part of HEAD and the worktree is clean.
        assert!(status(&repo, "r-1").unwrap().files.is_empty());

        // Reflog panel (reads HEAD) classifies the rewrite as amend and can
        // restore the pre-amend commit.
        let log = crate::infrastructure::git::reflog::list_reflog(&repo, "HEAD").unwrap();
        assert_eq!(log[0].action, "amend");
        assert_eq!(log[0].old_oid, old_sha.to_string());
        assert_eq!(log[0].new_oid, new_sha);
        let branch_log = crate::infrastructure::git::reflog::list_reflog(&repo, "main").unwrap();
        assert_eq!(branch_log[0].action, "amend");

        cleanup(&path);
    }

    #[test]
    fn amend_preserves_author_and_refreshes_committer() {
        let (path, repo) = build_linear_repo(1);
        let old_head = repo.head().unwrap().peel_to_commit().unwrap();
        let old_author = old_head.author();

        fs::write(path.join("late.txt"), "x\n").unwrap();
        stage_paths(&repo, &["late.txt".into()]).unwrap();
        amend_commit(&repo, "amended: author kept").unwrap();

        let head = repo.head().unwrap().peel_to_commit().unwrap();
        let author = head.author();
        assert_eq!(author.name(), old_author.name(), "author name preserved");
        assert_eq!(author.email(), old_author.email(), "author email preserved");
        assert_eq!(
            author.when().seconds(),
            old_author.when().seconds(),
            "author time preserved (git --amend semantics)"
        );

        cleanup(&path);
    }

    #[test]
    fn amend_with_empty_index_only_rewrites_message() {
        let (path, repo) = build_linear_repo(2);
        let old_head = repo.head().unwrap().peel_to_commit().unwrap();
        let old_tree = old_head.tree().unwrap().id();

        // Nothing staged — message-only amend is a legal `--amend` use.
        amend_commit(&repo, "amended: fix typo").unwrap();

        let head = repo.head().unwrap().peel_to_commit().unwrap();
        assert_eq!(head.tree().unwrap().id(), old_tree, "tree unchanged");
        assert_eq!(head.message().unwrap(), "amended: fix typo");

        cleanup(&path);
    }

    #[test]
    fn amend_refuses_unborn_and_detached_and_merge() {
        let (path, repo) = build_linear_repo(1);

        // Detached HEAD.
        let tip = repo.head().unwrap().peel_to_commit().unwrap();
        repo.set_head_detached(tip.id()).unwrap();
        let err = amend_commit(&repo, "nope").unwrap_err();
        assert_eq!(err.code(), codes::git::RESET_DETACHED_HEAD);

        // Merge in progress.
        repo.set_head("refs/heads/main").unwrap();
        fs::write(repo.path().join("MERGE_HEAD"), format!("{}\n", tip.id())).unwrap();
        let err = amend_commit(&repo, "nope").unwrap_err();
        assert_eq!(err.code(), codes::git::AMEND_MERGE_IN_PROGRESS);

        cleanup(&path);
    }

    #[test]
    fn amend_refuses_empty_message() {
        let (path, repo) = build_linear_repo(1);
        let err = amend_commit(&repo, "   ").unwrap_err();
        assert_eq!(err.code(), codes::git::COMMIT_MESSAGE_EMPTY);
        cleanup(&path);
    }

    #[test]
    fn status_sees_modified_and_untracked() {
        let (path, repo) = build_linear_repo(2);
        fs::write(path.join("file1.txt"), "changed\n").unwrap();
        fs::write(path.join("new.txt"), "fresh\n").unwrap();

        let wc = status(&repo, "r-1").unwrap();
        cleanup(&path);

        assert!(
            wc.files
                .iter()
                .any(|f| f.path == "file1.txt" && !f.staged && f.kind == FileStatusKind::Modified),
            "expected unstaged modified file1.txt, got {:?}",
            wc.files
        );
        assert!(
            wc.files
                .iter()
                .any(|f| f.path == "new.txt" && !f.staged && f.kind == FileStatusKind::Untracked),
            "expected untracked new.txt, got {:?}",
            wc.files
        );
    }

    #[test]
    fn stage_then_commit_roundtrip() {
        let (path, repo) = build_linear_repo(1);
        fs::write(path.join("extra.txt"), "x\n").unwrap();
        stage_paths(&repo, &["extra.txt".into()]).unwrap();
        let wc = status(&repo, "r-1").unwrap();
        assert!(wc.files.iter().any(|f| f.path == "extra.txt" && f.staged));

        let sha = commit(&repo, "add extra").unwrap();
        assert_eq!(sha.len(), 40);
        let wc = status(&repo, "r-1").unwrap();
        assert!(
            wc.files.is_empty(),
            "clean after commit, got {:?}",
            wc.files
        );
        cleanup(&path);
    }

    #[test]
    fn unstage_returns_to_untracked() {
        let (path, repo) = build_linear_repo(1);
        fs::write(path.join("u.txt"), "u\n").unwrap();
        stage_paths(&repo, &["u.txt".into()]).unwrap();
        unstage_paths(&repo, &["u.txt".into()]).unwrap();
        let wc = status(&repo, "r-1").unwrap();
        cleanup(&path);
        assert!(
            wc.files.iter().any(|f| f.path == "u.txt" && !f.staged),
            "expected unstaged after unstage: {:?}",
            wc.files
        );
    }

    #[test]
    fn discard_restores_modified_file_from_index() {
        let (path, repo) = build_linear_repo(2);
        fs::write(path.join("file1.txt"), "hacked\n").unwrap();

        discard_worktree_changes(&repo, &["file1.txt".into()]).unwrap();

        let restored = fs::read_to_string(path.join("file1.txt")).unwrap();
        let wc = status(&repo, "r-1").unwrap();
        cleanup(&path);
        assert_eq!(
            normalize(&restored),
            "v1\n",
            "worktree should match index content"
        );
        assert!(
            !wc.files.iter().any(|f| f.path == "file1.txt" && !f.staged),
            "no unstaged entry expected after discard: {:?}",
            wc.files
        );
    }

    #[test]
    fn discard_removes_untracked_file() {
        let (path, repo) = build_linear_repo(1);
        fs::write(path.join("fresh.txt"), "x\n").unwrap();

        discard_worktree_changes(&repo, &["fresh.txt".into()]).unwrap();

        let gone = !path.join("fresh.txt").exists();
        let wc = status(&repo, "r-1").unwrap();
        cleanup(&path);
        assert!(gone, "untracked file should be deleted");
        assert!(!wc.files.iter().any(|f| f.path == "fresh.txt"));
    }

    #[test]
    fn discard_restores_deleted_worktree_file() {
        let (path, repo) = build_linear_repo(1);
        fs::remove_file(path.join("file0.txt")).unwrap();
        assert!(!path.join("file0.txt").exists());

        discard_worktree_changes(&repo, &["file0.txt".into()]).unwrap();

        let restored = fs::read_to_string(path.join("file0.txt")).unwrap();
        cleanup(&path);
        assert_eq!(
            normalize(&restored),
            "v0\n",
            "deleted file should be recreated"
        );
    }

    #[test]
    fn ignore_appends_pattern_and_hides_path_from_status() {
        let (path, repo) = build_linear_repo(1);
        fs::write(path.join("gen.tmp"), "junk\n").unwrap();

        ignore_path(&repo, "*.tmp").unwrap();
        ignore_path(&repo, "*.tmp").unwrap(); // idempotent

        let gitignore = fs::read_to_string(path.join(".gitignore")).unwrap();
        let wc = status(&repo, "r-1").unwrap();
        cleanup(&path);
        assert_eq!(gitignore.lines().filter(|l| *l == "*.tmp").count(), 1);
        assert!(!wc.files.iter().any(|f| f.path == "gen.tmp"));
    }

    #[test]
    fn discard_restores_partial_staged_state_from_index() {
        let (path, repo) = build_linear_repo(2); // file1.txt = "v1\n" at HEAD
        fs::write(path.join("file1.txt"), "v2\n").unwrap();
        stage_paths(&repo, &["file1.txt".into()]).unwrap();
        fs::write(path.join("file1.txt"), "v3\n").unwrap();

        discard_worktree_changes(&repo, &["file1.txt".into()]).unwrap();

        let restored = fs::read_to_string(path.join("file1.txt")).unwrap();
        let wc = status(&repo, "r-1").unwrap();
        cleanup(&path);
        assert_eq!(
            normalize(&restored),
            "v2\n",
            "discard restores the staged version"
        );
        assert!(
            wc.files.iter().any(|f| f.path == "file1.txt" && f.staged),
            "staged v2 entry must survive discard: {:?}",
            wc.files
        );
    }

    #[test]
    fn discard_rejects_paths_outside_worktree() {
        let (path, repo) = build_linear_repo(1);

        let err = discard_worktree_changes(&repo, &["../outside.txt".into()])
            .expect_err("path escaping worktree must be rejected");
        cleanup(&path);
        assert_eq!(err.category(), "Protocol");
    }
}

/// Force-move the current branch to `oid` and refresh index + worktree
/// (`git reset --hard <oid>`). Recovery operation for the M2 undo panel —
/// user-confirmed upstream; detached HEAD is refused because there is no
/// branch to move.
pub fn reset_head_hard(repo: &Repository, oid_str: &str) -> Result<()> {
    let oid = git2::Oid::from_str(oid_str).map_err(|e| {
        AppError::protocol_with(
            codes::git::INVALID_OID,
            format!("invalid oid: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    let target = repo.find_commit(oid).map_err(|_| {
        AppError::protocol_with(
            codes::git::COMMIT_NOT_FOUND,
            format!("commit not found: {oid_str}"),
            &[("oid", oid_str.to_string())],
        )
    })?;

    let head = repo.head().map_err(map_git_err)?;
    if !head.is_branch() {
        return Err(AppError::protocol(
            codes::git::RESET_DETACHED_HEAD,
            "detached HEAD — checkout a branch before resetting",
        ));
    }
    repo.reset(&target.into_object(), git2::ResetType::Hard, None)
        .map_err(map_git_err)?;
    Ok(())
}

#[cfg(test)]
mod reset_tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::build_linear_repo;
    use std::fs;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    #[test]
    fn reset_hard_moves_branch_and_worktree() {
        let (path, repo) = build_linear_repo(3);
        let tip = repo.head().unwrap().peel_to_commit().unwrap();
        let older = tip.parent(0).unwrap();

        reset_head_hard(&repo, &older.id().to_string()).unwrap();

        let head = repo.head().unwrap().peel_to_commit().unwrap();
        assert_eq!(head.id(), older.id(), "branch ref must move back");
        assert!(
            !repo.workdir().unwrap().join("file2.txt").exists(),
            "worktree must match the older tree"
        );
        cleanup(&path);
    }

    #[test]
    fn reset_hard_appends_reflog_entry() {
        let (path, repo) = build_linear_repo(3);
        let tip = repo.head().unwrap().peel_to_commit().unwrap();
        let older = tip.parent(0).unwrap();

        reset_head_hard(&repo, &older.id().to_string()).unwrap();

        // The M2 panel refresh depends on the reset showing up in the reflog.
        let log = crate::infrastructure::git::reflog::list_reflog(&repo, "HEAD").unwrap();
        assert_eq!(log[0].action, "reset");
        assert_eq!(log[0].new_oid, older.id().to_string());
        cleanup(&path);
    }

    #[test]
    fn reset_hard_discards_dirty_worktree() {
        let (path, repo) = build_linear_repo(2);
        let tip = repo.head().unwrap().peel_to_commit().unwrap();
        let older = tip.parent(0).unwrap();
        fs::write(
            repo.workdir().unwrap().join("file1.txt"),
            "dirty edit
",
        )
        .unwrap();
        fs::write(
            repo.workdir().unwrap().join("stray.txt"),
            "stray
",
        )
        .unwrap();

        reset_head_hard(&repo, &older.id().to_string()).unwrap();

        // file1.txt was added by the reverted commit — it must be gone.
        assert!(
            !repo.workdir().unwrap().join("file1.txt").exists(),
            "file added by the reset-away commit must disappear"
        );
        // git semantics: reset --hard reverts tracked files but leaves
        // untracked files alone (that is `git clean`'s job).
        assert!(
            repo.workdir().unwrap().join("stray.txt").exists(),
            "untracked files survive a hard reset"
        );
        cleanup(&path);
    }

    #[test]
    fn reset_hard_refuses_detached_head() {
        let (path, repo) = build_linear_repo(2);
        let tip = repo.head().unwrap().peel_to_commit().unwrap();
        let older = tip.parent(0).unwrap();
        repo.set_head_detached(older.id()).unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();

        let err = reset_head_hard(&repo, &tip.id().to_string()).unwrap_err();
        assert_eq!(err.category(), "Protocol");
        cleanup(&path);
    }
}
