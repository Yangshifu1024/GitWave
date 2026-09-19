# feat: stash 面板 UX 改造（可发现性 + 内容可查看 + 操作安全）

## 一、问题（用户的原始反馈 → 核实后的问题）

用户反馈三件事：**① 操作不流畅；② 3 个按钮没有 tooltip、新用户不知道如何使用；③ stash 内的改动无法查看和操作。**

核实后的事实（文件:行号级证据）：

1. 侧栏 `StashPanel` 每条 stash 的 3 个操作按钮是 **12px 纯图标**、只有原生 `title`、**无 `aria-label`**，且 **drop 无二次确认**（破坏且不可恢复）——`StashPanel.tsx`。
2. **查看入口在生产中不存在**：`Eye`（查看 diff）按钮被 `!compact` 条件挡住，而唯一调用点是 `App.tsx` 的 `<StashPanel compact />`；`compact=false` 的预览分支是 dead code，且只渲染「文件名 + 计数」，不渲染 hunks。
3. **查看能力虽有 API，但结果不完整**：`cmd_get_stash_diff` = `diff_commit_vs_parent`（只 diff **第一父提交**），而 `git stash -u`（GitWave 默认，`save_stash` 唯一 flag 是 `INCLUDE_UNTRACKED`）把未跟踪文件放在 stash 的**第 3 个父提交**（无父提交）。实测（临时仓库，已记录原始输出）：

   ```
   stash commit parents = <HEAD> <index-commit> <untracked-commit>
   git diff stash@{0}^ stash@{0} --name-status   →  A staged-new.txt / M tracked.txt   ← 未 add 的新文件缺失
   git ls-tree -r --name-only stash@{0}^3        →  brand-new.txt
   git stash show -u --name-status stash@{0}     →  A brand-new.txt / A staged-new.txt / M tracked.txt
   ```

   后果：新建但未 `git add` 的文件在「查看 stash 内容」里**完全不可见**，而 `-u` 会 clean 掉工作区里这些文件，用户无法自行核对；反之若用户 `git add` 过，它又会以 `A` 出现 —— 同一功能的可见性随 staged 与否跳变。

## 二、已确认的设计决策（需求评审结论）

| 决策点 | 结论 |
|---|---|
| 查看形态 | 弹窗：左「文件列表」+ 右「DiffViewer」（照 `WorkingCopyModal` 范式） |
| 查看触发 | 点整行打开详情 **+** 操作行含「查看」按钮 |
| 操作粒度 | 仅整体 apply / pop / drop（不做单文件、不做 `stash branch`、不做部分 pop） |
| 按钮布局 | **操作换独立一行、常驻**（每条 stash 两行：消息行 + 操作行） |
| 按钮形态 | **文字按钮**：查看 / 应用 / 应用并删除 / 删除 |
| apply / pop | **脏工作区预检**：有未提交改动时提示数量，可继续或取消 |
| drop | **富内容二次确认**：消息 + 序号 + 文件数与文件名摘录 + 「删除后无法通过 UI 恢复」 |
| pop / drop 成功后 | **自动关闭详情弹窗** + 状态区提示成功 |
| 未跟踪文件 | 后端修复后出现在结果里，并在文件列表标「未跟踪」徽标 / `?` 状态图标 |
| tooltip | 用项目既有 `ui/Tooltip`，并补齐 `aria-label`（修掉无障碍缺口） |
| `include_untracked` | 后端参数默认 true，**不暴露 UI 开关**（参数留给 API/测试） |

**非目标**：① 单文件粒度操作；② 从 stash 创建分支；③ stash 列表字段扩展（时间 / 来源分支 / 文件数）；④ save-stash 流程与文案改造。

## 三、实现（文件级改动）

