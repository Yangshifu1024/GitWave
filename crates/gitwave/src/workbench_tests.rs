//! Checks of the window shell. They render the real workbench in a test window
//! and drive it the way a user would.

use std::path::{Path, PathBuf};
use std::time::{Duration, Instant};

use gpui_kit::component::menu::AppMenuBar;
use gpui_kit::component::{ActiveTheme as _, GlobalState, ThemeMode};
use gpui_kit::test::TestWindowExt as _;
use gpui_kit::{
    AnyWindowHandle, App, AppContext as _, Background, Entity, MenuItem, OwnedMenu, OwnedMenuItem,
    Role, TestAppContext, WindowOptions, px, size, solid_background,
};

use gitwave_core::application::use_cases::{
    add_local_repo, create_workspace, get_commit_page, get_language_settings, get_shell_settings,
    get_workspace, list_repos,
};
use gitwave_core::domain::error::AppError;
use gitwave_core::domain::error_codes as codes;
use gitwave_core::domain::working_copy::WorkingCopy;
use gitwave_core::domain::workspace::RepoRef;

use super::Workbench;
use crate::{
    app_state, error_text,
    locales::{self, LanguageChoice},
    menus::{self, SwitchLanguage},
    runtime, workbench,
};

/// Starts the application the way `main` does, with nothing in its state. The
/// system reports no language, so the interface starts in English whatever the
/// machine running the tests prefers. A test puts its own state in before the
/// window opens.
fn prepare(cx: &mut TestAppContext) -> Entity<AppMenuBar> {
    cx.update(|cx| {
        gpui_kit::init(cx);
        runtime::init(cx);
        app_state::init_in_memory(cx);
        locales::init(&app_state::context(cx), Vec::new(), cx);
        workbench::init(cx);
        menus::init(cx)
    })
}

/// Opens the workbench the way `main` does, with a test window in place of the
/// real one, on an empty state.
fn open(cx: &mut TestAppContext) -> (AnyWindowHandle, Entity<Workbench>) {
    show(prepare(cx), cx)
}

/// Opens the workbench in a test window.
fn show(
    app_menu_bar: Entity<AppMenuBar>,
    cx: &mut TestAppContext,
) -> (AnyWindowHandle, Entity<Workbench>) {
    cx.update(|cx| {
        let options = WindowOptions {
            window_bounds: Some(gpui_kit::WindowBounds::centered(
                None,
                size(px(1200.), px(800.)),
                cx,
            )),
            ..Default::default()
        };
        gpui_kit::open_window(options, cx, |window, cx| {
            cx.new(|cx| Workbench::new(app_menu_bar, window, cx))
        })
        .expect("open the test window")
    })
}

/// What gitwave-core has stored for the language.
fn stored_language(cx: &mut TestAppContext) -> Option<String> {
    cx.read(|cx| {
        get_language_settings(&app_state::context(cx))
            .expect("read the stored language")
            .locale
    })
}

/// Waits until `ready` holds. gitwave-core answers on tokio threads, which the
/// test scheduler does not control, so the thread has to be let wait.
fn wait_for(cx: &mut TestAppContext, what: &str, ready: impl Fn(&mut TestAppContext) -> bool) {
    cx.executor().allow_parking();
    let deadline = Instant::now() + Duration::from_secs(20);
    loop {
        cx.executor().run_until_parked();
        if ready(cx) {
            return;
        }
        assert!(Instant::now() < deadline, "timed out waiting for {what}");
        std::thread::sleep(Duration::from_millis(5));
    }
}

/// A directory of the test's own, under the system's temporary directory,
/// removed when its `TempDir` is dropped.
fn temp_dir() -> tempfile::TempDir {
    tempfile::tempdir().expect("a directory of the test's own")
}

/// A repository of the test's own.
struct Repo {
    /// Keeps the directory, and so the repository, there for as long as the test needs it.
    _dir: tempfile::TempDir,
    path: PathBuf,
    branch: String,
    /// The hashes of its commits, the newest first.
    commits: Vec<String>,
}

impl Repo {
    /// What the status bar says for it: the name of its directory, then its
    /// branch.
    fn identity(&self) -> String {
        let name = self.path.file_name().expect("a directory name");
        format!("{} · {}", name.to_string_lossy(), self.branch)
    }
}

/// A repository of the test's own, with one commit per message, in order. The
/// author's clock moves a minute per commit, so that no two share a time.
fn git_repo(name: &str, messages: &[&str]) -> Repo {
    let dir = temp_dir();
    let path = dir.path().join(name);
    let repository = git2::Repository::init(&path).expect("init a repository");
    // Whatever the machine's Git configuration names a first branch.
    repository
        .set_head("refs/heads/main")
        .expect("name the branch");
    let mut commits = Vec::new();
    for (number, message) in messages.iter().enumerate() {
        std::fs::write(path.join("file.txt"), format!("version {number}\n")).expect("write a file");
        let mut staging = repository.index().expect("the index");
        staging
            .add_path(std::path::Path::new("file.txt"))
            .expect("stage the file");
        staging.write().expect("write the index");
        let tree = repository
            .find_tree(staging.write_tree().expect("write a tree"))
            .expect("the tree");
        let who = git2::Signature::new(
            "Ada Lovelace",
            "ada@example.com",
            &git2::Time::new(1_700_000_000 + number as i64 * 60, 0),
        )
        .expect("a signature");
        let head = repository
            .head()
            .ok()
            .and_then(|head| head.peel_to_commit().ok());
        let parents: Vec<&git2::Commit> = head.iter().collect();
        let id = repository
            .commit(Some("HEAD"), &who, &who, message, &tree, &parents)
            .expect("a commit");
        commits.push(id.to_string());
    }
    commits.reverse();
    let branch = repository
        .head()
        .expect("a branch")
        .shorthand()
        .expect("a short name")
        .to_string();
    Repo {
        _dir: dir,
        path,
        branch,
        commits,
    }
}

