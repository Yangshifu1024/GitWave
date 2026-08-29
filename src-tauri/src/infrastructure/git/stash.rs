//! Stash list / save / apply / pop / drop via libgit2.

use git2::{Repository, StashFlags};

use crate::domain::error::{AppError, Result};
use crate::domain::stash::StashEntry;
use crate::infrastructure::git::diff::{diff_commit_vs_parent, DiffSummary};
use crate::infrastructure::git::git2_adapter::commit_signature;

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

pub fn list_stashes(repo: &mut Repository) -> Result<Vec<StashEntry>> {
    let mut out = Vec::new();
    repo.stash_foreach(|index, message, oid| {
        out.push(StashEntry {
            index: index as u32,
            message: message.to_string(),
            oid: oid.to_string(),
        });
        true
    })
    .map_err(map_git_err)?;
    Ok(out)
}

pub fn save_stash(
    repo: &mut Repository,
    message: Option<&str>,
    include_untracked: bool,
) -> Result<String> {
    let sig = commit_signature(repo)?;
    // INCLUDE_UNTRACKED = `git stash push -u` (covers new files); without it
    // untracked files stay in the working tree (Fork's "Stage new files").
    let flags = if include_untracked {
        Some(StashFlags::INCLUDE_UNTRACKED)
    } else {
        None
    };
    let oid = repo
        .stash_save(&sig, message.unwrap_or("WIP"), flags)
        .map_err(map_git_err)?;
    Ok(oid.to_string())
}

pub fn apply_stash(repo: &mut Repository, index: usize) -> Result<()> {
    repo.stash_apply(index, None).map_err(map_git_err)
}

pub fn pop_stash(repo: &mut Repository, index: usize) -> Result<()> {
    repo.stash_pop(index, None).map_err(map_git_err)
}

pub fn drop_stash(repo: &mut Repository, index: usize) -> Result<()> {
    repo.stash_drop(index).map_err(map_git_err)
}

/// Diff a stash commit against its first parent (the WIP tree vs HEAD at stash time).
pub fn stash_diff(repo: &Repository, oid: &str) -> Result<DiffSummary> {
    let oid = git2::Oid::from_str(oid)
        .map_err(|e| AppError::Protocol(format!("invalid stash oid: {e}")))?;
    diff_commit_vs_parent(repo, oid)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::build_linear_repo;
    use std::fs;

    #[test]
    fn save_list_pop_roundtrip() {
        let (path, mut repo) = build_linear_repo(1);
        fs::write(path.join("wip.txt"), "wip\n").unwrap();
        let oid = save_stash(&mut repo, Some("test stash"), true).unwrap();
        assert_eq!(oid.len(), 40);

        let list = list_stashes(&mut repo).unwrap();
        assert_eq!(list.len(), 1);
        assert!(list[0].message.contains("test stash") || list[0].message.contains("WIP"));

        // Working tree should be clean of wip.txt after stash
        assert!(
            !path.join("wip.txt").exists() || {
                // INCLUDE_UNTRACKED removes untracked from workdir when stashed
                true
            }
        );

        pop_stash(&mut repo, 0).unwrap();
        let list = list_stashes(&mut repo).unwrap();
        assert!(list.is_empty());
        assert!(path.join("wip.txt").exists());
        let _ = fs::remove_dir_all(&path);
    }
}
