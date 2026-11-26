//! The main window: a title toolbar with the workspace, the row of repository
//! tabs under it, the navigator beside the history, and a status bar.

use gitwave_core::domain::ShellSettings;
use gitwave_core::domain::error::AppError;
use gitwave_core::domain::working_copy::WorkingCopy;
use gitwave_core::domain::workspace::{RepoRef, RepoStatus, WorkspaceSummary};
use gpui_kit::component::searchable_list::{SearchableListItem, SearchableVec};
use gpui_kit::component::select::{Select, SelectEvent, SelectState};
use gpui_kit::component::sidebar::{Sidebar, SidebarGroup, SidebarMenuItem};
use gpui_kit::component::tab::{Tab, TabBar};
use gpui_kit::component::{
    ActiveTheme as _, Sizable as _, Theme, TitleBar, WindowExt as _,
    button::Button,
    h_flex,
    input::{Textarea, TextareaState},
    label::Label,
    menu::AppMenuBar,
    radio::{Radio, RadioGroup},
    resizable::{h_resizable, resizable_panel},
    status_bar::StatusBar,
    v_flex,
};
use gpui_kit::prelude::FluentBuilder as _;
use gpui_kit::{
    AnyElement, AnyWindowHandle, App, AppContext as _, Context, Entity, FocusHandle,
    InteractiveElement as _, IntoElement, KeyBinding, ParentElement as _, Render, Role,
    SharedString, StatefulInteractiveElement as _, Styled as _, TestSupportExt as _, Window, div,
    px,
};
use rust_i18n::t;

use crate::{
    app_state::{self, Origin},
    error_text,
    history::{History, Scope},
    locales::{self, LanguageChoice},
    menus::{
        AboutGitWave, BringAllToFront, HideGitWave, HideOtherApplications, Minimize, OpenSettings,
        Quit, SwitchLanguage, SwitchThemeMode, Zoom,
    },
    session::Session,
};

gpui_kit::actions!(gitwave, [ToggleSidebar]);

/// Binds the shortcuts that work everywhere in the window.
pub fn init(cx: &mut App) {
    let is_mac = cx.platform_info().is_mac();
    cx.bind_keys([KeyBinding::new(
        if is_mac { "cmd-b" } else { "ctrl-b" },
        ToggleSidebar,
        Some("GitWave"),
    )]);
}

/// Registers the application's actions on the application itself.
///
/// They must not hang on the workbench element. A dialog takes the focus and
/// is drawn outside the workbench, so an action dispatched from the focused
/// element, which is also how the macOS menu bar delivers its items, never
/// passes the workbench while a dialog is open.
fn register_application_actions(cx: &mut Context<Workbench>) {
    let about = cx.weak_entity();
    let settings = cx.weak_entity();
    // The application, as opposed to the view's own context, which has an
    // `on_action` of its own that is bound to a window.
    let app: &mut App = cx;
    app.on_action(move |_: &AboutGitWave, cx| {
        let about = about.clone();
        in_the_window(cx, move |window, cx| {
            let _ = about.update(cx, |this, cx| this.open_about(window, cx));
        });
    });
    app.on_action(move |_: &OpenSettings, cx| {
        let settings = settings.clone();
        in_the_window(cx, move |window, cx| {
            let _ = settings.update(cx, |this, cx| this.open_settings(window, cx));
        });
    });
    app.on_action(|action: &SwitchThemeMode, cx| {
        let mode = action.0;
        in_the_window(cx, move |window, cx| Theme::change(mode, Some(window), cx));
    });
    app.on_action(|action: &SwitchLanguage, cx| locales::switch_and_store(action.0, cx));
    app.on_action(|_: &HideGitWave, cx| cx.hide());
    app.on_action(|_: &HideOtherApplications, cx| cx.hide_other_apps());
    app.on_action(|_: &Minimize, cx| in_the_window(cx, |window, _| window.minimize_window()));
    app.on_action(|_: &Zoom, cx| in_the_window(cx, |window, _| window.zoom_window()));
    app.on_action(|_: &BringAllToFront, cx| cx.activate(true));
    app.on_action(|_: &Quit, cx| cx.quit());
}

