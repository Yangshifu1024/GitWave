# GitWave · 系统架构

> 与 `docs/tech/tech-selection/` 配套的总体架构视图，对照 `main` 当前的代码（2026-11-25）。
> Tauri 2 + React 的旧实现（v0.11.0）已从 `main` 移除，源码读提交 `8466d55`。它的架构记录在本文后半部分，是重写时对照的行为基线。

## 进程拓扑

```
┌──────────────────────────────────────────────────┐
│  gitwave（GPUI Kit 应用，一个 Rust 二进制）        │
│   └── 界面：目前只有窗口壳（标题栏、侧栏、状态栏）  │
└────────────────────┬─────────────────────────────┘
                     │ 进程内直接调用，没有 IPC（尚未接线）
┌────────────────────▼─────────────────────────────┐
│  gitwave-core                                    │
│   ├── application   用例编排、事务边界           │
│   ├── domain        Git / Workspace / AI 模型   │
│   ├── infrastructure libgit2 / SQLite /         │
│   │                  HTTP / Keychain            │
│   ├── bootstrap     状态库、libgit2 校验、代理    │
│   └── host          终端、编辑器                 │
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
| **presentation** | GPUI Kit 界面与界面状态 | `crates/gitwave` 里的窗口壳（标题栏、可调分栏、状态栏）；各功能的视图随里程碑加入 |
| **application** | 用例编排、事务边界 | `use_cases.rs` 中的自由函数（`clone_repo`、`generate_commit_message`、…），不是每用例一个 struct |
| **domain** | 核心模型与不变量 | `Workspace`, `RepoRef`, `FileChange`, `BranchInfo`, `StashEntry` |
| **infrastructure** | 外部能力适配 | `infrastructure::git::repo_adapter` 的自由函数（`init`、`clone_remote`、…）、`SqliteWorkspaceRepo`、`SqliteAppSettingsRepo`、`infrastructure::ai::provider` 与 `secrets` |

依赖方向：`presentation → application → domain ← infrastructure`。domain 层只依赖 serde，不依赖任何基础设施。

## Crate 划分

业务三层在 `gitwave-core`，界面在 `gitwave`（[ADR 0007](../decisions/00-overview.md#0007--桌面界面迁移到-gpui-kit)）：

| Crate | 内容 | 约束 |
|---|---|---|
| `gitwave-core` | `application`、`domain`、`infrastructure` 三层，加上不依赖界面框架的 `bootstrap`（状态库、libgit2 所有者校验、代理环境桥接）与 `host`（终端、编辑器）。网络操作的统一调度 `run_sync_op` 在 `application::sync_ops`，跑在 tokio 上 | 不得依赖任何界面框架，`src/lib.rs` 有测试守卫 |
| `gitwave` | GPUI Kit 应用，迁移中。目前有窗口壳（标题栏、可调分栏、状态栏）与界面文案，还没有接入 `gitwave-core` | 将直接调用 `gitwave-core` 的用例，不经 IPC |

## 界面与用例之间

`gitwave` 还没有接上 `gitwave-core`。下面是接线时要遵守的约定；应用状态与 tokio 运行时这两项属于 M2。

- **调用**：界面在进程内直接调用 `gitwave-core` 的用例函数，没有命令层，也没有序列化边界。
- **异步**：GPUI 自带执行器且不依赖 tokio，core 的网络操作跑在 tokio 上。`gitwave` 要自己启动一个 tokio 运行时，用例在上面运行，结果通过 GPUI 的任务回到主线程。
- **状态**：`gitwave-core` 的上下文（`AppContext`）由应用状态持有。接管磁盘上的状态库之前，先用内存库。
- **进度与通知**：旧实现靠事件推给 WebView；新应用里的对应机制随各功能迁移时再定。
- **错误**：用例返回带错误码的 `AppError`；文案在 `crates/gitwave/locales/errors-*.json`，界面按错误码取用。

## 旧实现的架构（v0.11.0，已移除）

以下描述 Tauri 2 + React 实现，是重写时对照的行为基线。源码读提交 `8466d55`。

### IPC 边界

- **Command**（request / response）：`src-tauri/src/lib.rs` 里的 `#[tauri::command]` 薄封装，转发到 `application::use_cases`；用于一次性动作（clone、commit、switch workspace、AI 生成等）
- **Event**（push）：后端 → 前端的进度 / 凭证存储结果等（如 sync-progress）。**AI 生成不是 event 流**
- **序列化**：JSON
- **类型**：前端 `src/lib/api.ts` **手工**镜像 Rust 结构与命令名。未使用 tauri-specta

### 性能热点处理

| 热点 | 策略 | 旧实现的做法 |
|---|---|---|
| history DAG（数万 commit） | 虚拟滚动；后端 cursor 分页 | `useFixedVirtualizer` 固定行高虚拟滚动，以 `useSyncExternalStore` 提供不可变可见行 |
| 大文件 diff | 超大文件走占位，不整块传给界面；前端按文件折叠 | 无 hunk 级 Tauri event 流 |
| syntax highlight | 按需加载语法；大文件回退纯文本 | Shiki 按需 grammar JSON + 独立 Oniguruma WASM，叠加 character-level 高亮 |
| 后台 git 操作 | tokio + 超时 / 取消；进度进状态区 | 已落地 |
| 仓库变化感知 | 无 FS watcher | **每 N 分钟轮询（可配置，默认 5）workspace 内所有仓库**（`useAutoRefresh`）+ 手动刷新（Cmd+R，仅 active 仓库） |
| AI 生成 | 单次 HTTP，`stream: false`，返回全文 | 非 SSE |

### 多 Workspace 并行

- **单一 SQLite** + `workspace_id` 列区分数据
- **切换语义**：记下 lastActiveRepo，切 Workspace 后重新拉该 Workspace 的 repo 列表与 lastActiveRepo
- **单 active repo 模型**：UI 全局只渲染一个 repo 的主视图
- **数据隔离**：Workspace settings 在 `workspaces.settings_json`。`repos.settings_override_json` 列存在，写入恒为 `None`，无 per-repo 覆盖 UI。AI rules 读各仓 `.gitwave/AI.md`

### Workspace 切换时序

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

没有 git watcher 的 subscribe / unsubscribe。自动刷新是一个全局定时器，按可配置的间隔（默认 5 分钟，在「设置 → 通用」里配置）扫过当前 Workspace 里的每一个仓库，对每个仓库尽力而为地抓取；一个仓库失败只让状态区显示部分成功，不把整轮刷新判成失败（`useAutoRefreshLoop` / `useRefreshWorkspaceRepos`）。⌘R / Ctrl+R 是按需刷新当前仓库（`useRefreshRepo`），它与自动刷新共用同一次本地重读，区别只在于抓取范围是当前仓库。

### AI 调用时序

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

### 架构层面遗留的课题

- 跨 Workspace 全局搜索的索引策略
- 大仓库（monorepo）的 lazy load 策略
- FS watcher 取代（或补强）按可配置间隔的轮询
- 大 diff hunk 流式 / 分页
