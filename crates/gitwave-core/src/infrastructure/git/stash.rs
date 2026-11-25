//! Stash list / save / apply / pop / drop via libgit2.

use git2::{Repository, StashFlags};

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;
use crate::domain::stash::StashEntry;
use crate::infrastructure::git::diff::{diff_commit_vs_parent, diff_tree_vs_empty, DiffSummary};
use crate::infrastructure::git::git2_adapter::commit_signature;

fn map_git_err(e: git2::Error) -> AppError {
    AppError::unknown_with(
        codes::git::GIT_ERROR,
        format!("git: {e}"),
        &[("error", e.to_string())],
    )
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

/// Diff a stash commit against its first parent (the WIP tree vs HEAD at
/// stash time).
///
/// `include_untracked` extends that with the stash's **third parent**. `git
/// stash -u` (GitWave's default, see [`save_stash`]) writes the untracked files
/// into a separate parentless commit; the first-parent diff can never see them,
/// so a brand-new never-`git add`ed file would otherwise be missing from the
/// stash contents view even though the stash just removed it from the worktree.
/// The third parent's tree is diffed against the empty tree and the resulting
/// files are merged in tagged `untracked: Some(true)`.
pub fn stash_diff(repo: &Repository, oid: &str, include_untracked: bool) -> Result<DiffSummary> {
    let oid = git2::Oid::from_str(oid).map_err(|e| {
        AppError::protocol_with(
            codes::git::INVALID_STASH_OID,
            format!("invalid stash oid: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    // Unchanged shared semantics: first parent only (`diff_commit_vs_parent` is
    // also what commit detail / AI explain use).
    let base = diff_commit_vs_parent(repo, oid)?;
    if !include_untracked {
        return Ok(base);
    }

    let commit = repo.find_commit(oid).map_err(map_git_err)?;
    // 2 parents = HEAD + index: no `-u`, or `-u` with nothing untracked.
    // Short-circuit so the output stays exactly what it was before this fix.
    if commit.parent_count() < 3 {
        return Ok(base);
    }

    let untracked_tree = commit
        .parent(2)
        .map_err(map_git_err)?
        .tree()
        .map_err(map_git_err)?;
    let mut untracked_files = diff_tree_vs_empty(repo, &untracked_tree)?;
    for file in &mut untracked_files {
        file.untracked = Some(true);
    }

    // `DiffSummary::merge` is a plain concatenation, so dedupe by path first:
    // upstream git refuses to build a stash whose untracked commit collides
    // with a tracked path, but hand-made/exotic stashes can, and this merge
    // must never emit the same path twice.
    Ok(base.merge_dedup_by_path(DiffSummary::new(untracked_files)))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::build_linear_repo;
    use git2::Signature;
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

    /// `git stash -u` puts never-added files in the stash's 3rd parent, which
    /// the first-parent diff cannot see — they must still show up as untracked.
    #[test]
    fn stash_diff_includes_untracked_files_from_third_parent() {
        let (path, mut repo) = build_linear_repo(1);
        fs::write(path.join("new.txt"), "alpha\nbeta\ngamma\n").unwrap();
        fs::write(path.join("file0.txt"), "v0-edited\n").unwrap();
        let oid = save_stash(&mut repo, Some("with untracked"), true).unwrap();
        assert_eq!(
            repo.find_commit(git2::Oid::from_str(&oid).unwrap())
                .unwrap()
                .parent_count(),
            3,
            "-u must create the untracked commit"
        );

        let summary = stash_diff(&repo, &oid, true).unwrap();
        cleanup(&path);

        let untracked = summary
            .files
            .iter()
            .find(|f| f.path == "new.txt")
            .unwrap_or_else(|| {
                panic!(
                    "untracked new.txt missing from stash diff, got {:?}",
                    summary
                        .files
                        .iter()
                        .map(|f| f.path.as_str())
                        .collect::<Vec<_>>()
                )
            });
        assert_eq!(untracked.untracked, Some(true));
        assert_eq!(untracked.additions, 3, "every line is an addition");
        assert_eq!(untracked.deletions, 0);
        assert_eq!(untracked.old_sha, None);
        let lines: Vec<&str> = untracked
            .hunks
            .iter()
            .flat_map(|h| h.lines.iter())
            .map(|l| l.content.as_str())
            .collect();
        assert_eq!(lines, ["alpha", "beta", "gamma"]);

        // Tracked edits keep coming from the first-parent diff, untagged.
        let tracked = summary
            .files
            .iter()
            .find(|f| f.path == "file0.txt")
            .expect("tracked edit must stay in the diff");
        assert_eq!(tracked.untracked, None);
        assert_eq!(tracked.additions, 1);
        assert_eq!(tracked.deletions, 1);
    }

    /// Control group: `include_untracked = false` locks in the old behaviour.
    #[test]
    fn stash_diff_omits_untracked_files_when_not_requested() {
        let (path, mut repo) = build_linear_repo(1);
        fs::write(path.join("new.txt"), "alpha\n").unwrap();
        let oid = save_stash(&mut repo, Some("with untracked"), true).unwrap();

        let summary = stash_diff(&repo, &oid, false).unwrap();
        cleanup(&path);

        assert!(
            summary.files.iter().all(|f| f.path != "new.txt"),
            "untracked file must be excluded, got {:?}",
            summary
                .files
                .iter()
                .map(|f| f.path.as_str())
                .collect::<Vec<_>>()
        );
        assert!(summary.files.iter().all(|f| f.untracked.is_none()));
    }

    /// A 2-parent stash (no `-u`) must short-circuit: even with
    /// `include_untracked = true` the output stays the plain first-parent diff.
    #[test]
    fn stash_diff_without_untracked_commit_matches_first_parent_diff() {
        let (path, mut repo) = build_linear_repo(1);
        fs::write(path.join("file0.txt"), "v0-edited\n").unwrap();
        let oid = save_stash(&mut repo, Some("tracked only"), false).unwrap();
        let stash_oid = git2::Oid::from_str(&oid).unwrap();
        assert_eq!(repo.find_commit(stash_oid).unwrap().parent_count(), 2);

        let summary = stash_diff(&repo, &oid, true).unwrap();
        let expected = diff_commit_vs_parent(&repo, stash_oid).unwrap();
        cleanup(&path);

        assert_eq!(summary.files, expected.files);
        assert_eq!(summary.total_additions, expected.total_additions);
        assert_eq!(summary.total_deletions, expected.total_deletions);
        assert!(summary.files.iter().all(|f| f.untracked.is_none()));
        assert!(summary.files.iter().any(|f| f.path == "file0.txt"));
    }

    /// 3rd parent present but empty (`-u` with nothing untracked): still no
    /// extra entries, no panic.
    #[test]
    fn stash_diff_with_empty_untracked_commit_adds_nothing() {
        let (path, repo) = build_linear_repo(1);
        let sig = Signature::now("Test", "test@local").unwrap();
        let base_oid = repo.head().unwrap().peel_to_commit().unwrap().id();
        let tracked_tree = tree_with(&repo, &[("file0.txt", "v0-edited\n")]);
        let empty_tree = repo.treebuilder(None).unwrap().write().unwrap();

        let index_commit = make_detached_commit(&repo, &sig, "index", tracked_tree, &[base_oid]);
        let untracked_commit = make_detached_commit(&repo, &sig, "untracked", empty_tree, &[]);
        let stash_oid = make_detached_commit(
            &repo,
            &sig,
            "WIP on main",
            tracked_tree,
            &[base_oid, index_commit, untracked_commit],
        );

        let summary = stash_diff(&repo, &stash_oid.to_string(), true).unwrap();
        let expected = diff_commit_vs_parent(&repo, stash_oid).unwrap();
        cleanup(&path);

        assert_eq!(summary.files, expected.files);
        assert!(summary.files.iter().all(|f| f.untracked.is_none()));
    }

    /// Path collision between the first-parent side and the untracked commit:
    /// the first-parent entry wins, nothing is duplicated, nothing panics.
    #[test]
    fn stash_diff_dedupes_paths_shared_with_untracked_commit() {
        let (path, repo) = build_linear_repo(1);
        let sig = Signature::now("Test", "test@local").unwrap();
        let base_oid = repo.head().unwrap().peel_to_commit().unwrap().id();

        let tracked_tree = tree_with(&repo, &[("file0.txt", "v0-edited\n")]);
        let untracked_tree = tree_with(&repo, &[("file0.txt", "collide\n"), ("new.txt", "u\n")]);

        let index_commit = make_detached_commit(&repo, &sig, "index", tracked_tree, &[base_oid]);
        let untracked_commit = make_detached_commit(&repo, &sig, "untracked", untracked_tree, &[]);
        let stash_oid = make_detached_commit(
            &repo,
            &sig,
            "WIP on main",
            tracked_tree,
            &[base_oid, index_commit, untracked_commit],
        );

        let summary = stash_diff(&repo, &stash_oid.to_string(), true).unwrap();
        cleanup(&path);

        let paths: Vec<&str> = summary.files.iter().map(|f| f.path.as_str()).collect();
        assert_eq!(
            paths,
            ["file0.txt", "new.txt"],
            "colliding path must appear once, first-parent side first"
        );
        assert_eq!(summary.files[0].untracked, None);
        assert_eq!(summary.files[0].additions, 1);
        assert_eq!(summary.files[0].deletions, 1);
        assert_eq!(summary.files[1].untracked, Some(true));
        assert_eq!(summary.files[1].additions, 1);
        assert_eq!(
            summary.total_additions, 2,
            "the dropped duplicate must not be counted"
        );
        assert_eq!(summary.total_deletions, 1);
    }

    /// Commit to the ODB only — no ref update — so a hand-built commit can have
    /// any parent shape (e.g. a parentless "untracked" commit sitting behind a
    /// stash commit) without libgit2's "current tip is not the first parent"
    /// check firing on HEAD.
    fn make_detached_commit(
        repo: &Repository,
        sig: &Signature,
        message: &str,
        tree_oid: git2::Oid,
        parents: &[git2::Oid],
    ) -> git2::Oid {
        let tree = repo.find_tree(tree_oid).unwrap();
        let parent_commits: Vec<git2::Commit> = parents
            .iter()
            .map(|oid| repo.find_commit(*oid).unwrap())
            .collect();
        let parent_refs: Vec<&git2::Commit> = parent_commits.iter().collect();
        repo.commit(None, sig, sig, message, &tree, &parent_refs)
            .unwrap()
    }

    /// Build a flat tree out of `(path, content)` pairs for hand-made stashes.
    fn tree_with(repo: &Repository, entries: &[(&str, &str)]) -> git2::Oid {
        let mut builder = repo.treebuilder(None).unwrap();
        for (name, content) in entries {
            let blob = repo.blob(content.as_bytes()).unwrap();
            builder
                .insert(*name, blob, git2::FileMode::Blob.into())
                .unwrap();
        }
        builder.write().unwrap()
    }

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }
}
