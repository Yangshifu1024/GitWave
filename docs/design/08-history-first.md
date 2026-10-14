# History-first interface

Status: included in v0.10.3. Accepted on 2026-10-14. Task: [implementation and verification](../tasks/feat-history-first-ui/plan.md).

This specification supersedes older card-based sidebar, full-width graph and centered titlebar-status guidance where they conflict. Workspace remains the primary entry; one repository is active. Git operations still require explicit user actions.

## Shell and navigation

Reading order: Workspace → repository → branch → history → selected commit → changed files.

- Keep one 40px titlebar with Workspace, external tools and grouped Git operations. Ordinary changes use accent text; unresolved conflicts use danger text.
- Repository tabs use intrinsic widths and an accent underline. Preserve wheel scrolling, active-tab reveal, drag reorder and menus.
- Start sidebar/inspector at 240/360px with 200/300px minima. History absorbs remaining width with a 360px minimum. Fit columns to the measured window; temporary reductions preserve preferred widths.
- Resize with mouse, ArrowLeft/ArrowRight (10px), Shift+Arrow (40px), Home or double-click reset. Keyboard focus remains visible.
- A 32px bottom status bar carries branch, progress, cancel and recent result without overlapping titlebar controls.

## Sidebar

Source-list groups have no card border, fill or rounded wrapper. Titles retain at least 28px height; expanded content scrolls within the remaining space. Empty groups retain static headers.

Health uses one native disclosure row. Clean/dirty counts are neutral; Git operation residue is visible in danger text even while collapsed. Expand for full metrics and AI advice. Errors remain explicit. Ordinary branches show one name; the selected row retains upstream/commit context. `HEAD` distinguishes the checked-out branch from the selected row.

## History

Keep fixed 28px rows, the virtualizer, 14-row overscan, memoized SVG/commit rows, cursor paging and cached dates. Default graph width is at most 112px: an SVG viewBox compresses horizontal geometry without removing nodes or edges. Full graph restores original spacing and horizontal scrolling while preserving the scroll element, vertical position and selection.

Messages precede refs/SHA, use regular weight and receive remaining space. Selected/HEAD messages use medium weight. Author/time disappear in narrow panes; SHA also hides below its breakpoint and remains available in details/menus.

Merge matching local/remote refs before counting. Show one primary ref (prefer local) and `+N` for others. The count opens a dialog with full names and existing ref context menus without selecting the row or changing virtual row height. Icons distinguish ref kinds; only the current branch uses accent tint. Names truncate in history and wrap in expanded lists.

## Inspector

Subjects wrap, including long unbroken strings. The header is capped at 45% of inspector height and scrolls internally to keep the file list accessible. SHA, author and date are secondary; author email remains in a tooltip. AI explanation stays visible; Tag/cherry-pick/revert move into More actions with existing confirmations.

Collapsed files use a single 36px row with disclosure icon, change-kind letter and truncatable filename. The whole row is an expand button labeled with the full path. Expanded context shows path, blob identifiers, available statistics and Blame, then the lazy-loaded diff. Metadata-only results do not claim zero changes. Preserve preview limits, working-copy/stash contexts, staged/unstaged identity and image diffs.

## Theme

Native Blue and Tide remain supported. Sidebar/status use window chrome, history uses panel canvas, inspector uses elevated surface. Selection uses accent; warning/danger retain semantic roles. Diff addition/deletion colors are unchanged.

Nine shared lane bases: `#5187bd`, `#9074ad`, `#58988c`, `#ad8b50`, `#b57587`, `#678aa3`, `#8d9655`, `#6588be`, `#8a80b1`. Each token mixes 80% base with 20% active `text-primary`, following light/dark appearance. Lane colors identify topology, not errors.

## Verification

Check 1440/1280/1024px, both languages/themes, long subjects/paths, many refs/lanes and long sidebars. DOM tests cover behavior; browser fixtures exercise real components with simulated IPC. Native macOS WebView rendering and Windows/Linux appearance remain release smoke-test responsibilities.
