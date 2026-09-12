//! UI-driven interactive rebase (no `git rebase -i`).
//!
//! Todo actions: pick / reword / edit / squash / fixup / drop.
//! See `docs/tech/tech-selection/00-overview.md` and scope item 6.

use std::fs;
use std::path::PathBuf;

use git2::{Oid, Repository, ResetType};

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;
use crate::infrastructure::git::git2_adapter::commit_signature;
use crate::infrastructure::git::worktree_guard::{classify_dirty, ensure_clean};

fn map_git_err(e: git2::Error) -> AppError {
    AppError::unknown_with(
        codes::git::GIT_ERROR,
        format!("git: {e}"),
        &[("error", e.to_string())],
    )
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
    /// Branch tip before `execute` moved it — abort/conflict restore target.
    /// `#[serde(default)]` keeps pre-existing pause files readable.
    #[serde(default)]
    original_head: Option<String>,
}

fn pause_path(repo: &Repository) -> PathBuf {
    repo.path().join("gitwave-interactive-rebase.json")
}

fn write_pause(repo: &Repository, state: &PauseState) -> Result<()> {
    let json = serde_json::to_string_pretty(state).map_err(|e| {
        AppError::unknown_with(
            codes::git::SERIALIZE_PAUSE,
            format!("serialize pause: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    fs::write(pause_path(repo), json).map_err(|e| {
        AppError::unknown_with(
            codes::git::WRITE_PAUSE,
            format!("write pause: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    Ok(())
}

/// Best-effort restore after a failed replay: put the branch back where
/// `execute` found it and drop the pause file, so a conflict never leaves a
/// half-rebased branch behind.
fn restore_original(repo: &Repository, original: Option<Oid>) {
    if let Some(orig) = original {
        if let Ok(obj) = repo.find_object(orig, None) {
            let _ = repo.reset(&obj, ResetType::Hard, None);
        }
        let _ = repo.cleanup_state();
    }
    let _ = fs::remove_file(pause_path(repo));
}

/// Fold the current index into HEAD, keeping message and parents
/// (≡ `git commit --amend --no-edit` of staged changes). Used by `continue`
/// so Edit-pause modifications land in the paused commit instead of being
/// silently mixed into the replayed todos.
fn amend_head_with_index(repo: &Repository) -> Result<()> {
    let head = repo
        .head()
        .map_err(map_git_err)?
        .peel_to_commit()
        .map_err(map_git_err)?;
    let mut index = repo.index().map_err(map_git_err)?;
    let tree_oid = index.write_tree().map_err(map_git_err)?;
    let tree = repo.find_tree(tree_oid).map_err(map_git_err)?;
    let sig = commit_signature(repo)?;
    head.amend(
        Some("HEAD"),
        Some(&sig),
        Some(&sig),
        None,
        None,
        Some(&tree),
    )
    .map_err(map_git_err)?;
    Ok(())
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
        .ok_or_else(|| AppError::protocol(codes::git::HEAD_UNBORN, "HEAD is unborn"))?;
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
        let summary = commit
            .summary()
            .ok()
            .flatten()
            .unwrap_or("(no message)")
            .to_string();
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
        let oid_short = commit_oid.to_string()[..7.min(commit_oid.to_string().len())].to_string();
        return Err(AppError::protocol_with(
            codes::git::APPLY_CONFLICT,
            format!(
                "conflict while applying {oid_short}: {}",
                conflicts.join(", ")
            ),
            &[("commit", oid_short), ("conflicts", conflicts.join(", "))],
        ));
    }
    Ok(())
}

fn commit_index(repo: &Repository, parents: &[&git2::Commit], message: &str) -> Result<Oid> {
    let mut index = repo.index().map_err(map_git_err)?;
    let tree_oid = index.write_tree().map_err(map_git_err)?;
    let tree = repo.find_tree(tree_oid).map_err(map_git_err)?;
    let sig = commit_signature(repo)?;
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
///
/// Refuses a dirty worktree before the first reset (both paths below move
/// HEAD with `ResetType::Hard`, which would silently discard uncommitted
/// edits). The pre-execute tip is persisted in the pause file so abort — or
/// a mid-replay conflict — can put the branch back.
pub fn execute_interactive_rebase(
    repo: &Repository,
    upstream: &str,
    todos: &[InteractiveRebaseTodo],
) -> Result<InteractiveRebaseResult> {
    let upstream_oid = resolve_upstream(repo, upstream)?;
    ensure_clean(repo)?;
    let original = repo
        .head()
        .map_err(map_git_err)?
        .target()
        .ok_or_else(|| AppError::protocol(codes::git::HEAD_UNBORN, "HEAD is unborn"))?;
    let active: Vec<&InteractiveRebaseTodo> = todos
        .iter()
        .filter(|t| t.action != InteractiveRebaseAction::Drop)
        .collect();
    // Validate before anything destructive: the replay-time check would fire
    // only after the branch was already reset onto upstream.
    if let Some(first) = active.first() {
        if first.action == InteractiveRebaseAction::Squash
            || first.action == InteractiveRebaseAction::Fixup
        {
            return Err(AppError::protocol(
                codes::git::SQUASH_FIRST_COMMIT,
                "cannot squash/fixup the first commit in the todo list",
            ));
        }
    }
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

    // Persist the pause BEFORE the destructive reset so abort (and crash
    // recovery) can restore the pre-execute tip. The Edit arm below rewrites
    // it with the updated `remaining`.
    let remaining: Vec<InteractiveRebaseTodo> = active.iter().map(|t| (*t).clone()).collect();
    write_pause(
        repo,
        &PauseState {
            upstream: upstream.to_string(),
            remaining,
            original_head: Some(original.to_string()),
        },
    )?;

    // Hard reset to upstream base, then cherry-pick each todo.
    let obj = repo.find_object(upstream_oid, None).map_err(map_git_err)?;
    repo.reset(&obj, ResetType::Hard, None)
        .map_err(map_git_err)?;

    replay_todos(repo, upstream, &active, Some(original))
}

fn replay_todos(
    repo: &Repository,
    upstream: &str,
    todos: &[&InteractiveRebaseTodo],
    original: Option<Oid>,
) -> Result<InteractiveRebaseResult> {
    let mut i = 0;
    while i < todos.len() {
        let todo = todos[i];
        let oid = Oid::from_str(&todo.oid).map_err(|e| {
            AppError::protocol_with(
                codes::git::BAD_OID,
                format!("bad oid {}: {e}", todo.oid),
                &[("oid", todo.oid.clone()), ("error", e.to_string())],
            )
        })?;
        let commit = repo.find_commit(oid).map_err(map_git_err)?;

        match todo.action {
            InteractiveRebaseAction::Drop => {
                i += 1;
            }
            InteractiveRebaseAction::Pick | InteractiveRebaseAction::Reword => {
                if let Err(e) = cherry_pick_onto_head(repo, oid) {
                    restore_original(repo, original);
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
                    restore_original(repo, original);
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
                // Always (re)write, even when `remaining` is empty: `execute`
                // pre-wrote a pause covering all todos, and leaving it behind
                // would let Continue replay already-landed commits. An empty
                // `remaining` still pauses (user may amend), and Continue
                // finishes it as a no-op replay.
                write_pause(
                    repo,
                    &PauseState {
                        upstream: upstream.to_string(),
                        remaining,
                        original_head: original.map(|o| o.to_string()),
                    },
                )?;
                return Ok(InteractiveRebaseResult {
                    kind: InteractiveRebaseKind::PausedForEdit,
                    conflicts: Vec::new(),
                    new_head: Some(new_oid.to_string()),
                });
            }
            InteractiveRebaseAction::Squash | InteractiveRebaseAction::Fixup => {
                // Squash/fixup into the previous commit: must not be first.
                if i == 0 {
                    return Err(AppError::protocol(
                        codes::git::SQUASH_FIRST_COMMIT,
                        "cannot squash/fixup the first commit in the todo list",
                    ));
                }
                if let Err(e) = cherry_pick_onto_head(repo, oid) {
                    restore_original(repo, original);
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
                let sig = commit_signature(repo)?;
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
        AppError::Protocol { message: s, .. } | AppError::Unknown { message: s, .. } => s.clone(),
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
    let raw = fs::read_to_string(&path).map_err(|_| {
        AppError::protocol(
            codes::git::NOT_PAUSED,
            "no interactive rebase paused for edit",
        )
    })?;
    let state: PauseState = serde_json::from_str(&raw).map_err(|e| {
        AppError::unknown_with(
            codes::git::PARSE_PAUSE,
            format!("parse pause state: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    // The Edit pause is where the user amends: staged changes fold into the
    // paused commit (≡ `git commit --amend`), while unstaged/untracked
    // changes would be silently mixed into the replay — refuse those.
    let (staged, unstaged, untracked) = classify_dirty(repo)?;
    if unstaged || untracked {
        return Err(AppError::protocol(
            codes::git::DIRTY_WORKTREE,
            "working copy has unstaged or untracked changes — stage them in Working Copy or stash first (staged changes are amended into the paused commit on continue)",
        ));
    }
    if staged {
        amend_head_with_index(repo)?;
    }
    let original = state
        .original_head
        .as_deref()
        .and_then(|s| Oid::from_str(s).ok());
    let refs: Vec<&InteractiveRebaseTodo> = state.remaining.iter().collect();
    replay_todos(repo, &state.upstream, &refs, original)
}

pub fn abort_interactive_rebase_pause(repo: &Repository) -> Result<()> {
    let path = pause_path(repo);
    // A true abort: put the branch back where `execute` found it instead of
    // leaving the already-replayed commits behind. Pause files written
    // before `original_head` existed fall back to the old drop-only path.
    if let Ok(raw) = fs::read_to_string(&path) {
        if let Ok(state) = serde_json::from_str::<PauseState>(&raw) {
            if let Some(orig) = state
                .original_head
                .as_deref()
                .and_then(|s| Oid::from_str(s).ok())
            {
                if let Ok(obj) = repo.find_object(orig, None) {
                    repo.reset(&obj, ResetType::Hard, None)
                        .map_err(map_git_err)?;
                }
                let _ = repo.cleanup_state();
            }
        }
    }
    let _ = fs::remove_file(path);
    Ok(())
}

pub fn interactive_rebase_paused(repo: &Repository) -> bool {
    pause_path(repo).exists()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::{
        build_linear_repo, make_commit, write_and_stage,
    };
    use std::fs;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    /// linear(3) with `base` at HEAD~2; returns (path, repo, pre-tip).
    fn edit_pause_setup() -> (std::path::PathBuf, Repository, Oid) {
        let (path, repo) = build_linear_repo(3);
        let tip = repo.head().unwrap().peel_to_commit().unwrap().id();
        let first = repo.revparse_single("HEAD~2").unwrap().id();
        repo.branch("base", &repo.find_commit(first).unwrap(), false)
            .unwrap();
        let mut todos = plan_interactive_rebase(&repo, "base").unwrap();
        assert_eq!(todos.len(), 2);
        todos[0].action = InteractiveRebaseAction::Edit;
        let res = execute_interactive_rebase(&repo, "base", &todos).unwrap();
        assert_eq!(res.kind, InteractiveRebaseKind::PausedForEdit);
        assert!(pause_path(&repo).exists());
        (path, repo, tip)
    }

    #[test]
    fn execute_refuses_dirty_worktree() {
        let (path, repo) = build_linear_repo(3);
        let tip = repo.head().unwrap().peel_to_commit().unwrap().id();
        let first = repo.revparse_single("HEAD~2").unwrap().id();
        repo.branch("base", &repo.find_commit(first).unwrap(), false)
            .unwrap();
        let todos = plan_interactive_rebase(&repo, "base").unwrap();
        fs::write(repo.workdir().unwrap().join("file0.txt"), "uncommitted\n").unwrap();
        let err = execute_interactive_rebase(&repo, "base", &todos).unwrap_err();
        assert_eq!(err.code(), codes::git::DIRTY_WORKTREE);
        assert_eq!(
            repo.head().unwrap().peel_to_commit().unwrap().id(),
            tip,
            "refused execute must not move HEAD"
        );
        assert!(
            !pause_path(&repo).exists(),
            "refused execute must not leave a pause file"
        );
        cleanup(&path);
    }

    #[test]
    fn execute_drop_all_refuses_dirty_worktree() {
        let (path, repo) = build_linear_repo(3);
        let tip = repo.head().unwrap().peel_to_commit().unwrap().id();
        let first = repo.revparse_single("HEAD~2").unwrap().id();
        repo.branch("base", &repo.find_commit(first).unwrap(), false)
            .unwrap();
        let mut todos = plan_interactive_rebase(&repo, "base").unwrap();
        for t in &mut todos {
            t.action = InteractiveRebaseAction::Drop;
        }
        fs::write(repo.workdir().unwrap().join("file0.txt"), "uncommitted\n").unwrap();
        let err = execute_interactive_rebase(&repo, "base", &todos).unwrap_err();
        assert_eq!(err.code(), codes::git::DIRTY_WORKTREE);
        assert_eq!(repo.head().unwrap().peel_to_commit().unwrap().id(), tip);
        cleanup(&path);
    }

    #[test]
    fn continue_amends_staged_edit_then_replays() {
        let (path, repo, _) = edit_pause_setup();
        // User fix during the pause, staged via Working Copy.
        fs::write(repo.workdir().unwrap().join("file1.txt"), "fixed\n").unwrap();
        {
            let mut index = repo.index().unwrap();
            index.add_path(std::path::Path::new("file1.txt")).unwrap();
            index.write().unwrap();
        }
        let res = continue_interactive_rebase(&repo).unwrap();
        assert_eq!(res.kind, InteractiveRebaseKind::Clean);
        assert!(!pause_path(&repo).exists());
        // Still exactly 2 commits over base: the fix amended the paused
        // commit instead of becoming an extra commit.
        let head = repo.head().unwrap().peel_to_commit().unwrap().id();
        let first = repo.revparse_single("base").unwrap().id();
        let mut walk = repo.revwalk().unwrap();
        walk.push(head).unwrap();
        walk.hide(first).unwrap();
        assert_eq!(walk.count(), 2);
        let tree = repo.find_commit(head).unwrap().tree().unwrap();
        let entry = tree.get_name("file1.txt").unwrap();
        let blob = repo.find_blob(entry.id()).unwrap();
        assert_eq!(blob.content(), b"fixed\n");
        cleanup(&path);
    }

    #[test]
    fn continue_clean_replays_remaining() {
        // Pause, then continue without touching anything: remaining todos
        // replay onto the paused tip.
        let (path, repo, _) = edit_pause_setup();
        let res = continue_interactive_rebase(&repo).unwrap();
        assert_eq!(res.kind, InteractiveRebaseKind::Clean);
        assert!(!pause_path(&repo).exists());
        let head = repo.head().unwrap().peel_to_commit().unwrap().id();
        let first = repo.revparse_single("base").unwrap().id();
        let mut walk = repo.revwalk().unwrap();
        walk.push(head).unwrap();
        walk.hide(first).unwrap();
        assert_eq!(walk.count(), 2);
        cleanup(&path);
    }

    #[test]
    fn continue_reads_legacy_pause_without_original_head() {
        // Pause files written before `original_head` existed must still
        // parse (serde default) and replay; only the restore step is skipped.
        let (path, repo) = build_linear_repo(3);
        let first = repo.revparse_single("HEAD~2").unwrap().id();
        repo.branch("base", &repo.find_commit(first).unwrap(), false)
            .unwrap();
        let todos = plan_interactive_rebase(&repo, "base").unwrap();
        let remaining: Vec<InteractiveRebaseTodo> = todos[1..].to_vec();
        let legacy = serde_json::json!({
            "upstream": "base",
            "remaining": remaining,
        });
        fs::write(
            pause_path(&repo),
            serde_json::to_string_pretty(&legacy).unwrap(),
        )
        .unwrap();
        let res = continue_interactive_rebase(&repo).unwrap();
        assert_eq!(res.kind, InteractiveRebaseKind::Clean);
        assert!(!pause_path(&repo).exists());
        cleanup(&path);
    }

    #[test]
    fn execute_rejects_squash_first_before_reset() {
        // The squash-first check must fire before the destructive reset, so
        // a bad plan can neither move the branch nor leave a pause behind.
        let (path, repo) = build_linear_repo(3);
        let tip = repo.head().unwrap().peel_to_commit().unwrap().id();
        let first = repo.revparse_single("HEAD~2").unwrap().id();
        repo.branch("base", &repo.find_commit(first).unwrap(), false)
            .unwrap();
        let mut todos = plan_interactive_rebase(&repo, "base").unwrap();
        todos[0].action = InteractiveRebaseAction::Squash;
        let err = execute_interactive_rebase(&repo, "base", &todos).unwrap_err();
        assert_eq!(err.code(), codes::git::SQUASH_FIRST_COMMIT);
        assert_eq!(repo.head().unwrap().peel_to_commit().unwrap().id(), tip);
        assert!(!pause_path(&repo).exists());
        cleanup(&path);
    }

    #[test]
    fn edit_last_todo_pauses_without_stale_state() {
        // Edit as the final todo: pause carries empty `remaining`, and
        // Continue finishes it instead of replaying landed commits.
        let (path, repo) = build_linear_repo(3);
        let first = repo.revparse_single("HEAD~2").unwrap().id();
        repo.branch("base", &repo.find_commit(first).unwrap(), false)
            .unwrap();
        let mut todos = plan_interactive_rebase(&repo, "base").unwrap();
        assert_eq!(todos.len(), 2);
        todos[1].action = InteractiveRebaseAction::Edit;
        let res = execute_interactive_rebase(&repo, "base", &todos).unwrap();
        assert_eq!(res.kind, InteractiveRebaseKind::PausedForEdit);
        let paused_tip = repo.head().unwrap().peel_to_commit().unwrap().id();
        let res = continue_interactive_rebase(&repo).unwrap();
        assert_eq!(res.kind, InteractiveRebaseKind::Clean);
        assert_eq!(
            repo.head().unwrap().peel_to_commit().unwrap().id(),
            paused_tip,
            "empty-remaining continue must not replay anything"
        );
        assert!(!pause_path(&repo).exists());
        cleanup(&path);
    }

    #[test]
    fn continue_refuses_unstaged_changes() {
        let (path, repo, _) = edit_pause_setup();
        fs::write(repo.workdir().unwrap().join("file1.txt"), "unstaged\n").unwrap();
        let err = continue_interactive_rebase(&repo).unwrap_err();
        assert_eq!(err.code(), codes::git::DIRTY_WORKTREE);
        assert!(
            pause_path(&repo).exists(),
            "refused continue must keep the pause"
        );
        cleanup(&path);
    }

    #[test]
    fn continue_refuses_untracked_file() {
        let (path, repo, _) = edit_pause_setup();
        fs::write(repo.workdir().unwrap().join("scratch.txt"), "new\n").unwrap();
        let err = continue_interactive_rebase(&repo).unwrap_err();
        assert_eq!(err.code(), codes::git::DIRTY_WORKTREE);
        cleanup(&path);
    }

    #[test]
    fn abort_restores_original_head() {
        let (path, repo, tip) = edit_pause_setup();
        // Simulate user edits during the pause — none may survive abort.
        fs::write(repo.workdir().unwrap().join("file1.txt"), "doomed\n").unwrap();
        abort_interactive_rebase_pause(&repo).unwrap();
        assert_eq!(
            repo.head().unwrap().peel_to_commit().unwrap().id(),
            tip,
            "abort must put the branch back where execute found it"
        );
        assert_eq!(
            fs::read_to_string(repo.workdir().unwrap().join("file1.txt")).unwrap(),
            "v1\n"
        );
        assert!(!pause_path(&repo).exists());
        cleanup(&path);
    }

    #[test]
    fn execute_conflict_restores_original_head() {
        let sig = git2::Signature::now("Test", "test@local").unwrap();
        let (path, repo) = build_linear_repo(1);
        let base = repo.head().unwrap().peel_to_commit().unwrap().id();
        // Upstream moves file0 one way…
        repo.branch("upstream", &repo.find_commit(base).unwrap(), false)
            .unwrap();
        repo.set_head("refs/heads/upstream").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
            .unwrap();
        let tree = write_and_stage(&repo, "file0.txt", "upstream\n");
        make_commit(&repo, &sig, "upstream edit", tree, &[base]);
        // …main moves it the other way.
        repo.set_head("refs/heads/main").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
            .unwrap();
        let tree = write_and_stage(&repo, "file0.txt", "main\n");
        let tip = make_commit(&repo, &sig, "main edit", tree, &[base]);

        let todos = plan_interactive_rebase(&repo, "upstream").unwrap();
        assert_eq!(todos.len(), 1);
        let res = execute_interactive_rebase(&repo, "upstream", &todos).unwrap();
        assert_eq!(res.kind, InteractiveRebaseKind::Conflicts);
        assert_eq!(
            repo.head().unwrap().peel_to_commit().unwrap().id(),
            tip,
            "conflict must restore the pre-execute tip, not a half-rebased branch"
        );
        assert!(
            !pause_path(&repo).exists(),
            "conflict restore must drop the pause file"
        );
        assert_eq!(
            fs::read_to_string(repo.workdir().unwrap().join("file0.txt")).unwrap(),
            "main\n"
        );
        cleanup(&path);
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
