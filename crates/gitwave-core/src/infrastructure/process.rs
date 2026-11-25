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

/// Post-exit grace for drain threads to hit EOF before we detach (see
/// [`join_drain`]): long enough for pipe buffers to flush, short enough
/// that a pipe held open by a daemonized grandchild cannot hang the UI.
const DRAIN_JOIN_GRACE: Duration = Duration::from_secs(5);

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
/// request before waiting — the same EOF `Child::wait` would provide).
///
/// Stdout/stderr are drained on dedicated threads *before* waiting: a child
/// producing more than the OS pipe buffer (~64 KiB) would otherwise block
/// on write while we block on exit — a classic deadlock for large outputs.
pub fn wait_with_output_timeout(
    mut child: Child,
    timeout: Duration,
    cancel: Option<&AtomicBool>,
) -> std::io::Result<Option<Output>> {
    drop(child.stdin.take());
    let stdout_drain = child.stdout.take().map(|mut pipe| {
        std::thread::spawn(move || {
            let mut buf = Vec::new();
            let _ = pipe.read_to_end(&mut buf);
            buf
        })
    });
    let stderr_drain = child.stderr.take().map(|mut pipe| {
        std::thread::spawn(move || {
            let mut buf = Vec::new();
            let _ = pipe.read_to_end(&mut buf);
            buf
        })
    });
    let Some(status) = wait_exit(&mut child, timeout, cancel)? else {
        // Timeout/cancel: the kill inside `wait_exit` closes the pipes, so
        // the drain threads reach EOF on their own — detach rather than
        // block the caller on a join.
        return Ok(None);
    };
    // A daemonized grandchild can inherit the pipes and hold them open after
    // our child exits; joining unconditionally would hang the caller on its
    // output. Bound the wait — detached threads keep draining in the
    // background and exit on EOF by themselves.
    let stdout = join_drain(stdout_drain);
    let stderr = join_drain(stderr_drain);
    Ok(Some(Output {
        status,
        stdout,
        stderr,
    }))
}

/// Join a drain thread, waiting at most [`DRAIN_JOIN_GRACE`] for EOF after
/// the child already exited. Times out to empty (with a warning) rather
/// than hanging the caller on an inherited pipe.
fn join_drain(handle: Option<std::thread::JoinHandle<Vec<u8>>>) -> Vec<u8> {
    let Some(handle) = handle else {
        return Vec::new();
    };
    let deadline = Instant::now() + DRAIN_JOIN_GRACE;
    while !handle.is_finished() && Instant::now() < deadline {
        std::thread::sleep(WAIT_POLL_INTERVAL);
    }
    if handle.is_finished() {
        handle.join().unwrap_or_default()
    } else {
        tracing::warn!("drain thread still alive after child exit; detaching with empty output");
        Vec::new()
    }
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
            kill_tree(child);
            let _ = child.wait();
            return Ok(None);
        }
        std::thread::sleep(WAIT_POLL_INTERVAL);
    }
}

/// Kill `child` and, where possible, the process tree it spawned.
///
/// Windows uses `taskkill /T /F` so child processes die with their parent
/// (falling back to `child.kill()` when taskkill itself cannot run).
/// Unix keeps a plain `child.kill()`: only the direct child is signaled,
/// so grandchildren that daemonize into their own process group survive.
/// A `setsid`/`killpg` group kill would close that gap but needs `nix` or
/// `libc`, neither of which is a dependency (see Cargo.toml) — the
/// limitation stands until one is added.
#[cfg(windows)]
fn kill_tree(child: &mut Child) {
    let pid = child.id().to_string();
    let ok = hidden_command("taskkill")
        .args(["/PID", &pid, "/T", "/F"])
        .stdin(std::process::Stdio::null())
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .status()
        .is_ok_and(|s| s.success());
    if !ok {
        let _ = child.kill();
    }
}

/// Unix process-group kill (see `kill_tree` docs): plain `child.kill()`
/// until `nix`/`libc` is available for `setsid`/`killpg`.
#[cfg(not(windows))]
fn kill_tree(child: &mut Child) {
    let _ = child.kill();
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

    /// 200k output lines (~2 MB, far past the 64 KiB pipe buffer) must be
    /// collected without deadlocking: the drain threads keep the pipes
    /// empty while the parent waits for exit. Generators are platform
    /// split — `cmd /C for /L` on Windows (no `sh`/`seq` there), POSIX
    /// `yes | head` elsewhere (no `seq` on stock macOS).
    #[test]
    fn timed_wait_handles_large_output_without_deadlock() {
        #[cfg(windows)]
        let mut cmd = {
            let mut c = hidden_command("cmd");
            c.args(["/C", "for /L %i in (1,1,200000) do @echo line-%i"]);
            c
        };
        #[cfg(not(windows))]
        let mut cmd = {
            let mut c = hidden_command("sh");
            c.args(["-c", "yes | head -n 200000"]);
            c
        };
        let child = cmd
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .expect("output generator must spawn");
        let output = wait_with_output_timeout(child, Duration::from_secs(60), None)
            .expect("wait must not error")
            .expect("generator exits on its own");
        assert!(output.status.success());
        let newlines = output.stdout.iter().filter(|&&b| b == b'\n').count();
        assert!(newlines >= 200_000, "expected 200k lines, got {newlines}");
    }
}