/// Runs `with` on the window the application acts on: the active one, or the
/// only one there is when the platform reports none as active. Nothing happens
/// without a window.
///
/// It waits for the current update to end first. A listener is reached while
/// the window is being updated to dispatch the action, and a window cannot be
/// updated again inside its own update.
fn in_the_window(cx: &mut App, with: impl FnOnce(&mut Window, &mut App) + 'static) {
    cx.defer(move |cx| {
        let window = cx
            .active_window()
            .or_else(|| match cx.windows().as_slice() {
                [only] => Some(*only),
                _ => None,
            });
        let Some(window) = window else {
            return;
        };
        // A window that closed in the meantime is the only reason to fail here.
        let _ = window.update(cx, |_, window, cx| with(window, cx));
    });
}

/// What could not be read, and is said of where it was to be shown. The
/// error, not what it says: that follows the language in force when it is
/// shown.
#[derive(Debug, Clone)]
enum Notice {
    Workspaces(AppError),
    Repositories(AppError),
    /// The copy of the state could not be brought up to date with the
    /// original.
    Copy(AppError),
}

impl Notice {
    fn copy(cx: &App) -> Option<Self> {
        app_state::origin_failure(cx).map(Notice::Copy)
    }

    fn text(&self) -> String {
        match self {
            Notice::Workspaces(error) => {
                t!("workspace.loadFailed", error = error_text::show(error)).to_string()
            }
            Notice::Repositories(error) => {
                t!("workspace.reposLoadFailed", error = error_text::show(error)).to_string()
            }
            Notice::Copy(error) => error_text::show(error),
        }
    }
}

/// A workspace in the switcher: named as the user knows it, and told apart by
/// its id, as two of them can have the same name.
#[derive(Clone)]
struct WorkspaceItem {
    id: String,
    name: SharedString,
}

impl SearchableListItem for WorkspaceItem {
    type Value = String;

    fn title(&self) -> SharedString {
        self.name.clone()
    }

    fn value(&self) -> &String {
        &self.id
    }
}

pub struct Workbench {
    /// Keys are dispatched from the focused element upwards, so the workbench
    /// takes the focus when the window opens for its own shortcut to reach it.
    focus: FocusHandle,
    app_menu_bar: Entity<AppMenuBar>,
    navigator_visible: bool,
    /// A multi-line input for trying the keyboard and the input method, until
    /// the commit editor takes its place.
    note: Entity<TextareaState>,
    window: AnyWindowHandle,
    /// The workspace switcher in the title toolbar.
    switcher: Entity<SelectState<SearchableVec<WorkspaceItem>>>,
    history: Entity<History>,
    /// What is shown: the workspaces, one of them, its repositories and the
    /// active one of those.
    session: Session,
    /// What could not be read, when something could not.
    notice: Option<Notice>,
    /// The head of the active repository, for the status bar.
    head: Option<WorkingCopy>,
    /// Bumped whenever other repositories are asked for, so that ones
    /// arriving late for the workspace before are dropped.
    generation: u64,
    /// Bumped whenever another head is asked for, for the same reason. It is
    /// its own: choosing another tab does not ask for repositories.
    head_generation: u64,
}

impl Workbench {
    pub fn new(
        app_menu_bar: Entity<AppMenuBar>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) -> Self {
        window.set_window_title("GitWave");
        let focus = cx.focus_handle();
        window.focus(&focus, cx);
        let note = cx.new(|cx| TextareaState::new(window, cx));
        let history = cx.new(|cx| History::new(window, cx));
        let switcher =
            cx.new(|cx| SelectState::new(SearchableVec::new(Vec::new()), None, window, cx));
        register_application_actions(cx);
        // Every label comes from the message files, so a new language means a new frame.
        cx.observe_global::<locales::InterfaceLanguage>(|_, cx| cx.notify())
            .detach();
        cx.subscribe_in(
            &switcher,
            window,
            |this, _, event: &SelectEvent<SearchableVec<WorkspaceItem>>, _, cx| {
                this.workspace_chosen(event, cx)
            },
        )
        .detach();
        let handle = window.window_handle();
        let mut workbench = Self {
            focus,
            app_menu_bar,
            navigator_visible: true,
            note,
            window: handle,
            switcher,
            history,
            session: Session::default(),
            // A copy of the state that could not be brought up to date is
            // said of before anything else is.
            notice: Notice::copy(cx),
            head: None,
            generation: 0,
            head_generation: 0,
        };
        workbench.start(cx);
        workbench
    }

