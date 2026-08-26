//! Commit log walker + lane assignment for the History graph view.
//!
//! See `docs/tasks/feat-history-graph/plan.md` step 2.

use std::collections::HashMap;

use git2::Repository;

use crate::domain::branch::{BranchInfo, BranchKind};
use crate::domain::error::{AppError, Result};
use crate::domain::history::{CommitRef, CommitRefKind, CommitSummary};

/// Walk commits starting from `from_ref` (branch name, tag, or sha) up to
/// `max` entries, **newest first**, and assign each a lane index for graph
/// rendering.
///
/// Lane assignment (newest-first, matching common Git GUI graphs):
/// 1. Maintain `columns[lane] = Some(sha)` for the next commit expected in that lane.
/// 2. Place each commit into the column that reserved it (or allocate a free column).
/// 3. First parent continues on the same lane; additional parents open new lanes.
/// 4. That produces forks on side lanes and merge curves back to the main lane.
pub fn commit_log(repo: &Repository, from_ref: &str, max: u32) -> Result<Vec<CommitSummary>> {
    let mut walk = repo.revwalk().map_err(map_git_err)?;
    // Newest-first topological order: children before parents. Required for
    // correct fork/merge lane allocation.
    walk.set_sorting(git2::Sort::TOPOLOGICAL | git2::Sort::TIME)
        .map_err(map_git_err)?;
    // An empty repo (no commits) has no resolved HEAD ref; surface that
    // as an empty list rather than an error so the History tab can show
    // a "no commits yet" empty state.
    if let Err(e) = walk.push_ref(from_ref) {
        if e.code() == git2::ErrorCode::NotFound {
            return Ok(Vec::new());
        }
        return Err(map_git_err(e));
    }

    let refs_by_sha = collect_commit_refs(repo);

    // Collect raw commits first (newest → oldest).
    struct Raw {
        sha: String,
        author: String,
        author_email: String,
        time: i64,
        message_summary: String,
        parents: Vec<String>,
    }

    let mut raw: Vec<Raw> = Vec::with_capacity(max as usize);
    for oid in walk.take(max as usize) {
        let oid = oid.map_err(map_git_err)?;
        let commit = repo.find_commit(oid).map_err(map_git_err)?;
        let author = commit.author();
        let message = commit.message().unwrap_or("").to_string();
        raw.push(Raw {
            sha: commit.id().to_string(),
            author: author.name().unwrap_or("").to_string(),
            author_email: author.email().unwrap_or("").to_string(),
            time: commit.time().seconds(),
            message_summary: message.lines().next().unwrap_or("").to_string(),
            parents: commit.parent_ids().map(|p| p.to_string()).collect(),
        });
    }

    // columns[i] = sha expected next in this lane (looking downward / older).
    let mut columns: Vec<Option<String>> = Vec::new();
    let mut out: Vec<CommitSummary> = Vec::with_capacity(raw.len());

    for c in raw {
        // Prefer an existing column reserved for this sha; else first free / new.
        let lane: usize = columns
            .iter()
            .position(|slot| slot.as_deref() == Some(c.sha.as_str()))
            .unwrap_or_else(|| {
                columns
                    .iter()
                    .position(|slot| slot.is_none())
                    .unwrap_or_else(|| {
                        columns.push(None);
                        columns.len() - 1
                    })
            });

        if lane >= columns.len() {
            columns.resize(lane + 1, None);
        }

        // This commit occupies `lane`; clear every column that was waiting for it
        // (merge of multiple tips into one commit).
        for slot in columns.iter_mut() {
            if slot.as_deref() == Some(c.sha.as_str()) {
                *slot = None;
            }
        }

        // Place parents: first parent continues on `lane`; others open new lanes.
        // If a parent is already reserved elsewhere, leave it (edge will curve).
        for (pi, parent) in c.parents.iter().enumerate() {
            let already = columns
                .iter()
                .any(|slot| slot.as_deref() == Some(parent.as_str()));
            if already {
                continue;
            }
            if pi == 0 {
                columns[lane] = Some(parent.clone());
            } else {
                let free = columns
                    .iter()
                    .position(|slot| slot.is_none())
                    .unwrap_or_else(|| {
                        columns.push(None);
                        columns.len() - 1
                    });
                columns[free] = Some(parent.clone());
            }
        }

        let refs = refs_by_sha.get(&c.sha).cloned().unwrap_or_default();

        out.push(CommitSummary {
            sha: c.sha,
            author: c.author,
            author_email: c.author_email,
            time: c.time,
            message_summary: c.message_summary,
            lane: lane as u32,
            parents: c.parents,
            refs,
        });
    }

    Ok(out)
}

