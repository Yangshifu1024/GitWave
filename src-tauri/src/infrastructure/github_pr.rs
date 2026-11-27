//! Create a same-repository PR through the user's authenticated GitHub CLI.
use crate::domain::error::{AppError, Result};
use crate::domain::error_codes::infra as codes;
use crate::infrastructure::process::{hidden_command, wait_with_output_timeout};
use git2::Repository;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::{io::Write, process::Stdio, time::Duration};

#[derive(Serialize, Deserialize, PartialEq, Debug)]
pub struct Target {
    pub repository: String,
    pub head: String,
    pub oid: String,
    pub base: String,
}

fn cli_error(error: impl ToString) -> AppError {
    let detail = error.to_string();
    AppError::protocol_with(
        codes::GITHUB_CLI_ERROR,
        format!("GitHub CLI: {detail}"),
        &[("detail", detail)],
    )
}
fn slug(remote: &str) -> Option<String> {
    let path = [
        "https://github.com/",
        "git@github.com:",
        "ssh://git@github.com/",
    ]
    .iter()
    .find_map(|prefix| remote.strip_prefix(prefix))?
    .trim_end_matches('/');
    let path = path.strip_suffix(".git").unwrap_or(path);
    let (owner, name) = path.split_once('/')?;
    if [owner, name].iter().any(|v| {
        v.is_empty()
            || matches!(*v, "." | "..")
            || !v
                .bytes()
                .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'-' | b'_' | b'.'))
    }) {
        return None;
    }
    Some(format!("{owner}/{name}"))
}
pub fn target(repo: &Repository) -> Result<Target> {
    let origin = repo.find_remote("origin")?;
    let repository = slug(origin.url()?).ok_or_else(|| {
        AppError::protocol(
            codes::GITHUB_ORIGIN,
            "A github.com origin remote is required.",
        )
    })?;
    let reference = repo.head()?;
    if !reference.is_branch() {
        return Err(AppError::protocol(
            codes::GITHUB_DETACHED,
            "Check out a branch before creating a pull request.",
        ));
    }
    let head = reference.shorthand()?.to_owned();
    let base = crate::infrastructure::git::branch::pr_base(repo).unwrap_or_default();
    Ok(Target {
        repository,
        oid: reference.peel_to_commit()?.id().to_string(),
        head,
        base: base.strip_prefix("origin/").unwrap_or(&base).to_owned(),
    })
}
fn api(endpoint: &str, body: Option<&Value>) -> Result<Value> {
    let mut cmd = hidden_command("gh");
    cmd.args(["api", "--hostname", "github.com", endpoint])
        .env("GH_PROMPT_DISABLED", "1")
        .env_remove("GH_DEBUG")
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    if body.is_some() {
        cmd.args(["--method", "POST", "--input", "-"])
            .stdin(Stdio::piped());
    } else {
        cmd.stdin(Stdio::null());
    }
    run_api_command(cmd, body, Duration::from_secs(45))
}

