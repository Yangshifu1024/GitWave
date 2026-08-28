//! Tag management: list / create (lightweight + annotated) / delete (S3).

use git2::{Repository, Signature};
use serde::Serialize;

use crate::domain::error::{AppError, Result};
use crate::infrastructure::git::git2_adapter::commit_signature;

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

/// One tag in the repository.
#[derive(Debug, Clone, Serialize)]
pub struct TagInfo {
    pub name: String,
    /// Commit the tag points at (peeled through tag objects).
    pub sha: String,
    /// Annotation text; `None` for lightweight tags.
    pub annotation: Option<String>,
}

fn tag_target_sha(repo: &Repository, name: &str) -> Option<String> {
    repo.find_reference(&format!("refs/tags/{name}"))
        .ok()
        .and_then(|r| r.peel_to_commit().ok())
        .map(|c| c.id().to_string())
}

/// All tags, sorted by name.
pub fn list_tags(repo: &Repository) -> Result<Vec<TagInfo>> {
    let mut names: Vec<String> = repo
        .tag_names(None)
        .iter()
        .flatten()
        .flatten()
        .map(str::to_string)
        .collect();
    names.sort();

    let mut out = Vec::with_capacity(names.len());
    for name in names {
        // Annotated tags resolve to tag objects; read their message.
        let annotation = repo
            .find_reference(&format!("refs/tags/{name}"))
            .ok()
            .and_then(|r| r.peel_to_tag().ok())
            .and_then(|t| t.message().map(str::to_string));
        out.push(TagInfo {
            sha: tag_target_sha(repo, &name).unwrap_or_default(),
            name,
            annotation,
        });
    }
    Ok(out)
}

/// Create a tag on `target_oid` (defaults to HEAD). With `message` an
/// annotated tag is created (signed with the configured identity), without
/// it a lightweight ref.
pub fn create_tag(
    repo: &Repository,
    name: &str,
    target_oid: Option<&str>,
    message: Option<&str>,
) -> Result<String> {
    let name = name.trim();
    if name.is_empty() {
        return Err(AppError::Protocol("tag name cannot be empty".into()));
    }
    let target = match target_oid {
        Some(oid) => repo
            .find_commit(
                git2::Oid::from_str(oid)
                    .map_err(|e| AppError::Protocol(format!("invalid oid: {e}")))?,
            )
            .map_err(|_| AppError::Protocol(format!("commit not found: {oid}")))?
            .into_object(),
        None => repo
            .head()
            .map_err(map_git_err)?
            .peel_to_commit()
            .map_err(map_git_err)?
            .into_object(),
    };

    let force = true; // re-tagging an existing name replaces it, mirroring `git tag -f`
    let oid = match message.map(str::trim).filter(|m| !m.is_empty()) {
        Some(msg) => {
            let tagger: Signature<'_> = commit_signature(repo)?;
            repo.tag(name, &target, &tagger, msg, force)
        }
        None => repo.tag_lightweight(name, &target, force),
    }
    .map_err(map_git_err)?;
    Ok(oid.to_string())
}

/// Delete a tag by short name (refs/tags/<name>).
pub fn delete_tag(repo: &Repository, name: &str) -> Result<()> {
    repo.tag_delete(name).map_err(map_git_err)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::build_linear_repo;
    use std::fs;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    #[test]
    fn create_list_delete_lightweight_and_annotated() {
        let (path, repo) = build_linear_repo(2);
        let tip = repo.head().unwrap().peel_to_commit().unwrap();
        let tip_sha = tip.id().to_string();
        let prev = tip.parent(0).unwrap().id().to_string();

        create_tag(&repo, "v1.0", Some(&prev), None).unwrap();
        create_tag(&repo, "v2.0", None, Some("release notes\n")).unwrap();

        let tags = list_tags(&repo).unwrap();
        assert_eq!(tags.len(), 2);
        let lw = tags.iter().find(|t| t.name == "v1.0").unwrap();
        assert_eq!(lw.sha, prev);
        assert!(lw.annotation.is_none());
        let ann = tags.iter().find(|t| t.name == "v2.0").unwrap();
        assert_eq!(ann.sha, tip_sha);
        assert_eq!(
            ann.annotation.as_deref().map(str::trim_end),
            Some("release notes")
        );

        delete_tag(&repo, "v1.0").unwrap();
        assert_eq!(list_tags(&repo).unwrap().len(), 1);
        cleanup(&path);
    }

    #[test]
    fn create_tag_rejects_empty_name() {
        let (path, repo) = build_linear_repo(1);
        let err = create_tag(&repo, "  ", None, None).unwrap_err();
        assert_eq!(err.category(), "Protocol");
        cleanup(&path);
    }
}
