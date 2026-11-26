# GitWave · 技术选型

> 当前 `main` 的选型一览（2026-11-25）。详细 ADR 见 `docs/tech/decisions/`。
> Tauri 2 + React 的旧实现（v0.11.0）已从 `main` 移除，源码读提交 `8466d55`。它的选型记录保留在本文后半部分，作为对照。

## 摘要

| 维度 | 选择 | 关键原因 |
|---|---|---|
| 桌面 | **GPUI Kit**（GPUI 之上的应用与组件框架） | 纯 Rust、单一二进制、没有 WebView。见 [ADR 0007](../decisions/00-overview.md#0007--桌面界面迁移到-gpui-kit) |
| 界面文案 | **rust-i18n + JSON 语言文件** | 组件库自身用的库；1108 条中英文案从旧实现原样带过来，迁移时又加了 19 条。见「界面文案」 |
| 系统语言 | **sys-locale** | 读用户在系统里偏好的语言。GPUI 自己就依赖它，加为直接依赖不引入新的包 |
| 本地日期 | **chrono** | 一周以前的提交显示本地日期。依赖树里已经有它（带默认特性），加为直接依赖不引入新的包 |
| Git 后端 | **libgit2（git2-rs，vendored + libssh2 + OpenSSL）** | 全量进程内运行，跨平台一致，无外部 CLI 依赖 |
| 本地存储 | **SQLite（rusqlite，bundled）** | 结构化查询 + 嵌入迁移 + 跨平台 + WAL |
| AI 集成 | **自研统一接口 + HTTP（reqwest）** | 控制最透，覆盖 BYOK + Ollama + 自定义 endpoint |
| 异步运行时 | **tokio**，桥接到 GPUI 的任务 | GPUI 自带执行器且不依赖 tokio；core 的网络操作跑在 tokio 上。`gitwave` 启动一个两线程的运行时，`runtime::use_case` 把用例交给它，结果连同错误码作为 GPUI 任务回到主线程 |
| 进程间通信 | **无** | 界面在进程内直接调用 `gitwave-core` 的用例，没有命令层 |

## 桌面：GPUI Kit

- **理由**：去掉 WebView 进程、IPC 边界与 Node.js 工具链，应用成为一个 Rust 二进制。选型时对照过的备选与风险见 ADR 0007 和 [调研笔记](./gpui-kit-research.md)。
- **版本**：精确钉在 `=0.11.0`。上游是 0.x，每个小版本都带破坏性变更，升级是独立的任务。
- **已知风险**：上游每 1 到 2 周一次破坏性变更；不提供打包、签名与自动更新；冷启动与数万 commit 的性能在 GPUI Kit 上没有测量数据；macOS 拼音候选窗与 Windows 平台层有未关闭的上游问题。

## 界面文案：rust-i18n

- **文件**：`crates/gitwave/locales/<域名>.<语言>.json`，每种语言 25 个文件、1138 条。其中 1108 条来自旧实现；30 条是迁移时加的：标准菜单项（撤销、粘贴、最小化等；旧实现用的是预定义菜单项，语言文件里没有这些文字）、语言选项的名称、语言设置的两条错误文案，以及只读工作台第一步的 11 条（状态来源的四种说法、历史加载失败与重新读取、三条只读版的空状态与缺失仓库的说法、三条错误文案）。
- **语言切换**：`crates/gitwave/src/locales.rs` 列出提供的语言。「跟随系统」在启动时用 `sys-locale` 读一次系统偏好的语言，按偏好的顺序取第一个能对应的：中文变体对应到 `zh-CN`，英语对应到 `en`；一个也对应不上时用英文。用户的选择经 `get_language_settings` / `set_language_settings` 存进 `app_settings` 表的 `language` 键。入口在「显示 › 语言」菜单和设置对话框，切换立即生效：菜单重建，视图重新渲染。保存失败时界面已经是新语言，只记一条日志，下次启动回到原先存的选择。
- **语言名各用自己的文字**：菜单和设置里写的是 English 和 简体中文，两种界面下都一样，看不懂当前语言的人也找得到自己的。
- **组件库自带的文字随语言切换**：gpui-component 的 `locales/ui.yml` 里每条文案带 8 种语言（en、ja、pt-BR、ru、zh-CN、zh-HK、zh-TW、it，简体中文 122 条全覆盖）。它和 GitWave 共用 rust-i18n 的进程级语言，所以它的内置文字跟着一起切换。
- **与旧实现的关系**：文案来自旧实现的 i18next 语言包。文件按 rust-i18n 的规则改名，占位符从 `{{name}}` 改成 `%{name}`（343 处；迁移时新增的文案里有 2 处，只读工作台第一步新增的文案里又有 6 处，现在共 351 处），其余逐字保留。
- **带着却还没用上的文案**：有四条从旧实现原样带来，说的是只读工作台第一步还做不到的事，所以这一步没有显示它们：`workspace.tabs.empty`（让用户从「仓库」菜单 init、clone 或添加）、`workspace.tabs.missingTitle`（右键可重新关联）、`workspace.empty.description`（创建一个工作区）、`branches.graph.selectRepoDesc`（从侧边栏选择仓库）。对应的功能进来之后再接上；在那之前，这一步用的是另加的只读版说法（`workspace.tabs.none`、`workspace.tabs.missingName`、`branches.graph.noRepository`）。
- **约束**：rust-i18n 没有复数与上下文。9 组 `_one` / `_other` 键由界面代码按数量选择。
- **守护**：`crates/gitwave/src/locales.rs` 的测试校验两种语言的键集合、占位符、每个错误码都有对应文案，并把每条文案经 `t!` 读回来与文件比对。
- **编译期内嵌**：rust-i18n 在编译时读入文件，cargo 自己不会追踪这些文件；`crates/gitwave/build.rs` 让改动语言文件触发重新编译。
- **只在 Rust 侧维护**：Tauri 版已删除，不再有第二份文案。

## Git 后端：libgit2（git2-rs）

- **理由**：全量进程内运行；diff / blame / log 高频读路径无 spawn 开销；跨平台一致
- **已知边界**：
  - **钩子不自动执行**：libgit2 不会跑 hooks（这是特性，不是 bug），与 CLI 默认行为不同；Hooks 面板只编辑 `.git/hooks`，不执行
  - **凭证交互**：`CredentialCallback` 桥接系统 `git credential helper`（交互被禁止，应用内弹窗承接）+ 应用 Keychain 镜像；SSH 走 ssh-agent（Unix `SSH_AUTH_SOCK` / Windows OpenSSH named pipe）
  - **worktree / submodule**：原生支持
  - **interactive rebase**：UI 层驱动（pick / reword / edit / squash / fixup / drop），不依赖 `git rebase -i`
  - **LFS**：git2 不支持，经隐藏 `git lfs` 子进程封装
- **降级**：LFS、部分 SSH 管理、打开外部工具等走 spawn；核心读写仍在进程内 libgit2

## 本地存储：SQLite

- **理由**：结构化查询 + 迁移成熟；WAL 模式支持多连接
- **库**：**rusqlite**（bundled SQLite）
- **存放位置**：`dirs::data_dir()/GitWave/state.db`
  - macOS：`~/Library/Application Support/GitWave/state.db`
  - Windows：`%APPDATA%/GitWave/state.db`
  - Linux：`$XDG_DATA_HOME/GitWave/state.db`（或 `~/.local/share/GitWave/state.db`）
- **表结构**（截至 migration 0004）：
  - `workspaces`：id, name, settings_json, last_active_repo_id, created_at, updated_at
  - `repos`：id, workspace_id, path, nickname, settings_override_json, status, missing_at, added_at, position
  - `app_settings`：key, value_json, updated_at。应用级设置，一个键一段 JSON：`proxy`（代理）、`language`（界面语言）、`shell`（上次所在的 Workspace）。加一个键不需要迁移
  - `schema_version`：已应用迁移号
- **没有的表**：`ai_cache`、`sessions`（规划草案，未建）。per-repo AI rules 读仓库内 `.gitwave/AI.md`，不进 SQLite
- **迁移**：`crates/gitwave-core/migrations/NNNN-name.sql`，启动时 `BEGIN IMMEDIATE` 一次应用
- **在指定的目录打开**：`sqlite::open_in(dir)` 打开（必要时创建）`dir/state.db`；`open()` 是它在用户数据目录里的那个。`bootstrap::open_app_context_in(dir)` 在那个目录上打开应用设置和工作区两个存储。能直接打开正式版那一份的入口有四个，文档里都写明了开发版不得调用：`bootstrap::open_app_context`、`sqlite::state_dir`、`sqlite::open`、`SqliteAppSettingsRepo::open`
- **新应用的现状**：开发版不打开正式版的 `state.db`。它把正式版的状态复制到旁边的 `GitWave-dev/`，只打开这份副本，日志也写在那里：`persistence::snapshot` 用 SQLite 的 `VACUUM INTO` 经只读连接生成。正式版的 `state.db` 和它已有的 `state.db-wal` 不被写入；只读连接会在旁边读或创建 `-shm` 索引，原库没有 `-wal` 时还会留下一个空的，两者都不含数据，正式版下次关闭时会删掉。空日志不算变化，否则正式版每跑一次，开发版就要多复制一次。旁边记录着副本对应原库的哪个版本，原库变了才在下次启动时重新复制；复制失败时照旧打开现有的副本，并把原因告诉用户。这样做是因为开发版一旦新增迁移，直接打开会把正式版的库升级到它不认识的架构（现在两边的迁移相同，都是 0001–0004）。用户在开发版里改的设置（语言、上次所在的 Workspace、活动仓库）只留在副本里。接管真实状态库的路径在 M7 的发布链路里定

## AI 集成：自研统一接口

- **形态**：`gitwave-core` 内 provider dispatch + failover 链；调用方拿完整生成结果
- **协议覆盖**：
  - OpenAI Chat Completions（兼容 OpenAI / DeepSeek / Qwen / Azure OpenAI / 自定义 endpoint）
  - Anthropic Messages
  - Ollama（本机探测，限回环）
- **请求**：HTTP JSON，`stream: false`（**不是** SSE）
- **BYOK 存储**：OS Keychain（macOS Keychain / Win Credential Manager / Linux Secret Service），内存中按需解密使用，不落盘
- **scrubber**：diff 发送前跑 secret scanner；可被用户显式禁用但需确认
- **离线模式**：所有云端 provider 一键禁用，仅本地 Ollama 可用
- **Provider 故障转移**：已落地。`ai_failover` 有序链；网络 / 5xx / 429 切下一个，401/403 停链

## 旧实现的选型（v0.11.0，已移除）

以下记录的是 Tauri 2 + React 实现的选型，保留作迁移对照。源码读提交 `8466d55`。

| 维度 | 选择 |
|---|---|
| 桌面框架 | **Tauri 2** |
| 前端栈 | **React 19 + TypeScript + Vite 8** |
| UI | **Tailwind CSS 4 + HeroUI v3 + Lucide**（ADR 0005） |
| IPC 类型 | 手工 typed command（`src/lib/api.ts` 镜像 Rust），未引入 tauri-specta |

- **Tauri 2 的理由与代价**：规划时 v0.1 仅 macOS，v0.2 扩 Windows，v0.3 加 Linux；tag 触发 CI 出包。代价是 WebView 内核在三端不一致（WebKit / WebView2 / WebKitGTK），部分 CSS 与 JS 行为需适配。
- **前端实际库**：虚拟滚动 `useFixedVirtualizer`、自研 `DiffViewer`（character-level 高亮、图片对比）、自研拖拽与命令面板、Zustand + TanStack Query、ESLint strict + Prettier；测试只有 Vitest 单元，Playwright E2E 未落地。
- **当时未选的桌面方案**：SwiftUI 原生（Windows 需重写）、Electron（体积与冷启动）、Flutter Desktop（Web 生态无法复用）。
