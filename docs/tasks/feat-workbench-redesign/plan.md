# GitWave workbench redesign · Engineering plan

Status: **phase 1 accepted and implemented; unreleased; validation recorded in review.md**.
Date: 2026-10-15. Source baseline: `fix/history-graph-geometry`, `85db011` (0.10.3).

Product source: [F018](../../pm/features/F018-workbench-redesign.md).
Design: [full redesign proposal](../../design/09-workbench-redesign.md).
Existing decisions: [architecture](../../tech/architecture/00-overview.md), [ADRs](../../tech/decisions/00-overview.md), [engineering conventions](../../tech/engineering/00-overview.md).

## 1. Goal and boundary

Replace the fixed History shell and task-specific overlays with a workspace/repository-scoped workbench. Preserve existing Git use cases, authentication, privacy controls, virtualized renderers and explicit mutation gates. No framework migration is necessary to evaluate this proposal.

This turn produces analysis, design documents and an isolated interactive illustration. It does not alter production UI, version metadata or repository state. Installed-app observations are from v0.10.4; source findings are from the baseline above.

## 2. Current coupling

- `App.tsx` fixes CommitGraph and MainContent into ThreePaneLayout; selection is local to App.
- `ActionBar.tsx` is 1,695 lines at this baseline and owns titlebar controls, menus, forms and mutations. Changing the shell without separating ownership would increase coupling.
- `BranchList.tsx` is 1,193 lines; navigation and several operation dialogs share the component.
- `ChangesPanel.tsx` is 855 lines with stacked/bar/modal variants. Its commit draft store is reusable; its container and presentation need separation.
- `DiffViewer.tsx` is 688 lines and combines source queries, file summary, multi-file expansion, image rendering, blame and global inspector maximize state.
- `ConflictPanel` has a custom overlay with local drafts; mounting/unmounting it in a router without moving session ownership could discard buffers.
- `layoutStore` only tracks inspector maximization; pane sizes and other preferences are stored separately. Navigation restoration requires an explicit model.
- Several mutations take workspaceId and resolve the active repository on the backend. More freely navigable UI must not assume that adding repoId to a React Query key pins the mutation target.
- Existing E2E script/dependency is not an implemented E2E suite. Do not report it as coverage.

## 3. Proposed component boundaries

```text
AppShell
  WorkspaceMenu / RepositoryTabs / RepositoryToolbar
  RepositoryNavigator
  RepositoryViewHost
    HistoryView
      CommitGraph + CommitSelectionHeader
      FileChangeList + DiffSurface
    ChangesView
      WorkingCopyFileList + CommitComposer + DiffSurface
    IntegrationView
      OperationHeader + ConflictEditor / RebasePlan
    StashView / RecoveryView / RepositoryToolsView
  ContextAssistant (on demand)
  OperationStatus / OperationHistory
  DialogHost (short forms, confirmations, credentials, update, quit)
```

These names are implementation suggestions, not new package requirements. Keep React, current state/query tooling, Tauri and Rust use cases. Reuse HeroUI/React Aria behavior; make product geometry and colors authoritative in wrappers. Do not replace accessible primitives with untested custom overlays solely for appearance.

Extract a shared command registry used by toolbar, menu, context menu and palette: id, label key, scope, enabled reason, shortcut, intent/confirmation requirement and handler. Existing gates remain authoritative. Local commands and AI interpretation are separate paths.

## 4. State and concurrency contract

Repository view key: `(workspaceId, repoId)`.

| State | Lifetime | Rule |
|---|---|---|
| mode, History selected SHA/file, filter, scroll anchor | bounded session map | Restore when returning; validate deleted/unreachable objects; use SHA anchor rather than only scroll pixels |
| Changes file+stage side, multi-selection | session | Reconcile after stage/unstage/refresh; never key just by path |
| commit draft/amend target | existing scoped draft store | Preserve message-only amend; clear only submitted draft; no cross-repo AI overwrite |
| conflict buffers, dirty state, plan edits | task session | Route transition goes through leave guard; use snapshot checks when disk changes |
| AI task result | session, scope+object+request id | Ignore superseded results; mark inputs stale if staged state changes |
| layout, last mode, section disclosure | versioned local preference | Migrate old keys safely; temporary fit does not overwrite preferred sizes |
| Git operation | captured operation identity | Store target workspace/repo/remote/ref and request id; UI navigation cannot retarget it |
| operation history | bounded in-memory ring | No credentials, raw diff, AI prompts or hidden persistent telemetry |

Before enabling navigation during mutations: audit every workspace-only IPC endpoint. Either pin/validate the repository in the backend use case or temporarily gate repository switching for the affected operation. Preferred end state is explicit repo identity. Keep Rust and manually mirrored api.ts types in sync; frontend guards alone are insufficient to claim this solved.

Do not unmount an operation controller when changing the visible view. Conversely, do not keep every heavy view and polling hook mounted merely to retain UI state. Retain lightweight state, mount the active content, and preserve a single operation coordinator.

## 5. Migration work packages

| Phase | Work | Exit evidence |
|---|---|---|
| A · baseline and design | Current screenshots/flow inventory; clickable History/Changes/Resolve illustration; identify source vs installed build | User chooses IA/visual direction; unresolved scope items listed |
| B · foundation | Product tokens/wrappers, command registry, repository view state, operation identity/gates | Component keyboard/focus contract; scope and late-result tests |
| C · core tasks | Replace shell; embed Changes; History vertical split; extract shared file list/diff/composer | Commit/history/workspace journeys; no performance regression against measured baseline |
| D · integration and tools | Conflict/rebase/stash/recovery/tools move into work areas | Every existing command mapped; dirty/abort/partial failure exercised |
| E · assistant and settings | Contextual AI, local palette, scope-aware settings, operation history | Offline/provider/fallback behavior; no implicit AI network calls on local search |
| F · hardening and docs | Three-platform, themes, localization, accessibility, QA, documentation and release readiness | Full gate, code-reviewer report, manual matrix, no untracked feature loss |

