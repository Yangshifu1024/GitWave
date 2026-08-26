# GitWave · 工程约定

> 工程实践与质量门禁。Git 工作流与分支 / Commit / PR 规则见 `AGENTS.md`，本目录仅补充工程层面未覆盖的内容。

## 代码风格

| 语言 | 格式化 | Lint | 类型检查 |
|---|---|---|---|
| Rust | rustfmt | clippy（deny warnings） | 编译期强制 |
| TypeScript | Prettier | ESLint（strict + type-checked） | tsc + tsconfig strict |

- **pre-commit hook**：rustfmt + clippy + Prettier + ESLint（仅对暂存文件）
- **CI 校验**：每个 PR 必须通过；CI fail 不允许合入
- **commit message**：Conventional Commits（见 `AGENTS.md`）；commitlint 校验

## 测试策略（测试金字塔）

| 层 | 范围 | 工具 |
|---|---|---|
| **单元** | domain / application / 纯函数 | `cargo test`（Rust）、Vitest（TS） |
| **集成** | infrastructure 适配（libgit2、SQLite、Keychain、HTTP） | 真实依赖 + fixture |
| **E2E** | 三个核心场景：commit→push / conflict 解决 / workspace 切换 | Playwright + tauri-driver |

- 关键算法（diff、3-way merge、scrubber、credential callback）必须有专项单测
- AI stream 必须有 mock provider 测试，覆盖中断、错误、流式边界
- v0.1 完成定义（见 `docs/pm/core/02-scope.md` §2.1）：三个核心场景端到端 + 主要场景无崩溃

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

- **Rust**：`tracing` + 结构化字段（JSON）；本地文件 rotation，默认 7 天，30 天可调
- **前端**：console + 可选 Sentry（v0.2+ 评估；**不采集 PII / 凭证 / 用户代码**）
- **崩溃**：仅上传堆栈 + 版本号 + 平台，不上传用户路径与仓库内容
- **PII 过滤**：所有日志输出前过 scrubber，与 AI 共享同一套规则

## CI/CD

### CI（GitHub Actions）

| Job | Runner | 校验内容 |
|---|---|---|
| `lint` | ubuntu-latest | rustfmt + clippy + ESLint + Prettier + commitlint |
| `test-unit` | macOS + linux | cargo test + vitest |
| `test-integration` | macOS + linux | 真实 libgit2 / SQLite / Keychain / HTTP fixture |
| `build` | macOS + windows + linux | tauri build 产出 .app / .exe / .AppImage |

### Release（v0.2 起）

- tag 触发；CHANGELOG 由 release-please 或 git-cliff 自动生成
- 自动产出：`.dmg`（含 notarization）/ `.exe` / `.AppImage`
- **签名**：macOS Developer ID；Windows code signing（视证书可用情况）
- v0.1 期间：CI 跑 lint + test，release 手动产出

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