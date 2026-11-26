# Contributing to GitWave

> Workflow rules and conventions. See `AGENTS.md` for the authoritative source.

## Quick orientation

- **New feature / idea** → product-manager workflow (see `AGENTS.md` §Requirements flow)
- **Bug / regression** → tester workflow (see `AGENTS.md` §Defect flow)
- **PR ready for review** → code-reviewer workflow (see `AGENTS.md` §Code review flow)

## Branch + commit conventions

- Branch from `main`: `feature/<name>` or `fix/<name>`
- Commit messages: [Conventional Commits](https://www.conventionalcommits.org/). Nothing enforces them automatically any more — `commitlint` left with the Node.js toolchain — so follow them by hand
- Merge into `main` as one squash commit that takes the PR's title, so the title follows Conventional Commits; so does every commit of the branch
- AI agents must not commit / push / merge without explicit user instruction

## Local quality gates

`make check` is the pre-delivery gate and the equivalent of what CI runs (`make check` = `fmt-check` + `lint` + `test`):

```bash
make check      # CI-equivalent: format check + clippy + all tests, changes nothing
make fmt-check  # cargo fmt --all -- --check
make lint       # cargo clippy --workspace --all-targets -- -D warnings
make test       # cargo test --workspace --all-targets
```

`make format` is the only target that rewrites files. Or run the commands individually, from the repository root:

```bash
cargo fmt --all -- --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace --all-targets
```

A `pre-commit` hook runs rustfmt and clippy on `git commit` whenever Rust files are staged:

```bash
pre-commit install --install-hooks   # once
pre-commit run --all-files           # manually
```

The repository is one Cargo workspace with two crates: `crates/gitwave-core` (the domain, application and infrastructure layers) and `crates/gitwave` (the GPUI Kit application). The workspace commands cover both.

No formatter or linter covers Markdown, YAML or JSON, so documentation, workflows and the interface catalogues under `crates/gitwave/locales/` are verified by hand; don't expect an automated check to stop a stale or badly formatted doc. The tests in `crates/gitwave/src/locales.rs` do check that both languages carry the same keys and placeholders.

## PR checklist

- [ ] Branch is `feature/<name>` or `fix/<name>` (not `main`)
- [ ] Commit messages and the PR title follow Conventional Commits
- [ ] `make check` passes locally
- [ ] All CI jobs pass on the PR
- [ ] PR description references the proposal (`docs/pm/features/F<编号>.md`) or task (`docs/tasks/<任务名>/plan.md`)
- [ ] At least one code-reviewer has approved
- [ ] No force pushes; no merge commits in the PR

## Engineering conventions

See `docs/tech/engineering/00-overview.md` for code style, testing strategy, error handling, CI, security, and performance budgets.

## Questions?

Open an issue using the appropriate template (`.github/ISSUE_TEMPLATE/`).