fn run_api_command(
    mut cmd: std::process::Command,
    body: Option<&Value>,
    timeout: Duration,
) -> Result<Value> {
    let mut child = cmd.spawn().map_err(|e| {
        AppError::protocol_with(
            codes::GITHUB_CLI_START,
            format!("Could not start GitHub CLI (gh): {e}"),
            &[("detail", e.to_string())],
        )
    })?;
    // A CLI blocked on authentication may never read stdin. Feed the body
    // concurrently so the timeout covers the complete request.
    let writer = body.map(|body| {
        let mut stdin = child.stdin.take().expect("piped request body");
        let bytes = body.to_string();
        std::thread::spawn(move || stdin.write_all(bytes.as_bytes()))
    });
    let output = wait_with_output_timeout(child, timeout, None)
        .map_err(cli_error)?
        .ok_or_else(|| {
            AppError::protocol(
                codes::GITHUB_TIMED_OUT,
                "GitHub CLI timed out. Check open pull requests before retrying.",
            )
        })?;
    if !output.status.success() {
        return Err(cli_error(String::from_utf8_lossy(&output.stderr).trim()));
    }
    if let Some(writer) = writer.filter(|writer| writer.is_finished()) {
        writer
            .join()
            .map_err(|_| cli_error("request body writer failed"))?
            .map_err(cli_error)?;
    }
    serde_json::from_slice(&output.stdout).map_err(cli_error)
}
pub fn create(
    repo: &Repository,
    expected: &Target,
    base: &str,
    title: &str,
    body: &str,
) -> Result<String> {
    publish(repo, expected, base, title, body, api)
}
fn publish(
    repo: &Repository,
    expected: &Target,
    base: &str,
    title: &str,
    body: &str,
    mut call: impl FnMut(&str, Option<&Value>) -> Result<Value>,
) -> Result<String> {
    let current = target(repo)?;
    if current != *expected {
        return Err(AppError::protocol(
            codes::GITHUB_CHANGED,
            "The repository or branch changed. Review the target again.",
        ));
    }
    if !git2::Reference::is_valid_name(&format!("refs/heads/{base}"))
        || current.head == base
        || title.trim().is_empty()
        || title.len() > 1024
        || body.len() > 262144
    {
        return Err(AppError::protocol(
            codes::GITHUB_INVALID_DRAFT,
            "Check the title, description and base branch.",
        ));
    }
    let mut endpoint = reqwest::Url::parse("https://api.github.com").expect("static URL");
    let (owner, name) = current
        .repository
        .split_once('/')
        .expect("validated repository");
    endpoint
        .path_segments_mut()
        .expect("static base")
        .extend(["repos", owner, name, "git", "ref", "heads"])
        .extend(current.head.split('/'));
    let remote = call(endpoint.path().trim_start_matches('/'), None)?;
    if remote.pointer("/object/sha").and_then(Value::as_str) != Some(&current.oid) {
        return Err(AppError::protocol(
            codes::GITHUB_PUSH_FIRST,
            "Push the reviewed branch to origin before creating its pull request.",
        ));
    }
    let result = call(
        &format!("repos/{}/pulls", current.repository),
        Some(&json!({"head":current.head,"base":base,"title":title,"body":body})),
    )?;
    let number = result["number"]
        .as_u64()
        .filter(|n| *n > 0)
        .ok_or_else(|| {
            AppError::protocol(
                codes::GITHUB_UNEXPECTED,
                "Unexpected GitHub response. Check open pull requests.",
            )
        })?;
    Ok(format!(
        "https://github.com/{}/pull/{number}",
        current.repository
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use git2::Signature;

    fn sample() -> (std::path::PathBuf, Repository) {
        let (dir, repo) = crate::infrastructure::git::test_helpers::init_empty_repo();
        repo.remote("origin", "git@github.com:example/Example.git")
            .unwrap();
        let sig = Signature::now("Example", "example@example.com").unwrap();
        let oid = repo.index().unwrap().write_tree().unwrap();
        let first = repo
            .commit(
                Some("refs/heads/main"),
                &sig,
                &sig,
                "base",
                &repo.find_tree(oid).unwrap(),
                &[],
            )
            .unwrap();
        repo.branch("work/topic", &repo.find_commit(first).unwrap(), false)
            .unwrap();
        repo.set_head("refs/heads/work/topic").unwrap();
        (dir, repo)
    }

    #[test]
    fn publish_checks_remote_head_and_posts_the_edited_body_once() {
        let (dir, repo) = sample();
        let expected = target(&repo).unwrap();
        let mut calls = Vec::new();
        let body = "First line\n\n- second line $literal";
        let url = publish(&repo, &expected, "main", "Title", body, |endpoint, data| {
            calls.push(endpoint.to_owned());
            if let Some(d) = data {
                assert_eq!(
                    *d,
                    json!({"title":"Title","body":body,"base":"main","head":"work/topic"})
                );
                Ok(json!({"number":42}))
            } else {
                Ok(json!({"object":{"sha":expected.oid}}))
            }
        })
        .unwrap();
        assert_eq!(url, "https://github.com/example/Example/pull/42");
        assert_eq!(
            calls,
            [
                "repos/example/Example/git/ref/heads/work/topic",
                "repos/example/Example/pulls"
            ]
        );
        drop(repo);
        std::fs::remove_dir_all(dir).unwrap();
    }

    #[test]
    fn unpublished_or_changed_branch_never_posts() {
        let (dir, repo) = sample();
        let expected = target(&repo).unwrap();
        assert!(publish(&repo, &expected, "main", "Title", "", |_, data| {
            assert!(data.is_none());
            Ok(json!({"object":{"sha":"different"}}))
        })
        .is_err());
        repo.set_head("refs/heads/main").unwrap();
        assert!(
            publish(&repo, &expected, "main", "Title", "", |_, _| panic!(
                "stale target"
            ))
            .is_err()
        );
        drop(repo);
        std::fs::remove_dir_all(dir).unwrap();
    }

    #[test]
    fn review_uses_advertised_default_branch() {
        let (dir, repo) = sample();
        repo.reference(
            "refs/remotes/origin/develop",
            repo.head().unwrap().target().unwrap(),
            true,
            "setup",
        )
        .unwrap();
        repo.reference_symbolic(
            "refs/remotes/origin/HEAD",
            "refs/remotes/origin/develop",
            true,
            "setup",
        )
        .unwrap();
        assert_eq!(target(&repo).unwrap().base, "develop");
        drop(repo);
        std::fs::remove_dir_all(dir).unwrap();
    }

    #[cfg(unix)]
    #[test]
    fn timeout_also_bounds_a_large_request_when_cli_does_not_read_stdin() {
        let mut cmd = hidden_command("sleep");
        cmd.arg("10")
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped());
        let body = json!({"body":"x".repeat(262_144)});
        let start = std::time::Instant::now();
        let error = run_api_command(cmd, Some(&body), Duration::from_millis(150)).unwrap_err();
        assert_eq!(error.code(), codes::GITHUB_TIMED_OUT);
        assert!(start.elapsed() < Duration::from_secs(5));
    }

    #[test]
    fn validates_origin_without_accepting_lookalike_hosts() {
        assert_eq!(
            slug("https://github.com/example/project.git").as_deref(),
            Some("example/project")
        );
        assert_eq!(
            slug("https://github.com/example/.github.git").as_deref(),
            Some("example/.github")
        );
        for bad in [
            "https://github.com/example/..",
            "https://github.com/./project",
            "https://github.com.evil/a/b",
            "https://alice:password@github.com/a/b",
            "git@github.com:foo/bar/baz",
        ] {
            assert!(slug(bad).is_none());
        }
    }
}
