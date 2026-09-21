# GitWave

> Local-first Git client with AI collaboration. Website: **[gitwave.work](https://gitwave.work)** · See `docs/pm/core/01-features.md` for product scope and `docs/tech/` for engineering decisions.

**Status:** v0.9.3 — the update manifest CI publishes now points at plain release download links (v0.9.2 shipped asset-API URLs, which answer 403 without a User-Agent header), on top of the commit list now stays smooth while scrolling large repositories after cutting the per-scroll re-render cost, on top of every panel now formats its timestamps through one shared formatter, on top of the stash panel now gives every stash a permanently visible row of labelled actions (View / Apply / Apply & Drop / Drop) and opens a detail window with the file list beside the diff, applies pre-check a working copy that has uncommitted changes and drops ask for confirmation before they run, and files saved as untracked now appear in the stash contents instead of staying invisible, on top of the repository tab strip now scrolls horizontally with the mouse wheel once its tabs overflow the window, and pulls the active tab back into view, on top of adding several local repositories to a workspace at once (the Add dialog takes multiple folders, skips paths already in the workspace and repeats inside the batch, and reports unusable paths without aborting the batch), on top of auto refresh now sweeps every repository in the workspace on a configurable interval set in Settings → General (default 5 minutes), on top of the frontend toolchain moved to TypeScript 6, Vitest 5 and Node 22 across CI, and the icon family upgraded to lucide 1.x (which redraws a number of the icons shown in the UI), on top of the operation bar now shares the custom title bar: a single 40px row carries the app menu, the workspace selector, the external-tool shortcuts, the changes / stash / fetch / pull / push actions and a status area that blends into the bar, with macOS traffic lights and Windows / Linux window controls kept clear, on top of the stash panel now follows the active repository (switching repos refreshes the list; drop / apply / pop act on the repo you are viewing), on top of image diffs of unstaged changes now render the live worktree content instead of a stale index snapshot, on top of macOS dmg downloads now carrying a stapled notarization ticket (CI re-notarizes the dmg right after upload), so a fresh download opens without Gatekeeper's "Unnotarized Developer ID" rejection, on top of checking out a branch from anywhere (history-graph ref badges included) revealing it in the branch sidebar: the prefix folder expands and the branch is selected, on top of three-platform builds (macOS / Windows / Linux) produced by tag-triggered CI, with macOS builds signed and notarized, in-app auto-updates served from GitHub Releases, one-click open of the active repo in the system file manager, a detected terminal, or a detected code editor (VS Code / Zed / VSCodium), side-by-side image diffs in the diff viewer (old vs new rendered for common image formats), a quit guard that prompts before closing while workspace repos have uncommitted changes, an amend option in the commit flow (update the last commit without a new entry), auto-switch to a newly created workspace, and the About dialog showing the build-time commit short-sha next to the version, plus proxy-aware networking (AI requests, sync, LFS and update checks follow the OS system proxy or a manual proxy URL from Settings → Network), fully in-app credential handling (no external credential-manager dialogs; when a remote challenges for HTTPS auth, an in-app prompt collects a username / access token — clone, fetch / pull / push, remote branch deletion and submodule operations included — remembers them across operations, and clone is time-boxed and cancellable like every network sync), repo adds that accept folders owned by another local account (e.g. after an elevated clone), multi-remote-aware sync (push to any configured remote, fetch across all of them) with per-operation progress attribution so concurrent fetches / pushes no longer cross-talk, merge and interactive rebase that refuse to run on a dirty working copy and auto-roll a conflict-aborted rebase back to its pre-rebase tip, hardened AI privacy (secrets scrubbed from prompts and error logs, HTTPS enforced for cloud providers, credentials stored in the per-platform OS keychain with multi-account support), Fork-style right-click menus across the commit graph and branch list, double-clicking a remote branch to create its tracked local branch and switch to it (git-switch style DWIM), tag pushes that send only the tags on the pushed commit (conflicting tags are skipped and named instead of failing the batch), a commit graph whose ref badges merge tracked remotes into single synced markers on a nine-color lane palette, a refined top bar (slim wide status area whose operation results auto-reset after 15 seconds, count buttons that keep their width when switching repos, a Ctrl+K command palette that also searches commits by message or author and jumps to them in the graph, and a title-free toolbar), and a fully bilingual (Chinese / English) UI with selectable AI reply language and adjustable UI / monospace font sizes. Current scope per `docs/pm/core/03-roadmap.md`.

## Download

Installers for macOS (Apple silicon, signed & notarized), Windows (NSIS) and Linux (deb / rpm / AppImage) are on <https://gitwave.work> and the [GitHub Releases](https://github.com/Yangshifu1024/GitWave/releases/latest) page.

## Features

- **Workspace management** — multiple workspaces, repo tabs with drag-reorder that scroll horizontally with the mouse wheel once they overflow the window (the active tab is kept in view), adding several local repositories at once (already-added paths and repeats inside the batch are skipped, unusable paths are reported without failing the batch), per-workspace AI context; a workspace is an abstraction, not a directory
- **Working copy** — stage / unstage, discard, ignore, commit with conventional-commit type chips (amend the last commit included), commit message AI assist
- **Branches & sync** — create / switch / delete / rename / set upstream tracking (the picker lists every remote-tracking branch, `origin/main` included), double-click a remote branch to create its tracked local branch and switch to it (DWIM), push / pull with confirm (pick the target remote), tag pushes that send only the tags on the pushed commit (conflicting tags are skipped and named instead of failing the batch), fetch across all remotes with stale tracking-ref pruning, time-boxed network syncs with an in-flight cancel button, an in-app auth prompt that collects a username / PAT and retries in place when a remote challenges for credentials (optionally saved to your system keychain), merge (ff & no-ff) with conflict panel, and a collapsible sidebar that groups prefixed branches into folders and auto-expands to the branch you check out
- **History** — commit graph with fork-style edges, commit details, blame, reflog, tags, and right-click menus on commits / branch-tag badges (checkout, cherry-pick, revert, reset, copy info) that reveal the checked-out branch in the sidebar; ref badges merge tracked remotes into a single synced badge and the graph runs on a nine-color lane palette
- **Diff viewer** — side-by-side and unified views, side-by-side image diffs (old / new versions rendered for png / jpg / gif / webp / bmp / ico / svg — for unstaged changes the new side reflects the live worktree), Shiki syntax highlighting, per-hunk operations
- **Advanced Git** — stash (scoped to the active repo; each entry opens a detail window with its file list and diff, its actions are labelled buttons with tooltips, apply pre-checks a working copy that has uncommitted changes and drop confirms before running, and files saved as untracked are listed), interactive rebase, worktrees, submodules, LFS, remotes, .gitignore editor, Git hooks panel, repo health checks
- **AI collaboration** — BYOK provider setup, commit explain, AI-drafted PR descriptions; diffs stay local unless you send them to your chosen provider. AI replies in Chinese / Japanese / Korean / English per your preference
- **Automatic updates** — in-app check for updates with signed downloads and one-click install (macOS / Windows / AppImage); deb / rpm installs get update prompts pointing at the releases page
- **Background refresh** — auto refresh keeps every repository in the workspace current on a configurable interval (default 5 minutes), fetching each repo best-effort; ⌘R / Ctrl+R refreshes the active repo on demand
- **Proxy-aware networking** — AI requests, sync, LFS and update checks follow the OS system proxy (Windows / macOS) or a manual proxy URL you set in Settings → Network, effective immediately; loopback addresses (local Ollama etc.) always bypass it
- **SSH key management** — generate / import keys, per-repo SSH configuration
- **Platform UX** — command palette, menu bar app mode, themes plus UI / monospace font family and size settings, one-click open of the active repo in the system file manager, a detected terminal, or a detected code editor (VS Code / Zed / VSCodium), Chinese / English interface with instant switching, and a self-drawn title bar that merges the app menu and the operation bar into a single row (traffic lights / window controls kept clear)

## Tech stack

- **Frontend:** React 19 + TypeScript + Vite 7, Tailwind CSS 4 + HeroUI v3, zustand, TanStack Query / Virtual
- **Backend:** Rust + [Tauri 2](https://tauri.app), clean-architecture layers (`domain` / `application` / `infrastructure`), `git2` (vendored libgit2 + libssh2 + OpenSSL) — no system Git dependency
- **Testing:** Vitest (unit), Playwright (e2e)

## Quick start

Prerequisites:

- Rust stable ([rustup](https://rustup.rs))
- Node.js ≥ 20
- macOS: Xcode command line tools (`xcode-select --install`)
- Linux: `webkit2gtk-4.1-dev`, `build-essential`, `cmake`, `curl`, `wget`, `file`, `libssl-dev`, `libxdo-dev`, `libayatana-appindicator3-dev`, `librsvg2-dev`, `patchelf`
- Windows: WebView2 runtime + MSVC build tools

```bash
pnpm install
pnpm tauri dev
```

## Scripts

| Command | What it does |
|---|---|
| `pnpm dev` | Vite dev server (frontend only, no IPC) |
| `pnpm build` | TypeScript check + Vite production build |
| `pnpm tauri dev` | Tauri app in dev mode (frontend + Rust core) |
| `pnpm tauri build` | Tauri production build (.dmg / .exe / .deb / .rpm / .AppImage) |
| `pnpm lint` | ESLint (`lint:fix` to auto-fix) |
| `pnpm format:check` | Prettier check (no write) |
| `pnpm format` | Prettier write |
| `pnpm typecheck` | TypeScript check |
| `pnpm test` | Vitest (unit) |
| `pnpm test:e2e` | Playwright e2e tests |

Rust commands (run inside `src-tauri/`):

| Command | What it does |
|---|---|
| `cargo check --all-targets` | Type check |
| `cargo clippy --all-targets -- -D warnings` | Strict lint |
| `cargo test --all-targets` | Run all tests |
| `cargo fmt` | Format Rust sources |

## CI

Workflows live in `.github/workflows/`:

- **lint / test** — on every push and PR: `rust-lint` + `frontend-lint`, `rust-test` + `frontend-test`, each on a macOS / Ubuntu / Windows matrix
- **build** — on tag push (`v*` or any tag): builds macOS (aarch64), Linux (deb / rpm / AppImage) and Windows (NSIS); when all three pass, a **draft GitHub release** is created with all artifacts and auto-generated release notes, and the updater manifest's download URLs are then rewritten to plain `releases/download` links (tauri-action writes asset-API URLs, which answer 403 without a User-Agent header and count against the unauthenticated API quota). macOS builds are signed and notarized via repository secrets (`APPLE_*`, `KEYCHAIN_PASSWORD`), and OpenSSL/libgit2 are statically linked so binaries are self-contained

### Cutting a release

1. Bump the version everywhere with `pnpm bump <x.y.z>` — it rewrites `package.json`, `src-tauri/tauri.conf.json`, `src-tauri/Cargo.toml` and refreshes the `gitwave` entry in `src-tauri/Cargo.lock`. Never edit versions by hand; `pnpm-lock.yaml` is untouched (it does not record the root package's own version)
2. Commit, then tag and push:
   ```bash
   git tag -a v0.x.0 -m "v0.x.0"
   git push origin main v0.x.0
   ```
3. When CI is green, find the draft under Releases, review the notes, and publish
   - Publishing the draft also publishes `latest.json` — the manifest the in-app updater polls; existing installs pick the new version up from there
   - Updater artifacts (`.app.tar.gz` / `.sig` / `-setup.exe.sig` / `latest.json`) are produced by CI; local `tauri build` now requires `TAURI_SIGNING_PRIVATE_KEY` (and `TAURI_SIGNING_PRIVATE_KEY_PASSWORD` if the key is encrypted) exported in the shell: `export TAURI_SIGNING_PRIVATE_KEY=$(cat ~/.tauri/gitwave.key)`

Per AGENTS.md, **AI agents must not commit / push / merge** — humans gate every change to `main`.

## Documentation

- Website — <https://gitwave.work> (landing page & downloads)
- `docs/pm/core/` — Product management (features, scope, roadmap)
- `docs/tech/` — Engineering decisions (architecture, selection, ADRs, conventions)
- `docs/design/` — UI/UX overview (3-pane layout, tokens, components)
- `docs/tasks/` — Per-task plans and reviews
- `AGENTS.md` — Workflow rules and agent boundaries

## License

[MIT](./LICENSE) © Yangzhenbiao
