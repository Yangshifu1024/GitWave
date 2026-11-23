//! Read-only Git history for the native desktop application.

use std::io::{self, BufRead, Write};
use std::path::PathBuf;

use git2::Repository;
use gitwave_lib::application::use_cases::read_image_content;
use gitwave_lib::domain::error::AppError;
use gitwave_lib::domain::error_codes as codes;
use gitwave_lib::infrastructure::git::{diff_preview, history};
use serde::Deserialize;
use serde_json::{json, Value};

#[derive(Deserialize)]
#[serde(tag = "method", rename_all = "snake_case")]
enum Request {
    Repository,
    History {
        cursor: Option<String>,
        filter: Option<String>,
    },
    Commit {
        sha: String,
    },
    Diff {
        sha: String,
        path: Option<String>,
        #[serde(default)]
        expanded: bool,
    },
    Image {
        path: String,
        oid: String,
    },
}

fn dispatch(repo: &Repository, request: Request) -> Result<Value, AppError> {
    let result = match request {
        Request::Repository => {
            let head = repo.head().ok();
            let branch = head.as_ref().and_then(|head| head.shorthand().ok());
            let sha = head.as_ref().and_then(|head| head.target());
            Ok(
                json!({ "branch": branch, "head": sha.map(|sha| sha.to_string()),
                "branches": history::list_branches(repo)? }),
            )
        }
        Request::History { cursor, filter } => serde_json::to_value(history::commit_log_page(
            repo,
            300,
            filter.as_deref(),
            cursor.as_deref(),
        )?),
        Request::Commit { sha } => serde_json::to_value(history::commit_details(repo, &sha)?),
        Request::Diff {
            sha,
            path,
            expanded,
        } => serde_json::to_value(diff_preview::preview(
            repo,
            path.as_deref(),
            None,
            Some(&sha),
            None,
            expanded,
        )?),
        Request::Image { path, oid } => {
            serde_json::to_value(read_image_content(repo, &path, Some(&oid))?)
        }
    };
    result.map_err(|error| AppError::unknown(codes::git::GIT_ERROR, error.to_string()))
}

fn main() -> io::Result<()> {
    let path = std::env::args_os()
        .nth(1)
        .map(PathBuf::from)
        .ok_or_else(|| io::Error::new(io::ErrorKind::InvalidInput, "expected a repository path"))?;
    let repo = Repository::open(path).map_err(|error| {
        AppError::unknown_with(
            codes::usecases::REPO_OPEN_FAILED,
            format!("git open: {error}"),
            &[("error", error.to_string())],
        )
    });
    let mut output = io::stdout().lock();
    for line in io::stdin().lock().lines() {
        let response = match serde_json::from_str::<Request>(&line?) {
            Ok(request) => match repo
                .as_ref()
                .map_err(Clone::clone)
                .and_then(|repo| dispatch(repo, request))
            {
                Ok(value) => json!({ "result": value }),
                Err(error) => json!({ "error": error }),
            },
            Err(error) => json!({ "error": { "message": error.to_string() } }),
        };
        serde_json::to_writer(&mut output, &response)?;
        output.write_all(b"\n")?;
        output.flush()?;
    }
    Ok(())
}
