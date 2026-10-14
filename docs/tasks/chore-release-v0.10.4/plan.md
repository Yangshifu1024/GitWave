# Release v0.10.4

## Scope

Release the history graph geometry and metadata alignment fixes merged through PRs #180 and #181, plus dependency PRs #164–#179. The user confirmed v0.10.4 and continuing on main. All PRs use merge commits.

## Execution

- Review all 16 dependency PRs using the seven dimensions in `.agents/agents/code-reviewer.md`.
- Re-run #175's failed Windows frontend job: the original failure was a synchronous locale-date formatting test timing out at 5 seconds; the rerun passed.
- Rebase #178 after explicit user authorization, resolving its lockfile conflict by retaining main's dependency graph and updating only Node types 26.6.4 → 26.6.5 with the original integrity. Independent review verified exact equality to that transformation. All 12 checks passed before merge.
- Fast-forward local main; verify no open PRs remain. Run frozen install and `make check` before bumping.
- Run `pnpm bump 0.10.4`; sync README, website and PM current-version references, preserving historical version rows. Release skill sync points remain applicable without changes. Icons are unchanged since v0.10.3.
- Commit the release; request the explicit main/tag push confirmation required by the release skill. Build CI creates a draft; verify three-platform assets, signatures and plain download URLs before manual publication.

## Validation

- All 16 dependency PRs passed 12 three-platform lint/test checks before merge.
- Final dependency combination: frozen pnpm install and production build passed. Build reported an advisory about an existing code-splitting group lacking a debugName.
- Updated main: `make check` passed, including 352 frontend tests in 56 files, 385 Rust unit tests and one updater restart integration test. Two existing Rust tests remain ignored.
- Historical v0.6.2 differs locally and remotely; it was preserved. Fetch/pull used `--no-tags`; v0.10.3 is the previous release tag.

## References

- [History geometry](../fix-history-graph-geometry/plan.md)
- [Metadata alignment](../fix-history-column-alignment/plan.md)
- [Review](./review.md)
- [Engineering conventions](../../tech/engineering/00-overview.md)
- [PRs](https://github.com/Yangshifu1024/GitWave/pulls?q=is%3Apr+is%3Amerged)
