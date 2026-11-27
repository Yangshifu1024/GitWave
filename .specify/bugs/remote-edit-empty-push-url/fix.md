# Bug Fix: Editing a remote with a blank push URL fails

- **Slug**: remote-edit-empty-push-url
- **Fixed**: 2026-10-22
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

Clearing a push URL is now idempotent in the git layer — libgit2's delete reports `NotFound` when `remote.<name>.pushurl` is already absent, which previously surfaced as a bogus "remote not found" and aborted the rest of the edit. The edit dialog also stops issuing the call when the field is unchanged.

## Changes

| File | Change | Notes |
|------|--------|-------|
| `src-tauri/src/infrastructure/git/remote.rs` | modified | `set_remote_push_url()` now verifies the remote exists, validates a non-empty `Some(url)`, and treats `NotFound` as success only when clearing (`None`). |
| `src-tauri/src/infrastructure/git/remote.rs` | added test | `clearing_push_url_is_idempotent` covers clear-when-absent, clear-when-set, and missing-remote-still-errors. |
| `src/components/RemotesPanel.tsx` | modified | `submitEdit()` calls `setRemotePushUrl` only when the field differs from the loaded `push_url`, so untouched blanks no longer write config. |

## Diff Highlights

```rust
// src-tauri/src/infrastructure/git/remote.rs
if let Some(u) = url {
    validate_remote_url(u)?;
}
// libgit2 clears the key by deleting it and reports NotFound when it is
// already absent, so check the remote separately to keep "clear" idempotent.
repo.find_remote(name.trim()).map_err(remote_op_err)?;
match repo.remote_set_pushurl(name.trim(), url) {
    Ok(()) => Ok(()),
    Err(e) if url.is_none() && e.code() == git2::ErrorCode::NotFound => Ok(()),
    Err(e) => Err(remote_op_err(e)),
}
```

```ts
// src/components/RemotesPanel.tsx
const pushUrl = editPushUrl.trim();
if (pushUrl !== (edit.push_url ?? "")) {
  await setRemotePushUrl(workspaceId, name, pushUrl || null);
}
```

## Tests Added or Updated

- `src-tauri/src/infrastructure/git/remote.rs::tests::clearing_push_url_is_idempotent` — clearing a push URL that was never set returns `Ok` and leaves `push_url` null; clearing a configured one still removes it; a genuinely missing remote still fails with `Protocol`.

## Local Verification

- Commands run: `cargo test --lib infrastructure::git::remote` → 14 passed, 0 failed (includes the new test).
- Commands run: `cargo fmt -- --check` → clean; `cargo clippy --all-targets -- -D warnings` → clean.
- Commands run: `pnpm exec prettier --check src/components/RemotesPanel.tsx`, `pnpm exec eslint src/components/RemotesPanel.tsx`, `pnpm typecheck` → all clean.
- Manual checks: none in-app; root cause was confirmed against the vendored libgit2 source (`remote.c` `set_url` → `config_file_delete`, which returns `GIT_ENOTFOUND` for a missing key).

## Deviations from Assessment

- None. The optional `Some("")` validation and the optional frontend test were handled as follows: validation added; the optional `RemotesPanel` test was skipped because the Rust regression test pins the behavior and no component-test harness exists for this panel.

## Follow-ups

- `submitEdit()` still applies fetch URL, push URL, and rename as three separate calls; a genuine failure in the middle leaves a partial edit. Collapsing them into one backend command would make edits atomic, but is out of scope for this fix.
- The `git.remote.not_found` copy is still used for real missing remotes; no change needed.
- Not committed — per project rules the agent does not commit unless asked. Suggested branch: `fix/remote-edit-empty-push-url`.
