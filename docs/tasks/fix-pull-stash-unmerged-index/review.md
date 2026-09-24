# Review: pull with an unmerged index

## Correctness

The preflight checks run before stash and fetch. The regression tests cover an unmerged index with stash enabled and disabled, plus an unfinished merge whose index has no conflicts. They verify that the affected branch, index, worktree content, stash, and remote tracking ref are preserved where applicable.

## Security and performance

The checks use the local repository index and state and perform no network operation. They add negligible work before a pull.

The UI now reads conflict entries even without `MERGE_HEAD`. An orphaned index conflict is presented with a Resolve action. The destructive Abort merge action is shown only when a merge is actually in progress.

## Maintainability and readability

Separate error codes distinguish unresolved files from a pending Git operation. Both have English and Chinese messages. The checks remain in the pull orchestration function beside the stash logic.

## Test coverage and practices

Code review initially identified missing assertions for fetch and worktree preservation and an unfinished merge after conflict resolution. Those were added before completion. Rust tests, Clippy, format check, and cargo check pass. Frontend tests, typecheck, format check, and lint pass (lint reports 40 existing warnings and no errors).

The follow-up UI review covered the reported zero-count state. A regression assertion now confirms conflict listing survives the absence of `MERGE_HEAD`.

The UI review also found that an old conflict poll could overwrite the state after a repository switch. The hook now keys its displayed snapshot by workspace and repository and discards stale overlapping responses using a request sequence.

## Conclusion

No unresolved critical issues identified. The CodeWave conflict itself still requires a user decision about how to resolve the file; this change makes the app report that condition before attempting pull.
