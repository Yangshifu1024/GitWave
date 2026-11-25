//! The main window: a self-drawn title bar over the sidebar, the list pane and the
//! diff inspector, with an operation status bar underneath.

use gpui_kit::component::{
    ActiveTheme as _, StyledExt as _, TitleBar, h_flex,
    label::Label,
    resizable::{h_resizable, resizable_panel},
    status_bar::StatusBar,
    v_flex,
};
use gpui_kit::{
    App, Context, InteractiveElement as _, IntoElement, KeyBinding, ParentElement as _, Render,
    Styled as _, Window, div,
};

gpui_kit::actions!(gitwave, [ToggleSidebar, Quit]);

/// Binds the shortcuts that work everywhere in the window.
pub fn init(cx: &mut App) {
    let is_mac = cx.platform_info().is_mac();
    cx.bind_keys([
        KeyBinding::new(
            if is_mac { "cmd-b" } else { "ctrl-b" },
            ToggleSidebar,
            Some("GitWave"),
        ),
        KeyBinding::new(
            if is_mac { "cmd-q" } else { "ctrl-q" },
            Quit,
            Some("GitWave"),
        ),
    ]);
}

pub struct Workbench {
    sidebar_visible: bool,
}

impl Workbench {
    pub fn new(window: &mut Window, _cx: &mut Context<Self>) -> Self {
        window.set_window_title("GitWave");
        Self {
            sidebar_visible: true,
        }
    }
}

impl Render for Workbench {
    fn render(&mut self, window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        let rem = window.rem_size();
        v_flex()
            .id("workbench")
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
            .on_action(|_: &Quit, _, cx| cx.quit())
            .child(
                TitleBar::new().child(
                    h_flex()
                        .size_full()
                        .px_1()
                        .pr_3()
                        .gap_2()
                        .child(div().text_sm().font_medium().child("GitWave")),
                ),
            )
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
                        .child(Label::new("No repository selected")),
                ),
            )
    }
}

fn pane(title: &str) -> impl IntoElement {
    v_flex()
        .size_full()
        .min_w_0()
        .min_h_0()
        .p_3()
        .child(Label::new(title))
}
