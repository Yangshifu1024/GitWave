# GitWave · 系统架构

> 与 `docs/tech/tech-selection/` 配套的总体架构视图，对照 `main` 当前的代码（2026-11-25）。
> Tauri 2 + React 的旧实现（v0.11.0）已从 `main` 移除，源码读提交 `8466d55`。它的架构记录在本文后半部分，是重写时对照的行为基线。

## 进程拓扑

```
┌──────────────────────────────────────────────────┐
│  gitwave（GPUI Kit 应用，一个 Rust 二进制）        │
│   ├── 界面：窗口壳、应用菜单、语言切换             │
│   ├── 应用状态（持有 gitwave-core 的上下文）       │
│   └── tokio 运行时 ── 桥接 ──▶ GPUI 任务           │
└────────────────────┬─────────────────────────────┘
                     │ 进程内直接调用，没有 IPC
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
| **presentation** | GPUI Kit 界面与界面状态 | `crates/gitwave` 里的窗口壳（标题栏、应用菜单、可调分栏、状态栏）、应用状态与语言切换；各功能的视图随里程碑加入 |
| **application** | 用例编排、事务边界 | `use_cases.rs` 中的自由函数（`clone_repo`、`generate_commit_message`、…），不是每用例一个 struct |
| **domain** | 核心模型与不变量 | `Workspace`, `RepoRef`, `FileChange`, `BranchInfo`, `StashEntry` |
| **infrastructure** | 外部能力适配 | `infrastructure::git::repo_adapter` 的自由函数（`init`、`clone_remote`、…）、`SqliteWorkspaceRepo`、`SqliteAppSettingsRepo`、`infrastructure::ai::provider` 与 `secrets` |

依赖方向：`presentation → application → domain ← infrastructure`。domain 层只依赖 serde，不依赖任何基础设施。

## Crate 划分

业务三层在 `gitwave-core`，界面在 `gitwave`（[ADR 0007](../decisions/00-overview.md#0007--桌面界面迁移到-gpui-kit)）：

| Crate | 内容 | 约束 |
|---|---|---|
| `gitwave-core` | `application`、`domain`、`infrastructure` 三层，加上不依赖界面框架的 `bootstrap`（状态库、libgit2 所有者校验、代理环境桥接）与 `host`（终端、编辑器）。网络操作的统一调度 `run_sync_op` 在 `application::sync_ops`，跑在 tokio 上 | 不得依赖任何界面框架，`src/lib.rs` 有测试守卫 |
| `gitwave` | GPUI Kit 应用，迁移中。目前有窗口壳（标题栏、应用菜单、可调分栏、状态栏）、界面文案与中英切换、tokio 桥接、应用状态，以及启动时的日志 | 直接调用 `gitwave-core` 的用例，不经 IPC |

## 界面与用例之间

`gitwave` 经应用状态接上了 `gitwave-core`。目前走通的是读取工作区列表，以及语言设置的读取和保存；其余用例随各功能迁移接上。接线遵守下面的约定。

- **调用**：界面在进程内直接调用 `gitwave-core` 的用例函数，没有命令层，也没有序列化边界。
- **异步**：GPUI 自带执行器且不依赖 tokio，core 的网络操作跑在 tokio 上。`gitwave` 启动一个两线程的 tokio 运行时（`crates/gitwave/src/runtime.rs`）；`runtime::blocking` 把用例放到它的阻塞线程池上，结果作为 GPUI 任务回到主线程。
- **什么放后台，看耗时**：短小的本地写入（比如保存一项设置）在主线程上同步做完，界面等它，不多开任务，也就没有并发的问题。只有耗时或时长不可预估的操作（网络、大仓库的扫描、AI 调用）才放到后台；放到后台的，才需要处理顺序与取消。同步做、界面等它是用户 2026-11-25 定下的，「看耗时」是实现方据此补的判据。保存语言选择就是这个做法（`locales::switch_and_store`）。
- **状态**：`gitwave-core` 的上下文（`AppContext`）由应用状态持有（`crates/gitwave/src/app_state.rs`），现在是 `AppContext::in_memory()`。接管磁盘上的状态库之前保持这样，状态栏因此写明状态只在内存里。
- **进度与通知**：旧实现靠事件推给 WebView；新应用里的对应机制随各功能迁移时再定。
- **错误**：用例返回带错误码的 `AppError`，文案在 `crates/gitwave/locales/errors-*.json`。界面按错误码取文案的代码还没有写，现在经桥接回来的错误只有英文的 `message`。
- **语言**：界面文案在编译期内嵌，`t!` 在运行时按当前语言查找。语言的选择与切换在 `crates/gitwave/src/locales.rs`，用户的选择经 core 的语言设置保存。视图和菜单观察语言的全局状态，变化时重新渲染、重建菜单。
- **日志**：应用启动时调用 core 的 `infrastructure::observability::tracing::init`（`crates/gitwave/src/logging.rs`），把 JSON 日志写进平台的数据目录，每天一个文件，保留 30 天。退出时先停掉写线程，把缓冲里的日志写完；日志目录建不出来时回落到 stderr。
- **动作**：应用级动作用 `App::on_action` 注册在应用上，不挂在视图的元素上。对话框会拿走焦点、绘制在视图之外，挂在元素上的动作在对话框打开时收不到。处理函数里要操作窗口的，等这次派发结束再更新窗口。

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