    /// Reads what was shown last and the workspaces, then shows a workspace.
    ///
    /// The tasks are detached, as the component library's are: each runs to
    /// its end and reaches the workbench through a weak reference, so it is
    /// harmless once the workbench is gone, and nothing has to keep it.
    fn start(&mut self, cx: &mut Context<Self>) {
        let settings = app_state::shell_settings(cx);
        let workspaces = app_state::workspaces(cx);
        let window = self.window;
        cx.spawn(async move |this, cx| {
            let (settings, workspaces) = (settings.await, workspaces.await);
            // The switcher lays its items out, which takes the window.
            let _ = cx.update_window(window, move |_, window, cx| {
                let _ = this.update(cx, |this, cx| {
                    this.started(settings, workspaces, window, cx)
                });
            });
        })
        .detach();
    }

    fn started(
        &mut self,
        settings: Result<ShellSettings, AppError>,
        workspaces: Result<Vec<WorkspaceSummary>, AppError>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        match workspaces {
            Ok(workspaces) => {
                let remembered = settings
                    .ok()
                    .and_then(|settings| settings.last_workspace_id);
                self.session = Session::new(workspaces, remembered.as_deref());
                self.notice = Notice::copy(cx);
                self.sync_switcher(window, cx);
                if let Some(workspace_id) = self.session.selected_id().map(str::to_string) {
                    self.load_repos(workspace_id, cx);
                }
            }
            Err(error) => self.notice = Some(Notice::Workspaces(error)),
        }
        cx.notify();
    }

    /// Hands the switcher the workspaces, with the shown one selected.
    fn sync_switcher(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let items: Vec<WorkspaceItem> = self
            .session
            .workspaces()
            .iter()
            .map(|workspace| WorkspaceItem {
                id: workspace.id.clone(),
                name: workspace.name.clone().into(),
            })
            .collect();
        let selected = self.session.selected_id().map(str::to_string);
        self.switcher.update(cx, |switcher, cx| {
            switcher.set_items(SearchableVec::new(items), window, cx);
            if let Some(selected) = selected {
                switcher.set_selected_value(&selected, window, cx);
            }
        });
    }

    fn workspace_chosen(
        &mut self,
        event: &SelectEvent<SearchableVec<WorkspaceItem>>,
        cx: &mut Context<Self>,
    ) {
        let SelectEvent::Confirm(Some(workspace_id)) = event else {
            return;
        };
        self.show_workspace(workspace_id.clone(), cx);
    }

    /// Shows another workspace and reads its repositories.
    fn show_workspace(&mut self, workspace_id: String, cx: &mut Context<Self>) {
        if self.session.selected_id() == Some(workspace_id.as_str()) {
            return;
        }
        if !self.session.show(&workspace_id) {
            return;
        }
        // One small local write, done before the interface goes on.
        app_state::remember_workspace(cx, Some(&workspace_id));
        // What was shown belongs to the workspace before, and so does
        // whatever is on its way for it. It is dropped now, and not once the
        // repositories of this workspace have been read: until then they
        // would still be taken.
        self.head = None;
        self.head_generation += 1;
        self.history.update(cx, |history, cx| history.forget(cx));
        self.load_repos(workspace_id, cx);
        cx.notify();
    }

    fn load_repos(&mut self, workspace_id: String, cx: &mut Context<Self>) {
        self.generation += 1;
        let generation = self.generation;
        self.notice = Notice::copy(cx);
        let repos = app_state::repos(cx, workspace_id.clone());
        cx.spawn(async move |this, cx| {
            let result = repos.await;
            let _ = this.update(cx, |this, cx| {
                this.repos_arrived(generation, workspace_id, result, cx)
            });
        })
        .detach();
    }

