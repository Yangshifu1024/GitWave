//! The application menus. macOS gets them in the system menu bar; Windows and
//! Linux get the menu bar drawn inside the title bar. Both read one definition,
//! so a menu item added here shows up on every platform.

use gpui_kit::component::{
    ActiveTheme as _, GlobalState, Theme, ThemeMode, input, menu::AppMenuBar,
};
use gpui_kit::{Action, App, Entity, KeyBinding, Menu, MenuItem};

gpui_kit::actions!(
    gitwave,
    [
        AboutGitWave,
        BringAllToFront,
        CheckForUpdates,
        HideGitWave,
        HideOtherApplications,
        Minimize,
        OpenSettings,
        Quit,
        Zoom,
    ]
);

/// Switches the appearance. It carries the target mode, so it is declared as a
/// struct instead of through the `actions!` list.
#[derive(Action, Clone, PartialEq)]
#[action(namespace = gitwave, no_json)]
pub struct SwitchThemeMode(pub ThemeMode);

pub fn init(cx: &mut App) -> Entity<AppMenuBar> {
    let app_menu_bar = AppMenuBar::new(cx);
    update(app_menu_bar.clone(), cx);

    // The View menu shows which appearance is selected, so rebuild it on change.
    cx.observe_global::<Theme>({
        let app_menu_bar = app_menu_bar.clone();
        move |cx| update(app_menu_bar.clone(), cx)
    })
    .detach();

    bind_keys(cx);
    app_menu_bar
}

fn update(app_menu_bar: Entity<AppMenuBar>, cx: &mut App) {
    cx.set_menus(build_menus(cx));
    let menus = build_menus(cx)
        .into_iter()
        .map(|menu| menu.owned())
        .collect();
    GlobalState::global_mut(cx).set_app_menus(menus);
    app_menu_bar.update(cx, |menu_bar, cx| menu_bar.reload(cx));
}

/// Binds the shortcuts that must work wherever the focus is, including inside
/// a dialog. The menus show them next to their items.
fn bind_keys(cx: &mut App) {
    let is_mac = cx.platform_info().is_mac();
    cx.bind_keys([
        KeyBinding::new(if is_mac { "cmd-," } else { "ctrl-," }, OpenSettings, None),
        KeyBinding::new(if is_mac { "cmd-q" } else { "ctrl-q" }, Quit, None),
        KeyBinding::new("cmd-m", Minimize, None),
        KeyBinding::new("cmd-h", HideGitWave, None),
        KeyBinding::new("cmd-alt-h", HideOtherApplications, None),
    ]);
}

pub(crate) fn build_menus(cx: &App) -> Vec<Menu> {
    let is_mac = cx.platform_info().is_mac();
    let dark = cx.theme().mode.is_dark();
    let mut app_items = vec![
        MenuItem::action("About GitWave", AboutGitWave),
        // Updating arrives with the release pipeline. The item stays where users
        // expect to find it.
        MenuItem::action("Check for Updates…", CheckForUpdates).disabled(true),
        MenuItem::separator(),
        MenuItem::action("Settings…", OpenSettings),
        MenuItem::separator(),
    ];
    if is_mac {
        app_items.extend([
            MenuItem::action("Hide GitWave", HideGitWave),
            MenuItem::action("Hide Others", HideOtherApplications),
            MenuItem::separator(),
        ]);
    }
    app_items.push(MenuItem::action("Quit GitWave", Quit));

    vec![
        Menu::new("GitWave").items(app_items),
        Menu::new("Edit").items(vec![
            MenuItem::action("Undo", input::Undo),
            MenuItem::action("Redo", input::Redo),
            MenuItem::separator(),
            MenuItem::action("Cut", input::Cut),
            MenuItem::action("Copy", input::Copy),
            MenuItem::action("Paste", input::Paste),
            MenuItem::action("Select All", input::SelectAll),
        ]),
        Menu::new("View").items(vec![
            MenuItem::action("Light", SwitchThemeMode(ThemeMode::Light)).checked(!dark),
            MenuItem::action("Dark", SwitchThemeMode(ThemeMode::Dark)).checked(dark),
        ]),
        Menu::new("Window").items(vec![
            MenuItem::action("Minimize", Minimize),
            MenuItem::action("Zoom", Zoom),
            MenuItem::separator(),
            MenuItem::action("Bring All to Front", BringAllToFront),
        ]),
    ]
}
