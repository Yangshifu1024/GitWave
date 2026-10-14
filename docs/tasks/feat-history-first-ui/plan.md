# History-first interface

## Decision and scope

Accepted by the user on 2026-10-14 after reviewing the history-first mockup. Work on `feature/history-first-ui`; commit, push and open a PR after the quality gate and independent code review. This changes presentation and navigation density, not Git execution or the release version.

The primary reading order is Workspace → repository → current branch → history → selected commit → changed files. Preserve every existing Git operation, explicit confirmation, repository isolation, keyboard access, lazy diff loading and history virtualization.

## Implementation

1. Start the sidebar at 240px and inspector at 360px. Fit resized columns inside the available window and support keyboard resizing and reset.
2. Replace sidebar card chrome with source-list sections. Show a collapsed repository-health summary, surface operation residue and errors without expansion, and keep full metrics and AI advice available.
3. Give commit messages priority. Use a bounded, horizontally compressed graph preview without dropping topology; offer a full-width graph with horizontal scrolling. Keep the same virtualizer, row height, selection and vertical position. Show one primary ref plus the remaining count and full names; retain all ref operations.
4. Use quiet shared ref styles, subdued theme-aware lane colors, consistent selected rows and a clear active repository tab. Group top-bar operations and reserve warning colors for exceptional states.
5. Wrap long commit titles and compress author/date/SHA metadata. Move secondary commit actions into a menu while retaining their confirmation dialogs. Replace two-line collapsed file cards with single-row expand controls; show path, blob identifiers and Blame in expanded context.
6. Update current design guidance and mark the new interface as unreleased in product documentation. Check README, release skill and site sync points without claiming that the current release already contains the change.

## Validation

- Behavior tests for column constraints/keyboard controls, health disclosure, graph mode/ref summaries, and file expansion/lazy requests.
- Browser inspection at 1440, 1280 and 1024px, light/dark and Chinese/English, including long messages/paths, many refs/lanes and scrolling.
- `make check` and production frontend build.
- Independent `.agents/agents/code-reviewer.md` review across all seven dimensions, recorded in `review.md`; resolve actionable findings before commit.

## References

- [Design overview](../../design/00-overview.md), [layout](../../design/03-layout.md), [theme](../../design/07-theme.md).
- [Frontend architecture](../../tech/architecture/00-overview.md).
- [Engineering conventions](../../tech/engineering/00-overview.md).

## Completed validation

- Final `make check`: formatting, version compatibility, ESLint, clippy, TypeScript and cargo check passed; 4 Node checks, 348 Vitest tests and 386 Rust tests passed (2 existing Rust tests remain ignored).
- `pnpm build` passed. The existing bundler timing diagnostic about `debugName` remains informational.
- Chromium with local simulated IPC: 1440/1280/1024px, Chinese/English × light/dark, 300 commits across 40 lanes, 120 branches, multiple refs, long filenames and a long unbroken subject. No page errors or viewport overflow. Sidebar scroll reached its final row while secondary section headers stayed visible. History rendered 41 virtual rows; compact/full switching retained selection and scrolling. File expansion loaded its preview only on demand.
- Very long subjects wrap and scroll inside a header capped at 45% of inspector height, keeping the file list accessible.
- README marks the change Unreleased; PM and design docs identify the feature-branch implementation. Checked site, released-version references and release-skill sync points: no version/branding change, so v0.10.2 download/status surfaces remain intact.
- Independent review: [review.md](./review.md). Native WebView and Windows/Linux visual smoke tests remain outside the local Chromium verification.
