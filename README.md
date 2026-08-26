# GitWave

> Local-first Git client with AI collaboration. See `docs/pm/core/01-features.md` for product scope and `docs/tech/` for engineering decisions.

**Status:** Sprint 0 (Tauri scaffold) ✅ shipped; Sprint 1 (Workspace CRUD) in progress per `docs/pm/core/04-sprint-v0.1.md`. No user-facing features yet — engineering gates (CI matrix, lint / format / test) must be green before features land.

## Quick start

Prerequisites:

- Rust stable ≥ 1.78 ([rustup](https://rustup.rs))
- Node.js ≥ 20
- macOS: Xcode command line tools (`xcode-select --install`)
- Linux: `webkit2gtk-4.1-dev`, `build-essential`, `cmake`, `curl`, `wget`, `file`, `libssl-dev`, `libxdo-dev`, `libayatana-appindicator3-dev`, `librsvg2-dev`, `patchelf`
- Windows: WebView2 runtime + MSVC build tools

```bash
npm install
npm run tauri dev
```

The app window opens with "Hello GitWave".

## Scripts

| Command | What it does |
|---|---|
| `npm run dev` | Vite dev server (frontend only, no IPC) |
| `npm run build` | TypeScript check + Vite production build |
| `npm run tauri dev` | Tauri app in dev mode (frontend + Rust core) |
| `npm run tauri build` | Tauri production build (.dmg / .exe / .AppImage) |
| `npm run lint` | ESLint |
| `npm run format:check` | Prettier check (no write) |
| `npm run format` | Prettier write |
| `npm run typecheck` | TypeScript check |
| `npm test` | Vitest (currently `--passWithNoTests`: empty test set is allowed until Sprint 1 lands; flag becomes a no-op once real specs exist) |

Rust commands (run inside `src-tauri/`):

| Command | What it does |
|---|---|
| `cargo check --all-targets` | Type check |
| `cargo clippy --all-targets -- -D warnings` | Strict lint |
| `cargo test --all-targets` | Run all tests |
| `cargo fmt` | Format Rust sources |

## CI

Three GitHub Actions jobs run on every push to `main` / `feature/**` / `fix/**` and on every PR to `main`:

| Job | Runner | What it runs |
|---|---|---|
| `rust-lint` | macOS | `cargo fmt -- --check`, `cargo clippy --all-targets -- -D warnings` |
| `frontend-lint` | Ubuntu | `prettier --check`, `eslint`, `tsc --noEmit` |
| `rust-test` | macOS + Ubuntu | `cargo test --all-targets` |
| `frontend-test` | Ubuntu | `npm test` (vitest) |
| `build-macos` | macOS | `cargo tauri build` (via `@tauri-apps/cli`) → `.dmg` / `.app` |
| `build-linux` | Ubuntu 22.04 | Tauri Linux build deps + `cargo tauri build` → `.deb` / `.AppImage` |

See `.github/workflows/` for full definitions. Per AGENTS.md, **AI agents must not push / merge** — humans gate every commit.

## Documentation

- `docs/pm/core/` — Product management (features, scope, roadmap, sprint plan)
- `docs/tech/` — Engineering decisions (selection, architecture, ADRs, conventions)
- `docs/tasks/` — Per-task plans and reviews
- `AGENTS.md` — Workflow rules

## License

TBD.