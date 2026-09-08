# fix-repo-owner-validation · Review

> 审查对象：`src-tauri/src/lib.rs` 启动时禁用 libgit2 owner 校验；分支 `fix/repo-owner-validation`。
> 审查维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践。

## ✅ 优点

- **根因诊断准确、修复最小化**：owner 校验是 libgit2 进程级全局开关（vendored libgit2 `repository.c` 的 `git_repository__validate_ownership = true`），启动时一次关闭是正确做法。
- **放置时机正确**：`main.rs` 唯一入口是 `run()`；进程内所有 `Repository::open`（`use_cases.rs:153`、`git2_adapter.rs:15`、submodule/worktree）都发生在 `tauri::Builder` 起线程之后，该调用在其之前、无并发 libgit2 操作，满足 `git_libgit2_opts` 线程约束。`Repository::init_opts` 不做 owner 校验，clone 只打开自己刚创建的目录，失败点覆盖完整、无误伤。
- **unsafe 使用恰当**：unsafe 仅为 FFI 边界标记，调用本身无需调用方维护不变量。
- **`let _ = ` 可接受**：与既有 proxy bridge 容错风格一致。
- **文档规范**：`plan.md` 含根因、方案、否决备选、安全评估与实测验证。

## 🔴 严重问题（必须修复）

无。

## 🟡 一般问题（建议修复）→ 已全部修订

| # | 问题 | 修订 |
|---|---|---|
| 1 | 注释/plan.md 安全论断缺上游依据，且是版本性质而非不变量 | plan.md 补充 vendored libgit2 1.9 依据（无 hooks/fsmonitor/外部 filter）+ 升级重评估提示 + git2 上游警告 URL |
| 2 | "repo paths always user-picked" 过度概括（submodule/worktree 为派生路径） | 注释与 plan.md 改为"根路径用户显式选择；派生路径沿用父仓库信任决策" |
| 3 | 只提 Windows，实际开关跨平台生效 | 注释与 plan.md 均已注明跨平台语义（POSIX uid / Windows SID） |
| 4 | 失败语义描述不准确（git2 wrapper 实际无条件返回 Ok） | 注释改为"wrapper always returns Ok, failure is not observable" |

## 🟢 已采纳的优化建议

- 禁用成功后加 `info!("libgit2 owner validation disabled")`，便于用户环境排查。

## 🟢 未采纳（记录在案）

- 提取 `infrastructure::git::configure_libgit2()` 具名函数：当前 `run()` 内联与既有 proxy bridge 风格一致，改动收益低；未来若新增 libgit2 全局配置再提取。
- 行为级单元测试（仓库属另一账户）：Windows 需提权、CI 不可移植；以手动实测（`WaveStudioRev` 默认失败 → 禁用后成功，错误与用户截图一致）记录于 plan.md。

## 📝 总体评价

修复方案正确且最小，无严重问题；主要偏差集中在注释与文档准确性，已修订。**通过，可合入。**

## 验证记录

- `cargo test`（src-tauri）：294 passed / 0 failed / 2 ignored
- 手动端到端：`D:/Work/SideProjects/WaveStudioRev` 默认打开报 `repository path ... is not owned by current user; class=Config (7); code=Owner (-36)`（与用户截图一致）；`set_verify_owner_validation(false)` 后打开成功
