//! Git LFS support — availability probe, per-repo install, and
//! `.gitattributes` pattern management.
//!
//! libgit2 has no LFS support, so the `git lfs` CLI (spawned hidden, see
//! `infrastructure::process`) handles the filter wiring; tracking patterns
//! are managed directly in `.gitattributes` for deterministic, testable
//! behavior without depending on the CLI.

use std::path::{Path, PathBuf};

use git2::Repository;

use crate::domain::error::{AppError, Result};
use crate::infrastructure::process::hidden_command;

/// Attribute flags `git lfs track` writes for a pattern.
const LFS_ATTRS: &str = "filter=lfs diff=lfs merge=lfs -text";

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

fn attributes_path(repo: &Repository) -> Result<PathBuf> {
    let workdir = repo
        .workdir()
        .ok_or_else(|| AppError::Protocol("bare repository has no working directory".into()))?;
    Ok(workdir.join(".gitattributes"))
}

fn read_attributes(repo: &Repository) -> Result<String> {
    let path = attributes_path(repo)?;
    match std::fs::read_to_string(&path) {
        Ok(content) => Ok(content),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(String::new()),
        Err(e) => Err(AppError::Unknown(format!("read .gitattributes: {e}"))),
    }
}

fn validate_pattern(pattern: &str) -> Result<()> {
    let trimmed = pattern.trim();
    if trimmed.is_empty() || trimmed.contains('\n') {
        return Err(AppError::Protocol("invalid LFS pattern".into()));
    }
    // gitattributes patterns are whitespace-delimited tokens; a pattern
    // with spaces would silently split into flags.
    if trimmed.chars().any(char::is_whitespace) {
        return Err(AppError::Protocol(
            "LFS patterns cannot contain spaces — use a glob like *.psd or assets/**".into(),
        ));
    }
    if trimmed.starts_with('#') {
        return Err(AppError::Protocol(
            "LFS patterns cannot start with '#' (that is a .gitattributes comment)".into(),
        ));
    }
    Ok(())
}

/// True when a working `git lfs` binary is on PATH.
pub fn lfs_available() -> bool {
    hidden_command("git")
        .args(["lfs", "version"])
        .output()
        .map(|o| o.status.success())
        .unwrap_or(false)
}

/// Wire the LFS clean/smudge filters into THIS repository only
/// (`git lfs install --local`) — never the user's global git config.
pub fn lfs_install(repo: &Repository) -> Result<String> {
    let workdir = repo
        .workdir()
        .ok_or_else(|| AppError::Protocol("bare repository has no working directory".into()))?;
    let output = hidden_command("git")
        .args(["lfs", "install", "--local"])
        .current_dir(workdir)
        .output()
        .map_err(|e| AppError::Unknown(format!("git lfs: {e}")))?;
    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(AppError::Unknown(format!(
            "git lfs install failed: {}",
            stderr.trim()
        )));
    }
    Ok(String::from_utf8_lossy(&output.stdout).trim().to_string())
}

/// Installed = this repo's local config wires the LFS filters.
pub fn lfs_installed(repo: &Repository) -> Result<bool> {
    let workdir = repo
        .workdir()
        .ok_or_else(|| AppError::Protocol("bare repository has no working directory".into()))?;
    let config_path = workdir.join(".git").join("config");
    if !config_path.exists() {
        return Ok(false);
    }
    let config = git2::Config::open(&config_path).map_err(map_git_err)?;
    Ok(config.get_string("filter.lfs.smudge").is_ok())
}

/// Patterns tracked with LFS: `.gitattributes` lines carrying `filter=lfs`.
pub fn list_tracked_patterns(repo: &Repository) -> Result<Vec<String>> {
    let content = read_attributes(repo)?;
    Ok(content
        .lines()
        .filter_map(|line| {
            let trimmed = line.trim();
            if trimmed.is_empty() || trimmed.starts_with('#') || !trimmed.contains("filter=lfs") {
                return None;
            }
            Some(
                trimmed
                    .split_whitespace()
                    .next()
                    .unwrap_or(trimmed)
                    .to_string(),
            )
        })
        .collect())
}

