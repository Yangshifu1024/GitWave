# GitWave · 技术选型

> 当前主版本选型一览。详细 ADR 见 `docs/tech/decisions/`。

## 摘要

| 维度 | 选择 | 关键原因 |
|---|---|---|
| 桌面框架 | **Tauri 2** | 跨平台路径 + Rust core 性能 + 较小包体积 |
| 前端栈 | **React + TypeScript** | 生态最广，虚拟滚动 / diff viewer / 可视化库成熟 |
| Git 后端 | **libgit2（git2-rs）** | 全量进程内运行，跨平台一致，无外部 CLI 依赖 |
| 本地存储 | **SQLite（sqlx）** | 结构化查询 + 迁移成熟 + 跨平台 + WAL 并发 |
| AI 集成 | **自研统一接口 + HTTP** | 控制最透，覆盖 BYOK + Ollama + 自定义 endpoint |

## 桌面框架：Tauri 2

- **理由**：v0.1 仅 macOS，v0.2 扩 Windows，v0.3 加 Linux；WebView 跨平台一致性 + Rust core 补足性能与原生能力
- **代价**：WebView 内核在 macOS / Win / Linux 三端不一致（WebKit / WebView2 / WebKitGTK），部分 CSS 与 JS 行为需适配与回归
- **未选**：SwiftUI 原生（macOS 极致但 Win 重写代价大）、Electron（包体积与冷启动偏重）、Flutter Desktop（Web 生态无法复用）

## 前端栈：React + TypeScript

- **理由**：虚拟滚动（react-window / react-virtuoso）、diff viewer（Monaco / CodeMirror 6）、拖拽（dnd-kit）、可视化（D3 / react-flow）均有成熟方案；AI SDK 与 TS 类型生态最丰富
- **构建**：Vite；Tauri 侧用 `tauri-specta` 或手工 typed command 生成 TS 类型绑定
- **状态**：Zustand（轻量 UI 状态）+ TanStack Query（IPC 结果缓存与失效）
- **代码风格**：ESLint strict（含 type-checked 规则）+ Prettier
- **测试**：Vitest + React Testing Library + Playwright（E2E）

## Git 后端：libgit2（git2-rs）

- **理由**：全量进程内运行；diff / blame / log 高频读路径无 spawn 开销；跨平台一致
- **已知边界**：
  - **钩子不自动执行**：libgit2 不会跑 hooks（这是特性，不是 bug），与 CLI 默认行为不同；如需触发 hooks，UI 层要显式调用
  - **凭证交互**：通过 `CredentialCallback` 桥接系统 `git credential helper`
  - **worktree / submodule**：原生支持
  - **interactive rebase**：UI 层驱动（pick / reword / edit / squash / fixup / drop），不依赖 `git rebase -i`
- **降级**：当 libgit2 缺失能力或遇到库 bug 时，可在测试覆盖范围内 spawn 真实 `git` 子进程作为 fallback
- **未选**：CLI + libgit2 双轨（多一套进程边界与并发模型）、全 CLI（spawn 开销 + 高频读路径性能）、isomorphic-git（高阶特性覆盖不足）

## 本地存储：SQLite

- **理由**：结构化查询 + 迁移成熟；WAL 模式支持多 Workspace 并发读
- **库**：`sqlx`（编译期 SQL 检查）或 `rusqlite`（更轻）
- **存放位置**：操作系统约定路径
  - macOS：`~/Library/Application Support/GitWave/state.db`
  - Windows：`%APPDATA%/GitWave/state.db`
  - Linux：`~/.local/share/GitWave/state.db`
- **表结构（草案）**：
  - `workspaces`：id, name, settings(JSON), last_active_repo_id, created_at, updated_at
  - `repos`：id, path, nickname, settings_override(JSON)
  - `ai_cache`：per-repo rules、prompt 模板缓存
  - `sessions`：多 Workspace 同时打开的活跃态快照
- **迁移**：单向前缀编号 `.sql` 文件，应用启动时检查并执行未应用项
- **备份**：每次启动自动 snapshot 到 `state.db.bak`（保留最近 5 个）

## AI 集成：自研统一接口

- **形态**：Rust 内 `Provider` trait + 各 provider 实现；前端只关心流式响应与 schema
- **协议覆盖**：
  - OpenAI Chat Completions（兼容 OpenAI / DeepSeek / Qwen / Azure OpenAI）
  - Anthropic Messages
  - Ollama（OpenAI 兼容）
  - 自定义 endpoint（任意 OpenAI 兼容 URL）
- **BYOK 存储**：OS Keychain（macOS Keychain / Win Credential Manager / Linux Secret Service），内存中按需解密使用，不落盘
- **流式**：SSE → Tauri event → 前端订阅 + 增量渲染
- **scrubber**：diff 发送前跑 secret scanner（GitHub token / AWS key / 私钥 regex + 可选 token 检测库），可被用户显式禁用但需确认
- **离线模式**：所有云端 provider 一键禁用，仅本地 Ollama 可用
- **Provider 故障转移**：v0.2 支持，按用户配置的优先级链尝试