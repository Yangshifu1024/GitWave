//! Application-wide state: where the state lives, and the gitwave-core
//! context every view shares.

use std::path::{Path, PathBuf};
use std::time::SystemTime;

use gitwave_core::application::use_cases::{
    AppContext, get_commit_page, get_shell_settings, get_working_copy, list_repos, list_workspaces,
    set_shell_settings,
};
use gitwave_core::bootstrap::open_app_context_in;
use gitwave_core::domain::ShellSettings;
use gitwave_core::domain::error::AppError;
use gitwave_core::domain::history::CommitPage;
use gitwave_core::domain::working_copy::WorkingCopy;
use gitwave_core::domain::workspace::{RepoRef, WorkspaceSummary};
use gitwave_core::infrastructure::persistence::snapshot;
use gpui_kit::{App, Global, SharedString, Task};
use rust_i18n::t;

use crate::{runtime, time_text};

/// The directory of the development build's own, beside the released
/// application's: the copy of the state, and this build's logs under `logs`.
const DEV_DIR_NAME: &str = "GitWave-dev";

/// Commits come a page at a time, from the newest.
pub(crate) const PAGE_SIZE: u32 = 300;

struct AppState {
    context: AppContext,
    origin: Origin,
}

impl Global for AppState {}

/// Where the state the application shows comes from.
#[derive(Debug, Clone)]
pub enum Origin {
    /// A copy of the released application's state. The time is `None` when no
    /// copy was ever taken, as there was no original to copy. `failure` is
    /// why the copy could not be brought up to date: what was there before is
    /// shown, or an empty state if there was nothing.
    Copy {
        taken_at: Option<SystemTime>,
        failure: Option<AppError>,
    },
    /// The stores could not be opened; nothing is saved.
    Memory,
}

/// The state as opened, with where it comes from.
pub struct Opened {
    pub context: AppContext,
    pub origin: Origin,
}

/// Opens the state.
///
/// This version does not open the released application's database. It shows
/// a copy, in a directory of its own, taken again whenever the original
/// changes. Opening the original would run this version's migrations on it
/// and leave the released version with a database it does not understand.
///
/// In the tests it refuses: a test that came here would reach the user's
/// state. Tests start from [`init_in_memory`].
pub fn init(cx: &mut App) {
    #[cfg(test)]
    {
        let _ = cx;
        panic!(
            "app_state::init opens the development state; a test starts from app_state::init_in_memory"
        );
    }
    #[cfg(not(test))]
    {
        let opened = open_development(development_original());
        cx.set_global(AppState {
            context: opened.context,
            origin: opened.origin,
        });
    }
}

/// Opens the stores in memory only: nothing on disk is read or written.
#[cfg(test)]
pub fn init_in_memory(cx: &mut App) {
    let context = AppContext::in_memory().expect("failed to open the in-memory state");
    cx.set_global(AppState {
        context,
        origin: Origin::Memory,
    });
}

/// Where the released application's state is, when the user's data directory
/// can be found. Nothing is created.
#[cfg(not(test))]
fn development_original() -> Option<PathBuf> {
    gitwave_core::infrastructure::persistence::sqlite::state_dir_path().ok()
}

/// The directory of the development build's own, when the user's data
/// directory can be found. Nothing is created.
pub fn development_dir() -> Option<PathBuf> {
    gitwave_core::infrastructure::persistence::sqlite::state_dir_path()
        .ok()
        .map(|original| original.with_file_name(DEV_DIR_NAME))
}

/// The state of the development version: a copy of the original's when there
/// is an original, in memory otherwise.
pub fn open_development(original: Option<PathBuf>) -> Opened {
    match original {
        Some(original) => open_copy(&original, &original.with_file_name(DEV_DIR_NAME)),
        None => Opened {
            context: AppContext::in_memory().expect("failed to open the in-memory state"),
            origin: Origin::Memory,
        },
    }
}

