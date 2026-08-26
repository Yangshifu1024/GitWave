# GitWave

> Local-first Git client with AI collaboration. See `docs/pm/core/01-features.md` for product scope and `docs/tech/` for engineering decisions.

**Status:** Sprint 0 (Tauri scaffold) — features land in subsequent sprints per `docs/pm/core/04-sprint-v0.1.md`.

## Quick start

Prerequisites:

- Rust stable ≥ 1.78 ([rustup](https://rustup.rs))
- Node.js ≥ 20
- macOS: Xcode command line tools (`xcode-select --install`)
- Linux: `webkit2gtk-4.1-dev`, `libappindicator3-dev`, `librsvg2-dev`, `patchelf`, `cmake`
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
| `npm test` | Vitest |

Rust commands (run inside `src-tauri/`):

| Command | What it does |
|---|---|
| `cargo check --all-targets` | Type check |
| `cargo clippy --all-targets -- -D warnings` | Strict lint |
| `cargo test --all-targets` | Run all tests |
| `cargo fmt` | Format Rust sources |

## Documentation

- `docs/pm/core/` — Product management (features, scope, roadmap, sprint plan)
- `docs/tech/` — Engineering decisions (selection, architecture, ADRs, conventions)
- `docs/tasks/` — Per-task plans and reviews
- `AGENTS.md` — Workflow rules

## License

TBD.