    /// Takes the repositories of the workspace they were asked for, makes the
    /// one shown before active again, and shows its history.
    fn repos_arrived(
        &mut self,
        generation: u64,
        workspace_id: String,
        result: Result<Vec<RepoRef>, AppError>,
        cx: &mut Context<Self>,
    ) {
        if generation != self.generation {
            return;
        }
        match result {
            Ok(repos) => {
                self.notice = Notice::copy(cx);
                // False when another workspace is shown by now.
                if !self.session.set_repos(&workspace_id, repos) {
                    return;
                }
                let shown_before = self
                    .session
                    .workspaces()
                    .iter()
                    .find(|workspace| workspace.id == workspace_id)
                    .and_then(|workspace| workspace.last_active_repo_id.clone());
                let active = self.session.restore_active(shown_before.as_deref());
                // The use cases that read the active repository read the
                // stored one, so it is stored before anything reads it. It
                // already is, most times: nothing is written then.
                if self.session.keep_active()
                    && let Some(active) = &active
                {
                    app_state::remember_active_repo(cx, &workspace_id, &active.id);
                }
                self.show_history(cx);
            }
            // They stay as good as unread: the main pane says why, and asks again when the user does.
            Err(error) => self.notice = Some(Notice::Repositories(error)),
        }
        cx.notify();
    }

    /// Reads the repositories of the shown workspace again, after they could
    /// not be read.
    fn repos_retry(&mut self, cx: &mut Context<Self>) {
        if let Some(workspace_id) = self.session.selected_id().map(str::to_string) {
            self.load_repos(workspace_id, cx);
        }
        cx.notify();
    }

    /// Makes the repository of a tab active and shows its history. The tab of
    /// the repository that is active already does nothing: its history is on
    /// screen.
    fn repo_chosen(&mut self, index: usize, cx: &mut Context<Self>) {
        let Some(selected) = self.session.selected() else {
            return;
        };
        let Some(repo) = selected.repos.as_deref().and_then(|repos| repos.get(index)) else {
            return;
        };
        let already_active = selected
            .active
            .and_then(|active| selected.repos.as_deref()?.get(active))
            .is_some_and(|active| active.id == repo.id);
        if already_active {
            return;
        }
        let (workspace_id, repo_id) = (selected.id.clone(), repo.id.clone());
        // False for a repository whose path is missing.
        if !self.session.activate(&repo_id) {
            return;
        }
        // Stored when it is not the stored one already.
        if self.session.keep_active() {
            app_state::remember_active_repo(cx, &workspace_id, &repo_id);
        }
        self.show_history(cx);
    }

    /// Shows the history of the active repository and reads its head, once:
    /// the status bar names the branch, and the history marks the commit.
    fn show_history(&mut self, cx: &mut Context<Self>) {
        self.head = None;
        let scope = self.session.selected().and_then(|selected| {
            self.session.active_repo().map(|repo| Scope {
                workspace_id: selected.id.clone(),
                repo_id: repo.id.clone(),
            })
        });
        let Some(scope) = scope else {
            return;
        };
        self.history
            .update(cx, |history, cx| history.show(scope.clone(), cx));
        self.head_generation += 1;
        let head_generation = self.head_generation;
        let head = app_state::working_copy(cx, scope.workspace_id);
        cx.spawn(async move |this, cx| {
            let result = head.await;
            let _ = this.update(cx, |this, cx| {
                this.head_arrived(head_generation, result, cx)
            });
        })
        .detach();
        cx.notify();
    }

    /// Takes the head asked for by the generation given. One that arrives for
    /// a repository shown before is dropped, the same repository included: it
    /// may have moved since.
    fn head_arrived(
        &mut self,
        generation: u64,
        result: Result<WorkingCopy, AppError>,
        cx: &mut Context<Self>,
    ) {
        cx.notify();
        if generation != self.head_generation {
            return;
        }
        // A head that cannot be read leaves the branch out of the status bar
        // and the mark off its commit.
        self.head = result.ok();
        let sha = self.head.as_ref().map(|head| head.sha.clone());
        self.history
            .update(cx, |history, cx| history.head_is(sha, cx));
    }

    /// The number of the head asked for last. One more for every head.
    #[cfg(test)]
    pub(crate) fn head_generation(&self) -> u64 {
        self.head_generation
    }

    /// Takes a head as if it had arrived for the generation given.
    #[cfg(test)]
    pub(crate) fn head_arrived_for_test(
        &mut self,
        generation: u64,
        result: Result<WorkingCopy, AppError>,
        cx: &mut Context<Self>,
    ) {
        self.head_arrived(generation, result, cx);
    }

