# Bug Assessment: Editing a remote with a blank push URL fails

- **Slug**: remote-edit-empty-push-url
- **Created**: 2026-10-22
- **Source**: pasted text
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

> 编辑 remote 时push url 留空会导致出错

No URL was supplied, so no fetch was attempted.

## Symptom

In the sidebar's Remote card, opening **编辑…** on a remote whose push URL is empty and saving the dialog (leaving the push URL field blank) fails with an error, and the rest of the edit — including the rename — is not applied. The field's own placeholder says `push URL（留空 = 与 fetch 相同）`, so a blank value is supposed to mean "inherit the fetch URL", not an error.

## Reproduction

1. Have a repository with a remote created by `git remote add` (or `git clone`) — these write only `remote.<name>.url`, not `remote.<name>.pushurl`.
2. In the sidebar's **Remote** card, right-click the remote → **编辑…**.
3. Leave the **push URL** field blank (it is blank because no push URL is configured) and click **保存**.
4. An error is shown in the status area, the modal stays open, and a name change entered in the same dialog is not applied. The fetch URL change *is* applied, because it is issued first.

[NEEDS CLARIFICATION: whether the reporter also tested blanking an *existing* push URL. Code evidence says that path succeeds; only the "no push URL configured" path fails.]

## Suspected Code Paths

- `src/components/RemotesPanel.tsx:144-157` — `submitEdit()` calls `setRemotePushUrl(workspaceId, name, editPushUrl.trim() || null)` **unconditionally**, so a blank field always becomes a "clear the push URL" call, even when there is nothing to clear. It runs before `renameRemote`, which is why the rename never happens.
- `src-tauri/src/lib.rs:1270-1277` — `cmd_set_remote_push_url` forwards `Option<String>` to the use case.
- `src-tauri/src/application/use_cases.rs:2046-2055` — thin pass-through to the git layer.
- `src-tauri/src/infrastructure/git/remote.rs:532-536` — `set_remote_push_url()` calls `repo.remote_set_pushurl(name.trim(), url)` with no idempotency handling.
- `src-tauri/src/infrastructure/git/remote.rs:486-495` — `remote_op_err()` maps `git2::ErrorCode::NotFound` to `git.remote.not_found` / "remote not found: …", which is where the misleading message comes from.
- Vendored libgit2 (`libgit2-sys-0.18.8+1.9.7`), `src/libgit2/remote.c:651-658` → `config_file.c:392-420` — `git_remote_set_pushurl(repo, name, NULL)` deletes the `remote.<name>.pushurl` key; `git_config_delete_entry` returns `GIT_ENOTFOUND` ("could not find key 'remote.origin.pushurl' to delete") when the key does not exist.

## Root Cause Hypothesis

Confidence: **high**.

A remote with no custom push URL has no `remote.<name>.pushurl` key in `.git/config`. The edit dialog shows that state as an empty field, and `submitEdit()` translates the empty field into `null`, which reaches libgit2 as "delete the pushurl key". libgit2's delete is not idempotent: `config_file_delete` returns `GIT_ENOTFOUND` when the key is absent. That code is mapped by `remote_op_err()` to `git.remote.not_found` with the message "remote not found: could not find key 'remote.origin.pushurl' to delete", so the user sees a failure and a message that wrongly implies the remote is gone — while the remote is perfectly fine. Because the push-URL call precedes the rename, a rename in the same dialog is silently dropped, and the fetch-URL change issued before it has already been written (partial update). The bug therefore hits the *default* state of most remotes, not an edge case.

## Proposed Remediation

**Preferred**: make clearing a push URL idempotent at the infrastructure boundary, and avoid the redundant write in the UI.

1. `src-tauri/src/infrastructure/git/remote.rs` — in `set_remote_push_url()`:
   - First confirm the remote exists with `repo.find_remote(name.trim()).map_err(remote_op_err)?`, so a genuinely missing remote still yields `git.remote.not_found`.
   - Then call `remote_set_pushurl`; when `url.is_none()` and the returned error is `git2::ErrorCode::NotFound`, treat it as success (clearing an already-absent key is a no-op).
   - Optionally reject `Some("")` with the existing `REMOTE_URL_EMPTY` protocol code, so the backend does not rely on the caller trimming.
2. `src/components/RemotesPanel.tsx` — in `submitEdit()`, only call `setRemotePushUrl` when the field actually differs from the loaded value (`edit.push_url ?? ""`). This removes the redundant config write on every edit and makes the dialog's no-op case free.

**Alternatives** (optional):
- Frontend-only guard: fix `submitEdit()` alone. Cheaper, but leaves a misleading non-idempotent API that other callers (or a future bulk-edit) can trip over, and it does not fix the misleading error text if a real failure occurs.
- Swallow `NotFound` in `remote_op_err()` globally: rejects — it would mask genuinely missing remotes in `set_remote_url`, `rename_remote`, `remove_remote`, etc.
- Shell out to `git remote set-url --push`: rejects — the project's remotes implementation deliberately goes through libgit2.

**Files likely to change**:
- `src-tauri/src/infrastructure/git/remote.rs`
- `src-tauri/src/infrastructure/git/remote.rs` (tests module)
- `src/components/RemotesPanel.tsx`

**Tests to add or update**:
- Rust: `set_remote_push_url(&repo, "origin", None)` on a remote with **no** push URL returns `Ok(())` and leaves `push_url == None` (the regression test for this bug).
- Rust: clearing a **configured** push URL with `None` still returns `Ok(())` and removes it.
- Rust: `set_remote_push_url(&repo, "missing", None)` still returns `Protocol` / `git.remote.not_found`, proving the remote-existence check was preserved.
- (Optional) Frontend: a `RemotesPanel` edit test asserting `setRemotePushUrl` is not invoked when the field is untouched and blank.

## Risks & Considerations

- Ordering the existence check before the delete is essential; otherwise the fix would silence the "remote not found" error for unknown remotes.
- `submitEdit()` still applies changes one call at a time: a genuine failure between `setRemoteUrl` and `renameRemote` leaves a partial edit. The fix removes the failure for the blank case; a transactional single command is out of scope but worth noting.
- Empty-string push URLs: the frontend already trims to `null`, but validating `Some("")` on the backend prevents a future caller from persisting an empty `pushurl` that `list_remote_details` would report oddly.
- No API breakage: the Tauri command signature (`Option<String>`) and `RemoteInfo` shape stay unchanged; no config migration is needed.
- The misleading `git.remote.not_found` wording could still surface for a truly missing remote; consider a dedicated `REMOTE_PUSHURL_MISSING`-style message only if the UX review asks for it.

## Open Questions

- [NEEDS CLARIFICATION: exact error text the reporter saw — the report only says "出错". The code predicts a Protocol-category "remote not found: could not find key 'remote.origin.pushurl' to delete". Confirming the text would close the loop.]
- [NEEDS CLARIFICATION: should a blank push URL field mean "inherit fetch URL" (current placeholder wording), or "leave unchanged"? The proposed fix assumes the former, matching `pushPlaceholder`.]
