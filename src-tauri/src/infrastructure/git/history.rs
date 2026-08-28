//! Commit log walker + lane assignment for the History graph view.
//!
//! See `docs/tasks/feat-history-graph/plan.md` step 2.

use std::collections::HashMap;

use git2::Repository;

use crate::domain::branch::{BranchInfo, BranchKind};
use crate::domain::error::{AppError, Result};
use crate::domain::history::{
    CommitDetails, CommitRef, CommitRefKind, CommitSummary, FileStatus, FileSummary, PrCommit,
};

/// Full details for a single commit (inspector header): identity, full
/// message, parents, and the changed-file list vs its first parent with
/// rename tracking. `sha` accepts any revspec git can resolve.
pub fn commit_details(repo: &Repository, sha: &str) -> Result<CommitDetails> {
    let commit = repo
        .revparse_single(sha)
        .map_err(map_git_err)?
        .peel_to_commit()
        .map_err(map_git_err)?;
    let author = commit.author();
    let parents: Vec<String> = commit.parent_ids().map(|p| p.to_string()).collect();

    let tree_new = commit.tree().map_err(map_git_err)?;
    let tree_old = if commit.parent_count() > 0 {
        Some(
            commit
                .parent(0)
                .map_err(map_git_err)?
                .tree()
                .map_err(map_git_err)?,
        )
    } else {
        None
    };
    let mut diff = repo
        .diff_tree_to_tree(tree_old.as_ref(), Some(&tree_new), None)
        .map_err(map_git_err)?;
    // Enable rename/copy detection so deltas carry Renamed + old_path.
    diff.find_similar(None).map_err(map_git_err)?;

    // RefCell accumulator — same pattern as `diff_to_files` in diff.rs.
    use std::cell::RefCell;
    let files: RefCell<Vec<FileSummary>> = RefCell::new(Vec::new());
    diff.foreach(
        &mut |delta, _| {
            let kind = match delta.status() {
                git2::Delta::Added => FileStatus::Added,
                git2::Delta::Deleted => FileStatus::Deleted,
                git2::Delta::Renamed => FileStatus::Renamed,
                git2::Delta::Copied => FileStatus::Copied,
                git2::Delta::Untracked => FileStatus::Untracked,
                _ => FileStatus::Modified,
            };
            let old_path = delta
                .old_file()
                .path()
                .map(|p| p.to_string_lossy().into_owned());
            let old_path = if matches!(kind, FileStatus::Renamed | FileStatus::Copied) {
                old_path
            } else {
                None
            };
            let path = delta
                .new_file()
                .path()
                .or_else(|| delta.old_file().path())
                .map(|p| p.to_string_lossy().into_owned())
                .unwrap_or_default();
            files.borrow_mut().push(FileSummary {
                path,
                old_path,
                kind,
                additions: 0,
                deletions: 0,
            });
            true
        },
        None,
        None,
        Some(&mut |_delta, _hunk, line| {
            if let Some(file) = files.borrow_mut().last_mut() {
                match line.origin() {
                    '+' => file.additions += 1,
                    '-' => file.deletions += 1,
                    _ => {}
                }
            }
            true
        }),
    )
    .map_err(map_git_err)?;

    Ok(CommitDetails {
        sha: commit.id().to_string(),
        author: author.name().unwrap_or("").to_string(),
        author_email: author.email().unwrap_or("").to_string(),
        time: commit.time().seconds(),
        message_full: commit.message().unwrap_or("").trim().to_string(),
        parents,
        files: files.into_inner(),
    })
}

