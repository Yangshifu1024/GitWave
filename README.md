# GitWave

English | [简体中文](./README.zh-CN.md)

> Local-first Git client with AI collaboration. Website: **[gitwave.work](https://gitwave.work)** · See `docs/pm/core/01-features.md` for product scope and `docs/tech/` for engineering decisions.

**Status:** v0.5.0 — three-platform builds (macOS / Windows / Linux) produced by tag-triggered CI, with macOS builds signed and notarized and in-app auto-updates served from GitHub Releases. Current scope per `docs/pm/core/03-roadmap.md` (v0.3: three platforms + collaboration + AI intelligence).

## Download

Installers for macOS (Apple silicon, signed & notarized), Windows (NSIS) and Linux (deb / rpm / AppImage) are on <https://gitwave.work> and the [GitHub Releases](https://github.com/Yangshifu1024/GitWave/releases/latest) page.

## Features

- **Workspace management** — multiple workspaces, repo tabs with drag-reorder, per-workspace AI context; a workspace is an abstraction, not a directory
- **Working copy** — stage / unstage, discard, ignore, commit with conventional-commit type chips, commit message AI assist
- **Branches & sync** — create / switch / delete / rename, push / pull with confirm, sync status area, merge (ff & no-ff) with conflict panel
- **History** — commit graph with fork-style edges, commit details, blame, reflog, tags
- **Diff viewer** — side-by-side and unified views, Shiki syntax highlighting, per-hunk operations
- **Advanced Git** — stash, interactive rebase, worktrees, submodules, LFS, remotes, .gitignore editor, Git hooks panel, repo health checks
- **AI collaboration** — BYOK provider setup, commit explain, AI-drafted PR descriptions; diffs stay local unless you send them to your chosen provider
- **Automatic updates** — in-app check for updates with signed downloads and one-click install (macOS / Windows / AppImage); deb / rpm installs get update prompts pointing at the releases page
- **SSH key management** — generate / import keys, per-repo SSH configuration
- **Platform UX** — command palette, menu bar app mode, themes and font settings

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
npm install
npm run tauri dev
```

## Scripts

| Command | What it does |
|---|---|
| `npm run dev` | Vite dev server (frontend only, no IPC) |
| `npm run build` | TypeScript check + Vite production build |
| `npm run tauri dev` | Tauri app in dev mode (frontend + Rust core) |
| `npm run tauri build` | Tauri production build (.dmg / .exe / .deb / .rpm / .AppImage) |
| `npm run lint` | ESLint (`lint:fix` to auto-fix) |
| `npm run format:check` | Prettier check (no write) |
| `npm run format` | Prettier write |
| `npm run typecheck` | TypeScript check |
| `npm test` | Vitest (unit) |
| `npm run test:e2e` | Playwright e2e tests |

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
