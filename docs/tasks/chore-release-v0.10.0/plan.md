# chore-release-v0.10.0 · Release preparation

## Scope

Prepare v0.10.0 from main, confirmed by the user on 2026-10-12. Since v0.9.6, this release includes the repository activation fix from [fix-add-repo-activation](../fix-add-repo-activation/plan.md), the pnpm lockfile duplicate-entry fix and dependency updates.

## Steps

1. Fetch tags, verify a clean main and fast-forward from origin.
2. Run the full Makefile check gate before changing version files.
3. Run `pnpm bump 0.10.0`; verify exactly the four version files changed and Cargo.lock only updates GitWave's own version.
4. Sync README, the website and PM current-version references; preserve historical release rows. Verify the existing branding copies match their source icons.
5. Obtain code-reviewer review, then commit `chore: bump version to v0.10.0`.
6. Wait for explicit push approval. Only then create the annotated tag and push main + v0.10.0. Release CI builds three-platform installers and latest.json into a draft; publishing is a separate manual user action.

## Validation

`make` is unavailable on PATH, so its equivalent commands were run individually: Prettier, cargo fmt check, ESLint, TypeScript, cargo clippy with warnings denied, cargo check and both test suites. All passed before the bump: frontend 49 files / 328 tests; Rust 385 passed / 2 ignored / 0 failed.

The frontend dependencies were installed with `pnpm install --frozen-lockfile`; no dependency lock changes were introduced. Rust emitted incremental-cache access notes on Windows, but clippy, check and tests completed successfully. No application GUI smoke test or three-platform installer build was performed locally; those remain separate from the verified local gates.

Release process: [.agents/skills/gitwave-release/SKILL.md](../../../.agents/skills/gitwave-release/SKILL.md). Engineering conventions: [docs/tech/engineering/00-overview.md](../../tech/engineering/00-overview.md).
