//! Bounded, literal-path diff previews. Listing files never allocates patch text.
use super::diff::DiffSummary;
use super::worktree_guard::reject_escaping_syntax;
use crate::domain::diff::{DiffHunk, DiffLine, DiffLineKind, FileDiff};
use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;
use git2::{Diff, DiffOptions, Oid, Patch, Repository};

#[derive(Debug, Clone, serde::Serialize)]
pub struct DiffPreview {
    pub diff: DiffSummary,
    pub truncated: bool,
    pub too_large: bool,
}

fn err(e: git2::Error) -> AppError {
    AppError::unknown_with(
        codes::git::GIT_ERROR,
        format!("git: {e}"),
        &[("error", e.to_string())],
    )
}

fn oid(id: Oid) -> Option<String> {
    (!id.is_zero()).then(|| id.to_string())
}

fn collect(
    repo: &Repository,
    diff: &Diff<'_>,
    staged: Option<bool>,
    untracked: bool,
    summary: bool,
    expanded: bool,
) -> Result<DiffPreview> {
    let mut files = Vec::new();
    let mut truncated = false;
    let mut too_large = false;
    let byte_limit = if expanded {
        8 * 1024 * 1024
    } else {
        2 * 1024 * 1024
    };
    let line_limit = if expanded { 50_000 } else { 5_000 };
    for (index, delta) in diff.deltas().enumerate() {
        let path = delta
            .new_file()
            .path()
            .or_else(|| delta.old_file().path())
            .unwrap_or_else(|| std::path::Path::new(""));
        let mut file = FileDiff {
            path: path.to_string_lossy().into_owned(),
            old_sha: oid(delta.old_file().id()),
            new_sha: oid(delta.new_file().id()),
            additions: 0,
            deletions: 0,
            hunks: Vec::new(),
            staged,
            untracked: untracked.then_some(true),
        };
        if !summary {
            let work_size = if staged == Some(false) {
                repo.workdir()
                    .and_then(|wd| std::fs::symlink_metadata(wd.join(path)).ok())
                    .map_or(0, |m| m.len())
            } else {
                0
            };
            // Tree deltas may not populate sizes until their blobs are inspected.
            let blob_size = [delta.old_file().id(), delta.new_file().id()]
                .iter()
                .filter_map(|id| repo.odb().ok()?.read_header(*id).ok())
                .map(|(size, _)| size as u64)
                .max()
                .unwrap_or(0);
            if work_size
                .max(blob_size)
                .max(delta.old_file().size())
                .max(delta.new_file().size())
                > byte_limit
            {
                too_large = true;
                truncated = true;
            } else if let Some(patch) = Patch::from_diff(diff, index).map_err(err)? {
                let (_, additions, deletions) = patch.line_stats().map_err(err)?;
                file.additions = additions as u32;
                file.deletions = deletions as u32;
                let mut emitted = 0;
                let mut bytes = 0;
                'hunks: for hi in 0..patch.num_hunks() {
                    let (h, count) = patch.hunk(hi).map_err(err)?;
                    let mut hunk = DiffHunk {
                        old_start: h.old_start(),
                        old_lines: h.old_lines(),
                        new_start: h.new_start(),
                        new_lines: h.new_lines(),
                        lines: Vec::new(),
                    };
                    for li in 0..count {
                        let line = patch.line_in_hunk(hi, li).map_err(err)?;
                        let kind = match line.origin() {
                            '+' => DiffLineKind::Added,
                            '-' => DiffLineKind::Removed,
                            ' ' => DiffLineKind::Context,
                            _ => continue,
                        };
                        if emitted >= line_limit
                            || bytes + line.content().len() > byte_limit as usize
                        {
                            truncated = true;
                            file.hunks.push(hunk);
                            break 'hunks;
                        }
                        bytes += line.content().len();
                        emitted += 1;
                        let text = String::from_utf8_lossy(line.content());
                        let text = text.trim_end_matches(['\r', '\n']);
                        let character_limit = if expanded { 200_000 } else { 50_000 };
                        let content = if text.chars().count() > character_limit {
                            truncated = true;
                            text.chars().take(character_limit).collect()
                        } else {
                            text.to_owned()
                        };
                        hunk.lines.push(DiffLine {
                            kind,
                            content,
                            old_line_no: line.old_lineno(),
                            new_line_no: line.new_lineno(),
                        });
                    }
                    file.hunks.push(hunk);
                }
            }
        }
        files.push(file);
    }
    Ok(DiffPreview {
        diff: DiffSummary::new(files),
        truncated,
        too_large,
    })
}

