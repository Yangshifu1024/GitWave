//! Apply selected changes from one locally generated hunk to the index.
use crate::domain::error::AppError;
use git2::{ApplyLocation, Delta, Diff, DiffOptions, ObjectType, Oid, Patch, Repository};
use serde::{Deserialize, Serialize};

#[derive(Serialize)]
pub struct Preview {
    pub token: String,
    pub hunks: Vec<Vec<String>>,
}

#[derive(Deserialize)]
pub struct Selection {
    pub token: String,
    pub hunk: usize,
    pub lines: Vec<usize>,
}

fn invalid() -> AppError {
    AppError::protocol(
        crate::domain::error_codes::git::PARTIAL_UNAVAILABLE,
        "Refresh the file. Partial staging requires a current, regular text-file modification.",
    )
}

struct Snapshot {
    header: String,
    preview: Preview,
}

fn snapshot(repo: &Repository, path: &str, staged: bool) -> Result<Snapshot, AppError> {
    if path.contains('\0') {
        return Err(invalid());
    }
    super::worktree_guard::reject_escaping_syntax(path)?;
    let mut opts = DiffOptions::new();
    opts.pathspec(path)
        .disable_pathspec_match(true)
        .id_abbrev(40)
        .reverse(staged);
    let diff = if staged {
        let tree = repo.head()?.peel_to_tree()?;
        repo.diff_tree_to_index(Some(&tree), None, Some(&mut opts))?
    } else {
        repo.diff_index_to_workdir(None, Some(&mut opts))?
    };
    let delta = diff.get_delta(0).ok_or_else(invalid)?;
    if diff.deltas().len() != 1
        || delta.status() != Delta::Modified
        || !matches!(
            delta.old_file().mode(),
            git2::FileMode::Blob | git2::FileMode::BlobExecutable
        )
        || delta.old_file().mode() != delta.new_file().mode()
        || delta.old_file().path() != Some(std::path::Path::new(path))
    {
        return Err(invalid());
    }
    let mut patch = Patch::from_diff(&diff, 0)?.ok_or_else(invalid)?;
    if patch.size(true, true, true) > 2 * 1024 * 1024 {
        return Err(invalid());
    }
    let bytes = patch.to_buf()?;
    let text = std::str::from_utf8(&bytes).map_err(|_| invalid())?;
    let mut header = String::new();
    let mut hunks: Vec<Vec<String>> = Vec::new();
    for line in text.split_inclusive('\n') {
        if line.starts_with("@@ ") {
            hunks.push(Vec::new());
        }
        if let Some(hunk) = hunks.last_mut() {
            if line.starts_with('\\') {
                return Err(invalid());
            }
            hunk.push(line.to_owned());
        } else {
            header.push_str(line);
        }
    }
    if hunks.is_empty() {
        return Err(invalid());
    }
    let repository =
        Oid::hash_object(ObjectType::Blob, repo.path().as_os_str().as_encoded_bytes())?;
    let content = Oid::hash_object(ObjectType::Blob, &bytes)?;
    let token = format!("{repository}:{staged}:{content}");
    Ok(Snapshot {
        header,
        preview: Preview { token, hunks },
    })
}

pub fn preview(repo: &Repository, path: &str, staged: bool) -> Result<Preview, AppError> {
    Ok(snapshot(repo, path, staged)?.preview)
}

pub fn apply(
    repo: &Repository,
    path: &str,
    staged: bool,
    selection: Selection,
) -> Result<(), AppError> {
    let snap = snapshot(repo, path, staged)?;
    if snap.preview.token != selection.token {
        return Err(invalid());
    }
    let hunk = snap.preview.hunks.get(selection.hunk).ok_or_else(invalid)?;
    let lines: std::collections::HashSet<_> = selection.lines.into_iter().collect();
    if lines.is_empty()
        || lines
            .iter()
            .any(|i| !hunk.get(*i).is_some_and(|s| s.starts_with(['+', '-'])))
    {
        return Err(invalid());
    }
    let old_start = hunk[0]
        .split_whitespace()
        .nth(1)
        .and_then(|s| s.strip_prefix('-'))
        .and_then(|s| s.split(',').next())
        .ok_or_else(invalid)?;
    let mut body = String::new();
    let mut old_count = 0;
    let mut new_count = 0;
    for (i, line) in hunk.iter().enumerate().skip(1) {
        match line.as_bytes()[0] {
            b'+' if !lines.contains(&i) => continue,
            b'-' if !lines.contains(&i) => {
                body.push(' ');
                body.push_str(&line[1..]);
                old_count += 1;
                new_count += 1;
            }
            b' ' => {
                body.push_str(line);
                old_count += 1;
                new_count += 1;
            }
            b'+' => {
                body.push_str(line);
                new_count += 1;
            }
            b'-' => {
                body.push_str(line);
                old_count += 1;
            }
            _ => return Err(invalid()),
        }
    }
    let start: usize = old_start.parse().map_err(|_| invalid())?;
    let new_start = if old_count == 0 {
        start + 1
    } else if new_count == 0 {
        start.saturating_sub(1)
    } else {
        start
    };
    let patch = format!(
        "{}@@ -{},{} +{},{} @@\n{}",
        snap.header, start, old_count, new_start, new_count, body
    );
    let diff = Diff::from_buffer(patch.as_bytes())?;
    repo.apply(&diff, ApplyLocation::Index, None)?;
    Ok(())
}

#[cfg(test)]
mod tests;
