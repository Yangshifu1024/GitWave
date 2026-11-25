//! Checks of the window shell. They render the real workbench in a test window
//! and drive it the way a user would.

use std::time::{Duration, Instant};

use gpui_kit::component::{ActiveTheme as _, GlobalState, ThemeMode};
use gpui_kit::test::TestWindowExt as _;
use gpui_kit::{
    AnyWindowHandle, App, AppContext as _, Background, Entity, MenuItem, OwnedMenu, OwnedMenuItem,
    Role, TestAppContext, WindowOptions, px, size, solid_background,
};

use gitwave_core::application::use_cases::get_language_settings;

use super::Workbench;
use crate::{
    app_state,
    locales::{self, LanguageChoice},
    menus::{self, SwitchLanguage},
    runtime, workbench,
};

/// Opens the workbench the way `main` does, with a test window in place of the
/// real one. The system reports no language, so the interface starts in English
/// whatever the machine running the tests prefers.
fn open(cx: &mut TestAppContext) -> (AnyWindowHandle, Entity<Workbench>) {
    cx.update(|cx| {
        gpui_kit::init(cx);
        runtime::init(cx);
        app_state::init(cx);
        locales::init(&app_state::context(cx), Vec::new(), cx);
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

/// What gitwave-core has stored for the language.
fn stored_language(cx: &mut TestAppContext) -> Option<String> {
    cx.read(|cx| {
        get_language_settings(&app_state::context(cx))
            .expect("read the stored language")
            .locale
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

// Every test below reads English, so each holds the turn on the process-wide
// language for its whole run: `open` sets the language, and another test
// switching it in between would change the labels.

#[gpui_kit::test]
fn status_bar_counts_the_workspaces_once_core_has_read_them(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let (_, workbench) = open(cx);

    let status = settle_status(cx, &workbench);

    assert_eq!(status, "In-memory state, nothing is saved · 0 workspace(s)");
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

/// Whether a text shows in the last frame.
fn shows(window: &mut gpui_kit::Window, text: &str) -> bool {
    gpui_kit::base::test_support::texts(window)
        .iter()
        .any(|found| found.text() == text && found.visible())
}

/// The shortcut for the sidebar on this platform.
fn sidebar_shortcut() -> &'static str {
    if cfg!(target_os = "macos") {
        "cmd-b"
    } else {
        "ctrl-b"
    }
}

/// The shortcut hides the sidebar and brings it back. The panes keep their
/// places, so the sidebar is hidden, not removed.
#[gpui_kit::test]
fn the_sidebar_shortcut_hides_the_sidebar_and_brings_it_back(cx: &mut TestAppContext) {
    locales::take_language_turn(cx);
    let (window, _) = open(cx);
    render(cx, window);

    let title_shows = |cx: &mut TestAppContext| {
        cx.update_window(window, |_, window, _| shows(window, "Sidebar"))
            .unwrap()
    };
    assert!(title_shows(cx), "the sidebar is shown at first");

    press(cx, window, sidebar_shortcut());
    assert!(!title_shows(cx), "the shortcut hides the sidebar");

    press(cx, window, sidebar_shortcut());
    assert!(title_shows(cx), "the shortcut brings the sidebar back");
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