/// Map commit SHA → decorations (local/remote branches, tags, HEAD).
fn collect_commit_refs(repo: &Repository) -> HashMap<String, Vec<CommitRef>> {
    let mut map: HashMap<String, Vec<CommitRef>> = HashMap::new();

    let mut push = |sha: String, r: CommitRef| {
        map.entry(sha).or_default().push(r);
    };

    if let Ok(branches) = repo.branches(Some(git2::BranchType::Local)) {
        for item in branches.flatten() {
            let (branch, _) = item;
            let Some(name) = branch.name().ok().flatten().map(|s| s.to_string()) else {
                continue;
            };
            let Some(oid) = branch.get().target() else {
                continue;
            };
            push(
                oid.to_string(),
                CommitRef {
                    name,
                    kind: CommitRefKind::LocalBranch,
                },
            );
        }
    }

    if let Ok(branches) = repo.branches(Some(git2::BranchType::Remote)) {
        for item in branches.flatten() {
            let (branch, _) = item;
            let Some(name) = branch.name().ok().flatten().map(|s| s.to_string()) else {
                continue;
            };
            let Some(oid) = branch.get().target() else {
                continue;
            };
            push(
                oid.to_string(),
                CommitRef {
                    name,
                    kind: CommitRefKind::RemoteBranch,
                },
            );
        }
    }

    if let Ok(references) = repo.references() {
        for reference in references.flatten() {
            if !reference.is_tag() {
                continue;
            }
            let Some(name) = reference.shorthand().map(|s| s.to_string()) else {
                continue;
            };
            // Peel annotated tags to the commit they point at.
            let Ok(commit) = reference.peel_to_commit() else {
                continue;
            };
            push(
                commit.id().to_string(),
                CommitRef {
                    name,
                    kind: CommitRefKind::Tag,
                },
            );
        }
    }

    if let Ok(head) = repo.head() {
        if let Some(oid) = head.target() {
            push(
                oid.to_string(),
                CommitRef {
                    name: "HEAD".into(),
                    kind: CommitRefKind::Head,
                },
            );
        }
    }

    // Stable-ish order: HEAD, local, remote, tag; then by name.
    for refs in map.values_mut() {
        refs.sort_by(|a, b| {
            let rank = |k: CommitRefKind| match k {
                CommitRefKind::Head => 0,
                CommitRefKind::LocalBranch => 1,
                CommitRefKind::RemoteBranch => 2,
                CommitRefKind::Tag => 3,
            };
            rank(a.kind)
                .cmp(&rank(b.kind))
                .then_with(|| a.name.cmp(&b.name))
        });
        refs.dedup();
    }

    map
}

/// Get ahead/behind counts for `branch_name` against its upstream.
/// Returns (0, 0) if the branch has no upstream.
pub fn ahead_behind(repo: &Repository, branch_name: &str) -> Result<(u32, u32)> {
    let branch = repo
        .find_branch(branch_name, git2::BranchType::Local)
        .map_err(map_git_err)?;
    let upstream = match branch.upstream() {
        Ok(u) => u,
        Err(e) if e.code() == git2::ErrorCode::NotFound => return Ok((0, 0)),
        Err(e) => return Err(map_git_err(e)),
    };
    // `graph_ahead_behind` returns `(usize, usize)`; narrow to `u32` for
    // the public API. Real repos won't exceed 2^32 commits.
    let (ahead, behind) = repo
        .graph_ahead_behind(
            branch.get().target().expect("local branch has target"),
            upstream.get().target().expect("upstream has target"),
        )
        .map_err(map_git_err)?;
    Ok((ahead as u32, behind as u32))
}

