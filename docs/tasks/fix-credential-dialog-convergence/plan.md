# fix-credential-dialog-convergence · 修复方案

## 现象

Windows 上执行 HTTPS git 操作（fetch 等）时，弹出系统级 **Git Credential
Manager** 外部对话框（要求输入 `https://git.chongdian.xyz/` 的
Username/Password）。期望：凭证类交互全部收敛到应用内（F012
AuthPromptDialog），不再出现任何外部弹框。

## 根因（tester 分析）

GitWave 查询凭证的唯一通道是
`credentials.rs` `query_helper` 的 `git credential fill`，它把请求转发给系统
配置的 credential helper——Windows 上 Git for Windows 默认安装 GCM。
`git.chongdian.xyz` 不是 GCM 已知 provider，GCM 在存储中查不到该 host 凭证
时按默认 `interactive` 设置**弹出自带 GUI**。既有防护
（`GIT_TERMINAL_PROMPT=0`、`GIT_ASKPASS`/`SSH_ASKPASS` 指向不存在程序，见
fix-auth-credential-not-persisted 补充修复）只能杀死 git core 的
terminal/askpass 提示链——GCM 不是 askpass fallback，它就是 helper 本身，
是否交互是它自己的决定（`credentials.rs` 旧注释「that is the helper's call,
not ours」即此意，本次任务推翻该语义）。

完整链路：GCM 框打开期间 fill 进程挂起（最多 120s `CREDENTIAL_FILL_TIMEOUT`）
→ 用户取消 → fill 非零退出 → `query_helper` None → 应用 keyring 兜底 →
也空 → 回调 auth_error → 操作以 `git.fetch_auth_failed` 失败 → 前端才弹
F012。fetch/pull/push 手动操作如此；60s 自动刷新触发的 fetch 连 F012 都不接
（只写状态栏），外部 GCM 框是用户看到的唯一弹框。

### 全量弹框面审计结论（tester）

外部弹框唯一来源就是 `query_helper` 一个点；但 F012 应用内弹窗覆盖有 4 个
缺口：clone（`git.clone.auth_failed` 不在前端 `AUTH_FAILED_CODES`，且
`cmd_clone_repo` 无 auth 参数、无 `run_sync_op` 超时/取消）、删除远端分支
（后端已支持 auth，前端两处调用未接）、submodule（`SubmoduleUpdateOptions`
完全没挂凭证回调，认证失败报 generic error）、自动刷新（产品决策：维持
状态栏提示，后台定时器不弹模态框）。

## 修复方案

确定性思路：**显式禁止 helper 交互，凭证交互权全部收归应用内**。

1. **`credentials.rs`：helper 子进程统一禁止交互**（核心修复）
   `query_helper`（fill）与 `notify_helper`（approve/reject）的重复「死提示
   环境」构造提取为共享 `helper_command`，追加 `GCM_INTERACTIVE=never`
   环境变量（主通道；GCM 官方语义：需要交互时立即报错而非挂起/弹框，
   已存凭证含 OAuth refresh token 仍静默返回）与
   `-c credential.interactive=never` 参数（对读 git config 的其它 helper
   的尽力提示）。GCM 需交互 → 快速失败 → keyring 兜底 → F012。
   参考：[GCM configuration.md](https://github.com/git-ecosystem/git-credential-manager/blob/main/docs/configuration.md) ·
   [environment.md](https://github.com/git-ecosystem/git-credential-manager/blob/main/docs/environment.md)
2. **clone 收敛**：`clone_https` 加 `auth` + `cancel` 参数（provider 选择
   复用 `remote.rs` `provider_for_operation`，提取为 `pub(super)`）；
   `cmd_clone_repo` 套 `run_sync_op`（补 180s 超时 + 取消 +
   `emit_storage_outcome`）；错误码 `git.clone.auth_failed` 统一为
   `git.clone_auth_failed` 并加入前端 `AUTH_FAILED_CODES`；ActionBar
   clone 认证失败 → F012 重试一次。
3. **删除远端分支收敛**：`deleteRemoteBranch` 前端 api 加 auth 透传，
   BranchList / RefBadgeContextMenu 两处调用接 F012 重试一次。
4. **submodule 收敛**：`update_submodule` / `add_submodule` 的
   FetchOptions 挂凭证回调（`provider_for_operation`，proxy attach 保留），
   认证失败改报新码 `git.submodule_auth_failed`；命令层套 `run_sync_op`；
   前端 SubmodulesPanel 接 F012 重试。
5. **ADR-0003 措辞更新**：原「GCM 自行决定弹出 GUI 的场景仍保留」改为
   「helper 交互显式禁止」，决策语义演进记录进 docs/tech/decisions。

## 回归测试要点

单元测试（不触网、不触真实凭据存储）：

- `helper_command` 产出断言：args 含 `-c credential.interactive=never`，
  env 含 `GCM_INTERACTIVE=never` / `GIT_TERMINAL_PROMPT=0` / dead askpass。
- 既有 FillOnce / vault 兜底 / url→host 用例回归不破。
- 错误码统一后 grep 确认 `git.clone.auth_failed` 无残留引用。

真机手动验收（Windows + GCM）：

- [ ] GCM 已存凭证（含 GitHub OAuth token）+ fetch/push → 静默成功，
      无任何弹框（`never` 只拒绝交互，缓存凭证照常返回）。
- [ ] 无任何存储凭证 + `git.chongdian.xyz` fetch → **不再弹 GCM 框**，
      快速失败 → F012 应用内弹窗 → 输入正确凭证原地重试成功。
- [ ] F012 勾选记住 → 下次操作静默成功（helper 或 keyring 命中其一即可，
      `credential-storage` 事件状态正确）。
- [ ] 存储凭证被远端 401 拒绝 → reject + keyring 擦除，F012 至多一次
      （5248489 回归）。
- [ ] 60s 自动刷新 / Ctrl+R 遇无凭证 fetch → 仅状态栏错误，无任何弹框。
- [ ] clone HTTPS 无凭证 → F012 弹窗可重试；180s 超时生效（取消注册表与
      后端取消链路已接入 clone；状态区 clone 专属取消按钮留待后续任务，
      超时兜底）。
- [ ] 删除远端分支 / submodule update·add 遇认证 → F012 弹窗；取消弹窗 →
      操作以 cancelled 收场（busy 释放，不挂起）。
- [ ] SSH 远端行为不变；macOS osxkeychain / Linux store 不受
      `GCM_INTERACTIVE` 影响（非 GCM helper 忽略该变量）。

## 分支

`fix/credential-dialog-convergence`（从 main 拉出）。

## 关联

- 前置：docs/tasks/fix-fetch-gcm-credential-prompt/、
  docs/tasks/fix-auth-credential-not-persisted/（askpass 死环境 + keyring
  兜底——本次在其上补齐「helper 自身交互禁止」）
- 决策：docs/tech/decisions/00-overview.md ADR-0003（语义演进：
  helper 主导 → 应用内主导，helper 仅作静默存储/读取）
