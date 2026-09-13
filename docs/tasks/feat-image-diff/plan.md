# feat-image-diff · plan

关联提案：[F016-image-diff](../../pm/features/F016-image-diff.md)
分支：`feature/image-diff`

## 需求

diff 视图中图片文件（png / jpg / jpeg / gif / webp / bmp / ico / svg，按扩展名识别）不再渲染文本 hunk / 空占位，改为左右分栏显示两个版本的图片：左 = 旧版本，右 = 新版本。单侧情形：新增文件只显示右栏、删除文件只显示左栏。覆盖 commit diff 视图与工作副本 diff 视图（共用 `FileDiffView` 渲染点）。

## 改动清单

### 后端（src-tauri）

| 文件 | 改动 |
|---|---|
| `Cargo.toml` | 新增 `base64 = "0.22"` 依赖 |
| `src/domain/diff.rs` | `ImageContent { base64: String, size: u64 }` |
| `src/infrastructure/git/diff.rs` | `read_file_content(repo, path, oid: Option<Oid>) -> Result<Vec<u8>>`：`Some(oid)` → `find_blob().content()`；`None` → `ensure_path_in_workdir` 防穿越（复用 worktree_guard，同 conflict.rs 用法）+ `fs::read` 工作目录文件；附单元测试 |
| `src/application/use_cases.rs` | `get_image_content(ctx, workspace_id, path, oid)`：解析活跃仓库 → `MAX_IMAGE_BYTES`（20 MiB）上限 → base64 编码 |
| `src/lib.rs` | `cmd_get_image_content` + `generate_handler!` 注册 |

数据来源关键点：commit diff 两侧均有 blob OID；staged diff 新版本有 index blob OID；工作副本 unstaged/untracked diff 的「新版本」无 OID（git2 不为 workdir 哈希），回退读工作目录文件。

### 前端

| 文件 | 改动 |
|---|---|
| `src/lib/api.ts` | `ImageContent` 接口、`getImageContent(workspaceId, path, oid?)` |
| `src/lib/diff.ts` | `isImagePath(path)`、`imageMimeFromPath(path)`（大小写不敏感），补 `diff.test.ts` 用例 |
| `src/components/DiffViewer.tsx` | `FileDiffView` 内 `isImagePath(fileDiff.path)` 时渲染新组件 `ImageDiffView`（文件头保留）；`ImageDiffView` 用 react-query 分别取旧/新版本（`old_sha`/`new_sha` 有 OID 走 OID，无 OID 且 workdir 模式走工作目录），`grid-cols-2` 分栏 + `object-contain` 限高，单侧居中空态 |
| `src/i18n/locales/{en,zh-CN}/diff.json` | `diff.image.*`：old / new / added / deleted / cannotDisplay / tooLarge（parity 强制） |

## 关键实现决策

- 图片字节走 base64 + `data:` URL：CSP 已允许 `img-src data:`，无需启用 asset protocol（避免扩大文件系统暴露面）。
- 20 MiB 上限在 use case 层拦截（超限报错），前端查询失败显示「图片过大」占位；`<img>` `onError` 兜底「无法显示」（覆盖 LFS 指针与损坏数据）。
- 按扩展名识别图片，不给 `FileDiff` 加 binary 标志，保持改动最小；非图片二进制维持现有占位。
- 图片按 OID 读取，rename 场景（`old_path` 丢失）旧版本仍可正确渲染。
- LFS 指针图片、stash 面板、图片对比滑块等增强交互不在本期范围。

## 验证

- `cargo test --manifest-path src-tauri/Cargo.toml --all-targets`（含新增读字节测试）与 `cargo clippy --all-targets -- -D warnings`
- `pnpm lint && pnpm test`（含 i18n parity、`diff.test.ts` 新用例）
- 手动：临时仓库提交 png → 修改 → commit 视图看双栏；工作副本改图 → workdir 视图；新增/删除图片 → 单栏空态