/// A workspace in the state, holding these repositories.
fn workspace(cx: &mut TestAppContext, name: &str, repos: &[&Path]) -> String {
    cx.update(|cx| {
        let context = app_state::context(cx);
        let workspace = create_workspace(&context, name.to_string()).expect("create a workspace");
        for path in repos {
            add_local_repo(
                &context,
                workspace.id.clone(),
                path.to_string_lossy().into_owned(),
            )
            .expect("add a repository");
        }
        workspace.id
    })
}

/// The repositories of a workspace as they are stored.
fn repos_of(cx: &mut TestAppContext, workspace_id: &str) -> Vec<RepoRef> {
    cx.update(|cx| {
        list_repos(&app_state::context(cx), workspace_id.to_string())
            .expect("list the repositories")
    })
}

/// The hashes on screen in the history, the newest first.
fn shown(cx: &mut TestAppContext, workbench: &Entity<Workbench>) -> Vec<String> {
    cx.read(|cx| {
        workbench
            .read(cx)
            .history
            .read(cx)
            .commits()
            .iter()
            .map(|commit| commit.sha.clone())
            .collect()
    })
}

/// Whether an element is in the last frame, and what it says. Unlike `says`,
/// an element that is not there is `None`, so this can be polled.
fn shown_now(cx: &mut TestAppContext, window: AnyWindowHandle, id: &'static str) -> Option<String> {
    cx.update_window(window, |_, window, cx| {
        window.render_frame(cx);
        window
            .try_find(id)
            .and_then(|found| found.label().map(str::to_string))
    })
    .unwrap()
}

/// Waits until the shell has read the repositories of the workspace on screen.
fn repositories_read(cx: &mut TestAppContext, workbench: &Entity<Workbench>) {
    wait_for(cx, "the repositories", |cx| {
        cx.read(|cx| {
            workbench
                .read(cx)
                .session
                .selected()
                .is_some_and(|selected| selected.repos.is_some())
        })
    });
}

/// What the element with that id says, in a freshly drawn frame.
fn says(cx: &mut TestAppContext, window: AnyWindowHandle, id: &'static str) -> Option<String> {
    cx.update_window(window, |_, window, cx| {
        window.render_frame(cx);
        window.find(id).label().map(str::to_string)
    })
    .unwrap()
}

// Every test below reads English, so each holds the turn on the process-wide
// language for its whole run: `open` sets the language, and another test
// switching it in between would change the labels.

/// The shell shows a workspace, a tab per repository, and the history of the
/// one shown before; the status bar names what is shown and where the state
/// comes from.
#[gpui_kit::test]
fn the_shell_shows_the_workspace_its_repositories_and_the_history_of_the_active_one(
    cx: &mut TestAppContext,
) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first", "second"]);
    let beta = git_repo("beta", &["other"]);
    let bar = prepare(cx);
    let id = workspace(cx, "Projects", &[&alpha.path, &beta.path]);
    let (window, workbench) = show(bar, cx);

    wait_for(cx, "the history of the first repository", |cx| {
        shown(cx, &workbench) == alpha.commits
    });

    cx.update_window(window, |_, window, cx| {
        window.render_frame(cx);
        let tabs = window.within("repo-tabs");
        assert_eq!(tabs.find(0).label(), Some("alpha"), "a tab per repository");
        assert_eq!(tabs.find(1).label(), Some("beta"));
        assert_eq!(
            tabs.find(0).selected().or_else(|| tabs.find(0).checked()),
            Some(true),
            "the first one is the active one"
        );
    })
    .unwrap();
    assert_eq!(
        says(cx, window, "status-identity"),
        Some(alpha.identity()),
        "the status bar names the repository and its branch"
    );
    assert_eq!(
        says(cx, window, "status-origin").as_deref(),
        Some("In-memory state, nothing is saved"),
        "and says where the state comes from"
    );
    cx.read(|cx| {
        assert_eq!(
            workbench
                .read(cx)
                .switcher
                .read(cx)
                .selected_value()
                .map(String::as_str),
            Some(id.as_str()),
            "the switcher has the workspace selected: it tells workspaces apart by id, as two can have the same name"
        );
    });
}

/// The tab of a repository shows that repository's history.
#[gpui_kit::test]
fn choosing_a_tab_shows_that_repositorys_history(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first"]);
    let beta = git_repo("beta", &["other", "another"]);
    let bar = prepare(cx);
    workspace(cx, "Projects", &[&alpha.path, &beta.path]);
    let (window, workbench) = show(bar, cx);
    wait_for(cx, "the history of the first repository", |cx| {
        shown(cx, &workbench) == alpha.commits
    });

    cx.update_window(window, |_, window, cx| {
        window.within("repo-tabs").click(1, cx)
    })
    .unwrap();
    wait_for(cx, "the history of the second repository", |cx| {
        shown(cx, &workbench) == beta.commits
    });

    assert_eq!(says(cx, window, "status-identity"), Some(beta.identity()));
}

/// A repository whose path is gone is marked on its tab and is not shown.
#[gpui_kit::test]
fn a_repository_whose_path_is_missing_is_not_shown(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first"]);
    let gone = git_repo("gone", &["elsewhere"]);
    let bar = prepare(cx);
    let id = workspace(cx, "Projects", &[&gone.path, &alpha.path]);
    std::fs::remove_dir_all(&gone.path).expect("the repository disappears");
    let (window, workbench) = show(bar, cx);
    repositories_read(cx, &workbench);

    cx.update_window(window, |_, window, cx| {
        window.render_frame(cx);
        let tabs = window.within("repo-tabs");
        assert_eq!(
            tabs.find(0).disabled(),
            Some(true),
            "the missing one is marked"
        );
    })
    .unwrap();
    wait_for(cx, "the history of the one that is still there", |cx| {
        shown(cx, &workbench) == alpha.commits
    });
    // Its tab being disabled is one guard. Asked directly, it is still not
    // activated, and the history on screen is left alone.
    workbench.update(cx, |this, cx| this.repo_chosen(0, cx));
    cx.executor().run_until_parked();
    assert_eq!(shown(cx, &workbench), alpha.commits);
    let stored = cx.update(|cx| {
        get_workspace(&app_state::context(cx), id.clone())
            .expect("read the workspace")
            .last_active_repo_id
    });
    let alpha_id = repos_of(cx, &id)
        .into_iter()
        .find(|repo| repo.path == alpha.path.to_string_lossy())
        .map(|repo| repo.id);
    assert_eq!(stored, alpha_id, "the repository shown is the stored one");
}

