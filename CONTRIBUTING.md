# Contributing to GitWave

> Workflow rules and conventions. See `AGENTS.md` for the authoritative source.

## Quick orientation

- **New feature / idea** → product-manager workflow (see `AGENTS.md` §Requirements flow)
- **Bug / regression** → tester workflow (see `AGENTS.md` §Defect flow)
- **PR ready for review** → code-reviewer workflow (see `AGENTS.md` §Code review flow)

## Branch + commit conventions

- Branch from `main`: `feature/<name>` or `fix/<name>`
- Commit messages: [Conventional Commits](https://www.conventionalcommits.org/)
- Merge into `main` with a merge commit (no squash); branch commits follow Conventional Commits
- AI agents must not commit / push / merge without explicit user instruction

## Local quality gates

The desktop app uses the root Go module. Its desktop command is `cmd/gitwave`, including the NSGo configuration. Use Git 2.46 or newer and put `git-lfs` on `PATH` for the Git fixtures.

```bash
make check      # Go formatting, go vet, and Go tests.
make format     # Format Go source files in place.
make test-race  # Run Go tests with the race detector.
go -C cmd/gitwave tool nsgo test -target macos -run '^TestNativeCommandPaletteAIMutationsRequireConfirmation$'
```

`make fmt-check`, `make lint` and `make test` run the three checks individually. The Go tests cover the verified legacy behavior for History, diff, blame, the staging index, commits, discard and repository health.

### Pre-commit hooks

The pre-commit hook checks changed Go sources with gofmt. Run `pre-commit install` to enable it and `pre-commit run --all-files` to check all Go sources. After switching from the old toolchain, run `pre-commit uninstall --hook-type commit-msg` if you installed the old commit-message hook.

Review documentation changes by hand. Check Conventional Commits and Markdown by hand.

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
