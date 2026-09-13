# GitWave · AGENTS.md

> Project entry guide.

## Required reading

Read in this order:

1. [docs/pm/core/README.md](./docs/pm/core/README.md) — PM docs index
2. [01-features.md](./docs/pm/core/01-features.md) — product features and the out-of-scope list
3. [02-scope.md](./docs/pm/core/02-scope.md) — priorities and version scope
4. [03-roadmap.md](./docs/pm/core/03-roadmap.md) — release roadmap
5. [docs/pm/features/README.md](./docs/pm/features/README.md) — feature proposal workflow
6. [docs/tech/README.md](./docs/tech/README.md) — tech docs index (architecture / selection / ADRs)
7. [docs/design/00-overview.md](./docs/design/00-overview.md) — UI/UX design overview (3-pane / tokens / components / layout)
8. [docs/tasks/README.md](./docs/tasks/README.md) — task tracking (plan / review)

## Core constraints (from product principles)

- **AI is a collaborator, not a replacement**: no automatic commit / push / merge
- **Local-first + privacy under control**: diffs never leave the machine by default; HTTPS credentials go through the `git credential helper`; SSH uses the configured key
- **Workspace is the primary entry**: one active repo + multiple Workspaces open at once + Workspace-scoped AI
- **Workspace is an abstraction**: it has no filesystem entity and depends on no root directory

## PM / engineering boundary

Technology choices (Tauri / SwiftUI / Electron, …) are engineering decisions and **out of PM scope**. Engineering teams write architecture docs based on the user-perceivable constraints PM provides (performance, platforms, privacy, …). PM does not output tech choices; engineering does not output principles.

## Tech docs ownership (`docs/tech/` vs `docs/tasks/`)

Engineering docs split across two directories by "cross-task vs single-task" to keep ownership clear:

| Directory | Nature | One doc corresponds to | Typical content |
|---|---|---|---|
| `docs/tech/` | Cross-task engineering docs | Referenced by multiple tasks / PRs | System architecture, technology selection, ADRs, system design, engineering conventions |
| `docs/tasks/<feat\|fix>-<name>/` | Single-task execution artifacts | One PR / one task | `plan.md`, `review.md` |

Decision rules:

- Long-lived content reused across tasks (architecture diagrams, selection records, ADRs, naming / engineering conventions) → `docs/tech/`
- Execution process tightly bound to a specific PR / task (implementation plan, review report) → `docs/tasks/<task-name>/`

When `docs/tasks/<task-name>/plan.md` references an existing technical decision, link it as `docs/tech/<category>/<doc-name>`.

## Git workflow

### Branch strategy (GitHub Flow)

- `main` is the only long-lived branch
- New features / fixes branch off `main`: `feature/<name>` or `fix/<name>`
- Names align with `docs/pm/features/F<number>.md` or `docs/tasks/<feat|fix>-<name>/`

### Commit convention (Conventional Commits)

`<type>(<scope>): <subject>`, type ∈ `feat` · `fix` · `docs` · `refactor` · `test` · `chore`

**Commit messages must be written in English.**

Example: `feat(workspace): add lastActiveRepo persistence on workspace switch`

### PR merging (merge commit)

All PRs merge into `main` with a merge commit — branch history is preserved, no squash. **PR titles and descriptions (summary, change list, test plan) must be written in English** — they are the public face of the repository history.

### Key constraints

- **AI agents must not commit / push / merge automatically** (per P1) — exception: the user explicitly asks the agent to commit
- **main branch protection**: no force push; PRs must pass code-reviewer review
- **Every PR links a proposal or task**: the description references `docs/pm/features/F<number>.md` or `docs/tasks/<task-name>/plan.md`
- **Confirm the branch before new work / new issues**: before handling, ask whether to use a new branch
  - No: continue on the current branch
  - Yes: suggest a branch name (`feature/<name>` or `fix/<name>`, aligned with `docs/pm/features/F<number>.md` or `docs/tasks/<feat|fix>-<name>/`) and accept custom names; create it after confirmation

## Task wrap-up checklist

Before delivering a task (commit / push / PR), run the CI-equivalent checks and make them pass:

```bash
make check   # fmt-check (prettier + cargo fmt) + lint (eslint + clippy + typecheck) + all tests
```

Or individually — frontend: `pnpm format:check` · `pnpm lint` · `pnpm typecheck` · `pnpm test`; backend: `cargo fmt -- --check` · `cargo clippy --all-targets -- -D warnings` · `cargo test --all-targets`. CI runs the same gates on all three platforms — a red formatting/lint check on the PR means this step was skipped.

## Specialized agents

| Agent | When to use |
|---|---|
| **product-manager** | Requirements analysis, PRD, user stories, competitive analysis, prioritization |
| **code-reviewer** | Code review (correctness / security / performance / maintainability / readability / test coverage / best practices) |
| **tester** | Test case design, test strategy, defect analysis, automation advice |

Detailed agent behavior conventions live in `.agents/agents/<name>.md`.

Invoke the matching specialized agent per scenario.

### 1. Requirements flow (user files a new requirement)

Trigger: the user raises a new requirement / feature idea

1. Invoke `@.agents/agents/product-manager.md`
2. PM analyzes the requirement, asking clarifying questions when needed
3. PM writes it up as a structured proposal in `docs/pm/features/F<number>-<short-description>.md`
4. Engineering analyzes the requirement and produces the technical plan
5. The plan goes to `docs/tasks/<feat-task-name>/plan.md`
6. Status flow: proposal → accepted / rejected → merged

### 2. Defect flow (user reports an issue)

Trigger: the user reports a problem, bug, or unexpected behavior

1. Invoke `@.agents/agents/tester.md`
2. Tester reproduces the issue and analyzes the root cause
3. Tester proposes the best fix (change suggestions + regression test points)
4. The fix plan goes to `docs/tasks/<fix-task-name>/plan.md`
5. Apply and verify the fix

### 3. Code review flow (after development)

Trigger: development done, new code awaiting merge

1. Automatically invoke `@.agents/agents/code-reviewer.md`
2. Reviewer audits across 7 dimensions: correctness / security / performance / maintainability / readability / test coverage / best practices
3. Critical issues (🔴) must be fixed before merging
4. The review report goes to `docs/tasks/<task-name>/review.md`

## Specialized skills

- **gitwave-release** (`/gitwave-release`): version bump and release synchronization (the 4 hardcoded version spots, README status, the site pages) — use it whenever bumping a version, cutting a release, or syncing README / site content

## Terminology

- Technical terms like commit / rebase / merge / conflict / provider / prompt / BYOK / worktree stay in English
- Citations of external material must include a URL

## Documentation languages

Docs may be written in Chinese or English; agent-facing guides (like this file) are kept in English. Historical Chinese docs are not batch-translated. The app UI itself remains bilingual (Chinese / English).
