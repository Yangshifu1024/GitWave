# Contributing to GitWave

> Workflow rules and conventions. See `AGENTS.md` for the authoritative source.

## Quick orientation

- **New feature / idea** → product-manager workflow (see `AGENTS.md` §需求流程)
- **Bug / regression** → tester workflow (see `AGENTS.md` §缺陷流程)
- **PR ready for review** → code-reviewer workflow (see `AGENTS.md` §代码审查流程)

## Branch + commit conventions

- Branch from `main`: `feature/<name>` or `fix/<name>`
- Commit messages: [Conventional Commits](https://www.conventionalcommits.org/) — enforced by `commitlint` via `pre-commit` hook
- Squash merge with the same Conventional Commit subject
- AI agents must not commit / push / merge without explicit user instruction

## Local quality gates

All run automatically via `pre-commit` hook on `git commit`. To run manually:

```bash
pre-commit run --all-files
```

Or individually:

```bash
# Rust (from src-tauri/)
cargo fmt -- --check
cargo clippy --all-targets -- -D warnings
cargo test --all-targets

# Frontend
npm run lint
npm run format:check
npm run typecheck
npm test
```

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