/// Brings the copy in `dir` up to date with the original in `original_dir`
/// and opens it. A copy that could not be brought up to date is shown as it
/// is, and the reason is told, so that nobody takes a stale or empty copy for
/// the released application's state.
pub fn open_copy(original_dir: &Path, dir: &Path) -> Opened {
    let failure = snapshot::refresh(original_dir, dir).err();
    if let Some(error) = &failure {
        tracing::error!(
            code = error.code(),
            "the copy of the state was not brought up to date, it is shown as it is: {error}"
        );
    }
    Opened {
        context: open_app_context_in(dir),
        origin: Origin::Copy {
            taken_at: snapshot::taken_at(dir),
            failure,
        },
    }
}

/// The context gitwave-core's use cases run with.
pub fn context(cx: &App) -> AppContext {
    cx.global::<AppState>().context.clone()
}

pub fn origin(cx: &App) -> Origin {
    cx.global::<AppState>().origin.clone()
}

/// Why the copy of the state could not be brought up to date with the
/// original, when it could not.
pub fn origin_failure(cx: &App) -> Option<AppError> {
    match &cx.global::<AppState>().origin {
        Origin::Copy { failure, .. } => failure.clone(),
        Origin::Memory => None,
    }
}

/// What the status bar says about where the state comes from.
pub fn origin_label(origin: &Origin, now: SystemTime) -> SharedString {
    match origin {
        Origin::Memory => t!("app.workbench.state.memory").to_string(),
        Origin::Copy {
            failure: Some(_), ..
        } => t!("app.workbench.state.copyStale").to_string(),
        Origin::Copy { taken_at: None, .. } => t!("app.workbench.state.copy").to_string(),
        Origin::Copy {
            taken_at: Some(taken_at),
            ..
        } => {
            let seconds = taken_at
                .duration_since(SystemTime::UNIX_EPOCH)
                .map_or(0, |since| since.as_secs() as i64);
            t!(
                "app.workbench.state.copyTaken",
                ago = time_text::ago(now, seconds)
            )
            .to_string()
        }
    }
    .into()
}

/// The workspaces, read off the main thread.
pub fn workspaces(cx: &App) -> Task<Result<Vec<WorkspaceSummary>, AppError>> {
    let context = context(cx);
    runtime::use_case(cx, move || list_workspaces(&context))
}

/// The repositories of a workspace, read off the main thread.
pub fn repos(cx: &App, workspace_id: String) -> Task<Result<Vec<RepoRef>, AppError>> {
    let context = context(cx);
    runtime::use_case(cx, move || list_repos(&context, workspace_id))
}

/// One page of a repository's history, read off the main thread. The cursor
/// is the one the page before it ended with; without one, the page starts at
/// the newest commit.
pub fn commit_page(
    cx: &App,
    workspace_id: String,
    repo_id: String,
    cursor: Option<String>,
) -> Task<Result<CommitPage, AppError>> {
    let context = context(cx);
    runtime::use_case(cx, move || {
        get_commit_page(
            &context,
            &workspace_id,
            &repo_id,
            PAGE_SIZE,
            None,
            cursor.as_deref(),
        )
    })
}

/// The head of the workspace's active repository, read off the main thread.
pub fn working_copy(cx: &App, workspace_id: String) -> Task<Result<WorkingCopy, AppError>> {
    let context = context(cx);
    runtime::use_case(cx, move || get_working_copy(&context, &workspace_id))
}

/// The workspace to show when the application starts, read off the main thread.
pub fn shell_settings(cx: &App) -> Task<Result<ShellSettings, AppError>> {
    let context = context(cx);
    runtime::use_case(cx, move || get_shell_settings(&context))
}

/// Stores which workspace was shown last. One small local write, so the
/// interface waits for it; when it fails, the choice is not remembered and
/// that is logged.
pub fn remember_workspace(cx: &App, workspace_id: Option<&str>) {
    let settings = ShellSettings {
        last_workspace_id: workspace_id.map(str::to_string),
    };
    if let Err(error) = set_shell_settings(&context(cx), settings) {
        tracing::warn!(
            code = error.code(),
            "the workspace shown was not remembered: {error}"
        );
    }
}