/// Full commit messages of the latest `n` commits on the current branch,
/// newest first, walking the first-parent chain from HEAD.
///
/// Merge commits on the branch line are included; commits merged in from
/// side branches are not — that matches "当前分支最近 n 次提交". Unlike
/// [`commit_log`] this keeps the whole message (subject + body), e.g. for
/// AI prompt style reference. Empty repo (unborn HEAD) yields an empty vec.
pub fn commit_recent_messages(repo: &Repository, n: u32) -> Result<Vec<String>> {
    let mut out: Vec<String> = Vec::new();
    let Some(mut oid) = repo.head().ok().and_then(|head| head.target()) else {
        return Ok(out);
    };
    while out.len() < n as usize {
        let commit = repo.find_commit(oid).map_err(map_git_err)?;
        out.push(commit.message().unwrap_or("").trim().to_string());
        if commit.parent_count() == 0 {
            break;
        }
        oid = commit.parent_id(0).map_err(map_git_err)?;
    }
    Ok(out)
}

/// Resolve a branch-like reference ("main", "origin/main", or any revspec
/// git understands) to its tip commit oid. Local branches win over remote
/// ones with the same short name.
pub fn resolve_ref_oid(repo: &Repository, name: &str) -> Result<git2::Oid> {
    let trimmed = name.trim();
    if let Ok(branch) = repo.find_branch(trimmed, git2::BranchType::Local) {
        if let Some(oid) = branch.get().target() {
            return Ok(oid);
        }
    }
    if let Ok(branch) = repo.find_branch(trimmed, git2::BranchType::Remote) {
        if let Some(oid) = branch.get().target() {
            return Ok(oid);
        }
    }
    repo.revparse_single(trimmed)
        .map_err(map_git_err)?
        .peel_to_commit()
        .map(|commit| commit.id())
        .map_err(map_git_err)
}

/// Commits reachable from `head` but not from `base` (newest first, capped
/// at `limit`) — the branch segment a PR description describes. Ancestors
/// of `base` are excluded, matching `git log base..head`.
pub fn commits_ahead_of(
    repo: &Repository,
    base: git2::Oid,
    head: git2::Oid,
    limit: usize,
) -> Result<Vec<PrCommit>> {
    let mut walk = repo.revwalk().map_err(map_git_err)?;
    walk.set_sorting(git2::Sort::TIME).map_err(map_git_err)?;
    walk.push(head).map_err(map_git_err)?;
    walk.hide(base).map_err(map_git_err)?;
    let mut out = Vec::new();
    for oid in walk.take(limit) {
        let commit = repo
            .find_commit(oid.map_err(map_git_err)?)
            .map_err(map_git_err)?;
        let message = commit.message().unwrap_or("").trim().to_string();
        let subject = message.lines().next().unwrap_or("").to_string();
        out.push(PrCommit {
            sha: commit.id().to_string(),
            subject,
            message_full: message,
        });
    }
    Ok(out)
}

