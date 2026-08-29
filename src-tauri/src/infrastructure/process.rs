//! Helpers for spawning child processes from the GUI without flashing a console on Windows.

use std::process::Command;

/// Windows `CREATE_NO_WINDOW` — child process does not allocate a console.
#[cfg(windows)]
const CREATE_NO_WINDOW: u32 = 0x08000000;

/// Build a `Command` that will not flash a console window when spawned from the GUI app.
pub fn hidden_command(program: &str) -> Command {
    let mut cmd = Command::new(program);
    hide_console_window(&mut cmd);
    cmd
}

/// Apply platform-specific flags so an existing `Command` stays hidden when spawned.
pub fn hide_console_window(cmd: &mut Command) {
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        cmd.creation_flags(CREATE_NO_WINDOW);
    }
    #[cfg(not(windows))]
    {
        let _ = cmd;
    }
}
