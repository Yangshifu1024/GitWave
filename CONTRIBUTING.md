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

All run automatically via `pre-commit` hook on `git commit`. To run manually:

```bash
pre-commit run --all-files
```

`make check` is the same set of checks in one command, and is the equivalent entry point for the individual commands below (`make check` = `fmt-check` + `lint` + `test`):

```bash
make check      # CI-equivalent: format gates + lint + all tests, changes nothing
make fmt-check  # pnpm exec prettier --check . + cargo fmt -- --check
make lint       # pnpm lint + cargo clippy + pnpm typecheck + cargo check
make test       # pnpm test + cargo test --all-targets
```

`make format` is the only target that rewrites files. Or run the commands individually:

```bash
# Rust (from src-tauri/)
cargo fmt -- --check
cargo clippy --all-targets -- -D warnings
cargo test --all-targets

# Frontend
pnpm lint
pnpm format:check
pnpm typecheck
pnpm test
```

Markdown is not covered by Prettier — `.prettierignore` excludes `*.md` — so neither `pnpm format:check` nor `make fmt-check` ever inspects documentation. Doc changes are verified by hand; don't expect an automated check to stop a stale or badly formatted doc.

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