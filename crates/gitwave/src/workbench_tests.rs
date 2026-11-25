//! Checks of the window shell. They render the real workbench in a test window
//! and drive it the way a user would.

use std::time::{Duration, Instant};

use gpui_kit::component::{ActiveTheme as _, ThemeMode};
use gpui_kit::test::TestWindowExt as _;
use gpui_kit::{
    AnyWindowHandle, AppContext as _, Entity, MenuItem, TestAppContext, WindowOptions, px, size,
};

use super::Workbench;
use crate::{app_state, menus, runtime, workbench};

/// Opens the workbench the way `main` does, with a test window in place of the
/// real one.
fn open(cx: &mut TestAppContext) -> (AnyWindowHandle, Entity<Workbench>) {
    cx.update(|cx| {
        gpui_kit::init(cx);
        runtime::init(cx);
        app_state::init(cx);
        workbench::init(cx);
        let app_menu_bar = menus::init(cx);
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

/// The status bar waits for gitwave-core, which answers on a tokio thread.
fn settle_status(cx: &mut TestAppContext, workbench: &Entity<Workbench>) -> String {
    cx.executor().allow_parking();
    let deadline = Instant::now() + Duration::from_secs(10);
    loop {
        cx.executor().run_until_parked();
        let status = cx.read(|cx| workbench.read(cx).status.to_string());
        if !status.contains("Reading workspaces") {
            return status;
        }
        assert!(
            Instant::now() < deadline,
            "the status never settled: {status}"
        );
        std::thread::sleep(Duration::from_millis(5));
    }
}

#[gpui_kit::test]
fn status_bar_counts_the_workspaces_once_core_has_read_them(cx: &mut TestAppContext) {
    let (_, workbench) = open(cx);

    let status = settle_status(cx, &workbench);

    assert_eq!(status, "In-memory state, nothing is saved · 0 workspace(s)");
}

#[gpui_kit::test]
fn settings_shortcut_opens_a_dialog_with_a_text_input(cx: &mut TestAppContext) {
    let (window, _) = open(cx);
    let shortcut = if cfg!(target_os = "macos") {
        "cmd-,"
    } else {
        "ctrl-,"
    };

    cx.update_window(window, |_, window, cx| {
        window.render_frame(cx);
        assert!(window.try_find("settings-note").is_none());
        window.press(shortcut, cx);
        window.render_frame(cx);
        assert!(
            window.find("settings-note").visible(),
            "the settings dialog did not open"
        );
    })
    .unwrap();
}

#[gpui_kit::test]
fn switching_the_appearance_updates_the_view_menu(cx: &mut TestAppContext) {
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