    fn open_about(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        window.open_dialog(cx, move |dialog, _, _| {
            dialog
                .title(t!("about.title"))
                .child(
                    v_flex()
                        .gap_1()
                        .child(Label::new(concat!("GitWave ", env!("CARGO_PKG_VERSION"))))
                        .child(Label::new(t!("about.slogan"))),
                )
                .footer(close_button("close-about"))
        });
    }

    fn open_settings(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let note = self.note.clone();
        // The third parameter is the application: the dialog is built again for
        // every frame, so it reads the language in force each time.
        window.open_dialog(cx, move |dialog, _, cx| {
            dialog
                .title(t!("settings.title"))
                .child(
                    v_flex()
                        .gap_4()
                        .child(language_setting(locales::choice(cx)))
                        // Developer text, not in the message files: the box goes
                        // away once a real input takes its place.
                        .child(Label::new(
                            "The box below is for trying the keyboard and the input method.",
                        ))
                        .child(
                            div()
                                .id("settings-note")
                                .role(Role::TextInput)
                                .test_support()
                                .child(Textarea::new(&note)),
                        ),
                )
                .footer(close_button("close-settings"))
        });
    }

    /// What the status bar says about what is shown: the repository and its
    /// branch.
    fn identity(&self) -> SharedString {
        let Some(repo) = self.session.active_repo() else {
            return SharedString::new("");
        };
        match &self.head {
            Some(head) if head.repo_id == repo.id => {
                format!("{} · {}", tab_name(repo), head.branch).into()
            }
            _ => tab_name(repo).into(),
        }
    }
}

impl Render for Workbench {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let rem = window.rem_size();
        // The system menu bar owns the menus on macOS; elsewhere it sits in
        // the title toolbar, before the workspace.
        let leading = h_flex()
            .items_center()
            .gap_2()
            .when(!cx.platform_info().is_mac(), |this| {
                this.child(self.app_menu_bar.clone())
            })
            .child(
                Select::new(&self.switcher)
                    .placeholder(t!("workspace.title"))
                    .accessibility_label(t!("workspace.title"))
                    .disabled(self.session.workspaces().is_empty()),
            );
        let origin = app_state::origin(cx);
        v_flex()
            .id("workbench")
            .test_support()
            .track_focus(&self.focus)
            // It takes the focus when the window opens and gets it back whenever
            // a dialog closes, so it has to be a node assistive technology can
            // see: a focused element without a role is not one, and the whole
            // window gets announced instead. A group is the neutral container
            // on every platform.
            .role(Role::Group)
            .key_context("GitWave")
            .size_full()
            .min_w_0()
            .min_h_0()
            .bg(cx.theme().background)
            .text_color(cx.theme().foreground)
            .on_action(cx.listener(|this, _: &ToggleSidebar, _, cx| {
                this.navigator_visible = !this.navigator_visible;
                cx.notify();
            }))
            .child(title_bar_with(leading))
            .child(repo_tabs(&self.session, cx))
            .child(
                div().flex_1().min_h_0().child(
                    h_resizable("workbench-panes")
                        // The panel keeps its place when hidden, so that the
                        // navigator comes back where it was.
                        .child(
                            resizable_panel()
                                .size(rem * 14.)
                                .size_range(rem * 13. ..rem * 15.)
                                .visible(self.navigator_visible)
                                // In a wrapper of the shell's own: what the
                                // navigator registers inside is the library's.
                                .child(
                                    div()
                                        .id("navigator-pane")
                                        .test_support()
                                        .size_full()
                                        .child(navigator()),
                                ),
                        )
                        .child(resizable_panel().child(main_pane(
                            &self.session,
                            self.notice.clone(),
                            &self.history,
                            cx.listener(|this, _, _, cx| this.repos_retry(cx)),
                            cx,
                        ))),
                ),
            )
            .child(status_bar(
                self.identity(),
                app_state::origin_label(&origin, std::time::SystemTime::now()),
                // A copy that could not be brought up to date is not to be
                // mistaken for the released application's state.
                !matches!(origin, Origin::Memory)
                    && !matches!(origin, Origin::Copy { failure: None, .. }),
                cx,
            ))
    }
}

fn title_bar_with(leading: impl IntoElement) -> impl IntoElement {
    TitleBar::new().child(h_flex().size_full().px_1().pr_3().gap_2().child(leading))
}

