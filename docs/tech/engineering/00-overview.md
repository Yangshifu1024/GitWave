# GitWave · 工程约定

> 工程实践与质量门禁。Git 工作流与分支 / Commit / PR 规则见 `AGENTS.md`，本目录仅补充工程层面未覆盖的内容。
> CI / 测试 / 发布描述对照 **v0.9.3** 的 `.github/workflows/` 与仓库现状。

## 代码风格

| 语言 | 格式化 | Lint | 类型检查 |
|---|---|---|---|
| Rust | rustfmt | clippy（deny warnings） | 编译期强制 |
| TypeScript | Prettier | ESLint（strict + type-checked） | tsc + tsconfig strict |

- **pre-commit hook**：rustfmt + clippy + Prettier + ESLint（仅对暂存文件）
- **CI 校验**：每个 PR 必须通过；CI fail 不允许合入
- **commit message**：Conventional Commits（见 `AGENTS.md`）；commitlint 校验
- **工具链版本**：TypeScript 6（`package.json` 中为 `~6.0.3`）、Vitest 5、Node 22（CI 使用的版本，见 `.github/workflows/lint.yml:76`、`test.yml:99`、`build.yml:75/149/197`）
- **Markdown 不在 Prettier 管辖内**：`.prettierignore` 排除了 `*.md`，所以文档改动不会被格式检查拦住，排版与事实只能人工核对
- **`make check` 只覆盖代码门**：`fmt-check + lint + test`（见 `Makefile`），管的是代码的格式、静态检查与测试，不含任何文档检查

## 测试策略（测试金字塔）

| 层 | 范围 | 工具 | 现状（v0.9.3） |
|---|---|---|---|
| **单元** | domain / application / 前端纯函数与 store | `cargo test`、Vitest | 已落地 |
| **集成** | infrastructure 适配（libgit2、SQLite、HTTP、代理、凭证） | 真实依赖 + 临时 fixture，走同一套 `cargo test --all-targets` | 已落地 |
| **E2E** | 三个核心场景：commit→push / conflict 解决 / workspace 切换 | Playwright + tauri-driver | **未落地**。`@playwright/test` 在 package.json，`pnpm test:e2e` 只有脚本、没有测试套件：仓库内既无 `e2e/` 目录，也无 `playwright.config.*` |
| **组件** | React 组件 / hook | React Testing Library + jsdom | 已引入（下一版本）：草稿/冲突编辑、分页乱序、diff、blame、hooks 的行为测试 |

- 关键算法（diff、3-way merge、scrubber、credential callback）必须有专项单测
- AI provider 有 mock / 错误映射测试；当前请求是非流式 `stream: false`，没有 SSE 中断用例
- 产品完成定义见 `docs/pm/core/02-scope.md` §3；E2E 自动化仍是缺口，不把「文档里写了 Playwright」当成已有测试

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

所有错误必须带 **trace id**（短 UUID），日志与 UI 文案双向可查。

### 日志

- **Rust**：`tracing` + 结构化字段；日志写在 `dirs::data_dir()/GitWave/logs/`
- **前端**：console。**未接入 Sentry**（曾列为 v0.2 评估，未做）
- **崩溃**：无自动上报。不上传用户路径与仓库内容
- **PII 过滤**：AI prompt 与部分错误日志过 scrubber；不是每一条 tracing 事件都经过同一套规则

## CI/CD

### CI（GitHub Actions）

工作流在 `.github/workflows/`。push/PR 到 `main` 时 `docs/**` 与 `*.md` 被 paths-ignore，**纯文档变更不跑 lint/test**。

| Workflow / Job | Runner | 校验内容 |
|---|---|---|
| `lint.yml` · `rust-lint` | macOS + Ubuntu + Windows | rustfmt + clippy `-D warnings` |
| `lint.yml` · `frontend-lint` | 同上 | Prettier + ESLint + `tsc --noEmit` |
| `test.yml` · `rust-test` | 同上 | `cargo test --all-targets` + git2 `https` feature 断言 |
| `test.yml` · `frontend-test` | 同上 | Vitest |
| `build.yml` | tag（`**`） | 见 Release |
| `pages.yml` | 官网 | 部署 `site/` 到 GitHub Pages |

commitlint 只在本地 `pre-commit` 的 commit-msg hook，**不在 CI**。

### Release

- **触发**：推送任意 tag → `build.yml`
- **产物**：macOS aarch64 `.dmg`（Developer ID 签名 + 公证，CI 再 staple）；Windows NSIS；Linux deb / rpm / AppImage
- **发布**：先建 **draft** GitHub Release，说明来自 `git log`（不是 release-please / git-cliff）；人审后 publish。publish 同时发出 `latest.json` 给应用内更新
- **更新清单**：三个平台构建完成后由 `rewrite-manifest-urls` 任务把 `latest.json` 里的下载地址从接口地址（`api.github.com/.../releases/assets/<id>`）改写为普通下载地址（`github.com/.../releases/download/<tag>/<file>`）。接口地址缺请求头或接口额度耗尽（未登录每 IP 每小时 60 次）都会返回 403，普通下载地址不受影响
- **更新**：macOS / Windows / AppImage 可应用内下载安装；deb/rpm 只提示打开 Releases 页
- **签名密钥**：`TAURI_SIGNING_PRIVATE_KEY`；macOS 另需 `APPLE_*`

## 仓库目录约定

参考 Tauri 2 官方模板结构（`src-tauri/` 承载 Rust 核心，顶层 `src/` 承载前端）：

```
gitwave/
├── package.json
├── index.html
├── vite.config.ts
├── tsconfig.json
├── src/                      ← React 前端
│   ├── main.tsx
│   ├── App.tsx
│   └── ...
├── public/                   ← 静态资源（不经 Vite 处理）
├── src-tauri/                ← Rust core
│   ├── Cargo.toml
│   ├── Cargo.lock
│   ├── build.rs
│   ├── tauri.conf.json
│   ├── capabilities/
│   │   └── default.json
│   ├── icons/
│   │   ├── icon.png
│   │   ├── icon.icns
│   │   └── icon.ico
│   └── src/
│       ├── main.rs
│       ├── lib.rs
│       ├── domain/           ← DDD 领域层
│       ├── application/      ← DDD 应用层（用例编排）
│       └── infrastructure/   ← DDD 基础设施层（libgit2 / SQLite / Keychain / HTTP）
├── docs/
│   ├── pm/
│   ├── tech/
│   └── tasks/
├── AGENTS.md
└── .github/workflows/
```

要点：

- **顶层 `src/` 是前端**（Vite 默认入口），**Rust 核心在 `src-tauri/src/`**。不要把两者合并。
- **`tauri.conf.json` 与 `Cargo.toml` 同级**，均位于 `src-tauri/`。
- **`capabilities/`** 用于声明 Tauri 2 的 capability / permission 集合（IPC、白名单 API 等）。
- **DDD 分层只作用于 Rust 核心**（`src-tauri/src/` 内），前端按 React 习惯组织（components / hooks / stores / ipc）。

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