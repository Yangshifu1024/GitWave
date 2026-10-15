# Sync main into the workbench branch

Date: 2026-10-15. Branch: `fix/history-graph-geometry` (user-approved current branch).

## Situation

The user reported that the current branch lagged main. After fetching origin, HEAD `85db011` was an ancestor of `origin/main` `ac84fa5` by37 commits. No new merge commit or rebase was required. The working tree contained50 edited/untracked paths for the accepted Tide workbench and follow-up fixes.

## Recovery and integration

- Saved all50 paths plus the tracked binary diff and original status in `/private/tmp/gitwave-before-main-79rifgm1`.
- Preserved a separate stash at `87aad6cc241678e5f16132054dcf10bd6737ad2b`, message `codex: preserve Tide workbench before main sync`. The backup remains available; it must not be reapplied onto the already restored tree.
- Fast-forwarded the current branch to `ac84fa5` (`git merge --ff-only origin/main`). Did not change the separate local main branch or push any branch.
- Applied the stash, resolved6 conflicting files, and restored all changes to unstaged status. No original path is missing. Only8 paths differ from the backup, all overlapping main changes: README, the historical design,3 PM core docs, site, CommitGraph and tokens.
- CommitGraph retains full topology, grid-aligned columns, tag-first refs and branch-style selection; also retains upstream `history-scroll` behavior. Upstream CSS scrollbar rules coexist with workbench styles.
- README/PM/site retain released v0.10.4 facts and the unreleased workbench scope. Dependency/version files are exactly upstream; this task does not cut a release.

## Validation

Install the frozen pnpm lockfile, run `make check` and production build, review the8 integrated files, verify no unresolved paths and HEAD==origin/main. Preserve existing Git operation protection and the uncommitted workbench changes.

Status: complete, changes remain uncommitted.

## Results

- `pnpm install --frozen-lockfile` passed with the project proxy. Lockfile/version files remain identical to upstream.
- `make check` passed on the new dependencies:61 frontend files/367 tests plus4 Node tests;386 Rust unit tests passed,2 existing ignored,1 restart integration test passed.
- `pnpm build` and `git diff --check` passed.
- Independent review verified all50 original paths:42 byte-identical,8 expected integrations. No unresolved paths or staged changes remain.
- HEAD and origin/main both point to `ac84fa5`; ahead/behind relative to main is0/0. The remote feature branch was not pushed, so its tracking status locally reads ahead37.
- Recovery archive and stash are retained. No project commit, rebase or push was performed.