/// When no repository can be shown, the shell says so instead of a history.
#[gpui_kit::test]
fn when_no_repository_can_be_shown_the_shell_says_so(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let gone = git_repo("gone", &["elsewhere"]);
    let bar = prepare(cx);
    workspace(cx, "Projects", &[&gone.path]);
    std::fs::remove_dir_all(&gone.path).expect("the repository disappears");
    let (window, workbench) = show(bar, cx);
    repositories_read(cx, &workbench);

    assert_eq!(
        says(cx, window, "empty-active").as_deref(),
        Some("No repository can be shown: its path is missing."),
        "nothing is shown as active"
    );
}

/// Another workspace shows its own repositories. Repositories read for the
/// one before arrive after it: they are dropped.
#[gpui_kit::test]
fn another_workspace_shows_its_repositories_and_drops_what_arrives_late(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first"]);
    let beta = git_repo("beta", &["other"]);
    let bar = prepare(cx);
    let first = workspace(cx, "First", &[&alpha.path]);
    let second = workspace(cx, "Second", &[&beta.path]);
    let (window, workbench) = show(bar, cx);
    wait_for(cx, "the history of the first repository", |cx| {
        shown(cx, &workbench) == alpha.commits
    });
    let of_the_first = repos_of(cx, &first);

    workbench.update(cx, |this, cx| {
        this.show_workspace(second.clone(), cx);
        // What the first workspace's repositories would have been, arriving now.
        this.repos_arrived(this.generation - 1, first.clone(), Ok(of_the_first), cx);
    });
    wait_for(
        cx,
        "the history of the second workspace's repository",
        |cx| shown(cx, &workbench) == beta.commits,
    );

    cx.update_window(window, |_, window, cx| {
        window.render_frame(cx);
        assert_eq!(
            window.within("repo-tabs").find(0).label(),
            Some("beta"),
            "only the second workspace's repository is a tab"
        );
    })
    .unwrap();
    // What the switcher shows as selected is its own business: it changes
    // that before it tells the shell. What the shell keeps is checked here.
    cx.read(|cx| {
        assert_eq!(
            workbench.read(cx).session.selected_id(),
            Some(second.as_str()),
            "the second workspace is the one shown"
        );
        assert_eq!(
            get_shell_settings(&app_state::context(cx))
                .expect("read the shell settings")
                .last_workspace_id
                .as_deref(),
            Some(second.as_str()),
            "the workspace shown is the one to show next time"
        );
    });
}

/// A workspace asked for twice before anything was answered, as when the
/// user goes away and straight back: the answer to the first visit arrives
/// late, and is dropped.
#[gpui_kit::test]
fn a_late_answer_for_an_earlier_visit_to_the_same_workspace_is_dropped(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first"]);
    let bar = prepare(cx);
    let first = workspace(cx, "First", &[&alpha.path]);
    let second = workspace(cx, "Second", &[]);
    let (window, workbench) = show(bar, cx);
    wait_for(cx, "the shell to have read the workspaces", |cx| {
        cx.read(|cx| workbench.read(cx).session.selected_id() == Some(first.as_str()))
    });

    let after_the_late_answer = workbench.update(cx, |this, cx| {
        // The visit that is out: the repositories of the first workspace
        // were asked for when it was first shown.
        let first_visit = this.generation;
        this.show_workspace(second.clone(), cx);
        this.show_workspace(first.clone(), cx);
        // The answer to the first visit: nothing, as it happens.
        this.repos_arrived(first_visit, first.clone(), Ok(Vec::new()), cx);
        this.session
            .selected()
            .and_then(|selected| selected.repos.as_ref().map(Vec::len))
    });

    assert_eq!(
        after_the_late_answer, None,
        "the answer to the visit before is dropped, the repositories are still awaited"
    );
    wait_for(cx, "the history after coming back", |cx| {
        shown(cx, &workbench) == alpha.commits
    });
    cx.update_window(window, |_, window, cx| {
        window.render_frame(cx);
        assert_eq!(
            window.within("repo-tabs").find(0).label(),
            Some("alpha"),
            "from the answer to the visit that counts"
        );
    })
    .unwrap();
}

/// The history reads the page after the last one near its end, and a page
/// arriving for the repository shown before is dropped.
#[gpui_kit::test]
fn the_history_reads_on_near_its_end_and_drops_a_page_for_the_repository_before(
    cx: &mut TestAppContext,
) {
    locales::take_language_turn(cx);
    let messages: Vec<String> = (0..app_state::PAGE_SIZE as usize + 50)
        .map(|number| format!("commit {number}"))
        .collect();
    let messages: Vec<&str> = messages.iter().map(String::as_str).collect();
    let big = git_repo("big", &messages);
    let small = git_repo("small", &["only"]);
    let bar = prepare(cx);
    let id = workspace(cx, "Projects", &[&big.path, &small.path]);
    let (window, workbench) = show(bar, cx);
    wait_for(cx, "the first page", |cx| {
        shown(cx, &workbench).len() == app_state::PAGE_SIZE as usize
    });

    // The list is scrolled to its end, and asks for the page after the last
    // one itself.
    workbench.update(cx, |this, cx| {
        this.history
            .update(cx, |history, cx| history.scroll_to_end(cx))
    });
    render(cx, window);
    wait_for(cx, "the rest of the history", |cx| {
        shown(cx, &workbench) == big.commits
    });
    assert_eq!(says(cx, window, "status-identity"), Some(big.identity()));

    cx.update_window(window, |_, window, cx| {
        window.within("repo-tabs").click(1, cx)
    })
    .unwrap();
    wait_for(cx, "the other repository's history", |cx| {
        shown(cx, &workbench) == small.commits
    });
    let big = repos_of(cx, &id).remove(0);
    let late = cx.update(|cx| {
        get_commit_page(&app_state::context(cx), &id, &big.id, 10, None, None)
            .expect("a page of the big repository")
    });
    workbench.update(cx, |this, cx| {
        let before = this.history.read(cx).generation() - 1;
        this.history.update(cx, |history, cx| {
            history.page_arrived_for_test(before, Ok(late), cx)
        });
    });

    assert_eq!(
        shown(cx, &workbench),
        small.commits,
        "a page for the repository before changes nothing"
    );
}

