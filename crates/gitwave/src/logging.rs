//! Process-wide logging: gitwave-core's tracing subscriber, started with the
//! application, kept alive for as long as it runs and flushed when it quits.

use gitwave_core::infrastructure::observability::tracing as subscriber;
use gitwave_core::infrastructure::observability::tracing::WorkerGuard;
use gpui_kit::{App, Global};

/// The subscriber's writer. Dropping it flushes what is buffered and stops the
/// writer thread, and with it all logging.
struct LogGuard {
    /// `None` once the application is quitting and the writer has been
    /// dropped on purpose.
    writer: Option<WorkerGuard>,
}

impl Global for LogGuard {}

/// Starts logging and keeps it running. When the log directory cannot be
/// created the subscriber logs to stderr instead and says so there.
pub fn init(cx: &mut App) {
    keep(subscriber::init(), cx);
}

/// Keeps the writer until the application quits, and drops it then.
fn keep(writer: Option<WorkerGuard>, cx: &mut App) {
    let Some(writer) = writer else {
        return;
    };
    cx.set_global(LogGuard {
        writer: Some(writer),
    });
    // Quitting does not run destructors on every platform: macOS terminates
    // the process. Dropping the writer is what flushes the lines still held
    // in its buffer, so it is dropped by hand when the quit begins.
    cx.on_app_quit(|cx| {
        cx.global_mut::<LogGuard>().writer = None;
        async {}
    })
    .detach();
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use gpui_kit::TestAppContext;

    use super::{LogGuard, keep, subscriber};

    /// A directory of this test's own under the system's temporary directory.
    fn temp_dir() -> PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "gitwave-logging-test-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_millis()
        ));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        dir
    }

    /// The writer is kept while the application runs, which is what keeps
    /// logging alive, and dropped when it quits, which is what flushes the
    /// last lines.
    #[gpui_kit::test]
    fn the_writer_lives_until_the_application_quits(cx: &mut TestAppContext) {
        let dir = temp_dir();
        cx.update(|cx| {
            keep(subscriber::init_in(&dir), cx);
            assert!(
                cx.global::<LogGuard>().writer.is_some(),
                "the writer is kept"
            );
            cx.shutdown();
            assert!(
                cx.global::<LogGuard>().writer.is_none(),
                "the writer is dropped when the application quits"
            );
        });
        let _ = std::fs::remove_dir_all(&dir);
    }
}
