# History-first UI review

Date: 2026-10-14. Scope: the `feature/history-first-ui` working-tree changes against `main`, including presentation components, translations and regression tests. Reviewed using `.agents/agents/code-reviewer.md` across correctness, security, performance, maintainability, readability, test coverage and best practices.

## ✅ 优点

- ThreePaneLayout constrains column widths using the measured container, restores preferred widths after a temporary resize, uses cumulative mouse movement and removes drag listeners on unmount. Keyboard controls and localized separator labels are covered by regression tests.
- CommitGraph keeps its existing virtualizer and memoized rows. Compact mode transforms the complete SVG coordinate space rather than removing lanes; full mode retains the scroll element and selection. Primary refs and `+N` preserve access to ref menus and full names.
- DiffViewer retains per-file lazy requests and large-preview handling. The full-row expand button has an explicit accessible path label, and Blame stays outside that button.
- CommitInfoHeader moves actions into the existing React Aria dropdown pattern while preserving explicit cherry-pick/revert confirmation. No backend execution, credentials, network policy or external data transfer behavior changes.
- Both locale catalogs include the new controls. Lane colors derive from theme tokens; repository-tab wheel scrolling, active-tab reveal and reorder handlers remain intact. Operation status and cancellation remain available in the bottom bar.

## 🔴 严重问题（必须修复）

No outstanding blocking findings in the reviewed product code.

One issue was found and fixed during review: replacing HeroCard with a block section in `src/components/ui/SidebarSection.tsx` broke the flex height constraint chain. A long disclosure could extend beyond the section and be clipped instead of scrolling. The final section restores `flex flex-col`, keeps a 28px minimum for its heading and constrains Disclosure with `flex-1 min-h-0`; Disclosure.Content remains the scroll owner. The implementing agent reports a real-component Chromium check with 120 branches: content client height 493px, scroll height 3912px and scrollTop 3419px, reaching the final entry.

Final follow-up review also checked explicit normal whitespace and arbitrary-word wrapping in the subject, the inspector header's 45% height limit and internal scrolling, the 20% history ref budget, responsive SHA visibility and regular commit-message weight. These adjustments preserve row height and the remaining file-list area without changing execution behavior.

## 🟡 一般问题（建议修复）

No outstanding findings. The documentation follow-up corrected centered-titlebar-status text in the overview/layout diagrams, component descriptions and source comments. The current inspector specification records the 45% header height limit and internal scrolling. README and PM docs correctly identify this work as unreleased and retain v0.10.2 as the published version; the site and release-skill sync points remain consistent with that published release.

## 🟢 优化建议（可选）

- Keep native macOS WebView inspection of compressed many-lane graphs in the release smoke test. Compact mode intentionally reduces horizontal geometry; users can inspect original lane spacing through the full graph control. Chromium and DOM tests cannot establish native rendering performance or legibility by themselves.

## 📝 总体评价

The implementation follows the accepted history-first direction while preserving Git-operation confirmations, repository scope, lazy loading and virtualized history. The sidebar height-chain regression and repeated documentation inconsistencies were corrected; no outstanding findings remain. Final review decision: **approved** for commit, push and PR creation under the user's existing authorization.

Validation independently rerun after the final product adjustments: `pnpm exec vitest run src/components/CommitGraph.test.tsx src/components/ui/ThreePaneLayout.test.tsx src/components/HealthPanel.test.tsx src/components/DiffViewer.test.tsx` — 4 files and 15 tests passed; `git diff --check` passed. Local Markdown link targets in README, all changed design/PM docs, the new design specification and the task plan were checked (13 files, no missing targets).

Final validation reported by the implementing agent and recorded in the [task plan](./plan.md): `make check` and `pnpm build` both exited 0. The gate passed 4 Node checks, 348 Vitest tests in 56 files, 385 Rust unit tests and 1 integration test (2 existing ignored tests). Subsequent changes were limited to documentation and comments; `git diff --check` passed during this final review.

The real-component Chromium fixture with simulated IPC passed at 1440/1280/1024px and Chinese/English × light/dark with no viewport overflow or page errors. Its 120-branch sidebar reached scrollTop 3419px while collapsed section headings retained 28px height; a 300-commit history stayed bounded to 41 rendered rows. Graph mode switching preserved scrolling/selection and file previews loaded on expansion. A 540-character unbroken subject wrapped within the 318px inspector, remained within the bounded header and left the changed-file list visible. This approval does not claim native macOS WebView or three-platform runtime verification.