Avoid estimating calendar dates before prototype acceptance and IPC scope audit. Work packages can be reviewed separately while keeping the final whole-interface objective. Keep a temporary old-shell fallback during integration; remove it and dead layout branches after parity is verified.

## 6. High-risk details

1. History graph: retain cursor paging, memoized rows, graph geometry and virtualization. New layout must not trigger a second independent history query or refetch on every resize.
2. Diff: separate source identity from layout; preserve per-file loading, byte/line preview budgets, unknown statistics, binary/image states and small-file syntax highlighting. Add no hunk/line staging without a separately scoped backend implementation.
3. Conflict: model isolated index conflicts, merge, rebase edit pause, conflicts and auto-abort separately. Existing resolve writes+stages; a separate Save action needs a separate use case. Show only supported Continue/Abort actions.
4. Mutation isolation: capture repo and ref at trigger/confirmation time, revalidate before execution, prevent a stale dialog or retry from changing a different repo.
5. Stash: displayed OID is stable, index is not; revalidate target on apply/pop/drop after refresh. Preserve untracked-file previews and dirty checks.
6. AI: preserve scrubber/offline restrictions and explicit user trigger. No streaming UI claim when backend returns whole responses. Restoring a draft must remain possible.
7. Settings: preserve user's font sizes/palette and Workspace provider configuration. Don't accidentally turn Workspace settings into global settings while reorganizing panels.
8. Modal and pane accessibility: focus restoration, Escape/dirty guards, keyboard resize, rebase move controls, labels and platform shortcuts. Don't assume draggable equals keyboard operable.

## 7. Verification plan

For the initial proposal (completed): manually verify source references, official-source URLs, internal Markdown links, absence of production modifications, and illustrate primary navigation with local mock data. The inline illustration is not a production E2E test.

For implementation:

- Meaningful component/store tests: mode/repo round trip; late response after switch; stage selection movement; draft protection; pending operation target; failure retention; palette local Enter vs explicit Ask AI; conflict navigation guards; keyboard rebase sorting.
- Retain existing history/diff/checkout/auth/updater regression suites and Rust integration fixtures.
- Use disposable Git fixture repositories for dirty checkout, staged+unstaged same file, conflict, amend, rebase pause/auto-abort, stash untracked and pop conflicts. Never use user's working repos for destructive test setup.
- Manual native UI matrix: macOS/Windows/Linux, English/Chinese, Native Blue/Tide, light/dark, 1024×768/1280×800/1440×900, long paths/refs and increased font size. Current installed-app inspection covers only a subset.
- Record baseline and post-change cold start/history/diff timings using the engineering budget and the same hardware/repository/cache conditions. Verify topology and frame behavior on real WebViews.
- Before code delivery/commit/push/PR: run `make check`, fix failures, and invoke `.agents/agents/code-reviewer.md`; record the seven-dimension review. No automatic commit/push/merge.

## 8. Documentation on implementation

When accepted, update the design index and current IA decision; when delivered, sync README status/what's-new, PM features/scope/roadmap, site feature descriptions and release skill sync points as required. Do not mark a design proposal as shipped. Version bump/release remains a separate explicit step.

## 9. Implementation outcome · phase 1 (2026-10-15)

User approved Tide Workbench on the existing `fix/history-graph-geometry` branch. Phase 1 is implemented and unreleased; no commit, push, version bump, or user-repository Git mutation was performed.

- `Workbench`, `HistoryDetails`, `ChangesWorkspace` replace the App's three-column shell. Existing workspace/repo controls and all sidebar Git tools remain accessible. The toolbar Changes shortcut always navigates to Changes; explicit conflict resolution retains the existing guarded panel.
- `WorkbenchSplit` unifies pointer/keyboard resizing, reset and persistent proportions. Navigator collapse and diff focus preserve mounted view state. Only the active repo is mounted; the two core views retain lightweight interaction state across mode changes, while hidden workdir diff polling is suspended.
- Per-Workspace+repo sessions retain mode, commit, history file and working-copy file selection. Commit drafts retain existing protections; stage/unstage follows the same path, including partially staged side identity.
- Compact commit metadata/diff chrome increases code area. Dialog radius maps to the 12px product token; optional type picker replaces ten chips. Changes selected actions appear on selection, discard lives in More plus its original confirmation, and composer scrolls safely at constrained heights.
- Working-copy writes and AI commit generation carry explicit repo IDs; Rust verifies workspace membership. Commit metadata reads also pin repo identity. Legacy optional-ID callers remain compatible. Mutation callbacks invalidate original caches and finish original drafts even after unmount; failures are retained by repository. Historical diff caches are not invalidated by staging.
- Existing graph geometry, cursor pagination, context menus, bounded previews, images, blame, amend/discard/checkout protections and explicit Git actions remain in place.

Deferred full-design items: command registry and standalone local history search (current button opens existing palette), operation history, alternate right-hand history layout, composer internal resize, commit-success navigation, conflict/stash/recovery/tools/AI/settings container migrations. No hunk staging or new Git capability is implied.

Verification evidence and remaining platform limits are recorded in `review.md`. Native Blue/Tide and font preferences are preserved. Old layout components remain unused for this migration phase; no product-facing old-shell toggle is added.
