//! The application menus. macOS gets them in the system menu bar; Windows and
//! Linux get the menu bar drawn inside the title bar. Both read one definition,
//! so a menu item added here shows up on every platform.
//!
//! Every label comes from the message files, so the menus read the language in
//! force each time they are built. [`init`] therefore has to run after
//! `locales::init`, and the menus are rebuilt whenever the language changes.

use gpui_kit::component::{
    ActiveTheme as _, GlobalState, Theme, ThemeMode, input, menu::AppMenuBar,
};
use gpui_kit::{Action, App, Entity, KeyBinding, Menu, MenuItem};
use rust_i18n::t;

use crate::locales::{self, LanguageChoice};

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

/// Switches the interface language, for the same reason.
#[derive(Action, Clone, PartialEq)]
#[action(namespace = gitwave, no_json)]
pub struct SwitchLanguage(pub LanguageChoice);

/// Builds the menus once and keeps them current. Call after `locales::init`.
pub fn init(cx: &mut App) -> Entity<AppMenuBar> {
    let app_menu_bar = AppMenuBar::new(cx);
    update(app_menu_bar.clone(), cx);

    // The View menu shows which appearance and language are selected, and the
    // labels change with the language, so rebuild the menus when either changes.
    for rebuild in [
        cx.observe_global::<Theme>({
            let app_menu_bar = app_menu_bar.clone();
            move |cx| update(app_menu_bar.clone(), cx)
        }),
        cx.observe_global::<locales::InterfaceLanguage>({
            let app_menu_bar = app_menu_bar.clone();
            move |cx| update(app_menu_bar.clone(), cx)
        }),
    ] {
        rebuild.detach();
    }

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
    let language = locales::choice(cx);
    let mut app_items = vec![
        MenuItem::action(t!("menu.app.aboutGitWave"), AboutGitWave),
        // Updating arrives with the release pipeline. The item stays where users
        // expect to find it.
        MenuItem::action(t!("menu.app.checkUpdates"), CheckForUpdates).disabled(true),
        MenuItem::separator(),
        MenuItem::action(t!("menu.app.settings"), OpenSettings),
        MenuItem::separator(),
    ];
    if is_mac {
        app_items.extend([
            MenuItem::action(t!("menu.app.hide"), HideGitWave),
            MenuItem::action(t!("menu.app.hideOthers"), HideOtherApplications),
            MenuItem::separator(),
        ]);
    }
    app_items.push(MenuItem::action(t!("menu.app.quit"), Quit));

    // Each language names itself, so that a user lost in the wrong one can find theirs.
    let language_items = locales::choices()
        .map(|choice| {
            let name = match choice {
                LanguageChoice::System => t!("common.language.followSystem"),
                LanguageChoice::Fixed(language) => t!(language.name),
            };
            MenuItem::action(name, SwitchLanguage(choice)).checked(language == choice)
        })
        .collect::<Vec<MenuItem>>();

    vec![
        // The application's own name, which every platform shows as it is spelled.
        Menu::new("GitWave").items(app_items),
        Menu::new(t!("menu.menus.edit")).items(vec![
            MenuItem::action(t!("menu.edit.undo"), input::Undo),
            MenuItem::action(t!("menu.edit.redo"), input::Redo),
            MenuItem::separator(),
            MenuItem::action(t!("menu.edit.cut"), input::Cut),
            MenuItem::action(t!("menu.edit.copy"), input::Copy),
            MenuItem::action(t!("menu.edit.paste"), input::Paste),
            MenuItem::action(t!("menu.edit.selectAll"), input::SelectAll),
        ]),
        Menu::new(t!("menu.menus.view")).items(vec![
            MenuItem::action(
                t!("settings.appearance.themeLight"),
                SwitchThemeMode(ThemeMode::Light),
            )
            .checked(!dark),
            MenuItem::action(
                t!("settings.appearance.themeDark"),
                SwitchThemeMode(ThemeMode::Dark),
            )
            .checked(dark),
            MenuItem::separator(),
            MenuItem::submenu(Menu::new(t!("menu.view.language")).items(language_items)),
        ]),
        Menu::new(t!("menu.menus.window")).items(vec![
            MenuItem::action(t!("menu.window.minimize"), Minimize),
            MenuItem::action(t!("menu.window.zoom"), Zoom),
            MenuItem::separator(),
            MenuItem::action(t!("menu.window.bringAllToFront"), BringAllToFront),
        ]),
    ]
}
