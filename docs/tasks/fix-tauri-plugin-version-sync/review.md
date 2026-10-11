# Tauri npm/Rust version sync review

Reviewed on 2026-10-12 against the uncommitted fix on `main`, [plan.md](./plan.md), and `.agents/agents/code-reviewer.md`. Application version remains `0.10.0`; this review does not authorize a commit, push, tag change or release.

## ✅ 优点

- **正确性**：The npm opener, os and updater packages now resolve to `2.7.0`, `2.4.0` and `2.13.2`, matching the corresponding locked Rust major/minor values. The guard checks installed resolved versions, maps `@tauri-apps/api` to `tauri` and `@tauri-apps/plugin-*` to `tauri-plugin-*`, and allows patch differences. Generated Cargo.lock package blocks are parsed with multiline expressions compatible with LF and CRLF; version and package-name patterns stay anchored to individual lines.
- **安全性**：The check performs only local file reads. A missing npm package or unreadable/malformed package JSON fails with a nonzero process exit; a missing or mismatched Rust counterpart also fails. It does not fetch dependencies, alter the dependency graph or bypass Tauri's own compatibility check.
- **性能**：The Cargo.lock file is scanned once into a map and only the small set of relevant npm packages is read. No application runtime behavior changes are introduced.
- **可维护性**：The guard derives npm-facing pairs from root dependencies and intentionally excludes Rust-only plugins and the CLI. It has no new parser or semver dependency; the compact parser is appropriate for Cargo-generated lockfiles. Root resolution uses `import.meta.url`, `fileURLToPath` and `resolve`, so it does not depend on the caller's working directory and supports Windows paths.
- **可读性**：The checker has a pure exported comparison function and a separate CLI entry point. Errors identify the affected pair and both versions. README and CONTRIBUTING document the new compatibility command, and the task plan records the root cause and the user's fix-only scope.
- **测试覆盖**：Four Node regression cases cover the exact three failed plugin pairs, allowed patch differences, core API major/minor mismatches, missing Rust counterparts and CRLF. The Node test filename avoids Vitest discovery, while `pnpm test` runs both suites; existing test CI therefore runs the regression cases on all three platforms.
- **最佳实践**：Makefile lint and frontend lint CI execute the guard after frozen-lockfile installation on macOS, Linux and Windows. The dependency diff is limited to the three affected npm packages and matching lock entries, and no application version or Rust dependency changes are included. Independent `git diff --check` passed during review.

## 🔴 严重问题（必须修复）

无。

## 🟡 一般问题（建议修复）

无。

## 🟢 优化建议（可选）

无。The initial suggestion to mention Node tests in README command tables was addressed and the updated descriptions were rechecked.

## 📝 总体评价

No blocking correctness, security or maintainability issue was found; the fix addresses the observed dependency mismatch and adds an offline gate before future builds. The implementation agent reported the guard, four Node tests, 328 frontend tests, formatting, lint, typecheck, Rust fmt/clippy/check and the full Rust suite (385 passed / 0 failed / 2 ignored) passing, plus a successful Tauri debug build (`--no-bundle --ci`) producing `gitwave.exe`; these runs were not independently repeated, and macOS/Linux installer builds remain outside the local validation scope.
