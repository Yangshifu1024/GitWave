# Contributing to GitWave

> Workflow rules and conventions. See `AGENTS.md` for the authoritative source.

## Quick orientation

- **New feature / idea** → product-manager workflow (see `AGENTS.md` §Requirements flow)
- **Bug / regression** → tester workflow (see `AGENTS.md` §Defect flow)
- **PR ready for review** → code-reviewer workflow (see `AGENTS.md` §Code review flow)

## Branch + commit conventions

- Branch from `main`: `feature/<name>` or `fix/<name>`
- Commit messages: [Conventional Commits](https://www.conventionalcommits.org/) — enforced by `commitlint` via `pre-commit` hook
- Merge into `main` with a merge commit (no squash); branch commits follow Conventional Commits
- AI agents must not commit / push / merge without explicit user instruction

## Local quality gates

The Go desktop preview uses the root Go module. Its desktop command is `cmd/gitwave`, including the NSGo configuration. Use Git 2.46 or newer and put `git-lfs` on `PATH` for the Git fixtures.

```bash
make check      # Go formatting, go vet, and Go tests.
make format     # Format Go source files in place.
make test-race  # Run Go tests with the race detector.
go -C cmd/gitwave tool nsgo test -target macos -run '^TestNativeCommandPaletteAIMutationsRequireConfirmation$'
```

`make fmt-check`, `make lint` and `make test` run the three checks individually. The Go tests cover the verified legacy behavior for History, diff, blame, the staging index, commits, discard and repository health.

### Tauri preference exporter

Run `make legacy-check` for exporter changes. It runs the Rust and frontend format, lint, type and test checks. Its individual targets are `make legacy-fmt-check`, `make legacy-lint` and `make legacy-test`.

The exporter CI runs when its source, build inputs or the shared SQLite migrations change. Go source changes run through the Go desktop workflow.

### Pre-commit hooks

The pre-commit configuration checks changed Go sources with gofmt and retains the Rust and frontend hooks for exporter files. `pre-commit run --all-files` runs all source hooks, including the exporter hooks. The commit message hook uses commitlint.

Review documentation changes by hand. Markdown is outside the source format checks.

## PR checklist

- [ ] Branch is `feature/<name>` or `fix/<name>` (not `main`)
- [ ] Commit messages follow Conventional Commits
- [ ] All pre-commit hooks pass locally
- [ ] All CI jobs pass on the PR
- [ ] PR description references the proposal (`docs/pm/features/F<编号>.md`) or task (`docs/tasks/<任务名>/plan.md`)
- [ ] At least one code-reviewer has approved
- [ ] No force pushes; no merge commits in the PR

## Engineering conventions

See `docs/tech/engineering/00-overview.md` for code style, testing strategy, error handling, CI, security, and performance budgets.

## Questions?

Open an issue using the appropriate template (`.github/ISSUE_TEMPLATE/`).
