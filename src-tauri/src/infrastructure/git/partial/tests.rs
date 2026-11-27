use super::*;
use crate::infrastructure::git::test_helpers::{init_empty_repo, make_commit, write_and_stage};
use std::{fs, path::Path};

const FILE: &str = "notes.txt";

fn setup(base: &str, working: &str) -> (std::path::PathBuf, Repository) {
    let (dir, repo) = init_empty_repo();
    let tree = write_and_stage(&repo, FILE, base);
    make_commit(
        &repo,
        &git2::Signature::now("Test", "test@example.org").unwrap(),
        "base",
        tree,
        &[],
    );
    stage_file(&repo);
    fs::write(dir.join(FILE), working).unwrap();
    (dir, repo)
}

fn stage_file(repo: &Repository) {
    let mut index = repo.index().unwrap();
    index.add_path(Path::new(FILE)).unwrap();
    index.write().unwrap();
}

fn indexed(repo: &Repository) -> String {
    let mut index = repo.index().unwrap();
    index.read(true).unwrap();
    let id = index.get_path(Path::new(FILE), 0).unwrap().id;
    String::from_utf8(repo.find_blob(id).unwrap().content().to_vec()).unwrap()
}

fn choose(view: &Preview, hunk: usize, pick: impl Fn(&str) -> bool) -> Selection {
    Selection {
        token: view.token.clone(),
        hunk,
        lines: view.hunks[hunk]
            .iter()
            .enumerate()
            .filter(|(_, line)| line.starts_with(['+', '-']) && pick(line))
            .map(|(index, _)| index)
            .collect(),
    }
}

#[test]
fn stage_and_unstage_selected_lines_while_preserving_other_changes_and_worktree() {
    let base = "title\nold one\nold two\nend\n";
    let working = "title\nnew one\nnew two\nend\n";
    let (dir, repo) = setup(base, working);
    let view = preview(&repo, FILE, false).unwrap();
    assert_eq!(view.hunks.len(), 1);
    apply(
        &repo,
        FILE,
        false,
        choose(&view, 0, |line| {
            line.contains("old one") || line.contains("new two")
        }),
    )
    .unwrap();
    assert_eq!(indexed(&repo), "title\nold two\nnew two\nend\n");
    assert_eq!(fs::read_to_string(dir.join(FILE)).unwrap(), working);
    let view = preview(&repo, FILE, true).unwrap();
    apply(
        &repo,
        FILE,
        true,
        choose(&view, 0, |line| line.contains("new two")),
    )
    .unwrap();
    assert_eq!(indexed(&repo), "title\nold two\nend\n");
    assert_eq!(fs::read_to_string(dir.join(FILE)).unwrap(), working);
    drop(repo);
    fs::remove_dir_all(dir).unwrap();
}

#[test]
fn hunk_position_distinguishes_identical_changes_at_two_places() {
    let block = "1\n2\n3\n4\nold\n6\n7\n8\n9\n10\n";
    let base = block.repeat(2);
    let changed = block.replace("old\n", "new\n");
    let working = changed.repeat(2);
    let (dir, repo) = setup(&base, &working);
    let view = preview(&repo, FILE, false).unwrap();
    assert_eq!(view.hunks.len(), 2);
    apply(&repo, FILE, false, choose(&view, 1, |_| true)).unwrap();
    assert_eq!(indexed(&repo), format!("{block}{changed}"));
    let view = preview(&repo, FILE, true).unwrap();
    apply(&repo, FILE, true, choose(&view, 0, |_| true)).unwrap();
    assert_eq!(indexed(&repo), base);
    assert_eq!(fs::read_to_string(dir.join(FILE)).unwrap(), working);
    drop(repo);
    fs::remove_dir_all(dir).unwrap();
}

#[test]
fn external_worktree_and_index_edits_expire_the_reviewed_patch() {
    let (dir, repo) = setup("base\n", "proposal\n");
    let view = preview(&repo, FILE, false).unwrap();
    fs::write(dir.join(FILE), "external update\n").unwrap();
    assert!(apply(&repo, FILE, false, choose(&view, 0, |_| true)).is_err());
    assert_eq!(indexed(&repo), "base\n");
    stage_file(&repo);
    let staged_view = preview(&repo, FILE, true).unwrap();
    fs::write(dir.join(FILE), "new index content\n").unwrap();
    stage_file(&repo);
    assert!(apply(&repo, FILE, true, choose(&staged_view, 0, |_| true)).is_err());
    assert_eq!(indexed(&repo), "new index content\n");
    assert_eq!(
        fs::read_to_string(dir.join(FILE)).unwrap(),
        "new index content\n"
    );
    drop(repo);
    fs::remove_dir_all(dir).unwrap();
}

#[test]
fn handles_insertion_into_empty_tracked_file_and_only_changes_the_index() {
    let (dir, repo) = setup("", "a\nb\n");
    let view = preview(&repo, FILE, false).unwrap();
    apply(&repo, FILE, false, choose(&view, 0, |line| line == "+b\n")).unwrap();
    assert_eq!(indexed(&repo), "b\n");
    let view = preview(&repo, FILE, true).unwrap();
    apply(&repo, FILE, true, choose(&view, 0, |_| true)).unwrap();
    assert_eq!(indexed(&repo), "");
    assert_eq!(fs::read_to_string(dir.join(FILE)).unwrap(), "a\nb\n");
    drop(repo);
    fs::remove_dir_all(dir).unwrap();
}

#[test]
fn newline_markers_and_invalid_line_selections_cannot_stage_the_whole_file() {
    let (dir, repo) = setup("old\n", "without newline");
    assert!(preview(&repo, FILE, false).is_err());
    assert_eq!(indexed(&repo), "old\n");
    fs::write(dir.join(FILE), "new\n").unwrap();
    let view = preview(&repo, FILE, false).unwrap();
    for lines in [vec![], vec![0], vec![usize::MAX]] {
        assert!(apply(
            &repo,
            FILE,
            false,
            Selection {
                token: view.token.clone(),
                hunk: 0,
                lines
            }
        )
        .is_err());
        assert_eq!(indexed(&repo), "old\n");
    }
    assert_eq!(fs::read_to_string(dir.join(FILE)).unwrap(), "new\n");
    drop(repo);
    fs::remove_dir_all(dir).unwrap();
}

#[test]
fn identical_patches_are_bound_to_repository_and_direction() {
    let (a, repo_a) = setup("base\n", "changed\n");
    let (b, repo_b) = setup("base\n", "changed\n");
    let view = preview(&repo_a, FILE, false).unwrap();
    assert!(apply(&repo_b, FILE, false, choose(&view, 0, |_| true)).is_err());
    assert_eq!(indexed(&repo_b), "base\n");
    stage_file(&repo_a);
    fs::write(a.join(FILE), "base\n").unwrap();
    let unstaged = preview(&repo_a, FILE, false).unwrap();
    let staged = preview(&repo_a, FILE, true).unwrap();
    assert_eq!(unstaged.hunks, staged.hunks);
    assert!(apply(&repo_a, FILE, true, choose(&unstaged, 0, |_| true)).is_err());
    assert_eq!(indexed(&repo_a), "changed\n");
    drop((repo_a, repo_b));
    fs::remove_dir_all(a).unwrap();
    fs::remove_dir_all(b).unwrap();
}
