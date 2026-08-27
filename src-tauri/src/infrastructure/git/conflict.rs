//! Conflict listing / blob sides / resolve / abort for in-progress merges.

use std::path::Path;

use git2::{ObjectType, Repository};

use crate::domain::error::{AppError, Result};

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct ConflictFile {
    pub path: String,
    pub has_ours: bool,
    pub has_theirs: bool,
    pub has_base: bool,
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct ConflictSides {
    pub path: String,
    pub ours: Option<String>,
    pub theirs: Option<String>,
    pub base: Option<String>,
    pub working: Option<String>,
}

pub fn list_conflicts(repo: &Repository) -> Result<Vec<ConflictFile>> {
    let index = repo.index().map_err(map_git_err)?;
    let mut out = Vec::new();
    for c in index.conflicts().map_err(map_git_err)? {
        let ic = c.map_err(map_git_err)?;
        let path = ic
            .our
            .as_ref()
            .or(ic.their.as_ref())
            .or(ic.ancestor.as_ref())
            .map(|e| String::from_utf8_lossy(&e.path).into_owned())
            .unwrap_or_default();
        if path.is_empty() {
            continue;
        }
        out.push(ConflictFile {
            path,
            has_ours: ic.our.is_some(),
            has_theirs: ic.their.is_some(),
            has_base: ic.ancestor.is_some(),
        });
    }
    Ok(out)
}

fn blob_text(repo: &Repository, id: git2::Oid) -> Result<Option<String>> {
    let blob = repo.find_blob(id).map_err(map_git_err)?;
    if blob.is_binary() {
        return Ok(Some(String::from_utf8_lossy(blob.content()).into_owned()));
    }
    Ok(Some(String::from_utf8_lossy(blob.content()).into_owned()))
}

pub fn get_conflict_sides(repo: &Repository, path: &str) -> Result<ConflictSides> {
    let index = repo.index().map_err(map_git_err)?;
    let mut ours = None;
    let mut theirs = None;
    let mut base = None;
    for c in index.conflicts().map_err(map_git_err)? {
        let ic = c.map_err(map_git_err)?;
        let p = ic
            .our
            .as_ref()
            .or(ic.their.as_ref())
            .or(ic.ancestor.as_ref())
            .map(|e| String::from_utf8_lossy(&e.path).into_owned())
            .unwrap_or_default();
        if p != path {
            continue;
        }
        if let Some(e) = ic.our {
            ours = blob_text(repo, e.id)?;
        }
        if let Some(e) = ic.their {
            theirs = blob_text(repo, e.id)?;
        }
        if let Some(e) = ic.ancestor {
            base = blob_text(repo, e.id)?;
        }
        break;
    }

    let working = repo.workdir().and_then(|wd| {
        let full = wd.join(path);
        std::fs::read_to_string(full).ok()
    });

    Ok(ConflictSides {
        path: path.to_string(),
        ours,
        theirs,
        base,
        working,
    })
}

/// Write resolved content to the worktree and stage the path (clears conflict).
pub fn resolve_conflict(repo: &Repository, path: &str, content: &str) -> Result<()> {
    let wd = repo
        .workdir()
        .ok_or_else(|| AppError::Protocol("bare repo has no workdir".into()))?;
    let full = wd.join(path);
    if let Some(parent) = full.parent() {
        std::fs::create_dir_all(parent).map_err(|e| AppError::Unknown(format!("mkdir: {e}")))?;
    }
    std::fs::write(&full, content).map_err(|e| AppError::Unknown(format!("write: {e}")))?;

    let mut index = repo.index().map_err(map_git_err)?;
    let _ = index.conflict_remove(Path::new(path));
    index.add_path(Path::new(path)).map_err(map_git_err)?;
    index.write().map_err(map_git_err)?;
    Ok(())
}

pub fn abort_merge(repo: &Repository) -> Result<()> {
    // Reset index + worktree to HEAD; clear merge state files.
    let head = repo.head().map_err(map_git_err)?;
    let obj = head.peel(ObjectType::Commit).map_err(map_git_err)?;
    repo.reset(&obj, git2::ResetType::Hard, None)
        .map_err(map_git_err)?;
    let _ = std::fs::remove_file(repo.path().join("MERGE_HEAD"));
    let _ = std::fs::remove_file(repo.path().join("MERGE_MSG"));
    let _ = std::fs::remove_file(repo.path().join("MERGE_MODE"));
    Ok(())
}

pub fn is_merge_in_progress(repo: &Repository) -> bool {
    repo.path().join("MERGE_HEAD").exists()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::merge::merge_branch;
    use crate::infrastructure::git::test_helpers::build_linear_repo;
    use git2::Signature;
    use std::fs;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    #[test]
    fn conflict_list_resolve_and_abort() {
        let (path, repo) = build_linear_repo(1);
        let sig = Signature::now("Test", "test@local").unwrap();
        let base = repo.head().unwrap().peel_to_commit().unwrap().id();

        // Branch feature from base.
        {
            let c = repo.find_commit(base).unwrap();
            repo.branch("feature", &c, true).unwrap();
        }
        repo.set_head("refs/heads/feature").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
            .unwrap();
        fs::write(repo.workdir().unwrap().join("file0.txt"), "feature\n").unwrap();
        {
            let mut index = repo.index().unwrap();
            index.add_path(std::path::Path::new("file0.txt")).unwrap();
            let tree = index.write_tree().unwrap();
            let tree = repo.find_tree(tree).unwrap();
            let parent = repo.find_commit(base).unwrap();
            repo.commit(Some("HEAD"), &sig, &sig, "feature edit", &tree, &[&parent])
                .unwrap();
        }

        // Diverging edit on main.
        repo.set_head("refs/heads/main").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
            .unwrap();
        fs::write(repo.workdir().unwrap().join("file0.txt"), "main\n").unwrap();
        {
            let mut index = repo.index().unwrap();
            index.add_path(std::path::Path::new("file0.txt")).unwrap();
            let tree = index.write_tree().unwrap();
            let tree = repo.find_tree(tree).unwrap();
            let parent = repo.find_commit(base).unwrap();
            repo.commit(Some("HEAD"), &sig, &sig, "main edit", &tree, &[&parent])
                .unwrap();
        }

        let res = merge_branch(&repo, "feature").unwrap();
        assert_eq!(
            res.kind,
            crate::infrastructure::git::merge::MergeKind::ThreeWay
        );
        assert!(!res.conflicts.is_empty());
        assert!(is_merge_in_progress(&repo));

        let listed = list_conflicts(&repo).unwrap();
        assert_eq!(listed.len(), 1);
        assert_eq!(listed[0].path, "file0.txt");

        let sides = get_conflict_sides(&repo, "file0.txt").unwrap();
        assert!(sides.ours.as_deref().unwrap().contains("main"));
        assert!(sides.theirs.as_deref().unwrap().contains("feature"));

        resolve_conflict(&repo, "file0.txt", "resolved\n").unwrap();
        assert!(list_conflicts(&repo).unwrap().is_empty());
        assert!(is_merge_in_progress(&repo));

        abort_merge(&repo).unwrap();
        assert!(!is_merge_in_progress(&repo));
        cleanup(&path);
    }
}
