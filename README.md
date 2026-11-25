# GitWave

> Local-first Git client with AI collaboration. Website: **[gitwave.work](https://gitwave.work)**

**Status:** v0.11.0 · [Release notes](https://github.com/Yangshifu1024/GitWave/releases) · [Product scope](./docs/pm/core/01-features.md) · [Engineering decisions](./docs/tech/README.md)

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

Installers for macOS (Apple silicon, signed & notarized), Windows (NSIS) and Linux (deb / rpm / AppImage) are on <https://gitwave.work> and the [GitHub Releases](https://github.com/Yangshifu1024/GitWave/releases/latest) page.

## Features

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

## Go and NSGo development

The Go module is at the repository root. The desktop entry and its NSGo configuration live in `cmd/gitwave`, packaging tools in `cmd`, and application libraries in `internal`.

Use Go 1.27.3 or newer, Git 2.46 or newer, and Git LFS for the development fixtures. Linux development runs against the host GTK 4.14 or newer. The packaged Linux preview also supports its bundled GTK runtime.

```bash
make dev        # Start NSGo in development mode.
make check      # Check Go formatting, run go vet, and run Go tests.
make test-race  # Run Go tests with the race detector.
make build      # Run the checks and build the native preview for this host.
```

Use `go run ./cmd/prepare-unix-setup -out <directory>` on the supported macOS or Linux build host to create preview installers with bundled Git. The [Go desktop workflow](./.github/workflows/go-desktop.yml) documents the three-platform build and package checks.

## Tech stack in v0.11.0

- **Frontend:** React 19 + TypeScript 6 + Vite 8, Tailwind CSS 4 + HeroUI v3, zustand, TanStack Query
- **Backend:** Rust + [Tauri 2](https://tauri.app), clean-architecture layers (`domain` / `application` / `infrastructure`), `git2` (vendored libgit2 + libssh2 + OpenSSL) — no system Git dependency
- **Testing:** Vitest (unit and React Testing Library/jsdom component behavior); Playwright end-to-end tests have a script but no suite yet

## Tauri preference exporter development

Prerequisites:

- Rust stable ([rustup](https://rustup.rs))
- Node.js 22 (the version CI uses)
- macOS: Xcode command line tools (`xcode-select --install`)
- Linux: `webkit2gtk-4.1-dev`, `build-essential`, `cmake`, `curl`, `wget`, `file`, `libssl-dev`, `libxdo-dev`, `libayatana-appindicator3-dev`, `librsvg2-dev`, `patchelf`
- Windows: WebView2 runtime + MSVC build tools

```bash
pnpm install
make legacy-dev
make legacy-check
make legacy-build
```

See the local checks and pre-commit hooks in [CONTRIBUTING.md](./CONTRIBUTING.md).

## CI

Workflows live in `.github/workflows/`:

- **Go desktop** — validates the root Go module and builds NSGo preview installers on macOS, Linux and Windows
- **lint / test** — on every push to `main` and every PR: `rust-lint` + `frontend-lint`, `rust-test` + `frontend-test`, each on a macOS / Ubuntu / Windows matrix. Pure-documentation changes (`site/**`, `docs/**`, `**.md`) are skipped
- **Tauri preference exporter release** — runs manually on the exporter tag, verifies each installer signature and generates `latest.json` in the draft
- **Go desktop release** — runs manually from `main`, assembles the verified NSGo installers and all three update manifests, and optionally creates a draft for the matching existing tag
- **pages** — deploys `site/` to GitHub Pages on push to `main`

### Cutting a release

For the Tauri preference exporter, use `pnpm bump <x.y.z>` to update the Tauri and frontend versions. After creating the release tag, run **Tauri preference exporter release** on that tag. Wait for **Verify preference exporter update feed**, then review the draft installers and `latest.json` before publishing.

For a Go release, set the application version with `make bump VERSION=X.Y.Z` and confirm it with `make version`. Run **Go desktop release** from `main` with the release, exporter, and first Go bridge versions. To create a draft, use a release version that matches `cmd/gitwave/nsgo.yaml` and a release tag that points to the selected `main` commit. Wait for **Create and verify a draft for the verified tag** to confirm the uploaded files match the assembled release, then review and publish in GitHub Releases. [Signing and upgrade details](./docs/tasks/refactor-nsgo-native-ui/plan.md#三端包装更新与回退)

Per AGENTS.md, **AI agents must not commit / push / merge** — humans gate every change to `main`.

## Documentation

- Website — <https://gitwave.work> (landing page & downloads)
- [`docs/pm/core/`](./docs/pm/core/README.md) — product management (features, scope, roadmap)
- [`docs/tech/`](./docs/tech/README.md) — engineering decisions (architecture, selection, ADRs, conventions)
- [`docs/design/`](./docs/design/00-overview.md) — UI/UX overview (3-pane layout, tokens, components)
- [`docs/tasks/`](./docs/tasks/README.md) — per-task plans and reviews
- [`AGENTS.md`](./AGENTS.md) — workflow rules and agent boundaries
- [`CONTRIBUTING.md`](./CONTRIBUTING.md) — branch, commit and review conventions

## License

[MIT](./LICENSE) © Yangzhenbiao
