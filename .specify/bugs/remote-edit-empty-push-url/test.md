# Bug Verification: Editing a remote with a blank push URL fails

- **Slug**: remote-edit-empty-push-url
- **Tested**: 2026-10-22
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: partial

## Summary

The failing backend call no longer errors: clearing a push URL on a remote that never had one now returns `Ok`, and every Rust/frontend suite plus all lint and type gates pass. The GUI flow from the assessment (edit dialog → blank push URL → save) was **not** driven in the running app, so the end-to-end symptom is unconfirmed at the UI layer.

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (post-fix) | GUI flow from `assessment.md` | not-run | Would require launching the Tauri app and driving the edit dialog by hand. |
| Automated repro equivalent | `cargo test --lib infrastructure::git::remote` | pass | `clearing_push_url_is_idempotent` calls `set_remote_push_url(repo, "origin", None)` on a remote with no push URL — the exact call the dialog used to make. |
| New / updated tests | `cargo test --all-targets` | pass | 387 passed, 0 failed, 2 ignored; new test included. |
| Regression suite (frontend) | `pnpm test` | pass | 61 files / 367 tests passed. |
| Lint / type-check | `pnpm lint`, `pnpm typecheck`, `pnpm format:check` | pass | eslint `--max-warnings 0`, `tsc --noEmit`, prettier all clean. |
| Format / lint (Rust) | `cargo fmt -- --check`, `cargo clippy --all-targets -- -D warnings` | pass | Clean. |

## Output Excerpts

```
test infrastructure::git::remote::tests::clearing_push_url_is_idempotent ... ok
test result: ok. 387 passed; 0 failed; 2 ignored; 0 measured; 0 filtered out
```

```
Test Files  61 passed (61)
     Tests  367 passed (367)
```

## Residual Risks

- The UI path itself is unverified: the frontend guard (`pushUrl !== (edit.push_url ?? "")`) is covered only by typecheck/lint, not by a component test, and no manual smoke was performed. The backend fix means the UI call succeeds even if the guard is bypassed, but the dialog's visible behavior (no error toast, rename applied) was not observed.
- `submitEdit()` still issues fetch-URL / push-URL / rename as separate calls; a genuine mid-sequence failure can leave a partial edit (pre-existing, unchanged by this fix).
- Clearing an *already configured* push URL still passes through libgit2's delete path; covered by the unit test, not exercised in the app.

## Recommendation

Close after a 30-second manual smoke: open a repo whose remote has no `pushurl` (`git remote add origin …`), edit that remote with the push URL field blank, and confirm the save succeeds and an entered rename is applied. With that observation, this becomes `verified`; the backend root cause and the regression lock are already proven.
