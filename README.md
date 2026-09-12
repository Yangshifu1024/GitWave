# GitWave

> Local-first Git client with AI collaboration. Website: **[gitwave.work](https://gitwave.work)** · See `docs/pm/core/01-features.md` for product scope and `docs/tech/` for engineering decisions.

**Status:** v0.7.14 — three-platform builds (macOS / Windows / Linux) produced by tag-triggered CI, with macOS builds signed and notarized, in-app auto-updates served from GitHub Releases, one-click open of the active repo in the system file manager or a detected terminal, a quit guard that prompts before closing while workspace repos have uncommitted changes, an amend option in the commit flow (update the last commit without a new entry), auto-switch to a newly created workspace, and the About dialog showing the build-time commit short-sha next to the version, plus proxy-aware networking (AI requests, sync, LFS and update checks follow the OS system proxy or a manual proxy URL from Settings → Network), fully in-app credential handling (no external credential-manager dialogs; when a remote challenges for HTTPS auth, an in-app prompt collects a username / access token — clone, fetch / pull / push, remote branch deletion and submodule operations included — remembers them across operations, and clone is time-boxed and cancellable like every network sync), repo adds that accept folders owned by another local account (e.g. after an elevated clone), multi-remote-aware sync (push to any configured remote, fetch across all of them) with per-operation progress attribution so concurrent fetches / pushes no longer cross-talk, merge and interactive rebase that refuse to run on a dirty working copy and auto-roll a conflict-aborted rebase back to its pre-rebase tip, hardened AI privacy (secrets scrubbed from prompts and error logs, HTTPS enforced for cloud providers, credentials stored in the per-platform OS keychain with multi-account support), Fork-style right-click menus across the commit graph and branch list, double-clicking a remote branch to create its tracked local branch and switch to it (git-switch style DWIM), tag pushes that send only the tags on the pushed commit (conflicting tags are skipped and named instead of failing the batch), a commit graph whose ref badges merge tracked remotes into single synced markers on a nine-color lane palette, a refined top bar (slim wide status area whose operation results auto-reset after 15 seconds, count buttons that keep their width when switching repos, a Ctrl+K command palette that also searches commits by message or author and jumps to them in the graph, and a title-free toolbar), and a fully bilingual (Chinese / English) UI with selectable AI reply language and adjustable UI / monospace font sizes. Current scope per `docs/pm/core/03-roadmap.md`.

## Download

Installers for macOS (Apple silicon, signed & notarized), Windows (NSIS) and Linux (deb / rpm / AppImage) are on <https://gitwave.work> and the [GitHub Releases](https://github.com/Yangshifu1024/GitWave/releases/latest) page.

## Features

- **Workspace management** — multiple workspaces, repo tabs with drag-reorder, per-workspace AI context; a workspace is an abstraction, not a directory
- **Working copy** — stage / unstage, discard, ignore, commit with conventional-commit type chips (amend the last commit included), commit message AI assist
- **Branches & sync** — create / switch / delete / rename / set upstream tracking, double-click a remote branch to create its tracked local branch and switch to it (DWIM), push / pull with confirm (pick the target remote), tag pushes that send only the tags on the pushed commit (conflicting tags are skipped and named instead of failing the batch), fetch across all remotes with stale tracking-ref pruning, time-boxed network syncs with an in-flight cancel button, an in-app auth prompt that collects a username / PAT and retries in place when a remote challenges for credentials (optionally saved to your system keychain), merge (ff & no-ff) with conflict panel
- **History** — commit graph with fork-style edges, commit details, blame, reflog, tags, and right-click menus on commits / branch-tag badges (checkout, cherry-pick, revert, reset, copy info); ref badges merge tracked remotes into a single synced badge and the graph runs on a nine-color lane palette
- **Diff viewer** — side-by-side and unified views, Shiki syntax highlighting, per-hunk operations
- **Advanced Git** — stash, interactive rebase, worktrees, submodules, LFS, remotes, .gitignore editor, Git hooks panel, repo health checks
- **AI collaboration** — BYOK provider setup, commit explain, AI-drafted PR descriptions; diffs stay local unless you send them to your chosen provider. AI replies in Chinese / Japanese / Korean / English per your preference
- **Automatic updates** — in-app check for updates with signed downloads and one-click install (macOS / Windows / AppImage); deb / rpm installs get update prompts pointing at the releases page
- **Proxy-aware networking** — AI requests, sync, LFS and update checks follow the OS system proxy (Windows / macOS) or a manual proxy URL you set in Settings → Network, effective immediately; loopback addresses (local Ollama etc.) always bypass it
- **SSH key management** — generate / import keys, per-repo SSH configuration
- **Platform UX** — command palette, menu bar app mode, themes plus UI / monospace font family and size settings, one-click open of the active repo in the system file manager or a detected terminal, Chinese / English interface with instant switching

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
- **build** — on tag push (`v*` or any tag): builds macOS (aarch64), Linux (deb / rpm / AppImage) and Windows (NSIS); when all three pass, a **draft GitHub release** is created with all artifacts and auto-generated release notes. macOS builds are signed and notarized via repository secrets (`APPLE_*`, `KEYCHAIN_PASSWORD`), and OpenSSL/libgit2 are statically linked so binaries are self-contained

### Cutting a release

1. Bump the version in all four places: `package.json`, `src-tauri/tauri.conf.json`, `src-tauri/Cargo.toml`, `src-tauri/Cargo.lock` (the `gitwave` entry)
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