/// A head as the shell reads it, for a repository.
fn head_of(repo: &Repo, id: &str) -> WorkingCopy {
    WorkingCopy {
        repo_id: id.to_string(),
        branch: repo.branch.clone(),
        upstream: None,
        sha: repo.commits[0].clone(),
        head_message: Some("the subject".to_string()),
        ahead: 0,
        behind: 0,
        files: vec![],
    }
}

/// The id of a workspace's repository that is where `path` is.
fn repo_id(cx: &mut TestAppContext, workspace_id: &str, path: &Path) -> String {
    repos_of(cx, workspace_id)
        .into_iter()
        .find(|repo| Path::new(&repo.path) == path)
        .expect("the repository is in the workspace")
        .id
}

/// What gitwave-core has stored as the active repository of a workspace.
fn stored_active(cx: &mut TestAppContext, workspace_id: &str) -> Option<String> {
    cx.update(|cx| {
        get_workspace(&app_state::context(cx), workspace_id.to_string())
            .expect("read the workspace")
            .last_active_repo_id
    })
}

/// Waits until the history on screen is the one given and the head behind it
/// was read: the status bar names the branch.
fn settled_on(
    cx: &mut TestAppContext,
    window: AnyWindowHandle,
    workbench: &Entity<Workbench>,
    repo: &Repo,
) {
    wait_for(cx, "the history and its head", |cx| {
        shown(cx, workbench) == repo.commits
            && says(cx, window, "status-identity").as_deref() == Some(repo.identity().as_str())
    });
}

/// The head of the repository shown marks its commit.
#[gpui_kit::test]
fn the_head_of_the_repository_marks_its_commit(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first", "second"]);
    let bar = prepare(cx);
    workspace(cx, "Projects", &[&alpha.path]);
    let (window, workbench) = show(bar, cx);
    settled_on(cx, window, &workbench, &alpha);

    cx.update_window(window, |_, window, cx| {
        window.render_frame(cx);
        assert!(
            window.find("history-head").visible(),
            "the commit the head is at is marked"
        );
    })
    .unwrap();
}

/// Another repository is chosen before the head of the one before was read:
/// that head arrives late, and is dropped. The same repository chosen again
/// after another one is shown is another repository, as far as its head goes.
#[gpui_kit::test]
fn a_head_that_arrives_for_the_repository_shown_before_is_dropped(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first"]);
    let beta = git_repo("beta", &["other"]);
    let bar = prepare(cx);
    let id = workspace(cx, "Projects", &[&alpha.path, &beta.path]);
    let alpha_id = repo_id(cx, &id, &alpha.path);
    let (window, workbench) = show(bar, cx);
    settled_on(cx, window, &workbench, &alpha);

    cx.update_window(window, |_, window, cx| {
        window.within("repo-tabs").click(1, cx)
    })
    .unwrap();
    let stale = workbench.update(cx, |this, _| this.head_generation() - 1);
    // The head of the repository before, arriving after another was asked for.
    workbench.update(cx, |this, cx| {
        this.head_arrived_for_test(stale, Ok(head_of(&alpha, &alpha_id)), cx)
    });
    assert_eq!(
        says(cx, window, "status-identity").as_deref(),
        Some("beta"),
        "it is not taken: no branch is named until the head read is the one asked for"
    );

    settled_on(cx, window, &workbench, &beta);
    workbench.update(cx, |this, cx| {
        this.head_arrived_for_test(stale, Ok(head_of(&alpha, &alpha_id)), cx)
    });
    assert_eq!(
        says(cx, window, "status-identity"),
        Some(beta.identity()),
        "and it is not taken once the other's was read either"
    );
}

/// The cursor went stale under the list: what is shown stays while the
/// history is read again, which is said of; then what was read is shown.
#[gpui_kit::test]
fn a_cursor_that_went_stale_is_read_again_while_what_was_shown_stays(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first", "second"]);
    let bar = prepare(cx);
    workspace(cx, "Projects", &[&alpha.path]);
    let (window, workbench) = show(bar, cx);
    settled_on(cx, window, &workbench, &alpha);

    // Taken, and said of, in one update: the history read again cannot
    // arrive in the middle of it.
    let said = cx
        .update_window(window, |_, window, cx| {
            let generation = workbench.read(cx).history.read(cx).generation();
            workbench.update(cx, |this, cx| {
                this.history.update(cx, |history, cx| {
                    history.page_arrived_for_test(
                        generation,
                        Err(AppError::unknown(
                            codes::git::HISTORY_CURSOR_EXPIRED,
                            "the cursor went stale",
                        )),
                        cx,
                    )
                })
            });
            window.render_frame(cx);
            window
                .find("history-restarting")
                .label()
                .map(str::to_string)
        })
        .unwrap();

    assert_eq!(
        said.as_deref(),
        Some("The history changed while it was read. Reading it again…"),
        "that it is read again is said of"
    );
    assert_eq!(shown(cx, &workbench), alpha.commits, "what was shown stays");
    wait_for(cx, "the history read again", |cx| {
        shown_now(cx, window, "history-restarting").is_none()
    });
    assert_eq!(
        shown(cx, &workbench),
        alpha.commits,
        "and what was read is shown"
    );
}

