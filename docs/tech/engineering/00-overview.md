# GitWave · 工程约定

> 工程实践与质量门禁。Git 工作流与分支 / Commit / PR 规则见 `AGENTS.md`，本目录仅补充工程层面未覆盖的内容。
> 对照 `main` 当前的 `.github/workflows/` 与仓库现状（2026-11-25）。Tauri 2 + React 的旧实现已从 `main` 移除，见 [ADR 0007](../decisions/00-overview.md#0007--桌面界面迁移到-gpui-kit)。

## 代码风格

| 语言 | 格式化 | Lint | 类型检查 |
|---|---|---|---|
| Rust | rustfmt（`rustfmt.toml`） | clippy（`-D warnings`，阈值见 `clippy.toml`） | 编译期强制 |

- **pre-commit hook**：rustfmt + clippy（只对 Rust 改动触发，见 `.pre-commit-config.yaml`）
- **CI 校验**：每个 PR 必须通过；CI fail 不允许合入
- **commit message**：Conventional Commits（见 `AGENTS.md`）。commitlint 随 Node 工具链一起移除，**不再自动校验**，靠人工遵守
- **工具链版本**：Rust 1.92 或更新（`Cargo.toml` 的 `rust-version`。GPUI Kit 自己不声明最低版本；它依赖树里声明最高的是 Linux 平台层带进来的 `oo7` 0.6.0，正好是 1.92）；CI 用 `dtolnay/rust-toolchain@stable`。系统依赖：`git` 要在 PATH 上（一个测试会启动 `git --version`；HTTPS 凭证桥接与 LFS 功能在运行时调用系统 `git`），Linux 另需 fontconfig、Wayland、X11/xkb、D-Bus 开发包，清单见 `.github/workflows/ci.yml`
- **没有自动格式检查的文件**：Markdown、YAML、JSON（语言文件、工作流、dependabot 配置）。它们的排版与事实只能人工核对
- **`make check` 只覆盖代码门**：`fmt-check + lint + test`（见 `Makefile`），不含任何文档检查

## 测试策略（测试金字塔）

| 层 | 范围 | 工具 | 现状 |
|---|---|---|---|
| **单元** | `gitwave-core` 的 domain / application | `cargo test` | 已落地（90 个，含内联测试） |
| **集成** | infrastructure 适配（libgit2、SQLite、HTTP、代理、凭证） | 真实依赖 + 临时 Git fixture，同一套 `cargo test --all-targets` | 已落地（306 个；另有 `host` 3 个、`lib.rs` 的守卫测试 3 个） |
| **界面** | `gitwave`（19 个）：语言文件的一致性与语言的选择（`locales.rs`，9 个）、tokio 桥接（`runtime.rs`，2 个）、窗口壳的交互（`workbench_tests.rs`，7 个）、日志（`logging.rs`，1 个） | gpui-kit 的 test-support：无头窗口渲染真实组件并模拟输入 | 已起步，随各里程碑补充 |
| **E2E** | 三个核心场景：commit→push / conflict 解决 / workspace 切换 | 未定 | **未落地**。旧实现的 Playwright 也从未落地 |

- 关键算法（diff、3-way merge、scrubber、credential callback）必须有专项单测
- AI provider 有 mock / 错误映射测试；当前请求是非流式 `stream: false`，没有 SSE 中断用例
- 界面测试不覆盖系统输入法组合、原生对话框与读屏器，这些在三端实机验收
- rust-i18n 的语言是进程全局的，并行的测试会互相改语言。渲染窗口的测试在开头调用 `locales::take_language_turn(cx)`：锁由测试的应用持有，到应用销毁才释放，因为框架在测试函数返回后还会再画一帧。不渲染窗口的测试持有 `locales::lock_language()` 返回的 guard
- 产品完成定义见 `docs/pm/core/02-scope.md` §3；E2E 自动化仍是缺口

## 错误处理与日志

### 错误分类

| 类别 | 例子 | 用户文案策略 |
|---|---|---|
| **网络** | fetch / push 失败 | "网络不可达，检查代理 / VPN" |
| **凭证** | auth 失败 | "凭证被拒绝，请检查 SSH key / HTTPS helper 配置" |
| **权限** | EACCES | "权限不足，尝试以管理员 / 调整文件权限" |
| **版本冲突** | rebase / merge 冲突 | "需要手动解决冲突，已为你打开冲突面板" |
| **协议** | 远端协议不支持 | 明确说明支持范围（git / https / ssh / ssh+git） |
| **未知** | 其他 | "未知错误，已记录，请反馈 issue（附 trace id）" |

所有错误必须带 **trace id**（短 UUID），日志与 UI 文案双向可查。每个错误码常量（`gitwave-core` 的 `domain::error_codes`）都要有中英文文案：`crates/gitwave/locales/errors-*.json`，由测试双向校验。

### 日志

- **Rust**：`tracing` + 结构化字段。`gitwave-core` 提供日志初始化（`infrastructure::observability::tracing::init`）：JSON 输出到 `dirs::data_dir()/GitWave/logs/`，每天一个文件 `app.log.YYYY-MM-DD`（日期按 UTC 算），保留 30 天，级别由 `RUST_LOG` 决定（默认 `info`）。GPUI Kit 应用在启动时调用它，并把写线程的 guard 存成全局状态（`crates/gitwave/src/logging.rs`）；guard 一旦被丢弃，日志就停，所以应用在退出时主动丢弃它，把缓冲里的日志写完。测试用 `tracing::init_in` 把日志写到临时目录，不碰用户真实的日志目录
- **崩溃**：无自动上报。不上传用户路径与仓库内容
- **PII 过滤**：AI prompt 与部分错误日志过 scrubber；不是每一条 tracing 事件都经过同一套规则

## CI/CD

### CI（GitHub Actions）

工作流在 `.github/workflows/`。push/PR 到 `main` 时 `docs/**`、`site/**` 与 `**.md` 被 paths-ignore，**纯文档与官网变更不跑 CI**。

| Workflow / Job | Runner | 校验内容 |
|---|---|---|
| `ci.yml` · `Rust` | macOS + Ubuntu + Windows | rustfmt（仅 Ubuntu）+ clippy `-D warnings` + `cargo test --workspace --all-targets --locked` + 两条 libgit2 TLS 断言 |
| `pages.yml` | 官网 | 部署 `site/` 到 GitHub Pages |

Conventional Commits 不在 CI 校验（见「代码风格」）。

### Release

**`main` 目前没有发布流水线。** 旧的 tag 触发构建（`build.yml`）随 Tauri 应用一起移除；最近发布的版本仍是 v0.11.0（Tauri 应用，可从 Releases 页下载）。新应用的打包、签名公证与应用内更新是迁移计划的 M7 里程碑，见 `docs/tasks/refactor-gpui-kit-desktop/plan.md`。在此之前推送 tag 不会产生任何构建。

## 仓库目录约定

一个 Cargo workspace，两个 crate。DDD 三层在 `gitwave-core`，界面在 `gitwave`（[ADR 0007](../decisions/00-overview.md#0007--桌面界面迁移到-gpui-kit)）：

```
gitwave/
├── Cargo.toml               ← workspace 清单与各 crate 共用的包信息
├── Cargo.lock
├── assets/
│   ├── icons/               ← 应用图标（icon.icns / icon.ico / icon.png 及各尺寸）
│   └── gitwave-screenshot.png
├── crates/
│   ├── gitwave-core/        ← 应用核心，不含任何界面框架
│   │   ├── Cargo.toml
│   │   ├── migrations/      ← SQLite 迁移脚本（编译期内嵌）
│   │   └── src/
│   │       ├── application/      ← DDD 应用层（用例编排、同步操作调度）
│   │       ├── domain/           ← DDD 领域层
│   │       ├── infrastructure/   ← DDD 基础设施层（libgit2 / SQLite / Keychain / HTTP）
│   │       ├── bootstrap.rs      ← 状态库、libgit2 所有者校验、代理环境桥接
│   │       └── host.rs           ← 终端与编辑器的探测、启动
│   └── gitwave/             ← GPUI Kit 应用（迁移中）
│       ├── Cargo.toml
│       ├── build.rs         ← 让改动语言文件触发重新编译
│       ├── locales/         ← 界面文案：<域名>.<语言>.json
│       └── src/
│           ├── main.rs          ← 启动顺序：日志、tokio、应用状态、语言、菜单、窗口
│           ├── locales.rs       ← 提供哪些语言、启动时选哪个、怎样切换
│           ├── runtime.rs       ← tokio 运行时与桥接
│           ├── app_state.rs     ← 持有 gitwave-core 的上下文
│           ├── logging.rs       ← 启动日志，并保住写线程
│           ├── menus.rs         ← 应用菜单
│           └── workbench.rs     ← 主窗口（workbench_tests.rs 是它的测试）
├── scripts/
│   └── bump-version.sh      ← 改 workspace 版本并刷新 Cargo.lock
├── site/                    ← 官网（单独部署）
├── docs/
│   ├── pm/
│   ├── tech/
│   ├── design/
│   └── tasks/
├── AGENTS.md
└── .github/workflows/
```

要点：

- **`gitwave-core` 不得依赖任何界面框架**，`src/lib.rs` 有测试守卫。界面层直接调用它的用例，进程内调用，没有 IPC。
- **界面文案只在 `crates/gitwave/locales/` 维护**。文件名是 `<域名>.<语言>.json`，占位符写 `%{name}`。新增一条文案，两种语言的文件都要加，`locales.rs` 的测试会校验键集合、占位符和错误码对应关系；改动语言文件会经 `crates/gitwave/build.rs` 触发重新编译。rust-i18n 没有复数：从 i18next 带过来的 9 组 `_one` / `_other` 键（每种语言 18 个）由界面代码按数量选择。
- **版本号只有一处**：`Cargo.toml` 的 `[workspace.package] version`。改版本用 `scripts/bump-version.sh <x.y.z>`，不要手改锁文件。

## 旧实现的源码

Tauri 应用（`src-tauri/`）、React 前端（`src/`）与 Node 工具链已在 2026-11-25 从 `main` 移除。需要对照旧实现时读提交 `8466d55`，例如 `git show 8466d55:src/components/DiffViewer.tsx`。历史文档（`docs/design/`、`docs/tasks/`、`docs/pm/`、`docs/tech/planning/`、`.specify/`）里指向 `src/`、`src-tauri/` 的路径都指这个提交；它们记录的是当时的事实，不随这次移除改写。

## 安全

- **API key**：仅 OS Keychain；内存中按需解密使用，不落盘
- **diff scrubber**：发送 AI 前 regex + token 扫描（GitHub token / AWS key / 私钥 / 常见 PII）
- **上传前必经 scrubber**；scrubber 可被用户显式禁用，但每次禁用需用户确认
- **不收集**任何用户代码、提交内容、commit message 至服务端（除非用户主动通过 AI 调用上传）
- **本地优先**：diff 默认不离开本机；所有网络调用显式列出

## 性能预算

- 应用冷启动到首个可交互界面：< 1s（macOS Apple Silicon 基线）
- history 图打开到首次可交互：< 500ms（10k commit 仓）
- 单文件 diff 渲染：< 200ms（1MB 文件）
- 后台 git 操作不阻塞 UI：取消令牌 + tokio task

## 文档同步

- 任何用户可感知行为变更 → 同步更新 `docs/pm/core/`
- 任何技术选型 / 架构变更 → 同步更新 `docs/tech/`
- 任何具体 PR / 任务执行过程 → 同步更新 `docs/tasks/<任务名>/`
- 见 `AGENTS.md` "技术文档归属"小节的判定规则