/// `path == None` returns metadata only; content always requires one literal path.
pub fn preview(
    repo: &Repository,
    path: Option<&str>,
    staged: Option<bool>,
    commit_oid: Option<&str>,
    stash_oid: Option<&str>,
    expanded: bool,
) -> Result<DiffPreview> {
    let mut opts = DiffOptions::new();
    opts.context_lines(3)
        .disable_pathspec_match(true)
        .include_untracked(true)
        .recurse_untracked_dirs(true)
        .show_untracked_content(true);
    if let Some(path) = path {
        reject_escaping_syntax(path)?;
        opts.pathspec(path);
    }
    let summary = path.is_none();
    if let Some(id) = stash_oid.or(commit_oid) {
        let commit = repo
            .find_commit(Oid::from_str(id).map_err(err)?)
            .map_err(err)?;
        let tree = commit.tree().map_err(err)?;
        let parent = if commit.parent_count() > 0 {
            Some(commit.parent(0).map_err(err)?.tree().map_err(err)?)
        } else {
            None
        };
        let diff = repo
            .diff_tree_to_tree(parent.as_ref(), Some(&tree), Some(&mut opts))
            .map_err(err)?;
        let mut result = collect(repo, &diff, None, false, summary, expanded)?;
        if stash_oid.is_some() && commit.parent_count() >= 3 {
            let untracked = commit.parent(2).map_err(err)?.tree().map_err(err)?;
            let diff = repo
                .diff_tree_to_tree(None, Some(&untracked), Some(&mut opts))
                .map_err(err)?;
            let extra = collect(repo, &diff, None, true, summary, expanded)?;
            result.diff = result.diff.merge_dedup_by_path(extra.diff);
            result.truncated |= extra.truncated;
            result.too_large |= extra.too_large;
        }
        return Ok(result);
    }
    let mut result = DiffPreview {
        diff: DiffSummary::new(Vec::new()),
        truncated: false,
        too_large: false,
    };
    if staged != Some(false) {
        let tree = match repo.head() {
            Ok(h) => Some(h.peel_to_tree().map_err(err)?),
            Err(e) if e.code() == git2::ErrorCode::UnbornBranch => None,
            Err(e) => return Err(err(e)),
        };
        let diff = repo
            .diff_tree_to_index(tree.as_ref(), None, Some(&mut opts))
            .map_err(err)?;
        result = collect(repo, &diff, Some(true), false, summary, expanded)?;
    }
    if staged != Some(true) {
        let diff = repo
            .diff_index_to_workdir(None, Some(&mut opts))
            .map_err(err)?;
        let other = collect(repo, &diff, Some(false), false, summary, expanded)?;
        result.diff = result.diff.merge(other.diff);
        result.truncated |= other.truncated;
        result.too_large |= other.too_large;
    }
    Ok(result)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::build_linear_repo;
    #[test]
    fn preview_is_literal_scoped_and_refreshes_same_length_edits() {
        let (dir, repo) = build_linear_repo(1);
        std::fs::write(dir.join("file0.txt"), "changed\n").unwrap();
        std::fs::write(dir.join("other.txt"), "unrelated\n").unwrap();
        let first = preview(&repo, Some("file0.txt"), Some(false), None, None, false).unwrap();
        assert_eq!(first.diff.files.len(), 1);
        std::fs::write(dir.join("file0.txt"), "another\n").unwrap();
        let next = preview(&repo, Some("file0.txt"), Some(false), None, None, false).unwrap();
        assert_ne!(first.diff.files, next.diff.files);
        let list = preview(&repo, None, None, None, None, false).unwrap();
        assert!(list.diff.files.iter().all(|f| f.hunks.is_empty()));
        assert!(preview(&repo, Some("../escape"), None, None, None, false).is_err());
    }
    #[test]
    fn large_content_is_bounded_and_can_be_expanded() {
        let (dir, repo) = build_linear_repo(1);
        std::fs::write(dir.join("large.txt"), "x\n".repeat(6000)).unwrap();
        let first = preview(&repo, Some("large.txt"), Some(false), None, None, false).unwrap();
        assert!(first.truncated);
        assert_eq!(
            first.diff.files[0]
                .hunks
                .iter()
                .map(|h| h.lines.len())
                .sum::<usize>(),
            5000
        );
        let more = preview(&repo, Some("large.txt"), Some(false), None, None, true).unwrap();
        assert!(!more.truncated);
    }

    #[test]
    fn long_line_truncation_is_explicit_and_expansion_preserves_tail() {
        let (dir, repo) = build_linear_repo(1);
        std::fs::write(dir.join("long.txt"), format!("{}TAIL", "x".repeat(100_000))).unwrap();
        let first = preview(&repo, Some("long.txt"), Some(false), None, None, false).unwrap();
        assert!(first.truncated);
        let more = preview(&repo, Some("long.txt"), Some(false), None, None, true).unwrap();
        assert!(!more.truncated);
        assert!(more.diff.files[0].hunks[0].lines[0]
            .content
            .ends_with("TAIL"));
    }

    #[test]
    fn literal_brackets_and_staged_sides_are_isolated() {
        let (dir, repo) = build_linear_repo(1);
        std::fs::write(dir.join("[a].txt"), "staged\n").unwrap();
        std::fs::write(dir.join("a.txt"), "unrelated\n").unwrap();
        let mut index = repo.index().unwrap();
        index.add_path(std::path::Path::new("[a].txt")).unwrap();
        index.write().unwrap();
        std::fs::write(dir.join("[a].txt"), "working\n").unwrap();
        let staged = preview(&repo, Some("[a].txt"), Some(true), None, None, false).unwrap();
        let unstaged = preview(&repo, Some("[a].txt"), Some(false), None, None, false).unwrap();
        assert_eq!(staged.diff.files.len(), 1);
        assert_eq!(unstaged.diff.files.len(), 1);
        assert_eq!(staged.diff.files[0].path, "[a].txt");
        assert_eq!(staged.diff.files[0].staged, Some(true));
        assert_eq!(unstaged.diff.files[0].staged, Some(false));
        assert!(staged.diff.files[0].hunks[0]
            .lines
            .iter()
            .any(|line| line.content == "staged"));
        assert!(unstaged.diff.files[0].hunks[0]
            .lines
            .iter()
            .any(|line| line.content == "working"));
    }
}
