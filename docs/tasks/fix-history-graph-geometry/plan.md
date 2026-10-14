# Preserve history graph geometry

## Problem and reproduction

The user reported flattened nodes and distorted merge curves in the v0.10.3 compact graph. Its rendered SVG width was capped at 112px while its viewBox retained the full lane width with `preserveAspectRatio="none"`. For 40 lanes, horizontal scale was 0.2 while vertical scale stayed 1; circles became ellipses and strokes were squeezed. The tester reproduced this with a failing unit-scale regression test before the fix.

Work on the user-confirmed `fix/history-graph-geometry` branch. No release/version change is requested.

## Fix

1. Lay out graph coordinates directly: 12px compact lane spacing, 18px full spacing and 8px horizontal padding. Keep node radii, row heights, stroke widths and vertical curve coordinates unchanged. Each SVG pixel corresponds to a CSS pixel; do not scale the SVG.
2. Keep a bounded graph viewport in compact mode. Native history horizontal scrolling moves the graph; an opaque sticky message/metadata group preserves reading space. Full mode exposes all lanes without the sticky overlay. Never remove topology or merge unrelated lanes.
3. Reveal selected high-lane nodes through horizontal scrolling while preserving vertical position. Reposition on selection/mode/locate changes, not on routine paging or refresh.
4. Retain cursor paging, virtualized/memoized rows and ref operations. Update Chinese/English labels and current design guidance to distinguish lane layout from image compression.

## Validation

- Tester confirms the old implementation fails the unit-scale test.
- Regression tests cover unscaled SVGs, mode spacing, node reveal, selection and vertical-position retention, and paging stability.
- Real-component Chromium checks measure circle width/height and SVG screen transform scale, verify merge geometry, message pinning, shared horizontal scrolling and selected-node visibility in light/dark at 1024/1440px.
- Run `make check`, build, and independent code-reviewer review; record results in this task.

References: [current design](../../design/08-history-first.md), [original task](../feat-history-first-ui/plan.md), [architecture](../../tech/architecture/00-overview.md).

## Completed validation

- The old 40-lane compact implementation failed the unit-scale regression (horizontal scale 0.2 versus vertical scale 1). The corrected graph/render-guard suite passes 15 tests, including same-SHA reselection after manual panning.
- Final `make check` passed: formatting, Tauri version parity, ESLint, TypeScript, Clippy, 352 frontend tests in 56 files, 385 Rust unit tests and one updater integration test. Two pre-existing Rust tests remain ignored. `pnpm build` and `git diff --check` passed.
- Headless Chrome loaded the actual application with a 300-commit, 40-lane fixture and a multi-parent merge. Every rendered SVG had screen transform scales `(1, 1)` and every circle had equal rendered width/height (within subpixel rounding). Horizontal scrolling moved SVGs at 1:1 scale while messages stayed pinned. High-lane selection, same-SHA reselection, full/compact switching and vertical-only native scrolling retained the intended selection and scroll position.
- Visual and hit-test checks confirmed an opaque full-height 28px message group, with 8px inner spacing to keep clipped branch edges away from text. Light/dark layouts at 1024px and 1440px, Chinese/English controls and bounded virtualization passed. No page overflow or runtime errors occurred. Chromium evidence does not replace native macOS WebView smoke testing for the next release.
- Independent [code review](./review.md) covers all seven required dimensions. Version remains v0.10.3; this fix is unreleased.
