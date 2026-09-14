# fix-image-diff-unstaged · plan

关联任务：[feat-image-diff](../feat-image-diff/plan.md)（F016 图片 diff）
分支：`fix/image-diff-unstaged`

## 现象

工作副本 diff 视图中查看未暂存（unstaged）的图片文件：旧版本正常或显示「新增」占位，新版本长时间停留在「加载中…」，约 7 秒后变成「无法显示该图片」。tracked 未暂存修改与 untracked 新增两种场景均复现。

## 根因

未暂存 diff（`diff_index_to_workdir`）中，libgit2 会对工作区文件内容计算哈希并写入 `delta.new_file().id()`，但该 blob **从未写入对象库（ODB）**——一个「幽灵 OID」。链路：

1. `src-tauri/src/infrastructure/git/diff.rs` `sha_opt` 把幽灵 OID 原样放进 `FileDiff.new_sha`；
2. `src/components/DiffViewer.tsx` `ImageDiffView`（workdir 模式）把 `new_sha` 当 OID 传给 `getImageContent`；
3. 后端 `read_file_content` 执行 `repo.find_blob(oid)` → `object not found`；
4. React Query 默认 retry 3 次（指数退避约 7 秒）→ 先「加载中…」后「无法显示该图片」。

用户截图中间接证实：文件头显示 `old_sha/new_sha` 各 7 位拼接，untracked 新增文件出现了 `0000000a4e68c8`（old 侧全零 + new 侧幽灵哈希）。

行为已用回归测试 `unstaged_workdir_new_sha_is_a_ghost_oid_not_in_odb` 锁定：tracked 未暂存修改与 untracked 新文件的 `new_sha` 均为 `Some(...)` 且 `find_blob` 失败。

可信侧不受影响：unstaged 旧侧 = index blob（`git add` 时入库）、staged 视图新侧 = index blob、commit diff 两侧 = tree blob，均在 ODB 中。

注意：`workdir` prop 在 `WorkingCopyModal` 中恒为 true（staged 视图也传），判定「未暂存侧」必须用 `fileDiff.staged === false`。

## 改动清单

| 文件 | 改动 |
|---|---|
| `src/components/DiffViewer.tsx` | `ImageDiffView` 新版本面板 `oid={fileDiff.staged === false ? null : fileDiff.new_sha}`（未暂存侧读工作区文件，后端 `fs::read` 路径已有实现与测试覆盖）；更正「git2 does not hash workdir content」错误注释；`ImageDiffPane` 的 useQuery 增加 `retry: false`，错误立即呈现，不再空转约 7 秒 |
| `src-tauri/src/infrastructure/git/diff.rs` | 更正 `read_file_content` doc 注释；新增回归测试 `unstaged_workdir_new_sha_is_a_ghost_oid_not_in_odb` |

## 关键实现决策

- **前端修，不动后端契约**：不在后端把未暂存 `new_sha` 置 None——该哈希对文件头摘要有信息价值，且 `staged === false` 在前端足以精确判定。
- **retry: false 只加在图片查询上**：图片版本缺失（文件被删、>20 MiB）属终态，重试无意义；其余查询保持全局默认。
- LFS 指针图片仍靠 `<img>` onError 兜底（F016 已列已知限制），不在本次范围。

## 勘误

[feat-image-diff/plan.md](../feat-image-diff/plan.md) 中「git2 不为 workdir 哈希（unstaged/untracked 新版本无 OID）」的假设与事实相反：libgit2 会哈希 workdir 内容，只是 blob 不入库、OID 不可查。原文档保留不改，以本文为准。

## 验证

- `cargo test --all-targets`（含新回归测试）、`cargo clippy --all-targets -- -D warnings`、`cargo fmt -- --check`
- `pnpm lint`、`pnpm typecheck`、`pnpm test`，最终 `make check` 全绿
- 手动（GUI）：未暂存修改的图片与 untracked 新增图片两侧均渲染；staged 视图不受影响；错误（如 >20 MiB）立即提示
