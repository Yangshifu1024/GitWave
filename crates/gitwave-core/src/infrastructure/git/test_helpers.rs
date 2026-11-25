//! Shared test helpers for `infrastructure::git::*` modules.
//!
//! These are gated to `#[cfg(test)]` so they're only compiled under
//! `cargo test`. They are not part of the public API.

#![cfg(test)]

use std::fs;

use git2::{Oid, Repository, Signature};

/// Make a unique temp dir for this test run.
///
/// Tests run in parallel; the process-wide counter keeps directory names
/// unique even when two tests start within the same clock tick.
fn temp_dir(label: &str) -> std::path::PathBuf {
    use std::sync::atomic::{AtomicU64, Ordering};
    static SEQ: AtomicU64 = AtomicU64::new(0);
    let nanos = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or(0);
    let seq = SEQ.fetch_add(1, Ordering::Relaxed);
    let dir = std::env::temp_dir().join(format!("gitwave-{label}-{nanos}-{seq}"));
    fs::create_dir_all(&dir).unwrap();
    dir
}

/// Configure the user.name / user.email on a fresh repo so subsequent
/// commits can be made. Also pins `core.autocrlf=false` so a Windows global
/// `autocrlf=true` can't make checkouts write CRLF (which would break
/// LF-content assertions and status comparisons).
fn configure_user(repo: &Repository) {
    repo.config().unwrap().set_str("user.name", "Test").unwrap();
    repo.config()
        .unwrap()
        .set_str("user.email", "test@local")
        .unwrap();
    repo.config()
        .unwrap()
        .set_bool("core.autocrlf", false)
        .unwrap();
}

/// Force HEAD to point at `refs/heads/main` on a freshly-init'd repo
/// (libgit2's `Repository::init` defaults to `master`, but the rest of
/// GitWave's tests assume `main`).
///
/// `set_head` creates the ref unborn if it doesn't exist; the first commit
/// then materialises it.
fn force_default_branch_main(repo: &Repository) {
    let _ = repo.set_head("refs/heads/main");
}

/// Write a single file in the repo, stage it, and return the resulting
/// tree Oid.
pub fn write_and_stage(repo: &Repository, rel_path: &str, content: &str) -> Oid {
    let path = repo.workdir().unwrap().join(rel_path);
    fs::create_dir_all(path.parent().unwrap()).unwrap();
    fs::write(&path, content).unwrap();
    let mut index = repo.index().unwrap();
    index.add_path(std::path::Path::new(rel_path)).unwrap();
    index.write_tree().unwrap()
}

/// Create one commit with the given tree + parents. Returns the commit
/// Oid. Encapsulates the `find_tree + commit` pattern so the `&Tree`
/// borrow ends at function return.
pub fn make_commit(
    repo: &Repository,
    sig: &Signature,
    message: &str,
    tree_oid: Oid,
    parent_oids: &[Oid],
) -> Oid {
    let tree = repo.find_tree(tree_oid).unwrap();
    let parents_oids = parent_oids.to_vec();
    let parents: Vec<git2::Commit> = parents_oids
        .iter()
        .map(|oid| repo.find_commit(*oid).unwrap())
        .collect();
    let parent_refs: Vec<&git2::Commit> = parents.iter().collect();
    repo.commit(Some("HEAD"), sig, sig, message, &tree, &parent_refs)
        .unwrap()
}

/// Init a git repo with N linear commits on `main`, return (path, repo).
/// Each commit adds a file `fileN.txt` with content `vN\n`.
pub fn build_linear_repo(n: u32) -> (std::path::PathBuf, Repository) {
    let tmp = temp_dir("linear");
    let repo = Repository::init(&tmp).unwrap();
    force_default_branch_main(&repo);
    configure_user(&repo);
    let sig = Signature::now("Test", "test@local").unwrap();

    for i in 0..n {
        let tree_oid = write_and_stage(&repo, &format!("file{i}.txt"), &format!("v{i}\n"));

        if i == 0 {
            // Root commit: no parents
            let _ = make_commit(&repo, &sig, &format!("commit {i}"), tree_oid, &[]);
        } else {
            let parent_oid = repo.head().unwrap().peel_to_commit().unwrap().id();
            let _ = make_commit(&repo, &sig, &format!("commit {i}"), tree_oid, &[parent_oid]);
        }
    }

    (tmp, repo)
}

