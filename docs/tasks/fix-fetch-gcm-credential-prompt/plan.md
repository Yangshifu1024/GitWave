# fix-fetch-gcm-credential-prompt · 修复方案

## 现象

对 HTTPS remote 的仓库，GitWave 里每次 Fetch（工具栏按钮 / ⌘R / 命令面板 /
60s 自动刷新）都会弹出 Git Credential Manager 的 GUI 登录窗；输入凭据后
本次能成功，但下次 fetch 又弹，凭据像是从未被记住。pull（内部先 fetch）、
push、clone HTTPS 同样会弹。

## 根因（tester 分析）

GitWave 的同步操作全部走 libgit2（`git2` crate），HTTPS 凭据由
`src-tauri/src/infrastructure/git/credentials.rs` 的 `query_helper` 同步
spawn `git credential fill` 获取，链路上三个问题叠加：

1. **核心缺陷：拿到凭据后从不回写**（确定原因）。`git credential` 协议
   是 fill（取）→ 使用 → approve（存）/ reject（清）。git CLI 在认证成功
   后会自动调 `approve`，让 GCM 把凭据存进 Windows 凭据管理器；GitWave
   用 libgit2 绕过了这一步，全仓库没有任何 `approve` 调用（grep 实证）。
   于是 GCM 弹窗里输入的凭据**永远不落盘**，下一次 fetch 对 GCM 而言仍是
   "无凭据可用"，只能再弹。这是"每次都弹"的直接原因。
2. **单次操作内可能连弹**。libgit2 一次网络操作会多次进入凭据回调
   （先要 username 再要 password；服务器 401 后重试），每次回调都重新
   spawn 一次 `fill`——凭据被拒或用户取消后，同一次 fetch 内还会再弹。
3. **触发层放大**。`use_cases.rs::fetch` 在 remote 为 `None`（工具栏 /
   命令面板 / 自动刷新都传 None）时遍历所有 remote，N 个 HTTPS remote
   弹 N 轮。

另有一个健壮性小问题：`fill` 子进程未设 `GIT_TERMINAL_PROMPT=0`，在
未配置任何 helper 的机器上可能挂在终端提示上（stdin 是管道，GUI 进程无
tty）。GCM 的 GUI 弹窗由 `GCM_INTERACTIVE` 控制，不受该变量影响。

参照：`docs/tech/decisions/00-overview.md`（0003 凭证策略：HTTPS 走系统
`git credential helper`）——本修复不改变该策略，只是补全协议的存/清两步。

## 修复方案

全部改动在 `src-tauri/src/infrastructure/git/`，UI 与前端不变：

1. **`credentials.rs`：凭据状态机补全**
   - `CredentialProvider` trait 增加默认空实现的 `approve()` / `reject()`
     （SSH provider 走默认 no-op）。
   - `GitCredentialHelper` 内部持共享的 `FillOnce` 门闩（`Arc<Mutex<_>>`）：
     - **单次弹窗去重**：一次操作内 `fill` 至多执行一次；libgit2 的 401
       重试直接返回错误，不再重复弹窗。用户取消（fill 返回空）同样锁存，
       取消一次即终止本次操作。
     - **成功回写**：`approve()` 把本次实际使用的凭据经
       `git credential approve` 交给系统 helper 落盘（GCM → Windows 凭据
       管理器），下次 fetch 静默复用。
     - **失败清除**：`reject()` 在凭据被服务器拒绝时经
       `git credential reject` 清掉失效的已存凭据，下次 fetch 重新弹窗
       获取新凭据，而不是永远重放坏凭据。
   - `fill` / `approve` / `reject` 子进程统一设 `GIT_TERMINAL_PROMPT=0`。
2. **`remote.rs`：在结果路径挂回调**
   - `fetch` / `push_with_options` / `delete_remote_branch`：成功 →
     `creds.approve()`；`ErrorCode::Auth` → `creds.reject()` 后按原错误码
     上报；其他错误不动凭据（网络失败凭据真伪未证）。
   - `pull_integrate` 内部复用 `fetch`，自动覆盖。
3. **`repo_adapter.rs`：`clone_with_creds` 同款处理**（成功 approve /
   Auth 失败 reject）。
