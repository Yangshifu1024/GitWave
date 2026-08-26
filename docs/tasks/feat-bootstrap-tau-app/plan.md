# feat-bootstrap-tau-app

> GitWave 项目脚手架：初始化 Tauri 2 + Vite + React + TS 工程，补齐 Rust 核心 + 前端 + 工程门禁 + CI 矩阵。v0.1 的第 0 步。

## 状态

草案。

## 目标

1. **跑通"空壳启动"**：macOS 上 `pnpm tauri dev` 启动 React UI 窗口
2. **完整工程门禁就位**：rustfmt / clippy / Prettier / ESLint / pre-commit / commitlint / vitest / playwright
3. **CI 矩阵**：lint / test / build 三 job 在 macOS + Linux runner 上跑通（Windows runner v0.2 加入）
4. **目录结构**遵循 `docs/tech/engineering/00-overview.md` "仓库目录约定"小节
5. **为 v0.1 第一个 must-have（W1 Workspace CRUD）** 提供可用的代码骨架（SQLite + libgit2 + typed IPC 三层都已连通但空壳）

## 范围

### In Scope

- Tauri 2 + Vite + React + TS 项目初始化
- Rust 核心 DDD 分层骨架（domain / application / infrastructure，每个一两个占位模块 + 测试）
- 前端基础组件骨架（layout / router / theme + 主页面占位）
- 工具链：cargo（stable ≥ 1.78）/ pnpm ≥ 9 / vite / vitest / playwright
- 工程门禁：rustfmt / clippy / Prettier / ESLint / pre-commit / commitlint
- SQLite 接入（sqlx 或 rusqlite，选型在 Sprint 1 锁定）
- libgit2 接入（git2-rs）
- tauri-specta typed command 骨架（一个样例 `get_app_version`）
- 错误处理：domain 层 `AppError` enum + infrastructure 层 `tracing` 初始化（JSON formatter + 文件 rotation）
- GitHub Actions：`lint` / `test` / `build` 三 job
- 占位文档：`README.md` / `CONTRIBUTING.md` / issue & PR 模板

### Out of Scope

- 任何 must-have 功能（W1–W4、5–14）—— 留给后续 sprint
- CI 自动 release / 签名 / notarization（v0.2 起）
- 文档站 / 官网
- 国际化文案（中英文翻译，v0.1 期间先用英文）

## 依赖 / 前置

- **代码前置**：无
- **本机工具链**：
  - Rust stable（≥ 1.78，`rustup` 安装）
  - Node ≥ 20
  - pnpm ≥ 9
  - macOS：Xcode command line tools
  - Linux：`webkit2gtk-4.1` + `libsoup-3.0` + `librsvg2` 等 Tauri Linux 依赖
  - Windows（v0.2+）：WebView2 runtime + MSVC build tools

## 步骤（按顺序）

### 1. 初始化项目骨架

- 用 `pnpm create tauri-app` 或 `cargo create-tauri-app` 生成 Tauri 2 + React + TS + Vite 模板
- 把模板生成的 Rust 部分迁入 `src-tauri/`，对齐 `docs/tech/engineering/00-overview.md` 的目录约定
- 顶层 `src/` 保留为 Vite 前端入口
- 删除模板自带的样例 UI 与欢迎页

### 2. Rust 工具链与格式化

- `src-tauri/rustfmt.toml`：`max_width = 100`、`imports_granularity = "Crate"`
- `src-tauri/clippy.toml`：`cognitive-complexity-threshold = 30`，CI 开 `cargo clippy -- -D warnings`
- 在 `src-tauri/src/` 下创建 `domain/` `application/` `infrastructure/` 三层骨架（每个一两个占位模块 + 单测）
- 引入依赖：`tokio`（full）、`tracing` + `tracing-subscriber` + `tracing-appender`、`serde` + `serde_json`、`thiserror`、`anyhow`、`rusqlite` 或 `sqlx`、`git2`
- `src-tauri/Cargo.toml` workspace 不开，单 crate 即可

### 3. 前端工具链与格式化

- `.prettierrc`：`singleQuote: true`、`printWidth: 100`、`trailingComma: "all"`
- `.eslintrc`：`@typescript-eslint` strict + type-checked + react-hooks 规则
- 引入依赖：`zustand`、`@tanstack/react-query`、`react-router-dom`
- `vite.config.ts`：alias `@/` → `src/`，含 `@tauri-apps/api` 集成
- `tsconfig.json`：`strict: true`、`noUncheckedIndexedAccess: true`

### 4. 工程门禁

- 装 `pre-commit`（macOS `brew install pre-commit`，或 `pipx install pre-commit`）
- `.pre-commit-config.yaml` hooks：
  - `cargo fmt -- --check`
  - `cargo clippy -- -D warnings`
  - `prettier --check`
  - `eslint`
  - `commitlint`（commit-msg hook，校验 Conventional Commits）
- 装 `commitlint`：`@commitlint/cli` + `@commitlint/config-conventional`
- 跑 `pre-commit install --install-hooks` 启用

