// Prevents an additional console window on Windows in release, DO NOT REMOVE!!
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod app_state;
mod error_text;
mod history;
mod locales;
mod logging;
mod menus;
mod runtime;
mod session;
mod time_text;
mod workbench;

// rust-i18n reads `locales/<domain>.<locale>.json` at compile time. `t!` forwards
// to a macro that this call defines at the crate root, so the registration has
// to live here.
rust_i18n::i18n!("locales", fallback = "en");

use gpui_kit::component::TitleBar;
use gpui_kit::{AppContext as _, WindowBounds, WindowOptions, px, size};
use workbench::Workbench;

fn main() {
    gpui_kit::application()
        .with_assets(gpui_kit::assets::Assets)
        .run(|cx| {
            // First, so that everything the startup has to say is written.
            logging::init(cx);
            // libgit2 refuses repositories owned by another account, such as
            // one cloned from an elevated prompt. Every path comes from the
            // user's own list, so the check is off, as v0.11.0 had it.
            gitwave_core::bootstrap::disable_git_owner_validation();
            // This must be called before using any GPUI Component features.
            gpui_kit::init(cx);
            runtime::init(cx);
            app_state::init(cx);
            // Before the first window opens, so that no window shows in the wrong language first.
            locales::init(&app_state::context(cx), locales::system_languages(), cx);
            workbench::init(cx);
            let app_menu_bar = menus::init(cx);

            let options = WindowOptions {
                window_bounds: Some(WindowBounds::centered(None, size(px(1200.), px(800.)), cx)),
                window_min_size: Some(size(px(1024.), px(768.))),
                app_id: Some("desktop.gitwave.work".into()),
                ..TitleBar::window_options()
            };
            // Opens a window with a `Root` wrapping the view, so dialogs, sheets,
            // notifications and menus work in it.
            gpui_kit::open_window(options, cx, |window, cx| {
                cx.new(|cx| Workbench::new(app_menu_bar, window, cx))
            })
            .expect("Failed to open the GitWave window");
            cx.activate(true);
        });
}
