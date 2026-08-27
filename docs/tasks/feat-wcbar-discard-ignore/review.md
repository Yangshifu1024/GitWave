# feat-wcbar-discard-ignore · Code Review 报告

> 审查日期：2026-08-27
> 审查对象：工作区未提交改动（feature/ui-native-studio-v2），对应 [plan.md](./plan.md)
> 审查维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践
> 结论先行：**需修复（1 项 🔴：`ignore_path` 读失败时整文件覆盖 `.gitignore`）**；该项为 ~5 行小改，修复后可直接合入。另有 6 条 🟡 与若干 🟢 不阻塞。

---

## 1. 审查范围与方法

### 1.1 改动文件（本任务归属）

| 文件 | 类型 | 内容 |
|---|---|---|
| `src-tauri/src/infrastructure/git/working_copy.rs` | 修改 | 新增 `discard_worktree_changes`（index 判 tracked；tracked 走 `checkout_index(force, pathspecs, update_index(false))`；untracked `remove_file`）与 `ignore_path`（幂等 append）；新增 4 个测试 |
| `src-tauri/src/application/use_cases.rs` | 修改 | use case `discard_changes` / `ignore_path`（`active_repo_path + open_repo` 范式）+ alias 导入 |
| `src-tauri/src/application/mod.rs` | 修改 | re-export 链 |
| `src-tauri/src/lib.rs` | 修改 | import、`cmd_discard_changes` / `cmd_ignore_path`、`generate_handler` 注册 |
| `src/lib/api.ts` | 修改 | `discardChanges` / `ignorePath` invoke 封装 |
| `src/hooks/useWorkingCopy.ts` | 修改 | `discard(paths)` / `ignore(pattern)` mutations |
| `src/lib/ignorePattern.ts`(+`.test.ts`) | 新增 | 纯函数 `deriveIgnorePatterns` + 4 个测试 |
| `src/components/ChangesPanel.tsx` | 修改 | FileSection 增 `onDiscardFile/onIgnoreFile`、行外包 ContextMenu、`pendingAction` state、`DiscardConfirmModal` / `IgnoreScopeModal` |
| `docs/design/04-working-copy.md` | 修改 | 右键菜单规格增补 |

### 1.2 同工作区的非本任务改动

当前 working tree 还混有 **feat-wcbar-controls** 的遗留改动（`WorkingCopyBar.tsx` 折叠/最大化、`layoutStore.ts`、`App.tsx` reset effect、`layoutStore.test.ts`），该任务已有独立 [review](../feat-wcbar-controls/review.md)。本报告不重复审它，但见 §7 流程建议。

### 1.3 方法

- 全量阅读 diff 与关联组件源码（`ContextMenu.tsx` / `Modal.tsx` / `FileListItem.tsx` / `Button.tsx`）、`test_helpers.rs`、radix `@radix-ui/react-menu` 打包产物。
- 本机复跑全部校验（结果 §8）。
- 对两处不确定语义做了**一次性实证探针**（/tmp 独立 crate，git2 0.20.4 vendored，未触碰工作区源码，已清理）：
  - merge 冲突态在 statuses 中是否携带 WT_\* 标志 → 结论：**只报 `CONFLICTED`**，即使冲突后再次编辑或删除该文件也一样；stage 0 缺失而 stage 1/2/3 存在；
  - 该实验同时证实 `index.get_path(path, 0)` 在冲突路径上返回 `None`（详见正确性 R3 分析）。

---

## 2. 正确性

### R1 · `checkout_index` 语义核对 —— 通过 ✅