/// Makes a repository the workspace's active one, for the use cases that read
/// the active repository. One small local write, so the interface waits for
/// it; when it fails, that is logged.
pub fn remember_active_repo(cx: &App, workspace_id: &str, repo_id: &str) {
    if let Err(error) = gitwave_core::application::use_cases::set_active_repo(
        &context(cx),
        workspace_id.to_string(),
        Some(repo_id.to_string()),
    ) {
        tracing::warn!(
            code = error.code(),
            "the active repository was not stored: {error}"
        );
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;
    use gitwave_core::application::use_cases::create_workspace;
    use gitwave_core::domain::error_codes as codes;

    /// A directory of the test's own, under the system's temporary directory,
    /// removed when its `TempDir` is dropped.
    fn temp_dir() -> tempfile::TempDir {
        tempfile::tempdir().expect("a directory of the test's own")
    }

    fn names(ctx: &AppContext) -> Vec<String> {
        list_workspaces(ctx)
            .expect("list the workspaces")
            .into_iter()
            .map(|workspace| workspace.name)
            .collect()
    }

    fn english() -> impl Drop {
        let _turn = crate::locales::lock_language();
        rust_i18n::set_locale("en");
        _turn
    }

    #[test]
    fn the_development_state_is_a_copy_beside_the_original() {
        let tmp = temp_dir();
        let tmp_path = tmp.path().to_path_buf();
        let original = tmp_path.join("GitWave");
        let released = open_app_context_in(&original);
        create_workspace(&released, "Mine".to_string()).expect("create a workspace");

        let opened = open_development(Some(original.clone()));

        assert!(
            matches!(
                opened.origin,
                Origin::Copy {
                    taken_at: Some(_),
                    failure: None
                }
            ),
            "a copy was taken, and nothing went wrong"
        );
        assert_eq!(
            names(&opened.context),
            ["Mine"],
            "the copy shows the original's workspaces"
        );
        assert!(
            tmp_path.join(DEV_DIR_NAME).join("state.db").exists(),
            "the copy has a directory of its own"
        );

        set_shell_settings(
            &opened.context,
            ShellSettings {
                last_workspace_id: Some("ws".to_string()),
            },
        )
        .expect("store something in the copy");
        assert_eq!(
            get_shell_settings(&released).expect("read the original"),
            ShellSettings::default(),
            "what the copy stores stays in the copy"
        );
    }

    #[test]
    fn without_a_place_for_the_original_the_state_is_in_memory() {
        let opened = open_development(None);

        assert!(
            matches!(opened.origin, Origin::Memory),
            "{:?}",
            opened.origin
        );
        assert!(names(&opened.context).is_empty());
    }

    #[test]
    fn the_label_names_where_the_state_comes_from() {
        let _english = english();
        let now = SystemTime::UNIX_EPOCH + Duration::from_secs(10_000);

        assert_eq!(
            origin_label(&Origin::Memory, now),
            "In-memory state, nothing is saved"
        );
        let copy = |taken_at, failure: Option<AppError>| {
            origin_label(&Origin::Copy { taken_at, failure }, now)
        };
        assert_eq!(copy(None, None), "A copy of the released app's state");
        assert_eq!(
            copy(Some(now - Duration::from_secs(120)), None),
            "A copy of the released app's state, taken 2m ago"
        );
        let broken = AppError::unknown(codes::infra::STATE_SNAPSHOT_FAILED, "broken");
        assert_eq!(
            copy(Some(now), Some(broken)),
            "A copy of the released app's state, not brought up to date"
        );
    }

    /// The original cannot be copied: what was copied before is shown, or an
    /// empty state when there was nothing, and the reason is kept to be told.
    #[test]
    fn a_copy_that_could_not_be_brought_up_to_date_carries_the_reason() {
        let tmp = temp_dir();
        let tmp_path = tmp.path().to_path_buf();
        let original = tmp_path.join("GitWave");
        let released = open_app_context_in(&original);
        create_workspace(&released, "One".to_string()).expect("create a workspace");
        drop(released);
        std::fs::write(original.join("state.db"), b"not a database at all")
            .expect("break the original");

        let opened = open_development(Some(original.clone()));

        let Origin::Copy {
            taken_at: None,
            failure: Some(failure),
        } = &opened.origin
        else {
            panic!("the failure is kept: {:?}", opened.origin)
        };
        assert_eq!(failure.code(), codes::infra::STATE_SNAPSHOT_FAILED);
        assert!(
            names(&opened.context).is_empty(),
            "an empty state is shown, not the released application's"
        );
    }
}
