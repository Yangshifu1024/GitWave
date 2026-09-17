# GitWave · 技术选型

> 当前主版本选型一览（对照 **v0.8.7** 代码）。详细 ADR 见 `docs/tech/decisions/`。

## 摘要

| 维度 | 选择 | 关键原因 |
|---|---|---|
| 桌面框架 | **Tauri 2** | 跨平台路径 + Rust core 性能 + 较小包体积 |
| 前端栈 | **React 19 + TypeScript + Vite 7** | 生态最广；虚拟滚动 / 自研 diff viewer |
| UI | **Tailwind CSS v4 + HeroUI v3 + Lucide** | 见 ADR 0005 修订 |
| Git 后端 | **libgit2（git2-rs，vendored + libssh2 + OpenSSL）** | 全量进程内运行，跨平台一致，无外部 CLI 依赖 |
| 本地存储 | **SQLite（rusqlite，bundled）** | 结构化查询 + 嵌入迁移 + 跨平台 + WAL |
| AI 集成 | **自研统一接口 + HTTP（reqwest）** | 控制最透，覆盖 BYOK + Ollama + 自定义 endpoint |
| IPC 类型 | **手工 typed command**（`src/lib/api.ts` 镜像 Rust） | 未引入 tauri-specta |

## 桌面框架：Tauri 2

- **理由**：规划时 v0.1 仅 macOS，v0.2 扩 Windows，v0.3 加 Linux；截至 v0.8.7 三平台均由 tag 触发 CI 出包。WebView 跨平台一致性 + Rust core 补足性能与原生能力
- **代价**：WebView 内核在 macOS / Win / Linux 三端不一致（WebKit / WebView2 / WebKitGTK），部分 CSS 与 JS 行为需适配与回归
- **未选**：SwiftUI 原生（macOS 极致但 Win 重写代价大）、Electron（包体积与冷启动偏重）、Flutter Desktop（Web 生态无法复用）

## 前端栈：React + TypeScript

- **实际库**（不是早期备选清单）：
  - 虚拟滚动：`@tanstack/react-virtual`（history 图）
  - diff viewer：自研 `DiffViewer`（character-level 高亮；图片左右对比）；**未**用 Monaco / CodeMirror
  - 拖拽：自研 pointer 逻辑（repo tab 排序、interactive rebase），**未**用 dnd-kit
  - 命令面板：自研 `CommandPalette`，**未**用 cmdk
- **构建**：Vite 7；Tauri 命令类型由 `src/lib/api.ts` 手工维护（`Cargo.toml` 曾预留 tauri-specta，未加）
- **状态**：Zustand（轻量 UI 状态）+ TanStack Query（IPC 结果缓存与失效）
- **代码风格**：ESLint strict（含 type-checked 规则）+ Prettier
- **测试**：Vitest 单元（纯函数 / store）；**无** React Testing Library 组件测试；Playwright 在 `package.json` 中但无 config / spec，E2E 未落地

## Git 后端：libgit2（git2-rs）

- **理由**：全量进程内运行；diff / blame / log 高频读路径无 spawn 开销；跨平台一致
- **已知边界**：
  - **钩子不自动执行**：libgit2 不会跑 hooks（这是特性，不是 bug），与 CLI 默认行为不同；Hooks 面板只编辑 `.git/hooks`，不执行
  - **凭证交互**：`CredentialCallback` 桥接系统 `git credential helper`（交互被禁止，应用内 F012 弹窗承接）+ 应用 Keychain 镜像；SSH 走 ssh-agent（Unix `SSH_AUTH_SOCK` / Windows OpenSSH named pipe）
  - **worktree / submodule**：原生支持
  - **interactive rebase**：UI 层驱动（pick / reword / edit / squash / fixup / drop），不依赖 `git rebase -i`
  - **LFS**：git2 不支持，经隐藏 `git lfs` 子进程封装
- **降级**：LFS、部分 SSH 管理、打开外部工具等走 spawn；核心读写仍在进程内 libgit2
- **未选**：CLI + libgit2 双轨（多一套进程边界与并发模型）、全 CLI（spawn 开销 + 高频读路径性能）、isomorphic-git（高阶特性覆盖不足）

## 本地存储：SQLite

- **理由**：结构化查询 + 迁移成熟；WAL 模式支持多连接
- **库**：**rusqlite**（bundled SQLite）。未用 sqlx
- **存放位置**：`dirs::data_dir()/GitWave/state.db`
  - macOS：`~/Library/Application Support/GitWave/state.db`
  - Windows：`%APPDATA%/GitWave/state.db`
  - Linux：`$XDG_DATA_HOME/GitWave/state.db`（或 `~/.local/share/GitWave/state.db`）
- **表结构**（截至 migration 0004）：
  - `workspaces`：id, name, settings_json, last_active_repo_id, created_at, updated_at
  - `repos`：id, workspace_id, path, nickname, settings_override_json, status, missing_at, added_at, position
  - `app_settings`：key, value_json, updated_at（F013 代理等应用级设置）
  - `schema_version`：已应用迁移号
- **没有的表**：`ai_cache`、`sessions`（规划草案，未建）。per-repo AI rules 读仓库内 `.gitwave/AI.md`，不进 SQLite
- **迁移**：`src-tauri/migrations/NNNN-name.sql`，启动时 `BEGIN IMMEDIATE` 一次应用
- **备份**：无自动 `state.db.bak` 快照。Workspace 导入 / 导出是 JSON 文件，不是数据库备份

## AI 集成：自研统一接口

- **形态**：Rust 内 provider dispatch + failover 链；前端走普通 Tauri command，拿完整生成结果
- **协议覆盖**：
  - OpenAI Chat Completions（兼容 OpenAI / DeepSeek / Qwen / Azure OpenAI / 自定义 endpoint）
  - Anthropic Messages
  - Ollama（本机探测，限回环）
- **请求**：HTTP JSON，`stream: false`（**不是** SSE → Tauri event 流式渲染）
- **BYOK 存储**：OS Keychain（macOS Keychain / Win Credential Manager / Linux Secret Service），内存中按需解密使用，不落盘
- **scrubber**：diff 发送前跑 secret scanner；可被用户显式禁用但需确认
- **离线模式**：所有云端 provider 一键禁用，仅本地 Ollama 可用
- **Provider 故障转移**：已落地。`ai_failover` 有序链；网络 / 5xx / 429 切下一个，401/403 停链
