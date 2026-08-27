//! macOS window chrome tweaks for Native Studio shell.
//!
//! Titlebar double-click must not use Tauri's `internal_toggle_maximize` (animated
//! AppKit zoom). The frontend calls `toggle_instant_zoom` instead.

#[cfg(target_os = "macos")]
mod imp {
    use objc2::MainThreadMarker;
    use objc2_app_kit::{NSScreen, NSWindow};
    use objc2_foundation::NSRect;
    use std::sync::Mutex;
    use tauri::window::Color;
    use tauri::WebviewWindow;

    static SAVED_FRAME: Mutex<Option<NSRect>> = Mutex::new(None);

    fn ns_window(window: &WebviewWindow) -> Result<&NSWindow, String> {
        let ptr = window.ns_window().map_err(|e| e.to_string())? as *mut NSWindow;
        Ok(unsafe { &*ptr })
    }

    fn visible_frame(ns_window: &NSWindow, mtm: MainThreadMarker) -> NSRect {
        if let Some(screen) = ns_window.screen() {
            return screen.visibleFrame();
        }
        NSScreen::mainScreen(mtm)
            .map(|screen| screen.visibleFrame())
            .unwrap_or_else(|| ns_window.frame())
    }

    pub fn configure_window(window: &WebviewWindow) -> Result<(), String> {
        window
            .set_background_color(Some(Color(244, 246, 248, 255)))
            .map_err(|e| e.to_string())?;

        Ok(())
    }

    pub fn toggle_instant_zoom(window: &WebviewWindow) -> Result<(), String> {
        let mtm = MainThreadMarker::new()
            .ok_or_else(|| "window zoom must run on the main thread".to_string())?;
        let ns_window = ns_window(window)?;

        let mut saved = SAVED_FRAME
            .lock()
            .map_err(|_| "window zoom state lock poisoned".to_string())?;

        if let Some(frame) = saved.take() {
            ns_window.setFrame_display_animate(frame, true, false);
            return Ok(());
        }

        let current = ns_window.frame();
        let target = visible_frame(ns_window, mtm);
        *saved = Some(current);
        ns_window.setFrame_display_animate(target, true, false);
        Ok(())
    }
}

#[cfg(target_os = "macos")]
pub use imp::{configure_window, toggle_instant_zoom};

#[cfg(not(target_os = "macos"))]
pub fn configure_window(_window: &tauri::WebviewWindow) -> Result<(), String> {
    Ok(())
}

#[cfg(not(target_os = "macos"))]
pub fn toggle_instant_zoom(_window: &tauri::WebviewWindow) -> Result<(), String> {
    Ok(())
}
