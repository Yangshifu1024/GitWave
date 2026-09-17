# GitWave · 系统架构

> 与 `docs/tech/tech-selection/` 配套的总体架构视图。对照 **v0.8.7** 代码，不是早期规划草案。

## 进程拓扑

```
┌──────────────────────────────────────────────────┐
│  WebView（React + TS）                            │
│   ├── UI 渲染层                                  │
│   ├── 状态管理（Zustand / TanStack Query）       │
│   └── IPC 客户端（typed command + event 订阅）    │
└────────────────────┬─────────────────────────────┘
                     │ Tauri command / event
┌────────────────────▼─────────────────────────────┐
│  Rust Core                                       │
│   ├── application   用例编排、事务边界           │
│   ├── domain        Git / Workspace / AI 模型   │
│   ├── infrastructure libgit2 / SQLite /         │
│   │                  HTTP / Keychain            │
│   └── async runtime tokio                       │
└────────────────────┬─────────────────────────────┘
                     │
        ┌────────────┼────────────┐
        ▼            ▼            ▼
   libgit2 (in-proc) Keychain  HTTP (AI)
                     SQLite
```

## 分层（DDD 四层）

| 层 | 职责 | 例子 |
|---|---|---|
| **presentation** | WebView 渲染 + UI 状态 | React components, hooks, stores |
| **application** | 用例编排、事务边界 | `use_cases.rs` 中的自由函数（`clone_repo`、`generate_commit_message`、…），不是每用例一个 struct |
| **domain** | 核心模型与不变量 | `Workspace`, `RepoRef`, `CommitMessage`, `RebaseAction` |
| **infrastructure** | 外部能力适配 | `Git2RepoAdapter`, `SqliteWorkspaceStore`, `KeychainSecretStore`, `HttpAiProvider` |

依赖方向：`presentation → application → domain ← infrastructure`。domain 层零外部依赖。

## IPC 边界

- **Command**（request / response）：`src-tauri/src/lib.rs` 里的 `#[tauri::command]` 薄封装，转发到 `application::use_cases`；用于一次性动作（clone、commit、switch workspace、AI 生成等）
- **Event**（push）：后端 → 前端的进度 / 凭证存储结果等（如 sync-progress）。**AI 生成不是 event 流**
- **序列化**：JSON
- **类型**：前端 `src/lib/api.ts` **手工**镜像 Rust 结构与命令名。未使用 tauri-specta；新增命令必须同时改 Rust 与 `api.ts`

## 性能热点处理

| 热点 | 策略 | 现状 |
|---|---|---|
| history DAG（数万 commit） | `@tanstack/react-virtual` 虚拟滚动；后端 `commit_log` 按需取 | 已落地 |
| 大文件 diff | 超大文件走占位、不把数十 MB 灌进 IPC；前端按文件折叠 | 无 hunk 级 Tauri event 流 |
| syntax highlight | Shiki 按需 grammar | **未接线**；DiffViewer 自绘 character-level 高亮 |
| 后台 git 操作 | tokio + 超时 / 取消；进度进状态区 | 已落地 |
| 仓库变化感知 | 无 FS watcher | **每 N 分钟轮询（可配置，默认 5）workspace 内所有仓库**（`useAutoRefresh`）+ 手动刷新（Cmd+R，仅 active 仓库） |
| AI 生成 | 单次 HTTP，`stream: false`，command 返回全文 | 非 SSE |

## 多 Workspace 并行

- **单一 SQLite** + `workspace_id` 列区分数据
- **切换语义**：Zustand 记下 lastActiveRepo，切 Workspace 后重新拉该 Workspace 的 repo 列表与 lastActiveRepo；无独立 git watcher 可卸
- **单 active repo 模型**：UI 全局只渲染一个 repo 的主视图
- **数据隔离**：Workspace settings 在 `workspaces.settings_json`。`repos.settings_override_json` 列存在，写入恒为 `None`，无 per-repo 覆盖 UI。AI rules 读各仓 `.gitwave/AI.md`

## Workspace 切换时序

```
用户点击 Workspace B
  ↓
记下 B 为 active（Zustand + lastActiveRepo）
  ↓
listRepos(B) → pickRestoredRepo
  ↓
React Query 按新 workspaceId / repoId 重拉
  ↓
各面板（history / working copy / sidebar）跟随 query key 刷新
```

没有 git watcher 的 subscribe / unsubscribe。自动刷新是全局 60s 定时器，只作用于当前 active repo。

## AI 调用时序

```
UI 触发 "生成 commit message"
  ↓
generate_commit_message 命令（可带 language）
  ↓
use case: 拼装最近 N 个 commit + 当前 diff + 可选 .gitwave/AI.md
  ↓
infrastructure: scrubber 扫描 → 注入 prompt
  ↓
failover 链上 generate_text（HTTP JSON，stream: false）
  ↓
命令返回全文 + provider_used
  ↓
用户编辑 / 确认 → 走确定性 commit 用例
```

## 后续 TODO（架构层面）

- 跨 Workspace 全局搜索的索引策略
- 大仓库（monorepo）的 lazy load 策略
- FS watcher 取代（或补强）60s 轮询
- 大 diff hunk 流式 / 分页
- IPC 类型生成（若再评估 tauri-specta）