### 5. tauri-specta typed IPC

- 在 `src-tauri/src/lib.rs` 定义样例 command：`#[tauri::command] fn get_app_version() -> String`
- 引入 `tauri-specta`：build script 生成 TS 类型到 `src/src-tauri-bindings/`（前端 src 内子目录）
- 前端用 `import { commands } from '@tauri-apps/api'` 或 specta 生成的 wrapper 调用
- 在 `capabilities/default.json` 允许该 command

### 6. SQLite + libgit2 接入

- `src-tauri/src/infrastructure/persistence/sqlite.rs`：
  - 连接（路径：`~/Library/Application Support/GitWave/state.db` 等平台约定）
  - migration runner（一个占位 migration 文件，未来 Sprint 1 扩展）
- `src-tauri/src/infrastructure/git/git2_adapter.rs`：
  - `open_local_repo(path)` —— 返回 `RepoView`
  - `current_head()` —— 返回 commit summary
- 两个 adapter 各写一个 happy-path 单测

### 7. Tracing + 错误处理

- `src-tauri/src/infrastructure/observability/tracing.rs`：
  - 初始化 tracing-subscriber，JSON formatter + 文件 rotation（`~/Library/Logs/GitWave/app.log`，默认 7 天）
- `src-tauri/src/domain/error.rs`：
  - 定义 `AppError` enum（Network / Credential / Permission / VersionConflict / Protocol / Unknown）+ `Result<T>` 类型别名
  - `impl std::error::type` + `serde::Serialize`（用于 IPC 错误返回前端）

### 8. GitHub Actions

- `.github/workflows/lint.yml`（ubuntu-latest）：rustfmt + clippy + ESLint + Prettier + commitlint
- `.github/workflows/test.yml`（ubuntu + macOS）：cargo test + vitest
- `.github/workflows/build.yml`（macOS + windows + linux）：tauri build 产出 .app / .exe / .AppImage
- 所有 job 触发条件：`push` 到 `main` 与所有 `feature/*` `fix/*` 分支；`pull_request` 到 `main`
- macOS runner 装 `webkit2gtk` 等系统依赖脚本

### 9. 占位文档

- `README.md`：项目简介 + 启动指令 + 链接到 `docs/`
- `CONTRIBUTING.md`：链接到 `AGENTS.md` + PR 流程
- `.github/ISSUE_TEMPLATE/bug_report.yml`、`.github/ISSUE_TEMPLATE/feature_request.yml`
- `.github/PULL_REQUEST_TEMPLATE.md`：勾选清单（关联 docs/tasks/.../plan.md、CI 全绿、reviewer 通过）

### 10. 冒烟验证

- `pnpm tauri dev` 启动 → UI 显示 "Hello GitWave" 主页
- 改一行 ESLint 违规 → `pre-commit` 拦下
- 故意 commit `badmsg:` → commitlint 拦下
- push 到 feature 分支 → CI 三 job 全跑

## 分支

推荐：`feature/bootstrap-tau-app`

## 验证（DoD）

1. CI 三 job（lint / test / build）在 macOS + Linux runner 上全绿
2. 本地 `pnpm tauri dev` 启动 UI 显示 "Hello GitWave"
3. `pre-commit run --all-files` 通过
4. `cargo test` + `vitest run` 全绿
5. 目录结构与 `docs/tech/engineering/00-overview.md` 一致
6. pre-commit hook 故意引入 lint 失败 → 拦下
7. commitlint 故意引入非法 commit → 拦下
8. typed IPC `get_app_version` 在前端 UI 正确显示版本号

## 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| WebView 三端差异 | Sprint 0/1 CI 不全绿 | v0.1 期间 CI 跑 macOS + Linux，Windows runner v0.2 加；前端避免使用 WebView 不一致的最新的 Web API |
| tauri-specta 成熟度 | typed IPC 体验下降 | 准备手工 typed wrapper 降级方案 |
| libgit2 与系统 git 行为偏差 | 后续 sprint 边界 bug | Sprint 3 起跑"真实 git CLI 对比测试"作为参考实现 |
| pnpm 与 npm 切换 | 团队协作锁版本 | `packageManager` 字段写死在 `package.json`；CI 用 `pnpm/action-setup` |
| tracing 性能开销 | 冷启动变慢 | release 构建用 env-filter 关闭 debug span；JSON formatter 在 dev 模式 |
| Linux runner webkit2gtk 安装慢 | CI 时间长 | 缓存系统依赖；按 Tauri 官方 setup action |

## 关联

- `docs/tech/engineering/00-overview.md`：仓库目录约定 + 工程门禁（本文档遵循）
- `docs/tech/tech-selection/00-overview.md`：选型依据
- `docs/tech/decisions/0001`：框架选型 ADR
- `docs/tech/decisions/0004`：AI 双轨边界
- `AGENTS.md` Git Workflow：分支 + commit 规范
- `docs/pm/core/04-sprint-v0.1.md`：Sprint 0 即本任务