/// A page does not come: why is said of, what was shown stays, and asking
/// again reads it.
#[gpui_kit::test]
fn a_page_that_does_not_come_is_said_of_and_can_be_asked_for_again(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first", "second"]);
    let bar = prepare(cx);
    workspace(cx, "Projects", &[&alpha.path]);
    let (window, workbench) = show(bar, cx);
    settled_on(cx, window, &workbench, &alpha);

    let gone = AppError::unknown_with(
        codes::git::GIT_ERROR,
        "the repository went away",
        &[("error", "the repository went away".to_string())],
    );
    let generation = cx.read(|cx| workbench.read(cx).history.read(cx).generation());
    workbench.update(cx, |this, cx| {
        this.history.update(cx, |history, cx| {
            history.page_arrived_for_test(generation, Err(gone.clone()), cx)
        })
    });
    assert_eq!(
        says(cx, window, "history-failure").as_deref(),
        Some(
            format!(
                "The history could not be loaded: {}",
                error_text::show(&gone)
            )
            .as_str()
        ),
        "why is said of, in the language in force"
    );
    assert_eq!(shown(cx, &workbench), alpha.commits, "what was shown stays");

    // As there is something shown, the page after it is what is asked for again.
    cx.update_window(window, |_, window, cx| window.click("history-retry", cx))
        .unwrap();
    wait_for(cx, "the page asked for again", |cx| {
        shown_now(cx, window, "history-failure").is_none()
    });
    assert_eq!(shown(cx, &workbench), alpha.commits);
}

/// The repositories of a workspace cannot be read: why is said of in the
/// main pane, with a way to ask again that reads them.
#[gpui_kit::test]
fn repositories_that_cannot_be_read_are_said_of_and_can_be_read_again(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first"]);
    let beta = git_repo("beta", &["other"]);
    let bar = prepare(cx);
    workspace(cx, "First", &[&alpha.path]);
    let second = workspace(cx, "Second", &[&beta.path]);
    let (window, workbench) = show(bar, cx);
    settled_on(cx, window, &workbench, &alpha);

    let gone = AppError::permission_with(
        codes::usecases::WORKSPACE_NOT_FOUND,
        "the workspace is gone",
        &[("id", second.clone())],
    );
    // Another workspace, and the failure to read its repositories: in one
    // update, before the answer to the read in flight can arrive.
    let (said, unread, has_way_to_ask_again) = cx
        .update_window(window, |_, window, cx| {
            workbench.update(cx, |this, cx| {
                this.show_workspace(second.clone(), cx);
                let generation = this.generation;
                this.repos_arrived(generation, second.clone(), Err(gone.clone()), cx);
            });
            window.render_frame(cx);
            let said = window.find("shell-notice").label().map(str::to_string);
            let unread = window.find("repos-unread").visible();
            let has_way_to_ask_again = window.try_find("repos-retry").is_some();
            (said, unread, has_way_to_ask_again)
        })
        .unwrap();
    assert_eq!(
        said.as_deref(),
        Some(format!("Failed to load repos: Workspace not found: {second}").as_str()),
        "what failed and why, in the language in force"
    );
    assert!(unread, "nothing pretends to be loading");
    assert!(
        has_way_to_ask_again,
        "and the way to ask again is where the failure is said of"
    );

    let asked_again = workbench.update(cx, |this, cx| {
        let before = this.generation;
        this.repos_retry(cx);
        before < this.generation
    });
    assert!(asked_again, "asking again reads the repositories again");
    wait_for(cx, "the repositories read again", |cx| {
        cx.read(|cx| {
            workbench
                .read(cx)
                .session
                .selected()
                .is_some_and(|selected| selected.id == second && selected.repos.is_some())
        }) && shown_now(cx, window, "shell-notice").is_none()
    });
    settled_on(cx, window, &workbench, &beta);
}

/// Coming back to a workspace shows the repository that was active when it
/// was left, not the one the application started on.
#[gpui_kit::test]
fn coming_back_to_a_workspace_shows_the_repository_it_was_left_on(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first"]);
    let beta = git_repo("beta", &["other", "another"]);
    let bar = prepare(cx);
    let first = workspace(cx, "First", &[&alpha.path, &beta.path]);
    let second = workspace(cx, "Second", &[]);
    let beta_id = repo_id(cx, &first, &beta.path);
    let (window, workbench) = show(bar, cx);
    settled_on(cx, window, &workbench, &alpha);

    cx.update_window(window, |_, window, cx| {
        window.within("repo-tabs").click(1, cx)
    })
    .unwrap();
    settled_on(cx, window, &workbench, &beta);
    assert_eq!(stored_active(cx, &first).as_deref(), Some(beta_id.as_str()));

    workbench.update(cx, |this, cx| this.show_workspace(second.clone(), cx));
    wait_for(cx, "the other workspace", |cx| {
        cx.read(|cx| workbench.read(cx).session.selected_id() == Some(second.as_str()))
    });
    workbench.update(cx, |this, cx| this.show_workspace(first.clone(), cx));

    settled_on(cx, window, &workbench, &beta);
    assert_eq!(stored_active(cx, &first).as_deref(), Some(beta_id.as_str()));
}

/// Choosing the tab of the repository already shown changes nothing: its
/// history is on screen, and nothing is read or stored again.
#[gpui_kit::test]
fn choosing_the_tab_of_the_repository_already_shown_changes_nothing(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first"]);
    let bar = prepare(cx);
    let id = workspace(cx, "Projects", &[&alpha.path]);
    let (window, workbench) = show(bar, cx);
    settled_on(cx, window, &workbench, &alpha);
    let generation = cx.read(|cx| workbench.read(cx).history.read(cx).generation());
    let stored = stored_active(cx, &id);

    cx.update_window(window, |_, window, cx| {
        window.within("repo-tabs").click(0, cx)
    })
    .unwrap();
    cx.executor().run_until_parked();

    assert_eq!(
        cx.read(|cx| workbench.read(cx).history.read(cx).generation()),
        generation,
        "the history is not read again"
    );
    assert_eq!(stored_active(cx, &id), stored);
    assert_eq!(shown(cx, &workbench), alpha.commits);
}

