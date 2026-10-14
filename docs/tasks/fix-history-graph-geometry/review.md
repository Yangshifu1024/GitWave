# History graph geometry review

Date: 2026-10-14. Scope: uncommitted changes against HEAD on `fix/history-graph-geometry`. Reviewed using `.agents/agents/code-reviewer.md` across correctness, security, performance, maintainability, readability, test coverage and best practices. See the [fix plan](./plan.md).

## ✅ 优点

- SVG geometry now maps directly to CSS pixels: no viewBox or nonuniform scaling. Compact/full modes place lanes at actual 12px/18px spacing with 8px side padding. Circle radii, stroke widths, row heights and vertical curve coordinates remain unchanged, including HEAD-ring clearance.
- Compact rows use one opaque sticky message/metadata group above the graph. Its width and the content's horizontal overflow budget are coordinated; selection uses an opaque color mix. A browser-detected gap above/below the text was corrected with `self-stretch`, filling the 28px row while keeping its contents vertically centered; 8px inner left padding separates text from the masking edge without changing the scroll budget. Full mode restores ordinary row flow.
- Horizontal reveal measures the compact graph viewport rather than the whole history pane. Selection, repeated selection of the same SHA, mode switching and locate requests are distinct triggers. The reveal key prevents routine page appends and refreshes from undoing manual panning.
- Existing cursor paging, memoized graph/commit rows and fixed-height virtualization remain. Container units size the message area without per-row layout reads; reveal reads only the shared scroll element on intentional changes. Horizontal-only scrolls do not change the virtualizer's vertical snapshot.
- Existing ref menus and dialogs remain available. No backend execution, credentials or external data-transfer behavior changes. Both language catalogs describe horizontal browsing and preserved node shape; README and design docs identify the correction as unreleased without changing v0.10.3.
- Regression tests cover unit-scale geometry, actual compact/full lane distances and merge endpoints, high-lane reveal, repeated same-SHA selection, locate handling, page/refresh stability, refs and bounded virtual rendering.

## 🔴 严重问题（必须修复）

No blocking findings in the reviewed diff.

## 🟡 一般问题（建议修复）

None requiring a code change.

## 🟢 优化建议（可选）

- Include native macOS WebView smoke testing of sticky positioning, shared horizontal scrolling and selected-node reveal when preparing the next release. DOM tests and Chromium establish the tested geometry but do not verify every supported desktop runtime.

## 📝 总体评价

The fix addresses the reported distortion through actual graph coordinates while retaining message readability and all topology. The scroll and reveal behavior is deliberate and does not introduce a per-row measurement cost or change Git-operation safety. Final review decision: **approved**, with no outstanding findings; the final quality gate, production build and real-browser geometry checks all passed.

Independent validation: `pnpm exec vitest run src/components/CommitGraph.test.tsx` passed all 7 tests; `git diff --check` passed. Local Markdown link targets in the changed README/design documents and new fix plan all resolve. The tester reports the unit-scale regression failed against the old implementation before the correction.

Real-browser results reported by the implementing agent: SVG screen transforms `(1, 1)` and circle width/height ratios approximately 1, pinned messages, selected/reselected visibility and compact/full switching passed at 1024/1440px in light/dark with English/Chinese labels. Native `scrollTo({ top })` retained horizontal position. Full-row 28px overlay hit tests and screenshot inspection passed without graph bleed above/below text.

The implementing agent's final `make check && pnpm build` exited 0 after the last product changes: 352 frontend tests in 56 files, 385 Rust unit tests and 1 integration test passed (2 existing ignored tests), along with the complete formatting/lint/typecheck/compatibility gates and production build. The [task plan](./plan.md) records these completed checks. Native macOS WebView and Windows/Linux runtime verification remain release smoke-test responsibilities.
