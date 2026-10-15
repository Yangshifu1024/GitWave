# Tide Workbench · phase 1 implementation review

Date: 2026-10-15. Branch: `fix/history-graph-geometry`. Status: code review passed; full gate result below. This section supersedes the proposal-only review preserved at the end.

## ✅ Strengths

- Stable Workspace/repo shell with peer History / Changes, resizable splits and file-first compact diff.
- Existing Git entry points, conflict editor, amend/discard dialogs, checkout gates and explicit commit/push behavior retained.
- Writes and commit-message AI requests pin repository identity; backend checks workspace membership. Selection/drafts are scoped independently.
- Bounded/lazy previews, virtual history and image/blame views reused without a renderer rewrite.

## 🔴 Critical findings

None found by the specialized code-reviewer in either implementation pass.

## 🟡 Findings resolved before delivery

| Finding | Fix / evidence |
|---|---|
| Write failures after repo switch disappeared with the old component | Repository-scoped error store; unmount → reject → return test |
| Changes actions could clip at narrow widths | Wrapping toolbar, selection-only actions, discard in More; source fixture checked at 1024×768 and 28% Changes split |
| Stage/unstage invalidated immutable history previews | Invalidation predicate limits refresh to working-copy previews |
| New compact diff needed direct test coverage | Component test exercises fold/unfold, split toggle and Blame/back |
| Initial Rust target test read cached index | Reopen repository for on-disk assertions; targeted test passed |

## Seven-dimension assessment

Correctness and security: explicit repo variables and server membership checks preserve operation targets. Performance: existing virtualized/bounded readers retained, with no duplicate full diff or history query added. Maintainability/readability: small workbench containers and reusable split separate layout from Git behaviors. Test coverage: added repo/session/late-result/selection/keyboard/compact-preview tests. Best practices: keyboard-resizable separators, explicit destructive confirmation and retained async feedback. The reviewer approved the second pass with no remaining blocker.

## Validation

- Frontend: **59 Vitest files / 359 tests passed**, plus **4 Node checks**.
- `pnpm build`: passed (TypeScript + production Vite bundle).
- New backend fixture uses two temporary repositories and an in-memory workspace DB: stage / unstage / ignore / discard operate on explicitly selected A while active repo is B; cross-workspace repo IDs are rejected.
- Browser source-component fixture (real Workbench, HistoryDetails, ChangesPanel, DiffViewer and BranchList; disposable in-memory IPC responses): History file preview; Changes select/stage and side-follow; mode round-trip retains draft/selection; minimum split via keyboard; More → discard confirmation → cancel; light and dark Tide. Viewports inspected: 1280×720 and 1024×768. Temporary fixture and server removed afterward.
- Full `make check`: **passed**, exit 0. Rust: **386 unit tests passed / 2 existing ignored**, plus **1 integration test passed**. Frontend/format/lint/typecheck/Clippy/cargo check all passed. The final run used permission to allow existing loopback listener and application-log tests; the initial sandbox failure and cached-index assertion were resolved before this run.
- `git diff --check` and relative Markdown links checked at delivery.

## Limits and scope

The UI evidence uses current source components with fake IPC data, **not the installed v0.10.4 app or a native end-to-end Git run**. No Windows/Linux UI run, full screen-reader audit, increased-font matrix or performance benchmark is claimed. Rust tests validate actual temporary repositories separately. Existing HeroUI PressResponder warnings were observed in the fixture; controls exercised above worked. Full App titlebar/menu integration remains a native smoke-test item.

No commit/push/version bump performed. README and PM/design docs mark this work unreleased; site release badge and published-history claims remain on v0.10.3. Release skill sync points were inspected and already cover these surfaces, so the skill itself needed no change. Full redesign phases beyond the user-approved core workspaces remain in the plan.

---

# Workbench redesign proposal review

Date: 2026-10-15. Scope: **design proposal and isolated interactive illustration**, not production implementation approval.

Reviewed by the code-reviewer specialist using `.agents/agents/code-reviewer.md`; product-manager supplied the product analysis. Findings were applied by the primary agent. Source baseline and installed-app observations remain explicitly separated in the design document.

## Strengths

- Product capabilities are mapped to a coherent workbench without a framework rewrite.
- Existing performance, privacy, mutation and draft protections are retained as acceptance criteria.
- Workspace-only mutation targeting is identified before introducing freely navigable task views.
- Proposed features, existing capabilities and unverified behavior are distinguished.

## Findings and resolution

| Finding | Resolution |
|---|---|
| Staged mock file lost its selectable entry after stage, preventing return/unstage | All mock files now have selectable staged entries and reversible stage/unstage; verified in browser |
| History and Changes mock shared file selection | Separate historyFile/changeFile state; round trip verified |
| Force push wording could imply existing lease protection | Design states current force push has no lease; adding it requires a separate backend implementation |
| Commit context-menu actions not individually mapped | Added checkout/reset/cherry-pick/revert/create branch/tag/copy mapping and failure constraints |
| Mock topology/diff could be mistaken for data validation | Design explicitly identifies sample graph/diff and excludes topology/content correctness claims |

## Seven-dimension assessment

| Dimension | Conclusion |
|---|---|
| Correctness | Sample navigation fixes verified; no claim of production Git correctness or full runtime parity |
| Security | Illustration uses local sample data and no network/Git mutation; design preserves offline/scrubber/explicit-action boundaries |
| Performance | Existing budget and virtualization retained in plan; no performance measurement performed this turn |
| Maintainability | Proposed shell/view/command/controller boundaries address current container coupling |
| Readability | Current evidence, recommended behavior and new capabilities have separate sections |
| Test coverage | Proposal has a concrete future regression matrix; current checks only cover documents and illustration |
| Best practices | Accessible primitive reuse, keyboard parity, state scoping and progressive migration are planned |

## Validation performed

- Checked new Markdown relative links: no missing targets.
- `git diff --check` passed.
- Illustration script syntax checked with Node; standalone preview generated with the visualization renderer.
- Browser: History/Changes switching, independent file selection, stage → select another file → return to staged file → unstage, conflict choice and resolved progress, design variant switch.
- Visually inspected the two layout directions at 1024px and the Changes view at 360px; checked 320px and 360px container widths for horizontal overflow (none).
- Browser error log was empty in the exercised paths.
- No production UI source or version metadata changed. No commit/push/merge performed.

## Remaining verification

Dark theme, native three-platform behavior, full keyboard/screen-reader interaction, actual Git mutations, all error states, topology correctness and performance remain implementation-stage verification. `make check` was not run for this documentation/illustration-only phase; it remains mandatory for production implementation delivery. No production merge approval is implied.