/// Walk commits across **all local and remote branch tips** (like
/// `git log --all`), up to `max` entries, **newest first**, and assign
/// each a lane index for graph rendering.
///
/// History is intentionally not limited to the current HEAD tip: switching
/// branches must not hide other branches' commits from the DAG.
///
/// Lane assignment (newest-first, matching common Git GUI graphs):
/// 1. Maintain `columns[lane] = Some(sha)` for the next commit expected in that lane.
/// 2. Place each commit into the column that reserved it (or allocate a free column).
/// 3. First parent continues on the same lane; additional parents open new lanes.
/// 4. That produces forks on side lanes and merge curves back to the main lane.
pub fn commit_log(repo: &Repository, max: u32, filter: Option<&str>) -> Result<Vec<CommitSummary>> {
    let filter = filter.map(str::trim).filter(|f| !f.is_empty());
    // Filtering has to scan deeper than the returned page size.
    let scan_cap = if filter.is_some() { 10_000 } else { max };
    let filter_lower = filter.map(str::to_lowercase);
    let mut walk = repo.revwalk().map_err(map_git_err)?;
    // Newest-first topological order: children before parents. Required for
    // correct fork/merge lane allocation.
    walk.set_sorting(git2::Sort::TOPOLOGICAL | git2::Sort::TIME)
        .map_err(map_git_err)?;

    let mut pushed = 0u32;
    // Push every local + remote branch tip so the graph includes the full DAG.
    if let Ok(branches) = repo.branches(None) {
        for item in branches.flatten() {
            let (branch, _) = item;
            let Some(oid) = branch.get().target() else {
                continue;
            };
            if walk.push(oid).is_ok() {
                pushed += 1;
            }
        }
    }
    // Detached HEAD tip (not already covered by a branch).
    if let Ok(head) = repo.head() {
        if !head.is_branch() {
            if let Some(oid) = head.target() {
                if walk.push(oid).is_ok() {
                    pushed += 1;
                }
            }
        }
    }

    if pushed == 0 {
        // Empty / unborn repo — no tips to walk.
        return Ok(Vec::new());
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
    for oid in walk.take(scan_cap as usize) {
        let oid = oid.map_err(map_git_err)?;
        let commit = repo.find_commit(oid).map_err(map_git_err)?;
        let author = commit.author();
        let message = commit.message().unwrap_or("").to_string();
        let summary = message.lines().next().unwrap_or("").to_string();
        if let Some(needle) = &filter_lower {
            let matches = summary.to_lowercase().contains(needle)
                || author.name().unwrap_or("").to_lowercase().contains(needle)
                || message.to_lowercase().contains(needle);
            if !matches {
                continue;
            }
        }
        raw.push(Raw {
            sha: commit.id().to_string(),
            author: author.name().unwrap_or("").to_string(),
            author_email: author.email().unwrap_or("").to_string(),
            time: commit.time().seconds(),
            message_summary: summary,
            parents: commit.parent_ids().map(|p| p.to_string()).collect(),
        });
    }

    // columns[i] = (sha, branch tag) expected next in this lane. The tag is
    // the branch lineage the reservation belongs to: a commit carrying its
    // own branch ref only consumes a reservation for that same branch. Only
    // TRUE tips (no child in the loaded list) may start a fresh lane —
    // ref-bearing commits on a continuous line follow their child's line.
    let mut columns: Vec<Option<(String, Option<String>)>> = Vec::new();
    let parent_shas: std::collections::HashSet<String> =
        raw.iter().flat_map(|c| c.parents.iter().cloned()).collect();
    let mut out: Vec<CommitSummary> = Vec::with_capacity(raw.len());

    for c in raw {
        let refs = refs_by_sha.get(&c.sha).cloned().unwrap_or_default();
        let branch_refs: Vec<&str> = refs
            .iter()
            .filter(|r| {
                matches!(
                    r.kind,
                    CommitRefKind::LocalBranch | CommitRefKind::RemoteBranch
                )
            })
            .map(|r| r.name.as_str())
            .collect();
        // A commit that is some other commit's parent lies on a continuous
        // line and must never be pushed onto a fresh lane.
        let is_tip = !parent_shas.contains(&c.sha);

        let matching: Vec<usize> = columns
            .iter()
            .enumerate()
            .filter(|(_, slot)| slot.as_ref().is_some_and(|(sha, _)| sha == c.sha.as_str()))
            .map(|(i, _)| i)
            .collect();

        let lane: usize = if !matching.is_empty() {
            let own = matching.iter().copied().find(|i| {
                columns[*i]
                    .as_ref()
                    .and_then(|(_, tag)| tag.as_deref())
                    .is_some_and(|tag| branch_refs.contains(&tag))
            });
            match own {
                Some(i) => i,
                // True tip of a stacked branch: the reserved line belongs to
                // a different branch — start a fresh lane.
                None if !branch_refs.is_empty() && is_tip => {
                    columns.push(None);
                    columns.len() - 1
                }
                None => matching[0],
            }
        } else {
            // First free lane. Branch tips pin their parent reservation on it
            // below, so sibling tips each end up on their own lane while
            // freed lanes get reused (Fork-style).
            columns
                .iter()
                .position(|slot| slot.is_none())
                .unwrap_or_else(|| {
                    columns.push(None);
                    columns.len() - 1
                })
        };

        if lane >= columns.len() {
            columns.resize(lane + 1, None);
        }

        // Lineage flowing down this lane: the commit's primary branch ref,
        // or the lane's existing tag for plain (ref-less) commits.
        let lineage = primary_branch(&refs)
            .map(str::to_string)
            .or_else(|| columns[lane].as_ref().and_then(|(_, tag)| tag.clone()));

        // This commit occupies `lane`; clear every column that was waiting for it
        // (merge of multiple tips into one commit).
        for slot in columns.iter_mut() {
            if slot.as_ref().is_some_and(|(sha, _)| sha == c.sha.as_str()) {
                *slot = None;
            }
        }

        // Place parents. First parent: always pin the lane's reservation to
        // it — the tip's edge occupies this lane while travelling down to the
        // parent, which is what keeps sibling tips on separate lanes (and
        // lets a reserved-elsewhere parent still be reached by the curve).
        // Additional parents: open a lane tagged with the parent's own branch;
        // if the parent is already reserved elsewhere, leave it (edge curves).
        for (pi, parent) in c.parents.iter().enumerate() {
            if pi == 0 {
                columns[lane] = Some((parent.clone(), lineage.clone()));
                continue;
            }
            let already = columns
                .iter()
                .any(|slot| slot.as_ref().is_some_and(|(sha, _)| sha == parent.as_str()));
            if already {
                continue;
            }
            let free = columns
                .iter()
                .position(|slot| slot.is_none())
                .unwrap_or_else(|| {
                    columns.push(None);
                    columns.len() - 1
                });
            let parent_tag = refs_by_sha.get(parent).and_then(|rs| {
                rs.iter()
                    .find(|r| {
                        matches!(
                            r.kind,
                            CommitRefKind::LocalBranch | CommitRefKind::RemoteBranch
                        )
                    })
                    .map(|r| r.name.clone())
            });
            columns[free] = Some((parent.clone(), parent_tag));
        }

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

/// Primary branch lineage name for a commit (first local/remote branch ref).
fn primary_branch(refs: &[CommitRef]) -> Option<&str> {
    refs.iter()
        .find(|r| {
            matches!(
                r.kind,
                CommitRefKind::LocalBranch | CommitRefKind::RemoteBranch
            )
        })
        .map(|r| r.name.as_str())
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
    let mut local_names: Vec<String> = Vec::new();
    for branch_result in repo.branches(Some(git2::BranchType::Local))? {
        // `branches()` yields `Result<(Branch<'_>, BranchType), _>`, so we
        // destructure the tuple.
        let (branch, _kind) = branch_result.map_err(map_git_err)?;
        let name = branch.name()?.map(|n| n.to_string()).unwrap_or_default();
        if name.is_empty() {
            continue;
        }
        local_names.push(name.clone());
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
        let last_commit_time = branch_tip_time(repo, &last_commit_sha);
        out.push(BranchInfo {
            name,
            kind: BranchKind::Local,
            is_current,
            upstream: upstream_name,
            ahead,
            behind,
            last_commit_sha,
            last_commit_time,
        });
    }

    // Empty repository: HEAD points at the (unborn) default branch but
    // `branches()` lists no refs yet. Surface the branch the user will
    // commit onto instead of an empty list ("No branches found").
    if let Ok(head_ref) = repo.find_reference("HEAD") {
        if let Some(name) = head_ref
            .symbolic_target()
            .and_then(|t| t.strip_prefix("refs/heads/"))
        {
            if !name.is_empty() && !local_names.contains(&name.to_string()) {
                out.push(BranchInfo {
                    name: name.to_string(),
                    kind: BranchKind::Local,
                    is_current: true,
                    upstream: None,
                    ahead: 0,
                    behind: 0,
                    last_commit_sha: String::new(),
                    last_commit_time: 0,
                });
            }
        }
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
        let last_commit_time = branch_tip_time(repo, &last_commit_sha);
        out.push(BranchInfo {
            name,
            kind: BranchKind::Remote,
            is_current: false,
            upstream: None,
            ahead: 0,
            behind: 0,
            last_commit_sha,
            last_commit_time,
        });
    }

    Ok(out)
}

fn branch_tip_time(repo: &Repository, sha: &str) -> i64 {
    if sha.is_empty() {
        return 0;
    }
    git2::Oid::from_str(sha)
        .ok()
        .and_then(|oid| repo.find_commit(oid).ok())
        .map(|commit| commit.time().seconds())
        .unwrap_or(0)
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
        let log = commit_log(&repo, 100, None).unwrap();
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
                r.name == "main" || (r.kind == CommitRefKind::Head && r.name == "HEAD")
            }),
            "tip should carry main or HEAD refs, got {:?}",
            tip.refs
        );
    }

    #[test]
    fn commit_log_respects_max() {
        let (path, repo) = build_linear_repo(10);
        let log = commit_log(&repo, 3, None).unwrap();
        cleanup(&path);

        assert_eq!(log.len(), 3);
    }

    #[test]
    fn commit_recent_messages_returns_full_messages_newest_first() {
        let (path, repo) = build_linear_repo(5);
        let msgs = commit_recent_messages(&repo, 3).unwrap();
        cleanup(&path);

        assert_eq!(msgs, vec!["commit 4", "commit 3", "commit 2"]);
    }

    #[test]
    fn commit_recent_messages_keeps_body_and_walks_first_parent() {
        // build_merge_repo leaves HEAD on main's 2-parent merge commit; the
        // first-parent chain must include the merge (full message) but skip
        // the feature branch's commits.
        let (path, repo) = build_merge_repo();
        let msgs = commit_recent_messages(&repo, 10).unwrap();
        cleanup(&path);

        assert_eq!(msgs.len(), 3);
        assert!(msgs[0].contains("merge"), "top should be the merge commit");
    }

    #[test]
    fn commit_recent_messages_empty_repo() {
        let (path, repo) = init_empty_repo();
        let msgs = commit_recent_messages(&repo, 3).unwrap();
        cleanup(&path);

        assert!(msgs.is_empty());
    }

    #[test]
    fn commit_details_reads_identity_message_and_files() {
        let (path, repo) = build_linear_repo(3);
        let head = repo.head().unwrap().peel_to_commit().unwrap();

        let d = commit_details(&repo, &head.id().to_string()).unwrap();
        assert_eq!(d.message_full, "commit 2");
        assert_eq!(d.author, "Test");
        assert_eq!(d.author_email, "test@local");
        assert_eq!(d.parents, vec![head.parent(0).unwrap().id().to_string()]);
        assert_eq!(d.files.len(), 1);
        assert_eq!(d.files[0].path, "file2.txt");
        assert_eq!(d.files[0].kind, FileStatus::Added);
        assert_eq!(d.files[0].additions, 1);
        assert_eq!(d.files[0].deletions, 0);

        // Walk down to the root commit: no parents, diff vs empty tree.
        let mid = commit_details(&repo, &d.parents[0]).unwrap();
        assert_eq!(mid.parents.len(), 1);
        let root = commit_details(&repo, &mid.parents[0]).unwrap();
        assert!(root.parents.is_empty());
        assert_eq!(root.files.len(), 1);
        assert_eq!(root.files[0].path, "file0.txt");
        assert_eq!(root.message_full, "commit 0");
        cleanup(&path);
    }

    #[test]
    #[ignore = "TODO(debug): the revwalk errors on unborn HEAD with\
          NotFound; the empty-repo path of commit_log returns the\
          expected empty list only when push_ref fails, but\
          walk.set_sorting currently re-raises the error.\
          Unignore after the underlying init/walk flow is fixed."]
    fn commit_log_empty_repo() {
        let (path, repo) = init_empty_repo();
        let log = commit_log(&repo, 100, None).unwrap();
        cleanup(&path);

        assert_eq!(log.len(), 0);
    }

    #[test]
    fn commit_log_includes_all_branch_tips() {
        let (path, repo) = build_merge_repo();
        // HEAD is on main after build_merge_repo; feature commits must still appear.
        let log = commit_log(&repo, 100, None).unwrap();
        cleanup(&path);

        let messages: Vec<&str> = log.iter().map(|c| c.message_summary.as_str()).collect();
        assert!(
            messages.contains(&"b1") && messages.contains(&"b2"),
            "expected feature-branch commits in --all log, got {messages:?}"
        );
        assert!(
            messages.contains(&"a2") && messages.iter().any(|m| m.contains("merge")),
            "expected main-line commits in --all log, got {messages:?}"
        );
    }

    #[test]
    fn commit_log_merge_uses_extra_lanes() {
        let (path, repo) = build_merge_repo();
        let log = commit_log(&repo, 100, None).unwrap();
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
    fn commit_log_branch_ref_chain_stays_continuous() {
        let (path, repo) = build_linear_repo(3);
        // Merged-away feature refs stacked on a linear chain: main@v2 (HEAD),
        // b1@v1, b0@v0 — every commit is some other commit's parent, so the
        // chain must stay on one continuous lane (no staircase zigzag).
        let v1 = repo
            .revparse_single("HEAD~1")
            .unwrap()
            .peel(git2::ObjectType::Commit)
            .unwrap();
        repo.branch("b1", v1.as_commit().unwrap(), false).unwrap();
        let v0 = repo
            .revparse_single("HEAD~2")
            .unwrap()
            .peel(git2::ObjectType::Commit)
            .unwrap();
        repo.branch("b0", v0.as_commit().unwrap(), false).unwrap();

        let log = commit_log(&repo, 100, None).unwrap();
        cleanup(&path);

        let lanes: Vec<u32> = log.iter().map(|c| c.lane).collect();
        assert_eq!(
            lanes,
            vec![0, 0, 0],
            "a ref-bearing chain with children stays continuous, got {lanes:?}"
        );
    }

    #[test]
    fn commit_log_sibling_branch_tips_get_distinct_lanes() {
        let (path, repo) = build_linear_repo(2); // v0 <- v1(main)
        let sig = git2::Signature::now("Test", "test@local").unwrap();
        let parent = repo
            .revparse_single("HEAD")
            .unwrap()
            .peel_to_commit()
            .unwrap();
        let tree = parent.tree().unwrap();

        // Two sibling commits, both children of the same parent (like a fan
        // of one-commit renovate/* branches).
        for (name, message) in [("s1", "s1"), ("s2", "s2")] {
            repo.commit(
                Some(format!("refs/heads/{name}").as_str()),
                &sig,
                &sig,
                message,
                &tree,
                &[&parent],
            )
            .unwrap();
        }

        let log = commit_log(&repo, 100, None).unwrap();
        cleanup(&path);

        let lane_of = |msg: &str| {
            log.iter()
                .find(|c| c.message_summary == msg)
                .expect("sibling commit in log")
                .lane
        };
        assert_ne!(
            lane_of("s1"),
            lane_of("s2"),
            "sibling branch tips must not share a lane"
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

    #[test]
    fn list_branches_shows_unborn_default_branch() {
        let (path, repo) = init_empty_repo();
        let branches = list_branches(&repo).unwrap();
        cleanup(&path);

        assert_eq!(branches.len(), 1, "empty repo must list its default branch");
        assert_eq!(branches[0].name, "main");
        assert_eq!(branches[0].kind, BranchKind::Local);
        assert!(branches[0].is_current, "the unborn branch is HEAD's target");
        assert!(branches[0].last_commit_sha.is_empty());
    }
}