/// The selection belongs to the repository it was made in.
#[gpui_kit::test]
fn choosing_another_repository_clears_the_selection(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let alpha = git_repo("alpha", &["first", "second", "third"]);
    let beta = git_repo("beta", &["other"]);
    let bar = prepare(cx);
    workspace(cx, "Projects", &[&alpha.path, &beta.path]);
    let (window, workbench) = show(bar, cx);
    settled_on(cx, window, &workbench, &alpha);

    cx.update_window(window, |_, window, cx| {
        window.render_frame(cx);
        window.click(("commit", 2usize), cx);
        window.render_frame(cx);
        assert_eq!(
            workbench.read(cx).history.read(cx).selected_row(cx),
            Some(2),
            "a row was selected"
        );
    })
    .unwrap();

    cx.update_window(window, |_, window, cx| {
        window.within("repo-tabs").click(1, cx)
    })
    .unwrap();
    settled_on(cx, window, &workbench, &beta);
    cx.read(|cx| {
        assert_eq!(
            workbench.read(cx).history.read(cx).selected_row(cx),
            None,
            "nothing is selected in the other repository"
        );
    });
}

/// The shell starts on the workspace it was left on.
#[gpui_kit::test]
fn the_shell_starts_where_it_was_left(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let bar = prepare(cx);
    workspace(cx, "First", &[]);
    let second = workspace(cx, "Second", &[]);
    cx.update(|cx| app_state::remember_workspace(cx, Some(&second)));
    let (window, workbench) = show(bar, cx);

    wait_for(cx, "the workspace", |cx| {
        cx.read(|cx| workbench.read(cx).session.selected_id().is_some())
    });

    assert_eq!(
        cx.read(|cx| workbench.read(cx).session.selected_id().map(str::to_string)),
        Some(second)
    );
    assert_eq!(
        says(cx, window, "tabs-no-repos").as_deref(),
        Some("No repositories in this workspace"),
        "a workspace without repositories says so"
    );
}

/// Without a workspace, the shell says so and the switcher is empty.
#[gpui_kit::test]
fn without_a_workspace_the_shell_says_so(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let (window, workbench) = open(cx);

    wait_for(cx, "the workspaces to be read", |cx| {
        says(cx, window, "empty-workspaces").is_some()
    });

    assert_eq!(
        says(cx, window, "empty-workspaces").as_deref(),
        Some("No workspaces")
    );
    cx.read(|cx| assert!(workbench.read(cx).session.workspaces().is_empty()));
}

/// The workbench takes the focus when the window opens and gets it back whenever
/// a dialog closes, so it has to be a node assistive technology can see. A
/// focused element that is not one gets the whole window announced instead, and
/// a line from GPUI every time the focus comes back to it. The test platform
/// does not activate the accessibility tree, so what is asserted is what GPUI
/// needs to build the node: a role.
#[gpui_kit::test]
fn the_workbench_is_exposed_to_assistive_technology(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let (window, _) = open(cx);
    render(cx, window);

    assert_the_workbench_holds_the_focus(cx, window);

    // The dialog takes the focus and gives it back when it closes.
    open_settings(cx, window);
    assert_settings_dialog_open(cx, window);
    press(cx, window, "escape");
    cx.update_window(window, |_, window, _| {
        assert!(
            window
                .try_find("settings-note")
                .is_none_or(|note| !note.visible()),
            "the dialog closed"
        );
    })
    .unwrap();
    assert_the_workbench_holds_the_focus(cx, window);
}

/// Asserts that the workbench's root is a node with the focus.
fn assert_the_workbench_holds_the_focus(cx: &mut TestAppContext, window: AnyWindowHandle) {
    cx.update_window(window, |_, window, _| {
        let root = window.find("workbench");
        assert_eq!(root.role(), Some(Role::Group), "the root has a role");
        assert_eq!(root.focused(), Some(true), "the root holds the focus");
    })
    .unwrap();
}

/// The shortcut for Settings on this platform.
fn settings_shortcut() -> &'static str {
    if cfg!(target_os = "macos") {
        "cmd-,"
    } else {
        "ctrl-,"
    }
}

/// Draws a frame, in an update of its own.
fn render(cx: &mut TestAppContext, window: AnyWindowHandle) {
    cx.update_window(window, |_, window, cx| window.render_frame(cx))
        .unwrap();
}

/// Presses a key, then draws a frame. What the key opens or closes waits for
/// the update that delivered the keystroke to end, so the frame comes after it.
fn press(cx: &mut TestAppContext, window: AnyWindowHandle, key: &str) {
    cx.update_window(window, |_, window, cx| window.press(key, cx))
        .unwrap();
    render(cx, window);
}

/// Presses the Settings shortcut.
fn open_settings(cx: &mut TestAppContext, window: AnyWindowHandle) {
    press(cx, window, settings_shortcut());
}

#[gpui_kit::test]
fn settings_shortcut_opens_a_dialog_with_a_text_input(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let (window, _) = open(cx);
    render(cx, window);

    cx.update_window(window, |_, window, cx| {
        assert!(window.try_find("settings-note").is_none());
        window.press(settings_shortcut(), cx);
    })
    .unwrap();

    assert_settings_dialog_open(cx, window);
}

/// Asserts that the Settings dialog is on screen.
fn assert_settings_dialog_open(cx: &mut TestAppContext, window: AnyWindowHandle) {
    render(cx, window);
    cx.update_window(window, |_, window, _| {
        assert!(
            window.find("settings-note").visible(),
            "the settings dialog did not open"
        );
    })
    .unwrap();
}

