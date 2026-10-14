# History column alignment review

Scope: `src/components/CommitGraph.tsx` and `docs/design/08-history-first.md`; see [plan](./plan.md). Static review performed following `.agents/agents/code-reviewer.md`.

## ✅ 优点

- **Correctness:** Every row reserves the same ref width, including rows without refs. Fixed SHA, author and time widths remove the content-dependent shifts shown in the reported screenshot. The shared graph width keeps the available message-group width consistent across rows in compact and full modes.
- **Security:** The change only renders existing React text and a native author tooltip; it introduces no network access, credential handling or HTML injection.
- **Performance:** Layout remains CSS-only with no new layout reads, effects or per-row state. Memoization and virtualization are unchanged.
- **Maintainability:** The correction stays within row presentation and updates the matching design specification.
- **Readability:** Separate author and time elements make column ownership explicit; the author tooltip preserves access to truncated names.
- **Best practices:** Existing container breakpoints still hide author/time and SHA in narrow panes. `min-w-0`, truncation and fixed metadata widths allow the message to yield space without changing row height or graph geometry. Ref menus and the additional-ref button retain their handlers.

## 🔴 严重问题（必须修复）

None found.

## 🟡 一般问题（建议修复）

None found in the reviewed diff.

## 🟢 优化建议（可选）

- **Test coverage / verification:** DOM tests cannot establish CSS pixel alignment. Complete the planned visual inspection with mixed authors, timestamps, empty refs and multiple refs at the existing visibility breakpoints, in compact/full modes and after horizontal scrolling. `make check` and browser layout verification were pending at review time; this report does not claim they passed.

## 📝 总体评价

The implementation directly addresses the variable metadata widths while preserving the existing responsive visibility and graph behavior. No blocking issue was found across the seven review dimensions. Delivery remains conditional on the planned quality gate and layout verification.

## Validation completed after review

`make check` and the Vite production build passed. A local WebKit layout fixture using generated production CSS confirmed identical column positions for mixed author lengths and empty/populated ref slots at six pane widths (320–1440px). Native app smoke testing remains separate from this isolated layout verification.
