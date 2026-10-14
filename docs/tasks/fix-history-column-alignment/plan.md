# Fix history column alignment

## Cause and goal

The author/time flex item has an intrinsic width that varies per commit, moving the preceding SHA and ref badges between rows. Keep refs, SHA, author and time in consistent columns in compact and full graph modes.

## Implementation

- Reserve the ref column even for commits without refs.
- Keep SHA width fixed; split author and time into fixed width columns, with author truncation and a full-name tooltip.
- Preserve responsive metadata visibility, graph geometry, virtual scrolling and ref interactions.
- Update the history design specification; this is an unreleased layout fix, so published version/status and feature lists remain accurate.

## Verification

Run `make check`. Inspect mixed authors, timestamps, refs/no refs and multiple refs at responsive breakpoints, including compact/full graph and horizontal scrolling. Review following `.agents/agents/code-reviewer.md`.

Reference: [history design](../../design/08-history-first.md).

## Results

- `make check`: passed (format, ESLint, Rust clippy, TypeScript, Cargo check and all frontend/backend tests).
- `pnpm exec vite build`: passed.
- Local WebKit geometry fixture using production CSS: matching ref/SHA/author/time positions for mixed short/long authors and empty/populated refs at 320, 510, 670, 680, 1024 and 1440px pane widths. This isolated CSS check does not replace native app smoke testing of graph scrolling and ref interactions.
- Static review: no blocking findings; see [review](./review.md).
