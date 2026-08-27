//! UI-driven interactive rebase (no `git rebase -i`).
//!
//! Todo actions: pick / reword / edit / squash / fixup / drop.
//! See `docs/tech/tech-selection/00-overview.md` and scope item 6.

use std::fs;
use std::path::PathBuf;

use git2::{Oid, Repository, ResetType};

use crate::domain::error::{AppError, Result};

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum InteractiveRebaseAction {
    Pick,
    Reword,
    Edit,
    Squash,
    Fixup,
    Drop,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct InteractiveRebaseTodo {
    pub oid: String,
    pub summary: String,
    pub action: InteractiveRebaseAction,
    /// Override message for `reword` (and optional squash combined message).
    pub message: Option<String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize)]
#[serde(rename_all = "snake_case")]
pub enum InteractiveRebaseKind {
    Clean,
    AlreadyUpToDate,
    Conflicts,
    /// Stopped after an `edit` action; call continue to finish remaining todos.
    PausedForEdit,
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct InteractiveRebaseResult {
    pub kind: InteractiveRebaseKind,
    pub conflicts: Vec<String>,
    pub new_head: Option<String>,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
struct PauseState {
    upstream: String,
    remaining: Vec<InteractiveRebaseTodo>,
}

fn pause_path(repo: &Repository) -> PathBuf {
    repo.path().join("gitwave-interactive-rebase.json")
}

fn resolve_upstream(repo: &Repository, upstream: &str) -> Result<Oid> {
    let obj = repo.revparse_single(upstream).map_err(map_git_err)?;
    Ok(obj
        .peel(git2::ObjectType::Commit)
        .map_err(map_git_err)?
        .id())
}

/// Commits reachable from HEAD but not from `upstream`, oldest-first.
pub fn plan_interactive_rebase(
    repo: &Repository,
    upstream: &str,
) -> Result<Vec<InteractiveRebaseTodo>> {
    let our_oid = repo
        .head()
        .map_err(map_git_err)?
        .target()
        .ok_or_else(|| AppError::Protocol("HEAD is unborn".into()))?;
    let upstream_oid = resolve_upstream(repo, upstream)?;

    if our_oid == upstream_oid {
        return Ok(Vec::new());
    }

    let mut walk = repo.revwalk().map_err(map_git_err)?;
    walk.set_sorting(git2::Sort::TOPOLOGICAL | git2::Sort::REVERSE)
        .map_err(map_git_err)?;
    walk.push(our_oid).map_err(map_git_err)?;
    walk.hide(upstream_oid).map_err(map_git_err)?;

    let mut todos = Vec::new();
    for oid_res in walk {
        let oid = oid_res.map_err(map_git_err)?;
        let commit = repo.find_commit(oid).map_err(map_git_err)?;
        let summary = commit.summary().unwrap_or("(no message)").to_string();
        todos.push(InteractiveRebaseTodo {
            oid: oid.to_string(),
            summary,
            action: InteractiveRebaseAction::Pick,
            message: None,
        });
    }
    Ok(todos)
}

fn collect_index_conflicts(repo: &Repository) -> Result<Vec<String>> {
    let index = repo.index().map_err(map_git_err)?;
    let mut out = Vec::new();
    for c in index.conflicts().map_err(map_git_err)? {
        let ic = c.map_err(map_git_err)?;
        if let Some(e) = ic.our.or(ic.their).or(ic.ancestor) {
            out.push(String::from_utf8_lossy(&e.path).into_owned());
        }
    }
    Ok(out)
}

fn abort_cherry_pick_state(repo: &Repository) {
    let _ = repo.cleanup_state();
}

fn cherry_pick_onto_head(repo: &Repository, commit_oid: Oid) -> Result<()> {
    let commit = repo.find_commit(commit_oid).map_err(map_git_err)?;
    let mut opts = git2::CherrypickOptions::new();
    repo.cherrypick(&commit, Some(&mut opts))
        .map_err(map_git_err)?;
    let conflicts = collect_index_conflicts(repo)?;
    if !conflicts.is_empty() {
        abort_cherry_pick_state(repo);
        return Err(AppError::Protocol(format!(
            "conflict while applying {}: {}",
            &commit_oid.to_string()[..7.min(commit_oid.to_string().len())],
            conflicts.join(", ")
        )));
    }
    Ok(())
}

fn commit_index(repo: &Repository, parents: &[&git2::Commit], message: &str) -> Result<Oid> {
    let mut index = repo.index().map_err(map_git_err)?;
    let tree_oid = index.write_tree().map_err(map_git_err)?;
    let tree = repo.find_tree(tree_oid).map_err(map_git_err)?;
    let sig = git2::Signature::now("GitWave", "noreply@gitwave.local").map_err(map_git_err)?;
    let oid = repo
        .commit(Some("HEAD"), &sig, &sig, message, &tree, parents)
        .map_err(map_git_err)?;
    abort_cherry_pick_state(repo);
    Ok(oid)
}

fn message_for(todo: &InteractiveRebaseTodo, commit: &git2::Commit<'_>) -> String {
    if let Some(ref m) = todo.message {
        if !m.trim().is_empty() {
            return m.clone();
        }
    }
    commit.message().unwrap_or("").to_string()
}

/// Reset HEAD to `upstream` and replay `todos` in order.
pub fn execute_interactive_rebase(
    repo: &Repository,
    upstream: &str,
    todos: &[InteractiveRebaseTodo],
) -> Result<InteractiveRebaseResult> {
    let upstream_oid = resolve_upstream(repo, upstream)?;
    let active: Vec<&InteractiveRebaseTodo> = todos
        .iter()
        .filter(|t| t.action != InteractiveRebaseAction::Drop)
        .collect();
    if active.is_empty() {
        // All dropped → move HEAD to upstream.
        let obj = repo.find_object(upstream_oid, None).map_err(map_git_err)?;
        repo.reset(&obj, ResetType::Hard, None)
            .map_err(map_git_err)?;
        let _ = fs::remove_file(pause_path(repo));
        return Ok(InteractiveRebaseResult {
            kind: InteractiveRebaseKind::Clean,
            conflicts: Vec::new(),
            new_head: Some(upstream_oid.to_string()),
        });
    }

    // Hard reset to upstream base, then cherry-pick each todo.
    let obj = repo.find_object(upstream_oid, None).map_err(map_git_err)?;
    repo.reset(&obj, ResetType::Hard, None)
        .map_err(map_git_err)?;
    let _ = fs::remove_file(pause_path(repo));

    replay_todos(repo, upstream, &active)
}

fn replay_todos(
    repo: &Repository,
    upstream: &str,
    todos: &[&InteractiveRebaseTodo],
) -> Result<InteractiveRebaseResult> {
    let mut i = 0;
    while i < todos.len() {
        let todo = todos[i];
        let oid = Oid::from_str(&todo.oid)
            .map_err(|e| AppError::Protocol(format!("bad oid {}: {e}", todo.oid)))?;
        let commit = repo.find_commit(oid).map_err(map_git_err)?;

        match todo.action {
            InteractiveRebaseAction::Drop => {
                i += 1;
            }
            InteractiveRebaseAction::Pick | InteractiveRebaseAction::Reword => {
                if let Err(e) = cherry_pick_onto_head(repo, oid) {
                    return conflict_from_err(e);
                }
                let head = repo
                    .head()
                    .map_err(map_git_err)?
                    .peel_to_commit()
                    .map_err(map_git_err)?;
                let msg = message_for(todo, &commit);
                let new_oid = commit_index(repo, &[&head], &msg)?;
                let _ = new_oid;
                i += 1;
            }
            InteractiveRebaseAction::Edit => {
                if let Err(e) = cherry_pick_onto_head(repo, oid) {
                    return conflict_from_err(e);
                }
                let head = repo
                    .head()
                    .map_err(map_git_err)?
                    .peel_to_commit()
                    .map_err(map_git_err)?;
                let msg = message_for(todo, &commit);
                let new_oid = commit_index(repo, &[&head], &msg)?;
                let remaining: Vec<InteractiveRebaseTodo> =
                    todos[i + 1..].iter().map(|t| (*t).clone()).collect();
                if !remaining.is_empty() {
                    let state = PauseState {
                        upstream: upstream.to_string(),
                        remaining,
                    };
                    let json = serde_json::to_string_pretty(&state)
                        .map_err(|e| AppError::Unknown(format!("serialize pause: {e}")))?;
                    fs::write(pause_path(repo), json)
                        .map_err(|e| AppError::Unknown(format!("write pause: {e}")))?;
                }
                return Ok(InteractiveRebaseResult {
                    kind: InteractiveRebaseKind::PausedForEdit,
                    conflicts: Vec::new(),
                    new_head: Some(new_oid.to_string()),
                });
            }
            InteractiveRebaseAction::Squash | InteractiveRebaseAction::Fixup => {
                // Squash/fixup into the previous commit: must not be first.
                if i == 0 {
                    return Err(AppError::Protocol(
                        "cannot squash/fixup the first commit in the todo list".into(),
                    ));
                }
                if let Err(e) = cherry_pick_onto_head(repo, oid) {
                    return conflict_from_err(e);
                }
                // Amend HEAD: same parents as HEAD, new tree from index, maybe combined msg.
                let head = repo
                    .head()
                    .map_err(map_git_err)?
                    .peel_to_commit()
                    .map_err(map_git_err)?;
                let parents: Vec<git2::Commit> = head.parents().collect();
                let parent_refs: Vec<&git2::Commit> = parents.iter().collect();
                let mut index = repo.index().map_err(map_git_err)?;
                let tree_oid = index.write_tree().map_err(map_git_err)?;
                let tree = repo.find_tree(tree_oid).map_err(map_git_err)?;
                let sig = git2::Signature::now("GitWave", "noreply@gitwave.local")
                    .map_err(map_git_err)?;
                let msg = if todo.action == InteractiveRebaseAction::Fixup {
                    head.message().unwrap_or("").to_string()
                } else if let Some(ref m) = todo.message {
                    m.clone()
                } else {
                    format!(
                        "{}\n\n{}",
                        head.message().unwrap_or("").trim_end(),
                        commit.message().unwrap_or("").trim_end()
                    )
                };
                // Detach and replace HEAD commit.
                repo.commit(Some("HEAD"), &sig, &sig, &msg, &tree, &parent_refs)
                    .map_err(map_git_err)?;
                abort_cherry_pick_state(repo);
                i += 1;
            }
        }
    }

    let new_head = repo
        .head()
        .ok()
        .and_then(|h| h.target())
        .map(|o| o.to_string());
    let _ = fs::remove_file(pause_path(repo));
    Ok(InteractiveRebaseResult {
        kind: InteractiveRebaseKind::Clean,
        conflicts: Vec::new(),
        new_head,
    })
}

fn conflict_from_err(e: AppError) -> Result<InteractiveRebaseResult> {
    let msg = match &e {
        AppError::Protocol(s) | AppError::Unknown(s) => s.clone(),
        other => format!("{other:?}"),
    };
    Ok(InteractiveRebaseResult {
        kind: InteractiveRebaseKind::Conflicts,
        conflicts: vec![msg],
        new_head: None,
    })
}

pub fn continue_interactive_rebase(repo: &Repository) -> Result<InteractiveRebaseResult> {
    let path = pause_path(repo);
    let raw = fs::read_to_string(&path)
        .map_err(|_| AppError::Protocol("no interactive rebase paused for edit".into()))?;
    let state: PauseState = serde_json::from_str(&raw)
        .map_err(|e| AppError::Unknown(format!("parse pause state: {e}")))?;
    let refs: Vec<&InteractiveRebaseTodo> = state.remaining.iter().collect();
    replay_todos(repo, &state.upstream, &refs)
}

pub fn abort_interactive_rebase_pause(repo: &Repository) -> Result<()> {
    let _ = fs::remove_file(pause_path(repo));
    Ok(())
}

pub fn interactive_rebase_paused(repo: &Repository) -> bool {
    pause_path(repo).exists()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::build_linear_repo;
    use std::fs;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    #[test]
    fn plan_lists_commits_oldest_first() {
        let (path, repo) = build_linear_repo(3);
        let root = {
            let tip = repo.head().unwrap().peel_to_commit().unwrap();
            let mut c = tip;
            while c.parent_count() > 0 {
                c = c.parent(0).unwrap();
            }
            c.id()
        };
        // Hide nothing except empty: plan against root's parent doesn't work.
        // Create branch at first commit and plan onto it from tip.
        let first = repo.revparse_single("HEAD~2").unwrap().id();
        repo.branch("base", &repo.find_commit(first).unwrap(), false)
            .unwrap();
        let todos = plan_interactive_rebase(&repo, "base").unwrap();
        assert_eq!(todos.len(), 2);
        assert!(todos
            .iter()
            .all(|t| t.action == InteractiveRebaseAction::Pick));
        let _ = root;
        cleanup(&path);
    }

    #[test]
    fn drop_all_resets_to_upstream() {
        let (path, repo) = build_linear_repo(3);
        let first = repo.revparse_single("HEAD~2").unwrap().id();
        repo.branch("base", &repo.find_commit(first).unwrap(), false)
            .unwrap();
        let mut todos = plan_interactive_rebase(&repo, "base").unwrap();
        for t in &mut todos {
            t.action = InteractiveRebaseAction::Drop;
        }
        let res = execute_interactive_rebase(&repo, "base", &todos).unwrap();
        assert_eq!(res.kind, InteractiveRebaseKind::Clean);
        assert_eq!(repo.head().unwrap().peel_to_commit().unwrap().id(), first);
        cleanup(&path);
    }

    #[test]
    fn reword_changes_message() {
        let (path, repo) = build_linear_repo(3);
        let first = repo.revparse_single("HEAD~2").unwrap().id();
        repo.branch("base", &repo.find_commit(first).unwrap(), false)
            .unwrap();
        let mut todos = plan_interactive_rebase(&repo, "base").unwrap();
        assert_eq!(todos.len(), 2);
        todos[0].action = InteractiveRebaseAction::Reword;
        todos[0].message = Some("rewritten one".into());
        let res = execute_interactive_rebase(&repo, "base", &todos).unwrap();
        assert_eq!(res.kind, InteractiveRebaseKind::Clean);
        let tip = repo.head().unwrap().peel_to_commit().unwrap();
        let parent = tip.parent(0).unwrap();
        assert_eq!(parent.message().unwrap().trim(), "rewritten one");
        cleanup(&path);
    }
}
