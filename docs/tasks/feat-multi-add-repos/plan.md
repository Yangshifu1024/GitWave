# feat-multi-add-repos · 「添加仓库」支持一次添加多个本地仓库

## 需求

「Repository → Add existing local repo」目前一次只能注册一个目录：`PathInput` 硬编码 `multiple: false`，`ActionBar` 只有单个 `localPath`，后端 `cmd_add_local_repo(path: String)` 单路径、且没有任何去重。需求：一次选择多个仓库目录并批量注册，已存在的跳过、无效的 best-effort 跳过并汇总。

## 决策

| 项 | 结论 |
|---|---|
| 交互 | 系统目录对话框多选；模态内列出已选路径（可逐个移除）后确认 |
| 重复路径 | 已在本 workspace / 批次内重复 → 跳过并汇总 |
| 无效路径 | best-effort：有效的照常添加，失败项列出不中断 |
| active 仓库 | 最后一个成功添加的成为 active |
| 范围 | 仅「添加本地仓库」，不改 init / clone / relink / import |
| 去重实现 | use case 内路径规范化比较，不改 schema（`repos` 无 UNIQUE 约束） |

## 方案

### 后端

1. **`src-tauri/src/domain/error.rs`**：`AppError` 增加 `Clone` derive（字段全为 `String` / `&'static str` / `Vec`），便于把失败错误放进返回结构。

2. **`src-tauri/src/domain/workspace.rs`**：新增
   ```rust
   pub struct AddRepoFailure { pub path: String, pub error: AppError }
   pub struct AddReposSummary {
       pub added: Vec<RepoRef>,
       pub skipped: Vec<String>,
       pub failed: Vec<AddRepoFailure>,
   }
   ```
   前端 `formatAppError(failure.error)` 复用现有 wire shape。

3. **`src-tauri/src/application/use_cases.rs`**
   - 私有 `normalize_repo_path(p: &str) -> String`：`std::fs::canonicalize` 成功用结果（Windows `cfg` 下小写化），失败回退原串；用于去重。
   - `pub fn add_local_repos(ctx, workspace_id: &str, paths: Vec<String>) -> Result<AddReposSummary>`：
     1. 读当前 workspace 已存仓库路径 → `HashSet<normalized>`；
     2. 遍历入参：已在集合 → `skipped`；`open_local` 失败 → `failed` 并继续；成功 → `RepoRef`（沿用 `add_local_repo` 字段，`new_repo_id()`）→ `add_repo`（`MAX(position)+1` 天然按顺序追加）→ `added`，并把规范化路径加入集合（批次内去重）；
     3. 返回汇总。
   - 既有 `add_local_repo` 保留（`WorktreePanel` 仍用）。

4. **`src-tauri/src/lib.rs`**：新增 `cmd_add_local_repos(ctx, workspace_id, paths: Vec<String>) -> Result<AddReposSummary, AppError>`（同步命令，与 `cmd_add_local_repo` 一致），注册进 invoke handler。

### 前端

5. **`src/lib/api.ts`**
   ```ts
   export interface AddRepoFailure { path: string; error: AppError }
   export interface AddReposSummary { added: RepoRef[]; skipped: string[]; failed: AddRepoFailure[] }
   export function addLocalRepos(workspaceId: string, paths: string[]): Promise<AddReposSummary>
   ```

6. **`src/components/ui/PathInput.tsx`**（向后兼容）：新增可选 `multiple?: boolean` 与 `onPickMany?: (paths: string[]) => void`；`browse()` 用 `multiple: multiple ?? false`，多选时把 `string[]` 交给 `onPickMany`，否则维持 `onChange`。其余调用点零改动。

7. **`src/lib/paths.ts`（新）**：纯函数 `mergeUniquePaths(existing: string[], incoming: string[]): string[]`（trim、丢空、精确去重，保序）+ 单测 `src/lib/paths.test.ts`。

8. **`src/components/ActionBar.tsx`**
   - 状态：保留 `localPath`（输入框文本），新增 `localPaths: string[]`（已选列表）。
   - Add-local 模态：`PathInput` 加 `multiple onPickMany` 追加到 `localPaths`；Enter 把当前输入追加进列表；下方渲染已选列表（逐项移除）+ 空态提示。
   - 主按钮 → `Add {{count}} repositories`，payload = `localPaths` 并入去重后的 `localPath`。
   - `localMut` → `addLocalRepos(ws, paths)`：
     - `added.length > 0`：`refreshRepos()` → `endAdd()` → `activateRepo(added.at(-1).id)`；有 skipped/failed 时状态区汇总（info）。
     - `added.length === 0`：保留模态，`actionError` 显示「全部已存在」或首个失败原因 + 计数。

### i18n（en + zh，parity 强制成对）

- `commits.repo`：更新 `addLocalTitle` / `addLocalDescription`；新增 `addLocalSelected`、`addLocalEmptyHint`、`addAllSkipped`、`addAllFailed`。
- `commits.action`：新增 `addRepositories`（`Add {{count}} repositories`）。
- `status`：新增 `repoAddedPartial`（`Added {{added}} repositories · skipped {{skipped}} · failed {{failed}}`）。

## 不做

- 不改 init / clone / relink / import；不动 `cmd_add_local_repo`；不加 DB UNIQUE 约束；不新增命令面板入口；不做父目录扫描。

## 验证清单

- [x] `pnpm format:check` · `pnpm lint`（0 errors） · `pnpm typecheck` · `pnpm test`（192 passed，含 `mergeUniquePaths` 测试、i18n parity）
- [x] `cargo fmt -- --check` · `cargo clippy --all-targets -- -D warnings` · `cargo test --all-targets`（355 passed）
  - 新增后端测试：全部有效顺序正确、已存在/批次内重复进 skipped、无效路径不中断且有效项照常 added
- [ ] 手动：选 2 个有效目录 + 1 个已存在 + 1 个非仓库目录 → 加入 2、跳过 1、失败 1，最后一个成为 active；重复选同一目录不重复入库