/// A dialog takes the focus and is drawn outside the workbench. The
/// application's actions have to keep working then: the macOS menu bar
/// delivers an item the same way a focused element does.
#[gpui_kit::test]
fn application_actions_work_while_a_dialog_is_open(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let (window, _) = open(cx);
    open_settings(cx, window);
    assert_settings_dialog_open(cx, window);

    // From here on the focus is in the dialog.
    cx.update_window(window, |_, window, cx| {
        window.dispatch_action(
            Box::new(SwitchLanguage(LanguageChoice::Fixed(
                &locales::SIMPLIFIED_CHINESE,
            ))),
            cx,
        );
        window.dispatch_action(Box::new(menus::SwitchThemeMode(ThemeMode::Dark)), cx);
    })
    .unwrap();

    cx.read(|cx| {
        assert_eq!(
            menus::build_menus(cx)[2].name,
            "显示",
            "the language switched while the dialog was open"
        );
        assert!(cx.theme().mode.is_dark(), "the appearance switched too");
    });
    assert_eq!(stored_language(cx).as_deref(), Some("zh-CN"));
}

#[gpui_kit::test]
fn switching_the_appearance_updates_the_view_menu(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let (window, _) = open(cx);

    cx.update_window(window, |_, window, cx| {
        window.render_frame(cx);
        window.dispatch_action(Box::new(menus::SwitchThemeMode(ThemeMode::Dark)), cx);
    })
    .unwrap();

    cx.read(|cx| {
        assert!(cx.theme().mode.is_dark());
        let menus = menus::build_menus(cx);
        let view = &menus[2];
        assert_eq!(view.name, "View");
        let MenuItem::Action { checked: light, .. } = &view.items[0] else {
            panic!("the first View item is an action");
        };
        let MenuItem::Action { checked: dark, .. } = &view.items[1] else {
            panic!("the second View item is an action");
        };
        assert!(!light, "Light stays checked after switching to Dark");
        assert!(dark, "Dark is not checked after switching to it");
    });
}

/// The backgrounds of the quads that cover (almost) the whole window in the
/// last frame.
fn window_fills(window: &mut gpui_kit::Window) -> Vec<Background> {
    let viewport = window.viewport_size();
    let full = viewport.scale(window.scale_factor());
    window
        .painted_quads()
        .into_iter()
        .filter(|quad| {
            quad.bounds.size.width.0 + 2.0 >= full.width.0 * 0.9
                && quad.bounds.size.height.0 + 2.0 >= full.height.0 * 0.9
        })
        .map(|quad| quad.background)
        .collect()
}

/// The other half of a theme switch: the menu's check mark moves, covered
/// above, and the window takes the new colours. The workbench picks its
/// background when it renders, so after the switch nothing may be left in the
/// colour of the theme it left.
#[gpui_kit::test]
fn switching_the_appearance_repaints_the_window(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let (window, _) = open(cx);
    render(cx, window);

    let light = cx.read(|cx| solid_background(cx.theme().background));
    cx.update_window(window, |_, window, _| {
        assert!(
            window_fills(window).contains(&light),
            "the window starts in the light theme"
        );
    })
    .unwrap();

    cx.update_window(window, |_, window, cx| {
        window.dispatch_action(Box::new(menus::SwitchThemeMode(ThemeMode::Dark)), cx);
        window.render_frame(cx);
    })
    .unwrap();

    let dark = cx.read(|cx| solid_background(cx.theme().background));
    assert_ne!(light, dark, "the two themes differ");
    cx.update_window(window, |_, window, _| {
        let fills = window_fills(window);
        assert!(
            fills.contains(&dark),
            "the window is painted in the dark theme: {fills:?}"
        );
        assert!(
            !fills.contains(&light),
            "something is still painted in the light theme: {fills:?}"
        );
    })
    .unwrap();
}

/// The shortcut for the sidebar on this platform.
fn sidebar_shortcut() -> &'static str {
    if cfg!(target_os = "macos") {
        "cmd-b"
    } else {
        "ctrl-b"
    }
}

/// The shortcut hides the navigator and brings it back. The panes keep their
/// places, so it is hidden, not removed.
#[gpui_kit::test]
fn the_sidebar_shortcut_hides_the_navigator_and_brings_it_back(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let (window, _) = open(cx);
    render(cx, window);

    let navigator_shows = |cx: &mut TestAppContext| {
        cx.update_window(window, |_, window, cx| {
            window.render_frame(cx);
            // Hidden, the panel is not laid out, so there is nothing to find.
            window
                .try_find("navigator-pane")
                .is_some_and(|pane| pane.visible())
        })
        .unwrap()
    };
    assert!(navigator_shows(cx), "the navigator is shown at first");

    press(cx, window, sidebar_shortcut());
    assert!(!navigator_shows(cx), "the shortcut hides the navigator");

    press(cx, window, sidebar_shortcut());
    assert!(
        navigator_shows(cx),
        "the shortcut brings the navigator back"
    );
}

#[gpui_kit::test]
fn switching_the_language_relabels_the_menus_and_stores_the_choice(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let (window, _) = open(cx);

    cx.update_window(window, |_, window, cx| {
        window.render_frame(cx);
        window.dispatch_action(
            Box::new(SwitchLanguage(LanguageChoice::Fixed(
                &locales::SIMPLIFIED_CHINESE,
            ))),
            cx,
        );
    })
    .unwrap();

    cx.read(|cx| {
        assert_eq!(rust_i18n::t!("menu.app.settings"), "设置…");
        let menus = menus::build_menus(cx);
        let view = &menus[2];
        assert_eq!(view.name, "显示");
        let MenuItem::Submenu(language) = view.items.get(3).expect("a language submenu") else {
            panic!("the fourth View item is the language submenu");
        };
        let offered: Vec<(&str, bool)> = language
            .items
            .iter()
            .map(|item| match item {
                MenuItem::Action { name, checked, .. } => (name.as_ref(), *checked),
                MenuItem::Separator | MenuItem::Submenu(_) | MenuItem::SystemMenu(_) => {
                    panic!("the language submenu holds actions only")
                }
            })
            .collect();
        assert_eq!(
            offered,
            [("跟随系统", false), ("English", false), ("简体中文", true)]
        );
    });
    assert_eq!(
        stored_language(cx).as_deref(),
        Some("zh-CN"),
        "the choice is stored by the time the switch returns"
    );

    // Back to the system, which reports nothing: English again, and nothing stored.
    cx.update_window(window, |_, window, cx| {
        window.dispatch_action(Box::new(SwitchLanguage(LanguageChoice::System)), cx);
    })
    .unwrap();
    cx.read(|cx| assert_eq!(menus::build_menus(cx)[2].name, "View"));
    assert_eq!(stored_language(cx), None);
}

