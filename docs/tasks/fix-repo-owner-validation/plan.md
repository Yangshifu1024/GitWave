# fix-repo-owner-validation

> 添加现有本地仓库时报错：`repository path '...' is not owned by current user; class=Config (7); code=Owner (-36)`。

## 缺陷现象

「添加现有本地仓库」对话框中选择 `D:\Work\SideProjects\WaveStudioRev`，点击添加后报「打开仓库失败」。git CLI（`git status` 等）在该目录可正常工作。

## 根因（确定原因）

- libgit2 自 1.5 起默认开启仓库所有者校验（对应 git CLI 的 `safe.directory` 机制；POSIX 按 uid、Windows 按 SID）：仓库路径所有者必须是当前用户，否则返回 `GIT_EOWNER`（error code -36，class Config）。本缺陷在 Windows 上暴露。
- 实测：`D:\Work\SideProjects\WaveStudioRev` 所有者为 `BUILTIN\Administrators`，GitWave 运行用户为 `Yangzhenbiao`，触发校验失败。
- GitWave 使用 git2 0.20（vendored libgit2），添加仓库入口 `git2_adapter::open_local`（src-tauri/src/infrastructure/git/git2_adapter.rs:14）与日常操作入口 `AppContext::open_repo`（src-tauri/src/application/use_cases.rs:153）都受影响 —— 即使仓库添加成功，目录所有者变更后所有 git 操作同样会失败。
- 用户的仓库目录所有者可能因管理员权限 clone / 迁移 / 同步工具而变成 Administrators，这是 Windows 上常见状态，git CLI 用户通常以 `safe.directory` 解决。

## 方案

在 `lib.rs::run()` 启动时（任何 `Repository::open` 之前）调用进程级开关：

```rust
let _ = unsafe { git2::opts::set_verify_owner_validation(false) };
```

- 一次调用覆盖进程内所有 `Repository::open` 调用点，无需逐点修改。
- 失败仅发生在 libgit2 初始化异常时，此时后续打开操作也会失败，忽略返回值并保持 UI 可用，与既有 proxy bridge 的容错策略一致。

### 备选方案（否决）

| 方案 | 否决理由 |
|---|---|
| 写全局 `safe.directory`（`~/.gitconfig`） | 修改用户全局配置，影响其 git CLI 行为，与「隐私可控」原则冲突 |
| 逐调用点重试 + 信任对话框 | 交互成本高；libgit2 的 `safe.directory` 只读全局/系统配置，无法进程内按仓库白名单 |

### 安全评估

owner 校验防护的是多用户机器上他人在共享目录投放恶意仓库的场景。GitWave 的根仓库路径全部由用户通过系统文件夹选择器显式选择；派生路径（submodule、worktree）沿用父仓库的信任决策。当前 vendored libgit2 1.9 不执行 hooks、无 fsmonitor、无 config 驱动的外部 filter，禁用校验不引入 config 驱动的执行面；**该结论依赖 vendored libgit2 版本，升级 libgit2 时需重新评估**。

注意：git2 官方文档对禁用此校验有安全警告（https://docs.rs/git2/latest/git2/opts/fn.set_verify_owner_validation.html ）。该校验跨平台（POSIX 按 uid、Windows 按 SID），本修改同样关闭 macOS/Linux 上的校验（如 sudo 创建的 root-owned 仓库也可打开），方向上与桌面客户端威胁模型一致；Fork、Sublime Merge 等桌面 Git 客户端同样不强制此校验。

## 验证

- 添加所有者为 `BUILTIN\Administrators` 的仓库（如 `D:\Work\SideProjects\WaveStudioRev`）：添加成功，HEAD/分支正常显示
- 添加非 git 目录：仍报「不是 git 仓库」错误（NotFound 分支不受影响）
- `cargo test`（src-tauri）全量回归通过