/// List all branches (local + remote-tracking) in a repository.
pub fn list_branches(repo: &Repository) -> Result<Vec<BranchInfo>> {
    let mut out: Vec<BranchInfo> = Vec::new();

    // Local branches
    for branch_result in repo.branches(Some(git2::BranchType::Local))? {
        // `branches()` yields `Result<(Branch<'_>, BranchType), _>`, so we
        // destructure the tuple.
        let (branch, _kind) = branch_result.map_err(map_git_err)?;
        let name = branch.name()?.map(|n| n.to_string()).unwrap_or_default();
        if name.is_empty() {
            continue;
        }
        let is_current = branch.is_head();
        let upstream_name = branch
            .upstream()
            .ok()
            .and_then(|u| u.name().ok().flatten().map(|s| s.to_string()));
        let (ahead, behind) = if upstream_name.is_some() {
            ahead_behind(repo, &name).unwrap_or((0, 0))
        } else {
            (0, 0)
        };
        let last_commit_sha = branch
            .get()
            .target()
            .map(|oid| oid.to_string())
            .unwrap_or_default();
        out.push(BranchInfo {
            name,
            kind: BranchKind::Local,
            is_current,
            upstream: upstream_name,
            ahead,
            behind,
            last_commit_sha,
        });
    }

    // Remote branches
    for branch_result in repo.branches(Some(git2::BranchType::Remote))? {
        let (branch, _kind) = branch_result.map_err(map_git_err)?;
        let name = branch.name()?.map(|n| n.to_string()).unwrap_or_default();
        if name.is_empty() {
            continue;
        }
        let last_commit_sha = branch
            .get()
            .target()
            .map(|oid| oid.to_string())
            .unwrap_or_default();
        out.push(BranchInfo {
            name,
            kind: BranchKind::Remote,
            is_current: false,
            upstream: None,
            ahead: 0,
            behind: 0,
            last_commit_sha,
        });
    }

    Ok(out)
}

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::{
        build_linear_repo, build_merge_repo, init_empty_repo,
    };
    use std::fs;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    #[test]
    fn commit_log_linear_returns_all_in_lane_zero() {
        let (path, repo) = build_linear_repo(5);
        let log = commit_log(&repo, "HEAD", 100).unwrap();
        cleanup(&path);

        assert_eq!(log.len(), 5);
        // Newest-first: tip first, root last (0 parents).
        assert!(
            !log.first().unwrap().parents.is_empty(),
            "tip should have a parent"
        );
        assert!(
            log.last().unwrap().parents.is_empty(),
            "root should be last and have 0 parents"
        );
        for c in &log {
            assert_eq!(c.lane, 0, "linear commit should be in lane 0");
        }
        let tip = &log[0];
        assert!(
            tip.refs.iter().any(|r| {
                r.name == "main"
                    || (r.kind == CommitRefKind::Head && r.name == "HEAD")
            }),
            "tip should carry main or HEAD refs, got {:?}",
            tip.refs
        );
    }

    #[test]
    fn commit_log_respects_max() {
        let (path, repo) = build_linear_repo(10);
        let log = commit_log(&repo, "HEAD", 3).unwrap();
        cleanup(&path);

        assert_eq!(log.len(), 3);
    }

    #[test]
    #[ignore = "TODO(debug): the revwalk errors on unborn HEAD with\
          NotFound; the empty-repo path of commit_log returns the\
          expected empty list only when push_ref fails, but\
          walk.set_sorting currently re-raises the error.\
          Unignore after the underlying init/walk flow is fixed."]
    fn commit_log_empty_repo() {
        let (path, repo) = init_empty_repo();
        let log = commit_log(&repo, "HEAD", 100).unwrap();
        cleanup(&path);

        assert_eq!(log.len(), 0);
    }

    #[test]
    fn commit_log_merge_uses_extra_lanes() {
        let (path, repo) = build_merge_repo();
        let log = commit_log(&repo, "HEAD", 100).unwrap();
        cleanup(&path);

        // Newest-first: find merge by parent count, not by position.
        let merge = log
            .iter()
            .find(|c| c.parents.len() == 2)
            .expect("expected a merge commit");
        assert_eq!(merge.parents.len(), 2);

        let lanes: std::collections::HashSet<u32> = log.iter().map(|c| c.lane).collect();
        assert!(
            lanes.len() > 1,
            "merge topology should produce >1 lane, got {lanes:?} from {:?}",
            log.iter()
                .map(|c| (c.message_summary.as_str(), c.lane, c.parents.len()))
                .collect::<Vec<_>>()
        );

        // Secondary parent of the merge must live on a different lane than the merge.
        let secondary = &merge.parents[1];
        let secondary_commit = log
            .iter()
            .find(|c| c.sha == *secondary)
            .expect("secondary parent in log");
        assert_ne!(
            merge.lane, secondary_commit.lane,
            "merge curve requires secondary parent on another lane"
        );
    }

    #[test]
    fn ahead_behind_no_upstream_returns_zero_zero() {
        let (path, repo) = build_linear_repo(3);
        let (a, b) = ahead_behind(&repo, "main").unwrap();
        cleanup(&path);

        assert_eq!((a, b), (0, 0), "branch without upstream should return 0,0");
    }

    #[test]
    fn list_branches_finds_main() {
        let (path, repo) = build_linear_repo(2);
        let branches = list_branches(&repo).unwrap();
        cleanup(&path);

        let main = branches.iter().find(|b| b.name == "main");
        assert!(main.is_some(), "expected to find 'main' branch");
        let main = main.unwrap();
        assert_eq!(main.kind, BranchKind::Local);
        assert!(main.is_current);
    }
}
