//! Git hooks editor support — list, read, and write hook scripts under
//! `.git/hooks`. GitWave only EDITS hooks and never executes them (P1 /
//! design note in `infrastructure::git`); the filesystem entry itself is
//! what git invokes.

use std::path::Path;
use std::path::PathBuf;

use git2::Repository;

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;
use crate::domain::hooks::HookInfo;

fn map_git_err(e: git2::Error) -> AppError {
    AppError::unknown_with(
        codes::git::GIT_ERROR,
        format!("git: {e}"),
        &[("error", e.to_string())],
    )
}

/// Hooks the editor offers (the common client-side set).
pub const COMMON_HOOKS: [&str; 8] = [
    "pre-commit",
    "prepare-commit-msg",
    "commit-msg",
    "post-commit",
    "pre-rebase",
    "post-merge",
    "pre-push",
    "post-checkout",
];

fn hooks_dir(repo: &Repository) -> Result<PathBuf> {
    let workdir = repo.workdir().ok_or_else(|| {
        AppError::protocol(
            codes::git::BARE_REPO,
            "bare repository has no working directory",
        )
    })?;
    Ok(workdir.join(".git").join("hooks"))
}

fn hook_path(repo: &Repository, name: &str) -> Result<PathBuf> {
    validate_name(name)?;
    Ok(hooks_dir(repo)?.join(name))
}

/// Hook script names are file names — restrict to the git convention
/// (kebab-case alphanumerics) so no path can escape `.git/hooks`.
fn validate_name(name: &str) -> Result<()> {
    let ok = !name.is_empty()
        && name
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_');
    if ok {
        Ok(())
    } else {
        Err(AppError::protocol_with(
            codes::git::INVALID_HOOK_NAME,
            format!("invalid hook name: {name}"),
            &[("name", name.to_string())],
        ))
    }
}

fn is_executable(path: &Path) -> bool {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::metadata(path)
            .map(|m| m.permissions().mode() & 0o111 != 0)
            .unwrap_or(false)
    }
    #[cfg(not(unix))]
    {
        let _ = path;
        false
    }
}

/// The known hooks with presence/executable markers.
pub fn list_hooks(repo: &Repository) -> Result<Vec<HookInfo>> {
    let dir = hooks_dir(repo)?;
    Ok(COMMON_HOOKS
        .iter()
        .map(|name| {
            let path = dir.join(name);
            HookInfo {
                name: name.to_string(),
                exists: path.is_file(),
                executable: is_executable(&path),
            }
        })
        .collect())
}

/// Read a hook's script (empty string when the hook does not exist yet).
pub fn read_hook(repo: &Repository, name: &str) -> Result<String> {
    let path = hook_path(repo, name)?;
    match std::fs::read_to_string(&path) {
        Ok(content) => Ok(content),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(String::new()),
        Err(e) => Err(AppError::unknown_with(
            codes::git::READ_HOOK,
            format!("read hook: {e}"),
            &[("error", e.to_string())],
        )),
    }
}

/// Write a hook script, creating the file if needed and marking it
/// executable on unix (mirroring what `git init` ships for samples).
/// To disable a hook, edit its content to a no-op — an empty or
/// non-executable hook file is skipped by git.
pub fn write_hook(repo: &Repository, name: &str, content: &str) -> Result<()> {
    let path = hook_path(repo, name)?;
    std::fs::write(&path, content).map_err(|e| {
        AppError::unknown_with(
            codes::git::WRITE_HOOK,
            format!("write hook: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o755)).map_err(|e| {
            AppError::unknown_with(
                codes::git::HOOK_CHMOD,
                format!("set hook permissions: {e}"),
                &[("error", e.to_string())],
            )
        })?;
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::init_empty_repo;
    use std::fs;

    #[test]
    fn list_reports_all_common_hooks_inactive_on_fresh_repo() {
        let (dir, repo) = init_empty_repo();
        let hooks = list_hooks(&repo).expect("list");
        assert_eq!(hooks.len(), COMMON_HOOKS.len());
        assert!(hooks.iter().all(|h| !h.exists && !h.executable));
        assert!(hooks.iter().any(|h| h.name == "pre-commit"));
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn write_then_read_roundtrips_and_marks_present() {
        let (dir, repo) = init_empty_repo();
        write_hook(&repo, "pre-commit", "#!/bin/sh\necho hi\n").expect("write");
        let content = read_hook(&repo, "pre-commit").expect("read");
        assert_eq!(content, "#!/bin/sh\necho hi\n");

        let hooks = list_hooks(&repo).expect("list");
        let pre = hooks.iter().find(|h| h.name == "pre-commit").expect("pre");
        assert!(pre.exists);
        #[cfg(unix)]
        assert!(pre.executable, "hook is chmod +x on unix");
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn traversal_names_are_rejected() {
        let (dir, repo) = init_empty_repo();
        assert!(validate_name("../evil").is_err());
        assert!(validate_name("pre commit").is_err());
        assert!(validate_name("").is_err());
        assert!(write_hook(&repo, "../evil", "x").is_err());
        let _ = fs::remove_dir_all(&dir);
    }
}
