# GitWave · 系统架构

> 与 `docs/tech/tech-selection/` 配套的总体架构视图。

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
| **application** | 用例编排、事务边界 | `CloneRepoUseCase`, `SwitchWorkspaceUseCase`, `GenerateCommitMessageUseCase` |
| **domain** | 核心模型与不变量 | `Workspace`, `RepoRef`, `CommitMessage`, `RebaseAction` |
| **infrastructure** | 外部能力适配 | `Git2RepoAdapter`, `SqliteWorkspaceStore`, `KeychainSecretStore`, `HttpAiProvider` |

依赖方向：`presentation → application → domain ← infrastructure`。domain 层零外部依赖。

## IPC 边界

- **Command**（request / response）：typed Rust function + typed params + typed result；用于一次性动作（如 clone、commit、switch workspace）
- **Event**（push）：后端 → 前端的流式 / 状态变更通知（如 clone 进度、git 操作状态、AI stream chunk）
- **序列化**：默认 JSON；未来若遇性能瓶颈可升级到 MessagePack / bincode
- **类型生成**：`tauri-specta` 自动生成 TS 端类型，前端可直接调用

## 性能热点处理

| 热点 | 策略 |
|---|---|
| history DAG（数万 commit） | virtual scroll + 视口窗口分片渲染；后端按需取 |
| 大文件 diff | 后端分块返回 + 前端 lazy 渲染；长行截断 + 折叠 |
| syntax highlight | 按需加载对应 grammar；diff 关闭时释放 worker |
| 后台 git 操作 | tokio task + 取消令牌；UI 主线程不等待；进度事件推送 |
| AI stream | SSE → Tauri event → 前端流式渲染；背压由 IPC 缓冲 |

## 多 Workspace 并行

- **单一 SQLite** + `workspace_id` 列区分数据
- **切换语义**：保存当前 React state snapshot + 重新加载 lastActiveRepo；切换过程不阻塞 UI（增量渲染）
- **单 active repo 模型**：UI 全局只渲染一个 repo 的主视图；侧边栏多 Workspace 平行排列
- **数据隔离**：每个 repo 的 per-repo 配置、AI 缓存、settings override 都以 `repo_id` 隔离

## Workspace 切换时序

```
用户点击 Workspace B
  ↓
保存 A 的 UI snapshot（Zustand persist）
  ↓
unload A 的 git watcher（释放订阅）
  ↓
load B 的 lastActiveRepo
  ↓
触发 React reconciliation（渐进）
  ↓
re-subscribe B 的 git watcher
```

## AI 调用时序

```
UI 触发 "生成 commit message"
  ↓
GenerateCommitMessageUseCase
  ↓
domain: 拼装最近 N 个 commit + 当前 diff
  ↓
infrastructure: scrubber 扫描 → 注入 prompt
  ↓
Provider::stream → SSE chunks
  ↓
Tauri event 推送 → 前端流式渲染
  ↓
用户编辑 / 确认 → 走确定性 commit 用例
```

## 后续 TODO（架构层面）

- 跨 Workspace 全局搜索的索引策略
- 大仓库（monorepo）的 lazy load 策略