/// The repositories of the shown workspace, one tab each. One whose path is
/// missing is marked and does not open.
fn repo_tabs(session: &Session, cx: &mut Context<Workbench>) -> AnyElement {
    let tabs: Option<Vec<Tab>> = session.selected().and_then(|selected| {
        selected.repos.as_ref().map(|repos| {
            repos
                .iter()
                .map(|repo| {
                    let name: SharedString = tab_name(repo).into();
                    let tab = Tab::new().label(name.clone());
                    if repo.status == RepoStatus::Missing {
                        // Only what is true of it as yet: there is no
                        // relinking here to point to.
                        tab.disabled(true).aria_label(
                            t!(
                                "workspace.tabs.missingName",
                                name = name.to_string(),
                                status = t!("workspace.tabs.missing").to_string()
                            )
                            .to_string(),
                        )
                    } else {
                        tab
                    }
                })
                .collect()
        })
    });
    let bar: AnyElement = match (session.selected(), tabs) {
        // Nothing to say in the row: the main pane says there is no workspace.
        (None, _) => div().into_any_element(),
        (Some(_), None) => div().into_any_element(),
        (Some(_), Some(tabs)) if tabs.is_empty() => {
            quiet(t!("workspace.tabs.none"), "tabs-no-repos")
        }
        (Some(selected), Some(tabs)) => {
            // The tabs scroll sideways, and the ones that do not fit are in
            // a menu. A tab bar highlights an index as soon as it is given
            // one, so it is given the active one only when there is one.
            let mut bar = TabBar::new("repo-tabs").menu(true).children(tabs);
            if let Some(active) = selected.active {
                bar = bar.selected_index(active);
            }
            bar.on_click(cx.listener(|this, index: &usize, _, cx| this.repo_chosen(*index, cx)))
                .into_any_element()
        }
    };
    h_flex()
        .h(px(32.))
        .flex_shrink_0()
        .px_2()
        .gap_2()
        .child(bar)
        .into_any_element()
}

fn quiet(text: impl Into<SharedString>, id: &'static str) -> AnyElement {
    let text: SharedString = text.into();
    div()
        .id(id)
        .test_support()
        .aria_label(text.clone())
        .child(Label::new(text))
        .into_any_element()
}

/// What the main pane shows: the history of the active repository, or why
/// there is none to show. What could not be read is said of at the top, with
/// a way to ask again where asking again is up to the user.
fn main_pane(
    session: &Session,
    notice: Option<Notice>,
    history: &Entity<History>,
    retry_repos: impl Fn(&gpui_kit::ClickEvent, &mut Window, &mut App) + 'static,
    cx: &App,
) -> AnyElement {
    let muted = cx.theme().muted_foreground;
    let empty = |id: &'static str, title: SharedString| empty_state(id, title, muted);
    let repos_failed = matches!(notice, Some(Notice::Repositories(_)));
    let content: AnyElement = match session.selected() {
        // As yet there is no way to add one.
        None => empty(
            "empty-workspaces",
            t!("workspace.empty.title").to_string().into(),
        ),
        Some(selected) if selected.repos.is_none() && repos_failed => {
            // Said of at the top; there is nothing else to show.
            div()
                .id("repos-unread")
                .test_support()
                .size_full()
                .into_any_element()
        }
        Some(selected) if selected.repos.is_none() => {
            empty("empty-reading", t!("common.loading").to_string().into())
        }
        Some(selected) if selected.repos.as_ref().is_some_and(Vec::is_empty) => {
            empty("empty-repos", t!("workspace.tabs.none").to_string().into())
        }
        // Every one of them is where it is not.
        Some(selected) if selected.repos.is_some() && selected.active.is_none() => empty(
            "empty-active",
            t!("branches.graph.noRepository").to_string().into(),
        ),
        Some(_) => history.clone().into_any_element(),
    };
    let retry: Option<AskAgain> = repos_failed.then(|| Box::new(retry_repos) as _);
    v_flex()
        .id("main-pane")
        .size_full()
        .min_w_0()
        .min_h_0()
        .when_some(notice, |this, notice| {
            this.child(notice_line("shell-notice", &notice, retry, cx))
        })
        .child(div().flex_1().min_h_0().child(content))
        .into_any_element()
}

