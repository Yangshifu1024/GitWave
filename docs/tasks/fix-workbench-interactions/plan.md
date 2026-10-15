# Workbench interaction and layout follow-ups

Status: implemented and verified; unreleased. Date: 2026-10-15.
Branch: `fix/history-graph-geometry`, explicitly retained by the user. No commit/push/release authorized.

## User reports and confirmed causes

1. Update modal cannot close through X, Later or Escape. The controlled state closes, but React Aria 3.53.0 retains exiting overlays when an animation `finished` promise rejects. Reproduced with real HeroUI components and canceled exit animations in the browser; the same rejection model made all four new dismissal tests fail before the fix. This reproduces a matching mechanism, not a native trace of the user's original event.
2. Selected-file diff stops at 560px with unused space below. DiffText kept its old bounded height within the new full-height workbench.
3. User requests removal of the entire history subheader and compact graph toggle; always show full topology.
4. User requests file-section title, count and action buttons on one line.
5. Blame attribution overflows its 200px gutter: SHA, author and locale date all had shrink-0; wrapped code further breaks visual row correspondence.
6. History metadata drifts horizontally because its flex children use content-dependent widths.
7. User requests branch-style selection across navigation/history/files, and full toolbar branch names when space permits.

## Implementation

- Unmount the controlled Modal backdrop immediately when open=false. Entry animation and focus cleanup remain; exit animation is intentionally skipped. Hide-download leaves updater state intact and ready reopens it.
- Give compact selected text diffs a complete bounded height chain. DiffText's existing virtualizer owns scrolling; multi-file and image paths retain their existing layout.
- Remove compact state, toggle, masking styles and unused translations. Keep 18px lanes, unscaled SVG geometry, paging, selection reveal and context-menu actions.
- Keep file-section controls on one row. Narrow sections scroll horizontally rather than losing controls.
- Blame uses fixed attribution/line-number columns, a truncatable author and nonwrapping date. Horizontal code scrolling keeps metadata pinned only in panes >=512px. Narrow panes use a 12rem attribution column, keep the date in title, and scroll all columns together. Full attribution is available through a native title; the inspector owns the single path header.
- History uses identical grid columns on every row, including an empty refs slot; SHA, author and relative time each have their own aligned column.

- Match branch selection styling: square full-width rows, 3px accent edge, 10% accent fill and normal text color. File multi-selection fill stays distinct from the active-only preview edge.
- Remove BranchIndicator fixed 200/120px maxima; local/upstream names share remaining space and retain full titles.

- Prioritize tags over local/remote branches for the primary ref badge and expanded refs list; preserve tracked-ref folding and counts.

## Validation

- Real Modal/Button integration: Later, X, Escape, canceled exit, reopen, hide-download/ready.
- DiffText: 3000 lines in unified/split, resized viewport and final-line reachability with bounded DOM count.
- Existing graph tests updated for full mode; preserve geometry, paging/selection and refs interactions.
- Blame late-response isolation and full attribution/header behavior.
- Real browser source fixtures with mocked IPC (no real Git writes or updater install): tall/narrow windows, aligned metadata, diff and blame scrolling, focus restoration.
- Full `make check`, production build, specialized code review before delivery.

## Boundaries

No changes to Git write/confirmation protections, updater installation or restart logic. No version bump. Browser fixtures are temporary and removed from the repository after verification. macOS browser verification does not establish Windows/Linux native WebView coverage.

## Verification results

- `make check`: passed (2026-10-15), frontend 61 files / 367 tests plus 4 Node checks; Rust 386 unit tests, 2 existing ignored, 1 restart integration test. Initial sandbox run failed on local listener/log permissions; the authorized unsandboxed run passed.
- `pnpm build` and `git diff --check`: passed.
- Browser: real source components with mocked IPC. X/Later/Escape remove overlays even in cancellation fixture; reopening and focus restoration pass. At 1440×1100, selected diff viewport measures 918px, 3000-line content renders 58 rows; at 1100×720 it shrinks to 538px and scrolling reaches line 3000.
- At 1024px window / Changes split58%, Blame pane345px uses191px attribution, static columns and title-retained date; code remains reachable. Wide attribution is aligned and the duplicate file header is removed.
- History sampled rows have identical refs/SHA/author/time x positions. Full local/upstream branch labels fit their content widths when space is available. Navigation/file selection has square edges, matching10% tint, normal text and3px accent edge.
- Screenshots: `/private/tmp/gitwave-history-final.png`, `/private/tmp/gitwave-blame-final.png`. Fixtures and browser tabs cleaned up. No native update install or real Git write was performed during browser QA.