`git restore <path>` 的默认 restore source 即 index（[git-restore 文档](https://git-scm.com/docs/git-restore)："--source… 默认对 worktree 模式是 index"）。实现用 `repo.index()` 现场加载 + `checkout_index(Some(&mut index), force + update_index(false))`：

- **modified**：force 覆盖 worktree 为 index 版本 ✅
- **deleted-in-worktree（WT_DELETED）**：index 中仍有该条目，force 会重建 ✅（有测试覆盖）
- **部分暂存**（index ≠ HEAD）：恢复自 index 而非 HEAD，staged 内容保留 ✅——这正是 plan 决策记录的意图。但注意现有测试 `discard_restores_modified_file_from_index` 场景中 index 内容恰等于 HEAD 内容（`build_linear_repo` 提交后未再 stage），无法区分"从 index 恢复"和"从 HEAD 恢复"，断言强度不足 → 见 T2。
- **force + update_index(false)** 组合不会误伤 index：[`GIT_CHECKOUT_DONT_UPDATE_INDEX`](https://libgit2.org/libgit2/#HEAD/type/git_checkout_options) 仅阻止把 checkout 产生的 stat 信息回写 index；restore 后内容一致，下次 status 靠内容比较收敛，无需担心脏标记。✅

### R2 · untracked 只会是文件路径 —— 通过 ✅

status 用了 `recurse_untracked_dirs(true)`（[working_copy.rs L26-L27](../../../src-tauri/src/infrastructure/git/working_copy.rs)），目录型 untracked 已展开为文件级路径，`remove_file` 分支成立。若将来收到目录路径，`remove_file` 返回 io 错误而非静默吞掉，行为安全。

### R3 · merge 冲突文件的 Discard 风险 —— 当前不可达 ✅（附加固建议 D2）

担心的场景：冲突文件 stage 0 条目已被移除（探针实测确认），若它以 unstaged 行出现在列表，Discard 会走 `remove_file` 分支把 tracked 冲突文件整个从磁盘删掉（比预期破坏更大）。**实证结论**：libgit2 在纯冲突态、以及冲突后编辑/删除该文件的状态下，statuses 都只报 `Status(CONFLICTED)`，不带任何 `WT_MODIFIED/WT_DELETED/WT_NEW` 位，因此 `worktree_kind()` 返回 None、不会生成 unstaged 行，菜单不可达。风险仅在上游行为变更时暴露 → 归入可维护性的预防性建议 D2。

### R4 · bare repo / 无 workdir —— 通过 ✅

两个函数都显式 `workdir().ok_or_else(AppError::Protocol)`，bare repo 下不会 panic。

### R5 · 🔴 `ignore_path` 读失败被当作空文件，可能整份覆盖用户的 `.gitignore`

[working_copy.rs L287](../../../src-tauri/src/infrastructure/git/working_copy.rs)：

```rust
let existing = std::fs::read_to_string(&gitignore).unwrap_or_default();
```

注释假设"缺文件视为空"，但 `unwrap_or_default()` 把**一切读取失败**都压成空串。最现实的一类失败：`.gitignore` 含**非 UTF-8 字节**（中文环境下 GBK 编码注释并不罕见），`read_to_string` 返回 `InvalidData` 错误。随后逻辑认为文件为空，`fs::write` 直接**静默清空并重写整份 `.gitignore`**，用户既有规则全部丢失且不可恢复（.gitignore 通常未被 track）。权限错误同理。

这正是"confuse 缺失与失败"的经典数据丢失模式，而同函数族里 `discard_worktree_changes` 已经示范了正确写法（匹配 `ErrorKind::NotFound` 才当不存在）。修复建议（~5 行）：

```rust
let existing = match std::fs::read_to_string(&gitignore) {
    Ok(s) => s,
    Err(e) if e.kind() == std::io::ErrorKind::NotFound => String::new(),
    Err(e) => return Err(AppError::Unknown(format!("fs: read .gitignore: {e}"))),
};
```

（可选加固：非 UTF-8 时拒绝写入并提示用户手动处理，避免写合法 UTF-8 内容进原本非 UTF-8 的文件造成编码混排。）

### R6 · 路径注入面 —— 正常流安全，后端校验缺失 🟡（S1 详述）

`workdir.join(path)` 存在两个经典陷阱：(a) Rust 的 `PathBuf::join` 遇到绝对路径会**整体替换 base**；(b) 相对路径含 `..` 可逃逸 workdir。正常流程下 path 来自 `entry.path()` 回传，无用户自由输入入口，所以不是实际漏洞，但这是 Tauri IPC 可直达的命令，纵深防御很便宜 → 见安全 S1。

### R7 · 幂等与 EOL

- 幂等：逐行精确比对，重复 append 被挡住 ✅（有测试）。CRLF 文件也不受影响——`str::lines()` 会剥掉 `\r`，比对仍相等 ✅。
- 🟢 追加时统一用 `\n`，在既有 CRLF 文件里产生混合换行；🟢 带 BOM 的首行不参与精确匹配，理论上会多追加一行。均为外观问题。
- 🟢 校验只拒 `\n` 未拒 `\r`；前端派生的受控输入下无可利用性。

### R8 · React 侧正确性 —— 通过 ✅

- **radix disabled 不触发 onSelect**：已核实 [`@radix-ui/react-menu` 源码](https://github.com/radix-ui/primitives/blob/main/packages/react/menu/src/Menu.tsx) 中 `MenuItem.handleSelect` 开头即 `if (!disabled && menuItem)` 早退，pointer click 与 SELECTION_KEYS 两条路径都不会派发 `ITEM_SELECT`；组件 CSS 又叠加 `data-[disabled]:pointer-events-none` 双保险。ChangesPanel 里的 `onSelect={() => file.kind !== "renamed" && …}` 属于防御性冗余，无竞态风险。
- **ContextMenuTrigger asChild 包 role="option" 行**：radix 经 Slot 合并 props 到子元素，不产生额外 DOM，listbox → option 结构保持不变（容器确有 `role="listbox"`，[ChangesPanel.tsx](../../../src/components/ChangesPanel.tsx) sectionOpen 分支）；trigger 只附加 `aria-haspopup` / `aria-expanded` / `data-state`，均为全局允许属性。左键打开 diff 与右键呼出菜单事件不冲突（浏览器右键不发 click）。
- **modal 受控模式**：`open` 恒真 + `onOpenChange={(open) => !open && onCancel()}` 是条件渲染下的标准做法，Esc / 点击遮罩 / 关闭按钮都会走 onCancel 清理 state；radix Dialog 自带焦点圈禁。
- **`IgnoreScopeModal` 的 radio 初值**：`useState(patterns.full)` 且组件随 `pendingAction` 卸载重建，两次打开之间选择状态正确复位。
- 唯一小口子见 M3（快照过期，影响很小）。

---

## 3. 安全

### S1 · 🟡 IPC 命令缺少路径边界校验（纵深防御）

`cmd_discard_changes` 最终触达 `std::fs::remove_file(workdir.join(path))`。建议在后端加一道廉价闸门：

```rust
let abs = workdir.join(path);
if !abs.starts_with(workdir) { // covers 绝对路径替换 base 与 ../ 逃逸
    return Err(AppError::Protocol(format!("path escapes repository: {path}")));
}
```

威胁模型是渲染进程被攻破或开发期误调用时，禁止删除仓库外任意文件；与 P1"隐私可控"的产品原则同向。`ignore_path` 写固定文件名 `.gitignore`，不受此影响。

### S2 · 破坏性操作防护链完整 ✅

菜单 `destructive` 红 + `Trash2` 图标 + `…` 结尾约定 → danger 样式确认 Modal → untracked 文案明确"永久删除文件"、按钮文案区分 `Delete file` / `Discard`。符合 AGENTS 的 P1 约束：全链路无自动 commit / push / merge（`discard` 只动 worktree，`update_index(false)` 连 stat 回写都不做；`ignore` 只写文件）。✅

### S3 · 🟡 允许 Ignore `.gitignore` 自身

`.gitignore` 首次生成后自身就是 untracked 行，用户可在它上面选 "Add to .gitignore…" 并把 `.gitignore` 加进自己——这会让所有后续规则对协作者失效（永远不被 pull 下发），大概率违背本意。建议 `onIgnoreFile` 对 `path == ".gitignore"`（含嵌套层级）置灰该项并在 title 说明。

---

## 4. 性能

无问题 ✅。`checkout_index` 被 pathspec 限定在少量路径上；`ignore_path` 单文件读写 O(size(.gitignore))；mutations 复用既有的 2s 轮询 + invalidate 架构，没有新增轮询或 N+1 调用。`deriveIgnorePatterns` 为 O(路径长度) 纯函数。数据量级（工作区文件列表）远不足以构成瓶颈。

## 5. 可维护性

### D1 · 注册链完整性 —— 通过 ✅

对照 `stage/unstage` 先例逐步核验 6 步接线：infra 函数定义 → `use_cases.rs` 包装（alias 导入）→ `application/mod.rs` re-export（字母序未破坏：`delete_workspace, discard_changes` / `get_workspace, ignore_path, init_repo` 均有序）→ `lib.rs` import → `#[tauri::command]` 定义 → `generate_handler!` 注册 → `api.ts` 的 `"cmd_discard_changes"` / `"cmd_ignore_path"` 与 camelCase 参数（Tauri 2 自动映射 snake_case）一一对应。**零遗漏**。

### D2 · 🟡 冲突分支的预防性护栏

R3 已实证当前不可达，但 `get_path(stage 0) → None` 就删盘的逻辑对上游行为变化过于敏感。低成本护栏：untracked 分支先查 `index.get_path(path, 1/2/3)` 任一存在则报 Protocol 错误跳过删除。放到现在做是三行代码，等出了事再定位代价极高。

### D3 · 🟡 快照型 pendingAction（React）

`pendingAction.file` 是点击菜单瞬间的快照，而 working-copy query 以 2s 轮询刷新；极端情况下（外部 git 进程并行改动）确认时磁盘状态可能与弹窗描述不符。操作按 path 执行、radix Dialog 又圈禁了应用内交互，实际窗口很小。可选守卫：confirm 时从最新 `unstagedFiles` 里按 path 重取 kind 再执行，不一致则二次确认。

### 其余

- `canDiscard`/`canIgnore` 的条件在 JSX 内联 onSelect 里又算了一遍——保留作防御冗余可接受，🟢 若抽成局部函数可读性更好一点。
- layout / store / API 各层职责清晰，未发现隐藏耦合。

## 6. 可读性

✅ 整体优秀。doc comments 把语义（≡ `git restore`、幂等、`.gitignore` 只影响 untracked）写在实现头部；确认弹窗文案区分两种语义且按钮差异化；`deriveIgnorePatterns` 注释点明 dotfile 边界并有对应测试；新增代码风格与既有 `stage_paths/unstage_paths` 完全一致。cargo fmt / prettier 全过。

🟢 `DiscardConfirmModal` description 里 "restored to the last staged version" 对未暂存任何东西的普通 modified 文件略有歧义（其实是"上次 stage 过（=HEAD）的版本"），换成 "the version git has recorded" 类措辞更准，可不改。

## 7. 测试覆盖

### T1 · ✅ 已覆盖

- Rust 4 个新测试（plan 写 5 个，实数 4 个：modified 还原 / untracked 删除 / 删除态重建 / ignore 生效+幂等+status 清空，幂等并入最后一个）断言均落实到具体文件系统或 status 结果，不是空转；本机复跑全绿。
- `deriveIgnorePatterns` 4 例覆盖嵌套、根目录无 dir、无扩展名、dotfile 两类特判（含 plan 关注的 `.env.local → *.local`）。`.env.local` 映射到 `*.local` 是有信息预览兜底的合理取舍（radio 选中即可见 code 预览），✅。

### T2 · 🟡 补"index ≠ HEAD"的部分暂存测试

`discard_restores_modified_file_from_index` 中 index 与 HEAD 相同（§2 R1），证明力弱。建议补一例：commit v1 → 改文件 → `stage_paths` 得到 staged v2 → 再改 worktree 成 v3 → discard → 断言 worktree == v2 且 staged 行仍在。这一例同时锁死"不伤 index"这条最关键的回归线（update_index(false) 失效会被它当场抓住）。

### T3 · 🟢 其他值得补的（不阻塞）

- 混合批次一次调用（tracked + untracked 同传）；
- `ignore_path` 追加到**末行无换行**的既有 `.gitignore`（目前实现对 is_empty 判定，非空无 `\n` 时会补 `\n`，但没有断言）；
- `..` / 绝对路径被拒（依赖 S1 落地）。

UI 层交互无 jsdom 设施不建自动化沿用项目现状，plan 已如实记录盲区，接受。

### 附注

计划与报告数字出入：plan 称 "5 个测试"，working_copy.rs 实际新增 4 个（+deriveIgnorePatterns 前端 4 个）。不影响评审，但建议 plan 更正或拆分表述，避免后续追溯困惑。🟢

## 8. 最佳实践与自动化验证

### 流程 🟡

AGENTS 要求"每个 PR 关联一个 task"。当前 working tree 同时包含本任务与已审完的 feat-wcbar-controls 改动。**Commit 时请分两批**（`feat(wcbar-controls): …` 与 `feat(wcbar-discard-ignore): …`）或开两个 PR，保住 task↔PR 一一对应；文档侧 `docs/design/04-working-copy.md` 一段属于本任务、一段属于 controls 任务，注意随之拆分。

### 本机复跑结果（2026-08-27，arm64 macOS）

| 检查 | 结果 |
|---|---|
| `cargo fmt --check`（src-tauri） | 通过（exit 0） |
| `cargo clippy --all-targets` | 通过（exit 0，无 warning） |
| `cargo test` | **109 passed**, 0 failed（含新增 4 例） |
| `tsc --noEmit` | 通过（exit 0） |
| `vitest run` | 6 files / **34 tests 全过**（含 ignorePattern 4 例） |
| `eslint .` | 0 error / 0 warning |
| `prettier --check`（本任务改动文件） | 通过 |

### 最佳实践 ✅

错误映射统一走 `map_git_err` / `AppError::Protocol/Unknown`；不加锁直接 std::fs 与既有 stage/unstage 同层；外部资料引用与决策记录齐全；命名/排序遵循模块惯例；未发现 magic number 或越层调用。

---

## 9. 发现汇总

| 编号 | 维度 | 级别 | 位置 | 摘要 |
|---|---|---|---|---|
| R5 | 正确性 | 🔴 | `working_copy.rs` `ignore_path` | 读失败（非 UTF-8/IO 错误）被当作空文件 → 整份覆盖 `.gitignore`，静默毁损用户规则 |
| S1 | 安全 | 🟡 | `working_copy.rs` `discard_worktree_changes` | IPC 入口无路径边界校验（绝对路径替换 join base / `..` 逃逸），建议 `starts_with(workdir)` 闸门 |
| S3 | 安全 | 🟡 | `ChangesPanel.tsx` | `.gitignore` 自身可被 Ignore 自指，隐性毁掉后续规则下发 |
| D2 | 可维护性 | 🟡 | `working_copy.rs` | 冲突路径护栏缺失（今日实证不可达，属预防性三行加固） |
| D1/T2 | 测试 | 🟡 | `working_copy.rs` tests | 缺 index ≠ HEAD 部分暂存用例，现有断言区分不出 index/HEAD 恢复源 |
| M3 | 可维护性 | 🟡 | `ChangesPanel.tsx` | 2s 轮询下 pendingAction 快照可能过期（低危，可选守卫） |
| S1b | 正确性 | 🟡 | `working_copy.rs` | checkout pathspec 有 glob/前缀语义，建议 `disable_pathspec_match(true)` 精确匹配（破坏上限仅为多余 restore，故并列次要） |
| 流程 | 最佳实践 | 🟡 | 工作区 | 与 feat-wcbar-controls 改动混合，需分批 commit / 分 PR |
| R7/E1/R8备注 | 正确性/可读性 | 🟢 | 多处 | 混合 EOL、BOM 首行不参与幂等比对、`\r` 未过滤、`last staged version` 措辞、手动 radiogroup 无方向键导航（可换原生 radio）、`canDiscard` 条件复算、测试计数与 plan 出入 |

> 补充说明（事实核查记录，非缺陷）：① radixed disabled item 不派发 select 已读其打包源码证实，JSX 内联守卫是纯冗余；② 冲突文件仅报 CONFLICTED 已用独立 crate 实测（合并/编辑/删除三态一致），Discard 误删冲突文件在当前版本链不可达；③ `git restore` 默认源为 index，与本实现语义一致（https://git-scm.com/docs/git-restore ；libgit2 checkout 选项见 https://libgit2.org/libgit2/#HEAD/type/git_checkout_t ）。

## 10. 结论

**需修复**：合入前处理 🔴-R5（必改，约 5 行）。强烈建议顺手带上 S1（同样几行、价值高）与 S3/D2/T2（小成本高回报）。以上完成后无需再审一轮，diff 级复核即可放行，最终评级「可以合入」。
