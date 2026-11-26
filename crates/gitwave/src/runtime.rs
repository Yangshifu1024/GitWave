//! The tokio runtime gitwave-core's use cases run on, bridged to GPUI's tasks.
//!
//! GPUI has its own executors and does not depend on tokio, while gitwave-core's
//! use cases need a tokio runtime (timeouts, the blocking pool, reqwest).

use gitwave_core::domain::error::{AppError, Result as CoreResult};
use gitwave_core::domain::error_codes as codes;
use gpui_kit::{App, AppContext as _, Global, Task};

struct Runtime(tokio::runtime::Runtime);

impl Global for Runtime {}

/// Starts the runtime. Call once, before any use case runs.
pub fn init(cx: &mut App) {
    let runtime = tokio::runtime::Builder::new_multi_thread()
        // GPUI already has its own executors, so keep this one small.
        .worker_threads(2)
        .enable_all()
        .build()
        .expect("failed to start the tokio runtime");
    cx.set_global(Runtime(runtime));
}

/// Runs a use case on tokio's blocking pool and hands its result back as a
/// GPUI task. Use it for work that is slow or open to take long, such as a
/// network sync or a walk over a large repository, so that the main thread
/// never waits on it. A short local write, storing a setting among them, is
/// done on the main thread instead; `docs/tech/architecture/00-overview.md`
/// has the rule.
///
/// The error comes back as it is, so that its code can be told apart. A panic
/// inside the job becomes an error of its own.
pub fn use_case<T, F>(cx: &App, job: F) -> Task<CoreResult<T>>
where
    T: Send + 'static,
    F: FnOnce() -> CoreResult<T> + Send + 'static,
{
    let join = cx.global::<Runtime>().0.handle().spawn_blocking(job);
    cx.background_spawn(async move {
        join.await.unwrap_or_else(|error| {
            Err(AppError::unknown_with(
                codes::usecases::BACKGROUND_JOB_FAILED,
                format!("the job did not finish: {error}"),
                &[("error", error.to_string())],
            ))
        })
    })
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use gpui_kit::TestAppContext;

    use super::*;

    /// The job runs on one of tokio's threads while the main thread stays free,
    /// and its value comes back through the GPUI task.
    #[gpui_kit::test]
    async fn a_use_case_runs_elsewhere_and_keeps_its_value(cx: &mut TestAppContext) {
        cx.update(|cx| {
            gpui_kit::init(cx);
            init(cx);
        });
        // The job finishes on a real tokio thread, which the test scheduler
        // does not control, so it has to let this thread wait for it.
        cx.executor().allow_parking();
        let caller = std::thread::current().id();

        let task = cx.update(|cx| {
            use_case(cx, move || {
                std::thread::sleep(Duration::from_millis(20));
                Ok((std::thread::current().id(), 7))
            })
        });
        let (worker, value) = task.await.expect("the job finished");

        assert_ne!(worker, caller, "the job must not run on the calling thread");
        assert_eq!(value, 7);
    }

    #[gpui_kit::test]
    async fn a_use_case_keeps_its_error_as_it_is(cx: &mut TestAppContext) {
        cx.update(|cx| {
            gpui_kit::init(cx);
            init(cx);
        });
        cx.executor().allow_parking();

        let task = cx.update(|cx| {
            use_case(cx, || {
                Err::<(), _>(AppError::unknown(
                    codes::usecases::LANGUAGE_INVALID,
                    "not a tag",
                ))
            })
        });
        let error = task.await.expect_err("the error is passed on");

        assert_eq!(error.code(), codes::usecases::LANGUAGE_INVALID);
    }

    #[gpui_kit::test]
    async fn a_panicking_use_case_comes_back_as_an_error(cx: &mut TestAppContext) {
        cx.update(|cx| {
            gpui_kit::init(cx);
            init(cx);
        });
        cx.executor().allow_parking();

        let task = cx.update(|cx| use_case::<(), _>(cx, || panic!("job failed")));
        let error = task.await.expect_err("a panic must not cross into the app");

        assert_eq!(error.code(), codes::usecases::BACKGROUND_JOB_FAILED);
        assert!(
            error.message().contains("job failed"),
            "unexpected error: {}",
            error.message()
        );
    }
}
