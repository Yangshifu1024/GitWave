# Reliability and workflow improvements

Status: Implemented and locally verified; independent code review approved. Branch: `fix/reliability-and-workflow`. See [review](./review.md) for evidence and remaining limitations.

## Scope and authorization

The user requested implementation of all findings from the project audit, grouped into commits and delivered as one pull request. This task does not release a new version or merge the PR.

## Delivery batches

1. **AI privacy**: redact complete private-key material (including diff fragments); enforce local-only endpoints, proxy bypass and redirect restrictions in offline mode, including fallback attempts.
2. **Workflow correctness**: protect conflict buffers and asynchronous results; retain repository-scoped commit drafts; support message-only amend; remove the message body limit; prevent obsolete history/blame responses; keep working-copy diffs fresh.
3. **Diff and history**: request only selected file/side content, bound large text payloads, virtualize text rendering, pair changed lines and add syntax highlighting; provide adjustable working-copy reading space; incrementally load history and explicitly describe bounded search results.
4. **Compatibility and access**: resolve hooks paths for linked worktrees and `core.hooksPath`; complete touched bilingual UI and keyboard file selection; isolate heavy local reads from async runtime workers.
5. **Verification and documentation**: behavioral regression coverage, independent code review, full frontend/Rust quality gates, user-facing and engineering documentation synchronized to the actual implementation.

## Acceptance and regression checks

- AI request payloads contain no private-key bodies; offline requests cannot reach remote endpoints through primary/fallback URLs, redirects or proxy settings.
- Repository/file changes cannot be overwritten by earlier async results. Editing an already-modified file refreshes the displayed diff even when its path, status and line counts do not change.
- Conflict edits survive file navigation or require an explicit discard; generated explanations and save actions remain attached to the correct file.
- Commit drafts survive dialog reopen/repository switches; failures preserve drafts; successful commit clears only the submitted draft. Message-only amend preserves the tree.
- Selecting a small file never serializes every changed file's hunks. Large text has explicit truncation/expansion behavior; history loading appends pages without repeatedly returning the entire prefix.
- Hooks resolve the path Git uses for ordinary repositories, worktrees and configured hooks paths.
- Keyboard navigation, split reading and translated loading/empty states work in both languages.

## Validation

Delivery is grouped into three commits: backend privacy and bounded repository APIs; frontend workflows, rendering, and regression tests; synchronized documentation and review evidence. The five work areas above describe scope rather than one commit each.

Run focused behavioral tests during each batch. Before committing/pushing, run `pnpm format:check`, `pnpm lint`, `pnpm typecheck`, `pnpm test`, `cargo fmt -- --check`, `cargo clippy --all-targets -- -D warnings`, `cargo check --all-targets`, and `cargo test --all-targets`. Record actual results and limitations in `review.md`; do not equate source-string guards with behavioral coverage.

Cross-task references: [architecture](../../tech/architecture/00-overview.md), [engineering conventions](../../tech/engineering/00-overview.md), [design overview](../../design/00-overview.md).