/// Track a path pattern with LFS by appending a `.gitattributes` line;
/// idempotent per exact pattern.
pub fn track_pattern(repo: &Repository, pattern: &str) -> Result<()> {
    validate_pattern(pattern)?;
    let pattern = pattern.trim();
    if list_tracked_patterns(repo)?.iter().any(|p| p == pattern) {
        return Ok(());
    }
    let path = attributes_path(repo)?;
    let mut content = read_attributes(repo)?;
    if !content.is_empty() && !content.ends_with('\n') {
        content.push('\n');
    }
    content.push_str(&format!("{pattern} {LFS_ATTRS}\n"));
    std::fs::write(&path, content)
        .map_err(|e| AppError::Unknown(format!("write .gitattributes: {e}")))
}

/// Remove the LFS tracking line for a pattern, leaving every other
/// `.gitattributes` line (comments, non-LFS attributes) untouched.
pub fn untrack_pattern(repo: &Repository, pattern: &str) -> Result<()> {
    validate_pattern(pattern)?;
    let pattern = pattern.trim();
    let path = attributes_path(repo)?;
    let content = read_attributes(repo)?;
    let kept: Vec<&str> = content
        .lines()
        .filter(|line| {
            let trimmed = line.trim();
            if trimmed.is_empty() || trimmed.starts_with('#') {
                return true;
            }
            let first = trimmed.split_whitespace().next().unwrap_or("");
            !(first == pattern && trimmed.contains("filter=lfs"))
        })
        .collect();
    let mut out = kept.join("\n");
    if !out.is_empty() {
        out.push('\n');
    }
    std::fs::write(&path, out).map_err(|e| AppError::Unknown(format!("write .gitattributes: {e}")))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::init_empty_repo;
    use std::path::PathBuf;

    #[test]
    fn pattern_validation_rejects_bad_input() {
        assert!(validate_pattern("").is_err());
        assert!(validate_pattern("  \n ").is_err());
        assert!(validate_pattern("a\nb").is_err());
        assert!(
            validate_pattern("my file.png").is_err(),
            "spaces unsupported"
        );
        assert!(validate_pattern("#comment").is_err());
        assert!(validate_pattern("*.psd").is_ok());
        assert!(validate_pattern("assets/**").is_ok());
    }

    #[test]
    fn track_is_idempotent_and_lists_patterns() {
        let (dir, repo) = init_empty_repo();
        track_pattern(&repo, "*.psd").expect("track");
        track_pattern(&repo, "*.psd").expect("retrack is a no-op");
        track_pattern(&repo, "*.zip").expect("track");
        let patterns = list_tracked_patterns(&repo).expect("list");
        assert_eq!(patterns, vec!["*.psd", "*.zip"]);
        let content = std::fs::read_to_string(dir.join(".gitattributes")).expect("read attributes");
        assert!(content.contains("*.psd filter=lfs diff=lfs merge=lfs -text"));
        std::fs::remove_dir_all(&dir).expect("cleanup");
    }

    #[test]
    fn untrack_removes_only_the_matching_lfs_line() {
        let (dir, repo) = init_empty_repo();
        std::fs::write(
            dir.join(".gitattributes"),
            "# keep comment\n*.png text\n*.psd filter=lfs diff=lfs merge=lfs -text\n",
        )
        .expect("seed attributes");
        untrack_pattern(&repo, "*.psd").expect("untrack");
        let content = std::fs::read_to_string(dir.join(".gitattributes")).expect("read attributes");
        assert!(content.contains("# keep comment"));
        assert!(content.contains("*.png text"));
        assert!(!content.contains("filter=lfs"));
        std::fs::remove_dir_all(&dir).expect("cleanup");
    }

    #[test]
    fn untrack_without_file_is_a_noop() {
        let (dir, repo) = init_empty_repo();
        untrack_pattern(&repo, "*.psd").expect("untrack on empty repo");
        assert!(list_tracked_patterns(&repo).expect("list").is_empty());
        std::fs::remove_dir_all(&dir).expect("cleanup");
    }
}
