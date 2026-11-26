# GitWave

> Local-first Git client with AI collaboration. Website: **[gitwave.work](https://gitwave.work)**

**Status:** v0.11.0 released · `main` is being rewritten on GPUI Kit and does not build a releasable app yet · [Release notes](https://github.com/Yangshifu1024/GitWave/releases) · [Product scope](./docs/pm/core/01-features.md) · [Migration plan](./docs/tasks/refactor-gpui-kit-desktop/plan.md)

### What's new in v0.11.x

- **v0.11.0** — Tide Workbench phase 1: peer History / Changes workspaces, a resizable history pane above a wide file/diff inspector, repository-scoped selections and drafts, a full-size working-copy editor with a compact commit composer, virtualized diff content, reliable dialog dismissal, readable Blame columns at narrow widths, and working-copy writes pinned to their originating repository with errors retained across navigation; updated @tanstack/react-query, i18next, @tauri-apps/cli, serde_json and development dependencies

### What's new in v0.10.x

- **v0.10.4** — restored circular history nodes and merge geometry, aligned refs / SHA / author / time columns, and hidden the horizontal scrollbar while preserving graph panning; updated Tauri, React Aria, Shiki, Lucide and development dependencies
- **v0.10.3** — history-first interface with compact sidebar and health summary, a bounded graph preview and full topology view, quieter refs, compact changed-file rows, keyboard-resizable columns and a bottom operation-status bar
- **v0.10.2** — fixed macOS relaunch after installing updates, with visible restart failures and retry; updated Tauri, React libraries and development tooling
- **v0.10.1** — restored macOS native menu actions so Settings, Add Repository and other dialogs open reliably after startup and menu rebuilds

Older releases are in the [release notes](https://github.com/Yangshifu1024/GitWave/releases).

![GitWave v0.11.0 window: the History workspace shows a full branch graph with ref badges, aligned SHA / author / time columns and commit search on the left, the selected commit's details and changed files on the right, and its diff below. A sidebar switches between History and Changes, lists branches, tags, stashes, worktrees, submodules and remotes, while the self-drawn title bar carries the app menu, workspace selector and sync actions.](./assets/gitwave-screenshot.png)

The screenshot shows v0.11.0 with the Tide Workbench. Its design is documented in [Tide Workbench](./docs/design/09-workbench-redesign.md); the earlier history-first layout it replaces is documented in the [v0.10.3 specification](./docs/design/08-history-first.md).

## Download

Installers for macOS (Apple silicon, signed & notarized), Windows (NSIS) and Linux (deb / rpm / AppImage) are on <https://gitwave.work> and the [GitHub Releases](https://github.com/Yangshifu1024/GitWave/releases/latest) page. v0.11.0 is the latest release, and the last one built from the Tauri-based application; no further release is cut until the rewrite below has a release pipeline.

## Features

What v0.11.0 does. The rewrite on `main` brings the same scope to the new application; the [migration plan](./docs/tasks/refactor-gpui-kit-desktop/plan.md) tracks what has landed so far.

- **Workspace management** — multiple workspaces, drag-reorderable repo tabs that scroll horizontally with the mouse wheel once they overflow the window (the active tab stays in view), adding several local repositories at once (already-added paths and repeats inside the batch are skipped, unusable paths are reported without failing the batch), per-workspace AI context; a workspace is an abstraction, not a directory
- **Tide Workbench** — the main area is split into peer History and Changes workspaces with a resizable history pane above a wide file/diff inspector; selections, drafts and the commit composer are scoped per repository, so switching repositories never mixes state. Diff content is virtualized to fill the pane, and working-copy writes stay pinned to the repository they started from even if you switch focus mid-operation
- **Working copy** — stage / unstage, discard, ignore, commit with an optional conventional-commit type selector (amend the last commit included), commit message AI assist
- **Branches & sync** — create / switch / delete / rename / set upstream tracking (the picker lists every remote-tracking branch, `origin/main` included), double-click a remote branch to create its tracked local branch and switch to it (DWIM), push / pull with a target-remote picker, tag pushes that send only the tags on the pushed commit (conflicting tags are skipped and named instead of failing the batch), fetch across all remotes with stale tracking-ref pruning, time-boxed network syncs with an in-flight cancel button, an in-app auth prompt that collects a username / token and retries in place when a remote challenges for credentials (optionally saved to your system keychain), merge (ff & no-ff) with a conflict panel that also exposes unresolved index entries outside an active merge, and a collapsible sidebar that groups prefixed branches into folders and auto-expands to the branch you check out
- **History** — commit graph with fork-style edges, commit details, blame, reflog, tags, and right-click menus on commits / branch-tag badges (checkout, cherry-pick, revert, reset, copy info) that reveal the checked-out branch in the sidebar; a bounded preview preserves circular nodes and merge geometry with horizontal panning, aligned metadata columns and full topology access; subdued refs merge matching tracked remotes and expand to show all names
- **Diff viewer** — compact file rows with full-row expansion and lazy content loading, side-by-side and unified views, character-level highlighting inside changed lines, and side-by-side image diffs (old / new versions rendered for png / jpg / gif / webp / bmp / ico / svg — for unstaged changes the new side reflects the live worktree)
- **Advanced Git** — stash (scoped to the active repo; each entry opens a detail window with its file list and diff, its actions are labelled buttons with tooltips, apply pre-checks a working copy that has uncommitted changes and drop confirms before running, and files saved as untracked are listed), interactive rebase, worktrees, submodules, LFS, remotes, .gitignore editor, Git hooks panel, repo health checks
- **AI collaboration** — BYOK provider setup, commit explain, AI-drafted PR descriptions; diffs stay local unless you send them to your chosen provider. AI replies in Chinese / Japanese / Korean / English per your preference
- **Automatic updates** — in-app check for updates with signed downloads and one-click install (macOS / Windows / AppImage); Restart Now reopens the app after installing a macOS update, with visible restart request failures and retry; deb / rpm installs get update prompts pointing at the releases page
- **Background refresh** — auto refresh keeps every repository in the workspace current on a configurable interval (default 5 minutes), fetching each repo best-effort; ⌘R / Ctrl+R refreshes the active repo on demand
- **Proxy-aware networking** — AI requests, sync, LFS and update checks follow the OS system proxy (Windows / macOS) or a manual proxy URL you set in Settings → Network, effective immediately; loopback addresses (local Ollama etc.) always bypass it
- **SSH key management** — generate / import keys, per-repo SSH configuration
- **Platform UX** — a compact source-list sidebar with expandable health metrics, keyboard-resizable panes, a bottom operation-status bar, command palette, menu bar app mode, themes plus UI / monospace font family and size settings, one-click open of the active repo in the system file manager, a detected terminal, or a detected code editor (VS Code / Zed / VSCodium), Chinese / English interface with instant switching, and a self-drawn title bar that merges the app menu and the operation bar into a single row (traffic lights / window controls kept clear)

## The rewrite on `main`

`main` no longer contains the Tauri application or the React frontend. Both were removed on 2026-11-25 and their source stays readable at commit `8466d55`. What remains is a Rust-only Cargo workspace:

- **`crates/gitwave-core`** — the domain, use-case and infrastructure layers v0.11.0 ran on: libgit2, SQLite, credential and proxy handling, the AI providers. The new application calls it in process, with no IPC.
- **`crates/gitwave`** — the new desktop application, built on [GPUI Kit](https://github.com/longbridge/gpui-kit). Today it has the workbench shell — a title toolbar with the workspace switcher, a row of repository tabs, a navigator, a status bar, the application menus, English / Chinese switching and a light / dark appearance — and the first step of the read-only workbench: the commits of a repository, read a page at a time as you scroll. It runs gitwave-core's use cases against a copy of the released app's state, in `GitWave-dev/` beside it, and keeps everything of its own there; the released app's directory is only ever read. It grows one milestone at a time.

Until the rewrite reaches parity there is nothing to download from `main`: no installer is built and no release is cut. The decision is recorded in [ADR 0007](./docs/tech/decisions/00-overview.md#0007--桌面界面迁移到-gpui-kit); milestones and the execution log are in the [migration plan](./docs/tasks/refactor-gpui-kit-desktop/plan.md).

## Tech stack

- **Application:** Rust with [GPUI Kit](https://github.com/longbridge/gpui-kit) 0.11, pinned to the exact version; the interface text has been moved into `rust-i18n` catalogues under `crates/gitwave/locales/` (Chinese and English), which the menus and the dialogs read
- **Core:** `gitwave-core`, layered as `domain` / `application` / `infrastructure`, on `git2` (vendored libgit2 + libssh2 + OpenSSL), `rusqlite` (bundled SQLite) and `reqwest`
- **Testing:** `cargo test` — unit and integration tests against real Git fixtures in the core, consistency checks on the interface catalogues, and headless-window tests that render the real shell and drive it. There is no end-to-end suite yet

## Quick start

Prerequisites:

- Rust 1.92 or newer ([rustup](https://rustup.rs)); CI runs the latest stable
- `git` on `PATH` — one test starts it, and the core shells out to it for HTTPS credential helpers and for Git LFS
- macOS: Xcode command line tools (`xcode-select --install`)
- Linux: `build-essential`, `clang`, `cmake`, `pkg-config`, `libdbus-1-dev`, `libfontconfig-dev`, `libwayland-dev`, `libxkbcommon-x11-dev`, `libx11-xcb-dev`, `libssl-dev`, `libzstd-dev`, `libasound2-dev`, `libvulkan1`, `vulkan-validationlayers`
- Windows: a standard Rust MSVC setup (Visual Studio C++ Build Tools); CI installs nothing beyond the toolchain

```bash
make dev      # cargo run -p gitwave
```

The application writes a JSON log, one file per day, under `logs/` in its own directory in the platform's application-data directory (`~/Library/Application Support/GitWave-dev/logs/` on macOS). `RUST_LOG` sets the level; the default is `info`.

That directory holds a copy of the state of the released app, whose own directory beside it, `GitWave/`, this build leaves alone. On launch the build reads the released app's `state.db` through a read-only connection and copies it into `GitWave-dev/`, taking a new copy whenever the original changes; it then opens the copy, never the original. The copy is what this build changes: the language, the workspace and repository you were on. Two files of SQLite's own can appear beside the released app's database after it is read — its `-shm` index, and an empty `state.db-wal` when it had none — which hold nothing and which the released app removes when it next closes. Opening the original itself waits for the release pipeline, which decides how the upgrade from v0.11.0 goes.

## Make targets

| Target | What it does |
|---|---|
| `make dev` | Run the application (`cargo run -p gitwave`), no checks first |
| `make build` | `check`, then build the debug binary |
| `make check` | `fmt-check` + `lint` + `test` — the CI-equivalent gate, writes nothing |
| `make fmt-check` | `cargo fmt --all -- --check` |
| `make lint` | `cargo clippy --workspace --all-targets -- -D warnings` |
| `make test` | `cargo test --workspace --all-targets` |
| `make format` | `cargo fmt --all` (rewrites files) |

A `pre-commit` hook runs rustfmt and clippy on `git commit` — see [CONTRIBUTING.md](./CONTRIBUTING.md). No formatter covers Markdown, so documentation is reviewed by hand.

## CI

Workflows live in `.github/workflows/`:

- **ci** — on every push to `main` and every PR: rustfmt, clippy and the tests on a macOS / Ubuntu / Windows matrix, plus two assertions that vendored libgit2 keeps its TLS transport. A pull request that changes only documentation or the website (`site/**`, `docs/**`, `**.md`) starts no run; the filter looks at the whole pull request, so a later push that adds only documentation to a pull request with code in it does start one
- **pages** — deploys `site/` to GitHub Pages on push to `main`

There is no build workflow. The tag-triggered pipeline that produced the v0.11.0 installers and the signed update manifest was removed together with the Tauri application.

### Cutting a release

Not yet possible from `main`. Packaging, signing, notarization and signed updates are milestone M7 of the [migration plan](./docs/tasks/refactor-gpui-kit-desktop/plan.md); until then pushing a tag starts no build. The version stays `0.11.0` until the first release of the new application is decided, and is then bumped with `scripts/bump-version.sh <x.y.z>`.

Per AGENTS.md, **AI agents must not commit / push / merge** — humans gate every change to `main`.

## Documentation

- Website — <https://gitwave.work> (landing page & downloads)
- [`docs/pm/core/`](./docs/pm/core/README.md) — product management (features, scope, roadmap)
- [`docs/tech/`](./docs/tech/README.md) — engineering decisions (architecture, selection, ADRs, conventions)
- [`docs/design/`](./docs/design/00-overview.md) — UI/UX overview (3-pane layout, tokens, components), written against v0.11.0
- [`docs/tasks/`](./docs/tasks/README.md) — per-task plans and reviews
- [`AGENTS.md`](./AGENTS.md) — workflow rules and agent boundaries
- [`CONTRIBUTING.md`](./CONTRIBUTING.md) — branch, commit and review conventions

## License

[MIT](./LICENSE) © Yangzhenbiao