4. **多 remote fetch 不改**：每个 remote 各自创建 provider，各自至多一次
   弹窗，行为符合"fetch all remotes"语义。

有意的行为变化：

- 同一次 fetch 内不再出现第二次 GCM 弹窗（以前 401/取消后会连弹）。
- 存量失效凭据会在首次认证失败时被清除（以前一直重放到永远失败）。

## 回归测试要点

单元测试（不触网、不触真实凭据存储）：

- `FillOnce` 门闩：首次查询返回答案并锁存；第二次直接 `AlreadyQueried`；
  fill 返回空也锁存；`answer()` 供 approve/reject 读取。
- 现有 local-path remote 的 fetch/pull/push 测试必须不受影响（凭据回调
  不触发，approve/reject 为无害 no-op）。

手动验收（真机）：

- [ ] 首次 fetch HTTPS 仓库：GCM 弹一次，输入后成功；打开 Windows 凭据
      管理器可见 `git:https://<host>` 条目
- [ ] 第二次起 fetch：不弹窗直接成功
- [ ] 凭据故意输错：本次报认证失败，再次 fetch 重新弹窗（坏凭据已被
      reject 清除）
- [ ] 弹窗点取消：本次 fetch 报认证失败，且同一次操作内不再二连弹
- [ ] 多 HTTPS remote 仓库工具栏 Fetch：每个 remote 至多弹一次
- [ ] SSH 仓库 fetch：行为不变，全程无 GCM 参与
- [ ] auto-refresh 开着挂后台：已存凭据时完全静默

## 分支

`fix/fetch-gcm-credential-prompt`（从 main 拉出）。

## 审查与验证

code-reviewer 审查（详见 [review.md](./review.md)）抓到一个 🔴 并已修复：
初版回调错误用 `git2::Error::from_str` 构造，raw code 是 `GIT_ERROR(-1)`，
而 libgit2 的 HTTP transport（winhttp.c / http.c，已核对 vendored 源码）
把回调错误**原样上抛**、只在自己判定时才置 `GIT_EAUTH`——导致
`ErrorCode::Auth` 分支永远匹配不上，`reject()` 是死代码（坏凭据被永远
重放）、认证失败被误报为 Network 类错误。修复：新增 `auth_error()` 以
`ErrorCode::Auth + ErrorClass::Http` 构造，三处回调错误改用，
`Fill::Empty` 由 fall-through 改为直接返回认证错误。🟡 三条也已修：
`notify_helper` 失败点补 `tracing::warn!`（否则无感退化为"每次都弹"却
无线索）；stdin payload 提取纯函数 `credential_request` 并单测协议格式；
trait doc 补回调串行 / approve 不并发的锁不变量说明。

有意的行为变化（较初版 plan 无新增）：认证失败现按 Credential 类错误码
上报（`FETCH_AUTH_FAILED` 等），此前实现实际报 Network。

验证：`cargo fmt` / `clippy --all-targets -D warnings` /
`cargo test --all-targets` 全绿，216 passed（含 8 个新增单测）。
本机（Windows + GCM）真机验收按上方"回归测试要点"清单执行。

## 审查 🟢 优化项落地（第二轮）

review.md 的 4 条 🟢 建议全部实现：

1. **去样板**：新增 `run_with_credentials` 骨架（credentials.rs），
   统一"成功 approve / Auth 失败 reject / 其他失败不动存储"协议，
   fetch / push / 删远端分支 / clone 四处调用点改造完毕，配
   `RecordingProvider` 单测防未来漂移。
2. **锁不变量**已在 trait `callbacks` doc 声明（第一轮已落）。
3. **payload 纵深防御**：`credential_request` 拒绝含 `\n` / `\r` 的值
   （协议无转义，防注入伪键），调用方跳过并告警。
4. **并发弹窗消除**：`use_cases::fetch` 按 workspace 串行化
   （`workspace_fetch_lock`），auto-refresh 与手动 fetch 不再并发各弹
   一次 GCM；不同 workspace 互不阻塞。

改动文件新增 `application/use_cases.rs`（fetch 入口加 per-workspace
锁守卫 + 单测）。


