//! Working-copy status, stage / unstage, and commit via libgit2.

use std::collections::HashMap;
use std::path::Path;

use git2::{Repository, Status, StatusOptions};

use crate::domain::error::{AppError, Result};
use crate::domain::working_copy::{FileChange, FileStatusKind, WorkingCopy};
use crate::infrastructure::git::history::ahead_behind;

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

/// Build a `WorkingCopy` snapshot for `repo_id`.
pub fn status(repo: &Repository, repo_id: &str) -> Result<WorkingCopy> {
    let (branch, upstream, sha) = head_meta(repo)?;
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
        ahead,
        behind,
        files,
    })
}

fn head_meta(repo: &Repository) -> Result<(String, Option<String>, String)> {
    match repo.head() {
        Ok(head) => {
            let sha = head
                .peel_to_commit()
                .map(|c| c.id().to_string())
                .unwrap_or_default();

            if head.is_branch() {
                let branch = head.shorthand().unwrap_or("(unknown)").to_string();
                let upstream = repo
                    .find_branch(&branch, git2::BranchType::Local)
                    .ok()
                    .and_then(|b| b.upstream().ok())
                    .and_then(|u| u.name().ok().flatten().map(str::to_string));
                Ok((branch, upstream, sha))
            } else {
                Ok(("(detached)".into(), None, sha))
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
            Ok((branch, None, String::new()))
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

/// Create a commit from the current index. Returns new commit SHA.
/// Never auto-runs: caller must pass an explicit message (P1).
pub fn commit(repo: &Repository, message: &str) -> Result<String> {
    let trimmed = message.trim();
    if trimmed.is_empty() {
        return Err(AppError::Protocol("commit message cannot be empty".into()));
    }

    let mut index = repo.index().map_err(map_git_err)?;
    let tree_oid = index.write_tree().map_err(map_git_err)?;
    let tree = repo.find_tree(tree_oid).map_err(map_git_err)?;
    let sig = repo
        .signature()
        .or_else(|_| git2::Signature::now("GitWave", "gitwave@local"))
        .map_err(map_git_err)?;

    let parent_commit = match repo.head() {
        Ok(h) => Some(h.peel_to_commit().map_err(map_git_err)?),
        Err(e) if e.code() == git2::ErrorCode::UnbornBranch => None,
        Err(e) => return Err(map_git_err(e)),
    };

    // Refuse empty commits (tree identical to parent).
    if let Some(ref parent) = parent_commit {
        let parent_tree = parent.tree().map_err(map_git_err)?;
        if parent_tree.id() == tree.id() {
            return Err(AppError::Protocol("nothing to commit".into()));
        }
    } else if tree.is_empty() {
        return Err(AppError::Protocol("nothing to commit".into()));
    }

    let parents: Vec<&git2::Commit<'_>> = parent_commit.iter().collect();
    let oid = repo
        .commit(Some("HEAD"), &sig, &sig, trimmed, &tree, &parents)
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
    let workdir = repo
        .workdir()
        .ok_or_else(|| AppError::Protocol("bare repository has no worktree".into()))?;

    let mut tracked: Vec<String> = Vec::new();
    for path in paths {
        // Reject anything that could leave the worktree before libgit2 sees
        // it: absolute paths replace the base in `PathBuf::join`, and both
        // it and libgit2's own checks treat `..` differently.
        if Path::new(path).is_absolute() || path.split(['/', '\\']).any(|seg| seg == "..") {
            return Err(AppError::Protocol(format!("path escapes worktree: {path}")));
        }
        let abs = workdir.join(path);
        if !abs.starts_with(workdir) {
            return Err(AppError::Protocol(format!("path escapes worktree: {path}")));
        }

        let status = repo.status_file(Path::new(path)).map_err(map_git_err)?;
        if status.contains(Status::CONFLICTED) {
            // Stage 0 is absent during a conflict — deleting here would
            // destroy one side of it.
            return Err(AppError::Protocol(format!(
                "resolve conflicts before discarding: {path}"
            )));
        }
        if status.contains(Status::WT_NEW) {
            match std::fs::remove_file(&abs) {
                Ok(()) => {}
                // Already gone — treat as discarded.
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
                Err(e) => return Err(AppError::Unknown(format!("fs: {e}"))),
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
        return Err(AppError::Protocol("invalid ignore pattern".into()));
    }
    let workdir = repo
        .workdir()
        .ok_or_else(|| AppError::Protocol("bare repository has no worktree".into()))?;
    let gitignore = workdir.join(".gitignore");
    // Missing file is the normal first-run case — treat as empty. Any other
    // read error (e.g. non-UTF-8 bytes) must abort rather than overwrite the
    // user's existing rules with an almost-empty file below.
    let existing = match std::fs::read_to_string(&gitignore) {
        Ok(content) => content,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => String::new(),
        Err(e) => return Err(AppError::Unknown(format!("fs: read .gitignore: {e}"))),
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
    std::fs::write(&gitignore, next)
        .map_err(|e| AppError::Unknown(format!("fs: write .gitignore: {e}")))?;
    Ok(())
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
        assert_eq!(restored, "v1\n", "worktree should match index content");
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
        assert_eq!(restored, "v0\n", "deleted file should be recreated");
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
        assert_eq!(restored, "v2\n", "discard restores the staged version");
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
