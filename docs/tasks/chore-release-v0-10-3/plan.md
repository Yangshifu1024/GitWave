# Release v0.10.3

## Scope and authorization

The user invoked gitwave-release and explicitly selected v0.10.3 on 2026-10-14. Prepare the release on main and stop before pushing main or creating/pushing a release tag until the user confirms that final step.

Since v0.10.2, main contains `7e3762c feat(ui): adopt a history-first interface` and merge commit `459bf1f` for [PR #163](https://github.com/Yangshifu1024/GitWave/pull/163). The PR's twelve macOS/Windows/Linux lint and test jobs succeeded. The UI implementation and review are recorded in [the feature task](../feat-history-first-ui/plan.md) and [the design specification](../../design/08-history-first.md).

## Steps

1. Verify clean, up-to-date main, unused v0.10.3 tag, and complete `make check` before any edits.
2. Run `pnpm bump 0.10.3`; verify exactly four version changes, including only the gitwave entry in Cargo.lock.
3. Move the interface from Unreleased into the v0.10.3 README entry; update Features, site badge/date/download/feature cards, PM version references and scope, and current design status. Preserve historical release rows and feature-task records.
4. Check version consistency, documentation links, unchanged branding and frozen frontend lockfile; run final quality gates and independent code-reviewer audit.
5. Commit locally as `chore: bump version to v0.10.3`, then present the final push decision.

## Release behavior

After explicit confirmation, create v0.10.3 at the release commit on main and push main plus that tag. The tag builds three-platform installers, a draft GitHub Release and latest.json. Publishing the draft is a separate manual user action and activates update delivery.

## Verification

The clean-main preflight and final post-bump `make check` both passed: formatting, Tauri compatibility, ESLint, clippy, TypeScript and cargo check, plus 4 Node checks, 348 Vitest tests and 386 Rust tests (2 existing ignored tests). Four version spots agree on 0.10.3; Cargo.lock changes only the gitwave version; pnpm-lock.yaml is unchanged. Site badge/download and the client-local date (2026-10-14) match; old-version residue is limited to historical release notes. Markdown targets resolve and `git diff --check` passes.

No icon files changed since v0.10.2; no branding copies are needed. The pre-existing README screenshot is explicitly labeled as an earlier release. Independent review is recorded in [review.md](./review.md). No release tag or remote release has been created during preparation.

References: [release workflow](../../tech/engineering/00-overview.md), [UI review](../feat-history-first-ui/review.md).
