# Fix frontend and backend release warnings

## Scope

Before v0.9.6, remove the warnings observed in the complete local release gate and frontend production build. Keep existing interaction and async isolation behavior; do not hide diagnostics by raising bundle limits or disabling lint rules.

## Findings and implementation

- A fresh `CARGO_TARGET_DIR` removes cached Tauri paths from the repository's former location. Rust strict Clippy passes; 385 tests pass and 2 remain intentionally ignored.
- Resolve 52 React lint warnings: reset state at scope transitions, keep refs out of render, make render calculations pure, and replace the mutable virtualizer API with immutable fixed-height viewport snapshots.
- Replace Vite's `__dirname` with native ESM paths and reduce oversized bundles through bounded syntax-highlighter imports and code splitting.
- Preserve Shiki Oniguruma behavior with lazy grammar JSON and WASM assets, and allow only same-origin resource fetches plus `wasm-unsafe-eval` in Tauri CSP.
- Restore strict React lint severities and make frontend lint fail on warnings.

## Validation

Run scope-change, stale-response, dialog reopen, split-direction and syntax-highlighting regressions, then `make check` and `pnpm build`. Inspect complete frontend/backend logs for warnings. Request code-reviewer review before the release commit.

## References

- [React state adjustment guidance](https://react.dev/learn/you-might-not-need-an-effect)
- [Rolldown code splitting](https://rolldown.rs/reference/OutputOptions.codeSplitting)
- [Engineering conventions](../../tech/engineering/00-overview.md)

## Completed validation (2026-09-27)

- `CARGO_TARGET_DIR=/tmp/gitwave-release-0.9.6-target make check`: passes, with zero warnings; 48 frontend test files / 320 tests, Rust 385 passed / 2 intentionally ignored.
- `pnpm build` equivalent (`tsc --noEmit` + `vite build`): passes without warnings; largest JavaScript chunk 406,354 bytes, below Vite's unchanged 500 kB threshold.
- Production Chrome smoke: actual emitted syntax module, grammar JSON and WASM under the Tauri CSP; 15 languages × 2 themes pass, no CSP violations. Full desktop UI requires the Tauri host (`metadata` is unavailable in plain Chrome), so this does not claim native three-platform UI validation.
- Code-reviewer review: [review.md](./review.md), no unresolved blocking findings. Sidebar refresh continuity, CSP, initial commit locate and package initialization order were corrected during review.
