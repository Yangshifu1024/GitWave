# fix-image-diff-unstaged · review

审查对象：分支 `fix/image-diff-unstaged`（对应 [plan.md](./plan.md)），维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践。

## 结论

无 🔴 Critical。1 处 🟡 Major（注释一致性，已在本分支修正），4 处 🟢 Minor（既有行为或可后续处理）。**可以合并。**

## 🟡 Major（已修正）

- **M1 同一事实链上的两处旧注释仍写「workdir 侧无 OID」**：`src/lib/api.ts` `getImageContent` doc 与 `src-tauri/src/application/use_cases.rs` `get_image_content` doc。该错误认知正是本 bug 的认知根源，plan.md 勘误节已指出，但初次改动漏掉了这两处。已随本分支更正为「unstaged delta 的 new OID 是未入库的幽灵哈希，不可查询」。

## 🟢 Minor（不阻塞）

- **m1** `MAX_IMAGE_BYTES`（20 MiB）检查发生在 `fs::read` 全量读入之后，工作区侧大图先占内存再被拒（use_cases.rs）。既有行为，非本次引入；可后续对 `None` 分支加 `fs::metadata` 预检。
- **m2** 前端 `staged === false ? null : new_sha` 与 `retry: false` 无组件测试锁定（仓库无 DiffViewer 组件测试惯例），靠后端幽灵 OID 不变量测试 + 手动 GUI 验证兜底。
- **m3** `hasNew` 依赖可选 `workdirKind`：未来调用方若传 `workdir` 漏传 `workdirKind`，已删除文件会快速失败为「无法显示」而非 Deleted 占位。既有结构问题。
- **m4** 新测试 panic 路径不执行 `cleanup`，与同文件既有测试风格一致，影响可忽略。

## 重点场景核查

- **`staged === false` 判定完备**：workdir 模式条目 `staged` 恒为 `Some(bool)`（`get_workdir_diff` 分别 `mark_staged`）；commit 模式为 `None`，严格 `=== false` 不会误伤。旧侧 `old_sha` 永远不是幽灵 OID。
- **`retry: false` 不影响重取语义**：仅关闭单次挂载内的失败重试循环；`staleTime: 0` 的 refetchOnMount / focus 重取不变。代价是暂态错误（如杀软短暂锁文件）立即呈现错误态，属 plan.md 明确取舍。
- **回归测试稳固**：characterization test，断言 libgit2 多年稳定的行为；若未来 libgit2 改为真正入库或不哈希，测试会响亮失败并强制重审。
- **边界**：diff 计算后文件被删 → `FS_ERROR` 立即呈现；>20 MiB → 「过大」占位；rename 被拆 delete+add（diff 未开 `find_similar`），新路径按 ADDED 读工作区，正确。
- **安全**：worktree 读取已有 `ensure_path_in_workdir` 防穿越，路径全部来自 libgit2 delta，未扩大攻击面。

## 验证记录

`make check` 全绿（prettier + cargo fmt、eslint + clippy + typecheck、前端 181 tests、后端 349 tests 含新增 `unstaged_workdir_new_sha_is_a_ghost_oid_not_in_odb`）。