/// The labels of the language radios, in the order they are offered. The group
/// numbers its children, so inside it a radio's id is its position.
fn language_radio_labels(window: &mut gpui_kit::Window) -> Vec<String> {
    let group = window.within("settings-language");
    (0..3)
        .map(|index| {
            group
                .find(index)
                .label()
                .expect("a radio has a label")
                .to_string()
        })
        .collect()
}

/// The language radio that is selected. A radio reports being selected, and
/// also toggled, for different kinds of assistive technology.
fn selected_radio(window: &mut gpui_kit::Window) -> usize {
    let group = window.within("settings-language");
    (0..3)
        .find(|index| {
            let radio = group.find(*index);
            radio
                .selected()
                .or_else(|| radio.checked())
                .unwrap_or_default()
        })
        .expect("one radio is selected")
}

#[gpui_kit::test]
fn the_settings_dialog_switches_the_language_from_its_radios(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let (window, _) = open(cx);
    open_settings(cx, window);

    cx.update_window(window, |_, window, cx| {
        assert_eq!(
            language_radio_labels(window),
            ["Follow system", "English", "简体中文"],
            "each language names itself"
        );
        assert_eq!(selected_radio(window), 0);

        // Choose 简体中文 the way a user would: the third radio of the group.
        window.within("settings-language").click(2, cx);
        window.render_frame(cx);
        assert_eq!(
            language_radio_labels(window),
            ["跟随系统", "English", "简体中文"],
            "the open dialog follows the switch"
        );
        assert_eq!(selected_radio(window), 2);
    })
    .unwrap();

    cx.read(|cx| {
        assert_eq!(menus::build_menus(cx)[2].name, "显示");
        assert_eq!(
            locales::choice(cx),
            LanguageChoice::Fixed(&locales::SIMPLIFIED_CHINESE)
        );
    });
    assert_eq!(stored_language(cx).as_deref(), Some("zh-CN"));
}

/// A menu the way the platform shows it: its name, then one line per item. A
/// separator is `-`, and a submenu is listed indented under its own name.
fn outline(menu: &OwnedMenu) -> Vec<String> {
    let mut lines = vec![menu.name.to_string()];
    for item in &menu.items {
        match item {
            OwnedMenuItem::Separator => lines.push("-".into()),
            OwnedMenuItem::Submenu(submenu) => {
                lines.extend(outline(submenu).into_iter().map(|line| format!("  {line}")));
            }
            OwnedMenuItem::SystemMenu(system) => lines.push(system.name.to_string()),
            OwnedMenuItem::Action { name, .. } => lines.push(name.clone()),
        }
    }
    lines
}

/// Both sets of menus: those the system shows, which is the macOS menu bar,
/// and those of the title bar on Windows and Linux.
fn menu_outlines(cx: &App) -> [Vec<Vec<String>>; 2] {
    [
        cx.get_menus()
            .expect("the application has set its menus")
            .iter()
            .map(outline)
            .collect(),
        GlobalState::global(cx)
            .app_menus()
            .iter()
            .map(outline)
            .collect(),
    ]
}

/// What the menus say in a language, written out here rather than read back
/// from the message files: a label that is wired to the wrong message, or to
/// no message at all, shows up as a difference.
fn expected_menus(english: bool) -> Vec<Vec<String>> {
    let mut application = if english {
        vec![
            "GitWave",
            "About GitWave",
            "Check for Updates…",
            "-",
            "Settings…",
        ]
    } else {
        vec!["GitWave", "关于 GitWave", "检查更新…", "-", "设置…"]
    };
    if cfg!(target_os = "macos") {
        application.extend(if english {
            ["-", "Hide GitWave", "Hide Others"]
        } else {
            ["-", "隐藏 GitWave", "隐藏其他"]
        });
    }
    application.extend([
        "-",
        if english {
            "Quit GitWave"
        } else {
            "退出 GitWave"
        },
    ]);
    let edit = if english {
        [
            "Edit",
            "Undo",
            "Redo",
            "-",
            "Cut",
            "Copy",
            "Paste",
            "Select All",
        ]
    } else {
        ["编辑", "撤销", "重做", "-", "剪切", "复制", "粘贴", "全选"]
    };
    let view = if english {
        [
            "View",
            "Light",
            "Dark",
            "-",
            "  Language",
            "  Follow system",
        ]
    } else {
        ["显示", "浅色", "深色", "-", "  语言", "  跟随系统"]
    };
    let window = if english {
        ["Window", "Minimize", "Zoom", "-", "Bring All to Front"]
    } else {
        ["窗口", "最小化", "缩放", "-", "前置全部窗口"]
    };
    vec![
        application.into_iter().map(String::from).collect(),
        edit.into_iter().map(String::from).collect(),
        view.into_iter()
            .chain(["  English", "  简体中文"])
            .map(String::from)
            .collect(),
        window.into_iter().map(String::from).collect(),
    ]
}

/// After a switch, both sets of menus follow by themselves, through the
/// observer, and every label of theirs comes from the message files. In
/// Chinese, nothing is left in English except the application's name and the
/// names of the languages, which name themselves.
#[gpui_kit::test]
fn both_sets_of_menus_follow_the_language_and_leave_nothing_in_english(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let (window, _) = open(cx);
    render(cx, window);
    assert_eq!(
        cx.read(menu_outlines),
        [expected_menus(true), expected_menus(true)]
    );

    cx.update_window(window, |_, window, cx| {
        window.dispatch_action(
            Box::new(SwitchLanguage(LanguageChoice::Fixed(
                &locales::SIMPLIFIED_CHINESE,
            ))),
            cx,
        );
    })
    .unwrap();

    assert_eq!(
        cx.read(menu_outlines),
        [expected_menus(false), expected_menus(false)]
    );
}
