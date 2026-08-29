# feat: Repository Tab 拖动排序

关联提案：[F005-repo-tab-drag-reorder](../../pm/features/F005-repo-tab-drag-reorder.md)

## 目标

Repository Tab（`WorkspaceRepoTabs`）支持按住拖动重排顺序，松手即生效并持久化；另提供右键菜单 Move Left / Move Right 作为非指针路径。

## 技术方案

### 顺序持久化（后端 SQLite，单一事实来源）

当前 `repos` 表无顺序字段，`list_repos` 按 `added_at` 排序（`workspace_repo.rs:213`）。引入显式 `position` 列，理由：tab 顺序是跨重启的用户数据，应与仓库列表同源存储；`list_repos` 的其他消费方（palette `use_cases.rs:1397`、active repo path `use_cases.rs:2136`）按 id 查找，顺序变化无影响。

- Migration 0003：`repos` 加 `position INTEGER NOT NULL DEFAULT 0`，按 `(added_at, id)` 回填——旧库初始顺序与现状一致，无行为变化
- `list_repos` → `ORDER BY position, added_at`
- `add_repo` → INSERT 时 `position = COALESCE(MAX(position), -1) + 1`（同 workspace 内追加末尾）
- 新 trait 方法 `reorder_repos(workspace_id, repo_ids)`：事务内校验 `repo_ids` 与该 workspace 现有集合完全一致（缺失 / 多余 / 跨 workspace → `Protocol` 错误），按索引写回 `position`
- `RepoRef` 领域结构不变：position 是存储细节，前端只依赖数组顺序

### 拖动交互（前端，不引入 dnd 依赖）

自研 pointer-events hook（项目无 dnd 库，tab 条场景足够小）：

- `pointerdown` 记录起点 + window 级 pointermove/up 监听；位移超过阈值 6px 才进入拖动态——与点击切换 tab、右键菜单互不干扰
- 拖动中：本地 `previewOrder` 实时预览（被拖 tab 半透明），按 id 对 query 数据排序渲染（refetch 安全，未知 id 追加末尾）
- 松手：乐观更新 `["repos", workspaceId]` 缓存 → invoke `cmd_reorder_repos` → 失败回滚 + `ErrorAlert`；拖动发生时抑制紧随的 click 切换；pointercancel 或拖回原位则放弃提交（`onAbort` 回滚 preview）
- 无障碍：右键菜单 Move Left / Move Right 复用同一命令，位于边界时禁用。missing（disabled）tab 因 HeroUI disabled 样式带 `pointer-events: none` 收不到 pointerdown，仅支持菜单重排、不支持拖动

### 修改点

| 层 | 文件 | 改动 |
|---|---|---|
| migration | `src-tauri/migrations/0003-repos-position.sql`（新） | 加列 + 回填 |
| migration | `src-tauri/src/infrastructure/persistence/migrations.rs` | 注册 version 3 |
| persistence | `src-tauri/src/infrastructure/persistence/workspace_repo.rs` | 排序 / position 追加 / `reorder_repos` + 测试 |
| use case | `src-tauri/src/application/use_cases.rs` + `application/mod.rs` | `reorder_repos` + 重导出 |
| command | `src-tauri/src/lib.rs` | `cmd_reorder_repos` + 注册 |
| api | `src/lib/api.ts` | `reorderRepos()` |
| hook | `src/hooks/useTabDragReorder.ts`（新） | 拖动状态机 + 纯函数 helpers |
| hook 测试 | `src/hooks/useTabDragReorder.test.ts`（新） | `arrayMove` / `computeTargetIndex` / `applyOrder` |
| ui | `src/components/ui/Tabs.tsx` | `TabsTrigger` passthrough 增加 `onPointerDown` |
| 组件 | `src/components/WorkspaceRepoTabs.tsx` | 集成 hook + 乐观提交 + 菜单项 |

### 不做（MVP 边界）

跨 Workspace 拖动、拖出新窗口、tab 条边缘自动滚动、触摸长按拖动、新增 dnd 第三方库。

## 验证

- `cargo test`（全量）：新增 persistence 测试（reorder 生效 / 集合校验 / 追加末尾 / 回填）
- `npm run typecheck && npm run lint && npm run test`
- 手动冒烟：拖动换位 → 重启 app 顺序保持；拖动后 active repo 选中态与 palette 不受影响