/// What the button that asks again calls.
type AskAgain = Box<dyn Fn(&gpui_kit::ClickEvent, &mut Window, &mut App)>;

/// One line at the top of the main pane: what could not be read, and why.
fn notice_line(id: &'static str, notice: &Notice, retry: Option<AskAgain>, cx: &App) -> AnyElement {
    let text = notice.text();
    h_flex()
        .id(id)
        .test_support()
        .aria_label(text.clone())
        .h(px(28.))
        .flex_shrink_0()
        .px_3()
        .gap_2()
        .text_color(cx.theme().danger)
        .child(Label::new(text))
        .when_some(retry, |this, retry| {
            this.child(
                Button::new("repos-retry")
                    .small()
                    .label(t!("common.retry"))
                    .on_click(move |event, window, cx| retry(event, window, cx)),
            )
        })
        .into_any_element()
}

fn empty_state(id: &'static str, title: SharedString, muted: gpui_kit::Hsla) -> AnyElement {
    v_flex()
        .id(id)
        .test_support()
        .aria_label(title.clone())
        .size_full()
        .justify_center()
        .items_center()
        .child(Label::new(title).text_color(muted))
        .into_any_element()
}

/// The modes of the workbench and what belongs to them. Only the history can
/// be shown as yet.
fn navigator() -> impl IntoElement {
    Sidebar::new("navigator").child(
        SidebarGroup::new(t!("app.workbench.navigation"))
            .child(SidebarMenuItem::new(t!("app.workbench.history")).active(true))
            .child(SidebarMenuItem::new(t!("app.workbench.changes")).disable(true)),
    )
}

fn status_bar(
    identity: SharedString,
    origin: SharedString,
    origin_failed: bool,
    cx: &App,
) -> impl IntoElement {
    let muted = cx.theme().muted_foreground;
    // Where the state comes from is not to be overlooked when it is not what
    // it should be.
    let origin_color = if origin_failed {
        cx.theme().danger
    } else {
        muted
    };
    StatusBar::new()
        .left(
            h_flex().px_2().text_xs().child(
                div()
                    .id("status-identity")
                    .role(Role::Status)
                    .test_support()
                    .aria_label(identity.clone())
                    .child(Label::new(identity)),
            ),
        )
        .right(
            h_flex().px_2().text_xs().text_color(origin_color).child(
                div()
                    .id("status-origin")
                    .test_support()
                    .aria_label(origin.clone())
                    .child(Label::new(origin)),
            ),
        )
}

/// What a tab and the status bar call a repository: its nickname, else the
/// name of its directory.
fn tab_name(repo: &RepoRef) -> String {
    repo.nickname.clone().unwrap_or_else(|| {
        std::path::Path::new(&repo.path)
            .file_name()
            .map(|name| name.to_string_lossy().into_owned())
            .unwrap_or_else(|| repo.path.clone())
    })
}

fn close_button(id: &'static str) -> impl IntoElement {
    Button::new(id)
        .label(t!("common.close"))
        .on_click(|_, window, cx| window.close_dialog(cx))
}

/// The language setting: its title, then the radios for following the system
/// or choosing one of the shipped languages.
fn language_setting(selected: LanguageChoice) -> impl IntoElement {
    v_flex()
        .gap_1()
        .child(Label::new(t!("settings.general.uiLanguage")))
        .child(language_radios(selected))
}

/// The radios of the language setting, in the order every list offers them.
///
/// The group numbers its children, so a radio's id is its position in the
/// list. The group itself is what tells assistive technology which of how many
/// each radio is.
fn language_radios(selected: LanguageChoice) -> impl IntoElement {
    RadioGroup::vertical("settings-language")
        .children(locales::choices().enumerate().map(|(index, choice)| {
            let name = match choice {
                LanguageChoice::System => t!("common.language.followSystem"),
                LanguageChoice::Fixed(language) => t!(language.name),
            };
            Radio::new(index).label(name.to_string())
        }))
        .selected_index(locales::choices().position(|choice| choice == selected))
        .on_change(|index, _, cx| {
            if let Some(choice) = locales::choices().nth(*index) {
                locales::switch_and_store(choice, cx);
            }
        })
}

#[cfg(test)]
#[path = "workbench_tests.rs"]
mod tests;
