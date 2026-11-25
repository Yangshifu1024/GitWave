//! The main window: a self-drawn title bar over the sidebar, the list pane and the
//! diff inspector, with an operation status bar underneath.

use gitwave_core::domain::workspace::WorkspaceSummary;
use gpui_kit::component::{
    ActiveTheme as _, Theme, TitleBar, WindowExt as _,
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
use gpui_kit::{
    AnyElement, App, AppContext as _, Context, Entity, FocusHandle, FontWeight,
    InteractiveElement as _, IntoElement, KeyBinding, ParentElement as _, Render, Role,
    SharedString, StatefulInteractiveElement as _, Styled as _, Task, TestSupportExt as _, Window,
    div,
};
use rust_i18n::t;

use crate::{
    app_state,
    locales::{self, LanguageChoice},
    menus::{
        AboutGitWave, BringAllToFront, HideGitWave, HideOtherApplications, Minimize, OpenSettings,
        Quit, SwitchLanguage, SwitchThemeMode, Zoom,
    },
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
/// They must not hang on the workbench element. A dialog takes the focus and is
/// drawn outside the workbench, so an action dispatched from the focused
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

pub struct Workbench {
    /// Keys are dispatched from the focused element upwards, so the workbench
    /// takes the focus when the window opens for its own shortcut to reach it.
    focus: FocusHandle,
    app_menu_bar: Entity<AppMenuBar>,
    sidebar_visible: bool,
    /// A multi-line input for trying the keyboard and the input method, until
    /// the commit editor takes its place.
    note: Entity<TextareaState>,
    status: SharedString,
    _workspaces: Task<()>,
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
        register_application_actions(cx);
        // Every label comes from the message files, so a new language means a new frame.
        cx.observe_global::<locales::InterfaceLanguage>(|_, cx| cx.notify())
            .detach();
        // The status line is developer text until there is something to report:
        // it says where the state lives so that nobody takes an isolated build
        // for one that reads the real workspaces.
        let status =
            SharedString::from(format!("{} · Reading workspaces…", app_state::storage(cx)));
        // gitwave-core runs on tokio; the result comes back on the main thread.
        let workspaces = app_state::workspaces(cx);
        let _workspaces = cx.spawn(async move |this, cx| {
            let result = workspaces.await;
            let _ = this.update(cx, |this, cx| {
                this.status = status_line(result, cx).into();
                cx.notify();
            });
        });
        Self {
            focus,
            app_menu_bar,
            sidebar_visible: true,
            note,
            status,
            _workspaces,
        }
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
}

impl Render for Workbench {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let rem = window.rem_size();
        let is_mac = cx.platform_info().is_mac();
        // The system menu bar owns the menus on macOS, so the freed up left side
        // names the window the way a Mac app does.
        let leading: AnyElement = if is_mac {
            div()
                .text_sm()
                .font_weight(FontWeight::MEDIUM)
                .child("GitWave")
                .into_any_element()
        } else {
            self.app_menu_bar.clone().into_any_element()
        };
        let title_bar = title_bar_with(leading);
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
                this.sidebar_visible = !this.sidebar_visible;
                cx.notify();
            }))
            .child(title_bar)
            .child(
                div().flex_1().min_h_0().child(
                    h_resizable("workbench-panes")
                        // Panels keep their sizes by position, so the sidebar
                        // stays a child when hidden instead of being removed.
                        // Their titles are placeholders for the views of the
                        // read-only workbench, not interface text.
                        .child(
                            resizable_panel()
                                .size(rem * 22.)
                                .size_range(rem * 16. ..rem * 40.)
                                .visible(self.sidebar_visible)
                                .child(pane("Sidebar")),
                        )
                        .child(
                            resizable_panel()
                                .size(rem * 36.)
                                .size_range(rem * 24. ..rem * 72.)
                                .child(pane("History")),
                        )
                        .child(
                            resizable_panel()
                                .size_range(rem * 40. ..rem * 200.)
                                .child(pane("Diff")),
                        ),
                ),
            )
            .child(
                StatusBar::new().left(
                    h_flex()
                        .px_2()
                        .text_xs()
                        .text_color(cx.theme().muted_foreground)
                        .child(
                            div()
                                .id("status-bar")
                                .role(Role::Status)
                                .test_support()
                                .aria_label(self.status.clone())
                                .child(Label::new(self.status.clone())),
                        ),
                ),
            )
    }
}

fn title_bar_with(leading: impl IntoElement) -> impl IntoElement {
    TitleBar::new().child(h_flex().size_full().px_1().pr_3().gap_2().child(leading))
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

fn status_line(result: Result<Vec<WorkspaceSummary>, String>, cx: &App) -> String {
    let workspaces = match result {
        Ok(workspaces) => format!("{} workspace(s)", workspaces.len()),
        Err(error) => format!("Could not read the workspaces: {error}"),
    };
    format!("{} · {workspaces}", app_state::storage(cx))
}

fn pane(title: &str) -> impl IntoElement {
    v_flex()
        .size_full()
        .min_w_0()
        .min_h_0()
        .p_3()
        .child(Label::new(title))
}

#[cfg(test)]
#[path = "workbench_tests.rs"]
mod tests;
