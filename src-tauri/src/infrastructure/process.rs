//! Helpers for spawning child processes from the GUI without flashing a console on Windows.

use std::io::Read;
use std::process::{Child, ExitStatus, Output};
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration, Instant};

use std::process::Command;

/// Windows `CREATE_NO_WINDOW` — child process does not allocate a console.
#[cfg(windows)]
const CREATE_NO_WINDOW: u32 = 0x08000000;

/// Poll interval for the timed wait helpers below; keeps cancel latency and
/// test runtimes low without busy-spinning.
const WAIT_POLL_INTERVAL: Duration = Duration::from_millis(25);

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

/// Wait for `child` to exit, collecting its piped stdout/stderr, but give up
/// after `timeout` or once `cancel` flips: the child is killed and `None`
/// returned. The child's stdin is closed on entry (callers write their
/// request before waiting — the same EOF `Child::wait` would provide), and
/// output is only read after exit, which is safe for the tiny payloads the
/// credential protocol exchanges.
pub fn wait_with_output_timeout(
    mut child: Child,
    timeout: Duration,
    cancel: Option<&AtomicBool>,
) -> std::io::Result<Option<Output>> {
    drop(child.stdin.take());
    let Some(status) = wait_exit(&mut child, timeout, cancel)? else {
        return Ok(None);
    };
    let mut stdout = Vec::new();
    if let Some(mut pipe) = child.stdout.take() {
        pipe.read_to_end(&mut stdout)?;
    }
    let mut stderr = Vec::new();
    if let Some(mut pipe) = child.stderr.take() {
        pipe.read_to_end(&mut stderr)?;
    }
    Ok(Some(Output {
        status,
        stdout,
        stderr,
    }))
}

/// Status-only variant of [`wait_with_output_timeout`] for children whose
/// output goes to `Stdio::null()`. Closes stdin on entry for the same
/// EOF reason.
pub fn wait_timeout(child: &mut Child, timeout: Duration) -> std::io::Result<Option<ExitStatus>> {
    drop(child.stdin.take());
    wait_exit(child, timeout, None)
}

/// Poll `try_wait` until exit, deadline, or cancellation; kills the child on
/// the latter two. `Ok(None)` means "did not exit in time".
fn wait_exit(
    child: &mut Child,
    timeout: Duration,
    cancel: Option<&AtomicBool>,
) -> std::io::Result<Option<ExitStatus>> {
    let deadline = Instant::now() + timeout;
    loop {
        if let Some(status) = child.try_wait()? {
            return Ok(Some(status));
        }
        let cancelled = cancel.is_some_and(|flag| flag.load(Ordering::Relaxed));
        if cancelled || Instant::now() >= deadline {
            // Kill, then reap so no zombie is left behind.
            let _ = child.kill();
            let _ = child.wait();
            return Ok(None);
        }
        std::thread::sleep(WAIT_POLL_INTERVAL);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::process::Stdio;

    /// A child that outlives any test timeout. Windows has no `sleep`
    /// binary (`timeout` is a shell builtin), so ping stands in.
    fn sleeper() -> Child {
        #[cfg(windows)]
        let mut cmd = hidden_command("ping");
        #[cfg(not(windows))]
        let mut cmd = hidden_command("sleep");
        #[cfg(windows)]
        cmd.args(["-n", "30", "127.0.0.1"]);
        #[cfg(not(windows))]
        cmd.args(["30"]);
        cmd.stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .expect("sleeper child must spawn")
    }

    #[test]
    fn timed_wait_collects_output_for_exiting_child() {
        let child = hidden_command("git")
            .arg("--version")
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .expect("git must spawn");
        let output = wait_with_output_timeout(child, Duration::from_secs(30), None)
            .expect("wait must not error")
            .expect("git --version exits on its own");
        assert!(output.status.success());
        assert!(String::from_utf8_lossy(&output.stdout).contains("git version"));
    }

    #[test]
    fn timed_wait_kills_child_at_deadline() {
        let start = Instant::now();
        let result = wait_with_output_timeout(sleeper(), Duration::from_millis(200), None)
            .expect("wait must not error");
        assert!(result.is_none(), "sleeping child must time out");
        assert!(
            start.elapsed() < Duration::from_secs(10),
            "must return near the deadline, not after the child's own 30s"
        );
    }

    #[test]
    fn timed_wait_kills_child_on_cancel_flag() {
        let cancelled = AtomicBool::new(true); // pre-set: cancel beats the 30s budget
        let start = Instant::now();
        let result = wait_with_output_timeout(sleeper(), Duration::from_secs(30), Some(&cancelled))
            .expect("wait must not error");
        assert!(result.is_none(), "cancelled child must be killed");
        assert!(start.elapsed() < Duration::from_secs(5));
    }
}
