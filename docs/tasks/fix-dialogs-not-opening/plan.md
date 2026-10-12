# Fix native menu dialogs not opening

## Problem and diagnosis

On macOS v0.10.0, clicking the native Settings menu does nothing, while Cmd+, opens the same dialog. This was reproduced in the installed desktop app. Settings, About, repository ingestion and workspace actions share the native menu dispatch path.

Tauri 2.12.1 registers menu action channels by native menu ID. Its menu resource Drop removes the channel for that ID. GitWave rebuilds menus after gating/language changes and replaces and closes the old menu, while assigning identical IDs to both generations. Old resource cleanup therefore removes callbacks belonging to the replacement. In addition, inline menu options create temporary Rust menu-item wrappers inside `MenuItemPayloadKind::with_item`; their immediate Drop removes each newly registered action channel even before replacement. Unique IDs alone do not fix that first-creation failure. The shared Modal itself passes real HeroUI open/close/reopen tests and production Chrome/WebKit checks.

## Fix

- Explicitly create MenuItem, PredefinedMenuItem and Submenu resources before Menu.new. Retain them until successful replacement, then close the previous root and children. Dispose partial builds when creation or installation fails.
- Give each native menu build a UUID namespace for the root, submenus and custom items. Keep logical action IDs in the existing closures so app dispatch behavior stays consistent.
- Preserve menu labels, accelerators, gating and predefined OS items.
- Add a regression simulating Tauri's ID-keyed channel registry and old-menu disposal, verifying replacement Settings and workspace actions survive.
- Add a real HeroUI Modal lifecycle test without mocking the shared wrapper.

## Validation

Run `make check`; verify native Settings and repository ingestion in the running development app after HMR and after switching workspace/rebuilding menus. Production Chrome and WebKit tests used read-only IPC stubs and confirmed dialog rendering; they do not validate native menu IPC.

## Documentation scope

This restores existing behavior and does not change version or feature scope. README, PM scope/roadmap, site version badges and release sync points remain accurate; no release is part of this task.

## References

- [Native menu technical decision](../../tech/decisions/00-overview.md)
- [Tauri menu channel registration/cleanup source](https://github.com/tauri-apps/tauri/blob/tauri-v2.12.1/crates/tauri/src/menu/plugin.rs)
- [Tauri menu resource Drop source](https://github.com/tauri-apps/tauri/blob/tauri-v2.12.1/crates/tauri/src/menu/mod.rs)

## Regression introduction

The native menu implementation originated in `f652d8f` (2026-08-30), while Tauri was 2.11.5. The observed failure was activated by dependency update `97e06c0`, merged as PR #143 in `35091bc` at 2026-10-10 03:31:29 Asia/Shanghai. That opener-plugin update also changed the lockfile's Tauri core from 2.11.6 to 2.12.1. The new core added action-channel removal in menu-wrapper Drop; 2.11.6 does not have that cleanup call. v0.9.6 still locked 2.11.6, and the first release tag containing the update is v0.10.0.

- [Introducing dependency update](https://github.com/Yangshifu1024/GitWave/commit/97e06c0)
- [PR #143](https://github.com/Yangshifu1024/GitWave/pull/143)

## Final validation

The user restarted `pnpm tauri dev` and confirmed native Settings and Add Repository now open. Initial unique-ID-only changes were insufficient; explicit child resource retention completed the fix. Development HMR may retain a previous generation of native resources until webview exit if the hook module itself is replaced; restarting dev clears this state. This does not affect normal production menu rebuilds.

Final `make check` passed: 331 Vitest tests, 4 Node regression tests, 385 Rust tests (2 existing ignored), formatting, lint and type checks. `pnpm build` passed. No commit, push or release was performed.