### 后端（Rust）
- `src-tauri/src/domain/diff.rs`：`FileDiff` 新增 `untracked: Option<bool>`，serde `skip_serializing_if = "Option::is_none"`（照 `staged` 先例），向后兼容（缺字段的旧 payload 仍可反序列化）。
- `src-tauri/src/infrastructure/git/diff.rs`：新增 `diff_tree_vs_empty(repo, tree)`（`diff_tree_to_tree(None, Some(tree))`，即空树 → tree，全新增且带完整 hunks，复用既有 `diff_to_files`）；`DiffSummary` 新增 `merge_dedup_by_path`（第一父侧优先，重算 totals）。
- `src-tauri/src/infrastructure/git/stash.rs`：`stash_diff(repo, oid, include_untracked)` —— 基础仍是 `diff_commit_vs_parent`；`include_untracked == true` 且 `parent_count() >= 3` 时叠加 `stash^3` 的 diff 并逐条标记 `untracked = Some(true)`，再按 path 去重；`< 3` 父短路（输出与修复前完全一致）。**未改动** `diff_commit_vs_parent`（提交详情 / AI explain 复用其语义）。
- `src-tauri/src/application/use_cases.rs` + `src-tauri/src/lib.rs`：`cmd_get_stash_diff` / use case 新增 `include_untracked: Option<bool>`，`unwrap_or(true)`（与 `cmd_save_stash` 同口径）。

### 前端（TS / React）
- `src/lib/api.ts`：`FileDiff` 新增 `untracked?: boolean | null`；`getStashDiff(workspaceId, oid, includeUntracked?)`。
- `src/components/DiffViewer.tsx`：导出 `DiffViewerProps`；新增 `stashOid?` 数据源分支（stash > workdir > commitOid），stash 空态文案；既有 workdir / commit 路径零改动。
- `src/components/StashDetailModal.tsx`（新增）：`Modal size="xl"`，左文件列表（`DiffSummary.files` 驱动，含 `?` 状态图标与「未跟踪」徽标）+ 右 `DiffViewer`（`stashOid` + 选中 `path` + `hideMaximize`），底部 应用 / 应用并删除 / 删除。
- `src/components/StashPanel.tsx`（改造主体）：每条 stash 两行——第一行 `ListItem`（点开详情），第二行**独立**操作行（4 个文字按钮，`Tooltip` + `aria-label`，`busy` 时 disabled）；删除 `compact` 预览分支与 `Eye` 按钮；新增脏工作区预检框、drop 富内容确认框、详情弹窗；apply / pop / drop 整段为不可重入临界区；成功走状态区提示。
- `src/App.tsx`：`<StashPanel compact />` → `<StashPanel />`。
- `src/lib/stashActions.ts`（新增，纯函数）：`fileDiffKind`（`untracked` → `"untracked"`；`old_sha === null` → added；`new_sha === null` → deleted；否则 modified）、`stashFileSummary`（文件名摘录）、`stashLabel` / `stashTitle`。
- i18n：`src/i18n/locales/{zh-CN,en}/changes.json` 的 `stash` 段重写（apply / 「应用并删除」/ 删除 的新措辞 + tooltip + 预检 + drop 确认 + 成功提示 + 未跟踪徽标），带 `{{count}}` 的文案按仓库惯例拆 `_one` / `_other`；清理不再引用的旧 key（`viewDiff` / `selectToPreview`）。

## 四、验证

- `cargo fmt -- --check` / `cargo clippy --all-targets -- -D warnings` / `cargo test --all-targets`：全绿（364 passed / 2 ignored，其中新增 9 个用例）。
- `pnpm typecheck` / `pnpm lint` / `prettier --check` / `pnpm test`：全绿（216 passed，29 files）。
- 后端新增用例覆盖：① `-u` stash 的未跟踪文件被包含且 `untracked == Some(true)`、hunk 内容逐行断言；② `include_untracked = false` 对照（不含）；③ 2 父 stash 短路（与修复前逐项相等）；④ 同 path 去重（不重复、不 panic、totals 正确）；另加空 tree 与 serde 向后兼容用例。
- 前端新增用例：`src/lib/stashActions.test.ts`（kind 推导含 untracked / 摘要截断 / 空输入）。
- 端到端语义用**真实 git 命令**独立复现过（见「一、问题」中的实测输出）。
- code-reviewer 对照本方案逐条出对齐表：规格 15 项全部达成，无 🔴，结论「通过」。
- 人工验证清单（前端无组件测试基建，需手点）：两行布局与文字按钮可见性、tooltip、点整行与「查看」都能开弹窗、弹窗左列表切换文件、`-u` stash 的未跟踪文件出现「未跟踪」徽标、脏工作区 apply 的预检框、drop 的富内容确认框、pop/drop 后弹窗自动关闭、空 stash 列表时侧栏收起、切换 repo 时弹窗与确认框被清空。

## 五、分支

`feature/stash-panel-ux`（基线 main）
