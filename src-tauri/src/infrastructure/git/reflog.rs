//! Reflog reading — the movement history of a reference (HEAD, branches).
//! v0.2 ships a read-only browser; recovery actions come with v0.3's
//! AI-assisted recovery work.

use git2::Repository;

use crate::domain::error::{AppError, Result};
use crate::domain::reflog::ReflogEntry;

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

/// Read a reference's reflog, newest first. A missing reflog (unborn
/// reference or a repo with no reflog files) reads as empty.
pub fn read_reflog(repo: &Repository, refname: &str) -> Result<Vec<ReflogEntry>> {
    let reflog = match repo.reflog(refname) {
        Ok(reflog) => reflog,
        // libgit2 raises NotFound for references without reflog files.
        Err(e) if e.code() == git2::ErrorCode::NotFound => return Ok(Vec::new()),
        Err(e) => return Err(map_git_err(e)),
    };
    let mut out = Vec::with_capacity(reflog.len());
    for entry in reflog.iter() {
        let committer = entry.committer();
        out.push(ReflogEntry {
            old_sha: if entry.id_old().is_zero() {
                None
            } else {
                Some(entry.id_old().to_string())
            },
            new_sha: entry.id_new().to_string(),
            message: entry.message().map(str::to_string),
            committer: committer.name().unwrap_or("").to_string(),
            time: committer.when().seconds(),
        });
    }
    // `reflog.iter()` already yields newest-first, matching the UI order.
    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::build_linear_repo;

    #[test]
    fn commits_produce_head_reflog_entries_newest_first() {
        let (path, repo) = build_linear_repo(3);
        let entries = read_reflog(&repo, "HEAD").expect("reflog");
        assert_eq!(entries.len(), 3, "one entry per commit");
        assert_eq!(
            entries[0].new_sha,
            repo.head()
                .unwrap()
                .peel_to_commit()
                .unwrap()
                .id()
                .to_string(),
            "newest entry points at HEAD"
        );
        // HEAD reflog messages carry the action prefix, e.g. "commit: <subject>".
        assert_eq!(entries[0].message.as_deref(), Some("commit: commit 2"));
        // Root commit's entry starts from the zero OID.
        assert!(entries[2].old_sha.is_none());
        std::fs::remove_dir_all(&path).expect("cleanup");
    }

    #[test]
    fn missing_reflog_reads_as_empty() {
        let (path, repo) = crate::infrastructure::git::test_helpers::init_empty_repo();
        let entries = read_reflog(&repo, "HEAD").expect("reflog");
        assert!(entries.is_empty());
        std::fs::remove_dir_all(&path).expect("cleanup");
    }
}
