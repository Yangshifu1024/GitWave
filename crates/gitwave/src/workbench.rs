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

use crate::{
    app_state,
    menus::{
        AboutGitWave, BringAllToFront, HideGitWave, HideOtherApplications, Minimize, OpenSettings,
        Quit, SwitchThemeMode, Zoom,
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

pub struct Workbench {
    /// Actions and keys are dispatched from the focused element upwards, so the
    /// workbench takes the focus when the window opens for them to reach it.
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
                .title("About GitWave")
                .child(
                    v_flex()
                        .gap_1()
                        .child(Label::new(concat!("GitWave ", env!("CARGO_PKG_VERSION"))))
                        .child(Label::new(
                            "A local-first Git client with AI collaboration.",
                        )),
                )
                .footer(close_button("close-about"))
        });
    }

    fn open_settings(&mut self, window: &mut Window, cx: &mut Context<Self>) {
        let note = self.note.clone();
        window.open_dialog(cx, move |dialog, _, _| {
            dialog
                .title("Settings")
                .child(
                    v_flex()
                        .gap_2()
                        .child(Label::new("Settings are not wired up yet."))
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
            .track_focus(&self.focus)
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
            .on_action(
                cx.listener(|this, _: &AboutGitWave, window, cx| this.open_about(window, cx)),
            )
            .on_action(
                cx.listener(|this, _: &OpenSettings, window, cx| this.open_settings(window, cx)),
            )
            .on_action(cx.listener(|_, action: &SwitchThemeMode, window, cx| {
                Theme::change(action.0, Some(window), cx);
            }))
            .on_action(|_: &HideGitWave, _, cx| cx.hide())
            .on_action(|_: &HideOtherApplications, _, cx| cx.hide_other_apps())
            .on_action(|_: &Minimize, window, _| window.minimize_window())
            .on_action(|_: &Zoom, window, _| window.zoom_window())
            .on_action(|_: &BringAllToFront, _, cx| cx.activate(true))
            .on_action(|_: &Quit, _, cx| cx.quit())
            .child(title_bar)
            .child(
                div().flex_1().min_h_0().child(
                    h_resizable("workbench-panes")
                        // Panels keep their sizes by position, so the sidebar
                        // stays a child when hidden instead of being removed.
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
        .label("Close")
        .on_click(|_, window, cx| window.close_dialog(cx))
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
