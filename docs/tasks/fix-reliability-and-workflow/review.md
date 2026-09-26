# Reliability and workflow review

Date: 2026-09-27. Task: [plan](./plan.md). Branch: `fix/reliability-and-workflow`.

## ✅ Strengths

- Offline AI requests validate loopback endpoints at the transport boundary, disable proxies and redirects, and filter fallback providers. Complete old/new blobs are screened for private-key markers before diff fragments are sent.
- Repository-scoped drafts, request generations, and explicit repository IDs protect the changed read workflows from stale responses. Conflict buffers survive file navigation and require confirmation before being discarded.
- Diff metadata and selected-file content are requested separately. Bounded previews expose truncation and expansion, and virtual rows keep rendered text bounded. History uses owned cursors and appends pages.
- Behavioral tests exercise draft retention, amend controls, conflict navigation, stale responses, lazy previews, hook paths, history paging, and privacy boundaries.

## 🔴 Critical issues

None remaining in the reviewed changes. The independent code-reviewer approved the final revision after rechecking all reported blockers.

The review identified and the implementation corrected three issues: repository IDs missing from WorkingCopy cache observers; unloaded/oversized diff previews showing misleading zero statistics or empty states; and silent horizontal clipping of long lines. All four canonical WorkingCopy observers now pass the repository ID matching their cache key. Backend long-line truncation is explicit and covered by expansion tests.

## 🟡 Remaining limitations

- History first constructs a bounded snapshot of up to 100,000 OIDs. Subsequent pages decode incrementally but recalculate graph lanes over the cached prefix; the global cache mutex can serialize work across repositories. Large-repository benchmarks and a fully incremental graph algorithm remain future performance work.
- Diff previews intentionally limit bytes, rows, and individual line lengths. Expanded previews remain bounded and show a notice. Syntax highlighting falls back to plain text above its own budget.
- Commit drafts are retained in memory for the application session, not persisted across application restarts.
- Private-key filtering is conservative: affected files are excluded from AI context, and long base64-like content can be over-redacted.
- Explicit repository binding was checked for the canonical WorkingCopy observers and changed history/diff/blame/image paths; this is not a claim that every existing workspace-only action has been migrated.
- ESLint succeeds with 52 warnings. The production build succeeds with Vite configuration compatibility and chunk-size warnings. These warnings are not hidden or treated as resolved by this task.

## 🟢 Follow-up opportunities

Measure history latency and cache contention on large repositories before changing its graph algorithm. Add native Windows/macOS/Linux walkthroughs for splitter dragging, keyboard file selection, image previews, and very wide diff lines. Consider reducing the syntax-highlighter language bundle after measuring packaged size.

## Validation evidence

All commands below passed locally on Windows using the project's pinned pnpm 12.3.4 and Rust 1.98.1:

| Check | Result |
|---|---|
| `pnpm format:check` | Passed |
| `pnpm lint` | Passed; 0 errors, 52 warnings |
| `pnpm typecheck` | Passed |
| `pnpm test` | 41 files, 271 tests passed |
| `pnpm build` | Passed |
| `cargo fmt -- --check` | Passed |
| `cargo clippy --all-targets -- -D warnings` | Passed |
| `cargo check --all-targets` | Passed |
| `cargo test --all-targets` | 385 passed, 0 failed, 2 ignored |
| `git diff --check` | Passed; independently repeated by reviewer |

Cargo commands ran against `src-tauri/Cargo.toml`. The two existing ignored tests are `history::tests::commit_log_empty_repo` and `merge::tests::merge_empty_repo_errors`; their existing unborn-HEAD/empty-repository investigation notes remain in the source. The new paged-history empty-repository test passes.

The independent reviewer performed static review across correctness, security, performance, maintainability, readability, coverage, and best practices, and did not rerun the parent's full test suite. No native UI walkthrough, cross-platform local validation, or large-repository performance benchmark was performed. Remote CI results must be checked on the PR.

## 📝 Overall assessment

### PR CI follow-up

The initial [test run](https://github.com/Yangshifu1024/GitWave/actions/runs/36265923805) passed all frontend jobs and Linux Rust tests, but failed two hooks assertions on macOS and Windows. The assertions compared lexical temporary paths with libgit2-resolved paths: `/var` versus `/private/var` on macOS and an 8.3 username alias versus its full name on Windows. The initial lint matrix passed on all three platforms.

The follow-up canonicalizes both independently computed, existing filesystem destinations in the two assertions. Content, existence, configured-location, and Unix executable checks remain intact; production behavior is unchanged. Tester analysis confirmed the cause, and independent code review approved the fix. The full local gate passed again: frontend formatting/lint/typecheck, 271 frontend tests, Rust formatting/Clippy, and 385 Rust tests with the same two ignored tests. The follow-up CI matrix is the source of cross-platform confirmation.

The scoped reliability and workflow fixes are ready for PR review, with all local quality gates passing and no remaining review blockers. README, product scope/roadmap, design, engineering notes, and the site describe these changes as unreleased; the application version remains 0.9.4.
