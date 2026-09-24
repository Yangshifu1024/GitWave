# Pull with stash rejects an unmerged index

## Defect

When the repository index contains unresolved conflicts, Pull with “Stash and reapply” calls libgit2 stash save. Stash cannot create a tree from unmerged index entries and returns `class=Index; code=Unmerged (-10)` without guidance. This was observed in CodeWave's `main` branch with an unresolved `src-tauri/src/core/agent/drive.rs` entry. CodeWave has no `MERGE_HEAD`, so the existing UI hides the conflict banner and reports zero changes.

## Fix

- Check the index for conflicts and the repository for an unfinished operation at the start of pull, before stash or fetch, for both values of the stash option.
- Return dedicated error codes with Chinese and English guidance for each condition.
- Preserve HEAD, index, worktree, and stash when this precondition fails.
- Poll index conflicts independently of `MERGE_HEAD`, show an actionable banner, count them in the toolbar, and open the conflict editor from the Changes button. Only offer Abort merge when `MERGE_HEAD` exists.

## Verification

- Rust regression test creates a conflicting merge index and checks that both pull modes return the dedicated error without changing HEAD, conflicts, worktree content, remote tracking ref, or stash.
- A second test checks a merge in progress with a resolved index.
- The conflict listing test checks that index conflicts remain visible after `MERGE_HEAD` is removed.
- Existing pull tests cover the clean and stash/reapply paths.
- Run the repository quality gates before delivery.