/// Build a 3-way merge: `main` has a1, a2; `feature` has b1, b2 (branched
/// from a1); then `main` merges `feature` on top with a 2-parent commit.
pub fn build_merge_repo() -> (std::path::PathBuf, Repository) {
    let tmp = temp_dir("merge");
    let repo = Repository::init(&tmp).unwrap();
    force_default_branch_main(&repo);
    configure_user(&repo);
    let sig = Signature::now("Test", "test@local").unwrap();

    // Commit a1 on main.
    let a1_tree = write_and_stage(&repo, "a.txt", "a1\n");
    let a1_oid = make_commit(&repo, &sig, "a1", a1_tree, &[]);

    // Branch off -> feature.
    {
        let a1 = repo.find_commit(a1_oid).unwrap();
        repo.branch("feature", &a1, true).unwrap();
    }
    repo.set_head("refs/heads/feature").unwrap();
    repo.checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
        .unwrap();

    // Commit b1 + b2 on feature.
    let b1_tree = write_and_stage(&repo, "b.txt", "b1\n");
    let b1_oid = make_commit(&repo, &sig, "b1", b1_tree, &[a1_oid]);
    let b2_tree = write_and_stage(&repo, "b.txt", "b2\n");
    let b2_oid = make_commit(&repo, &sig, "b2", b2_tree, &[b1_oid]);

    // Switch back to main, commit a2.
    repo.set_head("refs/heads/main").unwrap();
    repo.checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
        .unwrap();
    let a2_tree = write_and_stage(&repo, "a.txt", "a2\n");
    let a2_oid = make_commit(&repo, &sig, "a2", a2_tree, &[a1_oid]);

    // Merge commit on main with parents [a2_oid, b2_oid].
    // After checkout to main, b.txt is gone from the workdir — re-write it
    // before staging so the merge tree contains both files.
    fs::write(repo.workdir().unwrap().join("b.txt"), "b2\n").unwrap();
    let merge_tree = {
        let mut index = repo.index().unwrap();
        index.add_path(std::path::Path::new("a.txt")).unwrap();
        index.add_path(std::path::Path::new("b.txt")).unwrap();
        index.write().unwrap();
        index.write_tree().unwrap()
    };
    let _merge_oid = make_commit(&repo, &sig, "merge feature", merge_tree, &[a2_oid, b2_oid]);

    (tmp, repo)
}

/// Build a conflicted merge: after branching `feature` from base, both
/// branches edit `file0.txt` divergently, then `merge_branch` leaves the
/// merge in progress on `main` with that one conflict. Returns
/// (path, repo, feature tip Oid) — the tip must end up as the second
/// parent of the commit that finishes the merge.
pub fn build_conflicted_merge() -> (std::path::PathBuf, Repository, Oid) {
    let (tmp, repo) = build_linear_repo(1);
    let sig = Signature::now("Test", "test@local").unwrap();
    let base = repo.head().unwrap().peel_to_commit().unwrap().id();

    // Branch feature from base; edit file0.txt there.
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
    let feature_tip = repo.head().unwrap().peel_to_commit().unwrap().id();

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

    let res = crate::infrastructure::git::merge::merge_branch(&repo, "feature", false).unwrap();
    assert!(!res.conflicts.is_empty(), "expected a conflict");

    (tmp, repo, feature_tip)
}

/// Init a git repo with no commits (empty working tree). Sets user config
/// so subsequent commits can be made. Returns (path, repo).
pub fn init_empty_repo() -> (std::path::PathBuf, Repository) {
    let tmp = temp_dir("empty");
    let repo = Repository::init(&tmp).unwrap();
    force_default_branch_main(&repo);
    configure_user(&repo);
    (tmp, repo)
}
