//! Exact repository-relative path along the first-parent chain from a fixed commit.
use crate::domain::error::AppError;
use git2::{Oid, Repository, Tree};
use serde::Serialize;
use std::path::Path;

#[derive(Serialize)]
pub struct Page {
    pub entries: Vec<(String, String)>,
    pub next: Option<String>,
}

fn entry(tree: &Tree<'_>, path: &Path) -> Result<Option<(Oid, i32)>, AppError> {
    match tree.get_path(path) {
        Ok(e) => Ok(Some((e.id(), e.filemode()))),
        Err(e) if e.code() == git2::ErrorCode::NotFound => Ok(None),
        Err(e) => Err(e.into()),
    }
}
pub fn page(repo: &Repository, start: Oid, path: &Path) -> Result<Page, AppError> {
    let mut next = Some(start);
    let mut entries = Vec::new();
    for _ in 0..128 {
        let Some(oid) = next else { break };
        let commit = repo.find_commit(oid)?;
        next = commit.parent_ids().next();
        let before = match next {
            Some(id) => entry(&repo.find_commit(id)?.tree()?, path)?,
            None => None,
        };
        if entry(&commit.tree()?, path)? != before {
            entries.push((
                oid.to_string(),
                commit
                    .summary()
                    .ok()
                    .flatten()
                    .unwrap_or_default()
                    .to_owned(),
            ));
        }
    }
    Ok(Page {
        entries,
        next: next.map(|oid| oid.to_string()),
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::{init_empty_repo, make_commit};

    fn commit_tree(
        repo: &Repository,
        message: &str,
        files: &[(&str, &str)],
        parents: &[Oid],
    ) -> Oid {
        let mut tree = repo.treebuilder(None).unwrap();
        for (name, data) in files {
            tree.insert(name, repo.blob(data.as_bytes()).unwrap(), 0o100644)
                .unwrap();
        }
        let tree_id = tree.write().unwrap();
        make_commit(
            repo,
            &git2::Signature::now("Test", "test@example.org").unwrap(),
            message,
            tree_id,
            parents,
        )
    }

    #[test]
    fn merge_reports_the_first_parent_delta_and_skips_second_parent_commits() {
        let (dir, repo) = crate::infrastructure::git::test_helpers::build_merge_repo();
        let id = repo.head().unwrap().peel_to_commit().unwrap().id();
        let result = page(&repo, id, Path::new("b.txt")).unwrap();
        assert_eq!(result.entries.len(), 1);
        assert_eq!(result.entries[0].0, id.to_string());
        assert!(result.next.is_none());
        drop(repo);
        std::fs::remove_dir_all(dir).unwrap();
    }

    #[test]
    fn records_first_add_rename_and_final_delete_for_each_literal_name() {
        let (dir, repo) = init_empty_repo();
        let root = commit_tree(&repo, "add original", &[("old.md", "one")], &[]);
        let edit = commit_tree(&repo, "edit old name", &[("old.md", "two")], &[root]);
        let rename = commit_tree(&repo, "rename", &[("new.md", "two")], &[edit]);
        let head = commit_tree(&repo, "delete", &[], &[rename]);
        let old = page(&repo, head, Path::new("old.md")).unwrap();
        assert_eq!(
            old.entries.iter().map(|e| e.0.as_str()).collect::<Vec<_>>(),
            [rename.to_string(), edit.to_string(), root.to_string()]
        );
        assert!(old.next.is_none());
        let new = page(&repo, head, Path::new("new.md")).unwrap();
        assert_eq!(
            new.entries,
            [
                (head.to_string(), "delete".to_owned()),
                (rename.to_string(), "rename".to_owned())
            ]
        );
        assert!(page(&repo, head, Path::new("other/new.md"))
            .unwrap()
            .entries
            .is_empty());
        drop(repo);
        std::fs::remove_dir_all(dir).unwrap();
    }

    #[test]
    fn empty_scan_page_resumes_at_first_unvisited_parent_and_includes_root_once() {
        let (dir, repo) = init_empty_repo();
        let root = commit_tree(&repo, "add tracked", &[("tracked", "stable")], &[]);
        let mut last = root;
        for _ in 0..128 {
            last = commit_tree(&repo, "unrelated", &[("tracked", "stable")], &[last]);
        }
        let recent = page(&repo, last, Path::new("tracked")).unwrap();
        assert!(recent.entries.is_empty());
        let next = Oid::from_str(recent.next.as_deref().unwrap()).unwrap();
        assert_eq!(next, root);
        let older = page(&repo, next, Path::new("tracked")).unwrap();
        assert_eq!(
            older.entries,
            [(root.to_string(), "add tracked".to_owned())]
        );
        assert!(older.next.is_none());
        drop(repo);
        std::fs::remove_dir_all(dir).unwrap();
    }
}
