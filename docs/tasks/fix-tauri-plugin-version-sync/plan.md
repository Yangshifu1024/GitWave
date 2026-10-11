# fix-tauri-plugin-version-sync · Release build compatibility

## Defect analysis

[v0.10.0 build CI](https://github.com/Yangshifu1024/GitWave/actions/runs/38155450723) failed on macOS, Windows and Linux before compiling the app. Tauri reported these resolved npm/Rust mismatches:

| Plugin | npm | Rust |
|---|---|---|
| updater | 2.12.0 | 2.13.1 |
| os | 2.3.2 | 2.4.0 |
| opener | 2.6.0 | 2.7.0 |

The Rust and npm dependency updates were merged independently. The existing lint/test gates passed because they do not check the paired packages' resolved major/minor versions. This is a confirmed integration failure, not a platform-specific compilation error.

## Fix

1. Update the three npm dependency minimums and pnpm lock to match their Rust major/minor versions; allow patch differences.
2. Add an offline compatibility check over installed npm packages and Cargo.lock. Include core API/tauri and npm-facing plugins; Rust-only plugins do not need a JavaScript counterpart.
3. Run the check in Makefile lint and the frontend lint CI on all platforms. Add regression fixtures to the existing test entry point.
4. Run the full local gate and a real Tauri debug build without bundles, which exercises the CLI's actual compatibility check and application compilation without installer signing.

## Regression points

- Reject the three exact v0.10.0 mismatches and identify each pair.
- Accept matching major/minor with different patches and Windows CRLF lockfiles.
- Reject core API major/minor mismatches and a missing Rust counterpart.
- Run against the installed locked dependency graph after the fix.

## Scope and release state

The user confirmed continuing on main, initially requesting only the fix. On 2026-10-12 the user explicitly authorized committing, pushing and moving `v0.10.0` to the repair commit. Before this operation, the remote release was verified as an unpublished draft with no assets, and main and the tag both pointed at `21d70e3`. Keep the application version at 0.10.0, push main without force, then replace only the tag using a lease against its verified old object ID. This user-authorized exception to the release skill's default new-version policy retriggers release builds; draft publication remains manual. Do not bypass Tauri's check with `--ignore-version-mismatches`.

See [engineering conventions](../../tech/engineering/00-overview.md) and [release skill](../../../.agents/skills/gitwave-release/SKILL.md).

## Validation

- Full Makefile-equivalent gate passed: Prettier, Rust fmt, ESLint, TypeScript, version compatibility, clippy with warnings denied and cargo check.
- Tests passed: 4 Node compatibility regression cases, 49 Vitest files / 328 tests, Rust 385 passed / 2 ignored / 0 failed.
- `pnpm tauri build --debug --no-bundle --ci` passed the real CLI compatibility gate, built production frontend assets and compiled `src-tauri/target/debug/gitwave.exe`. Installer packaging, signing and macOS/Linux builds were not performed locally.
- The initial sandboxed Vitest run encountered temporary-file ENOENT errors; the rerun outside sandbox passed. Rust reported Windows incremental-cache access notes but completed successfully. The frontend build emitted an existing code-splitting debugName notice.
- `git diff --check` passed. The application version remains 0.10.0; the user authorized the commit, main push and tag move after validation.
