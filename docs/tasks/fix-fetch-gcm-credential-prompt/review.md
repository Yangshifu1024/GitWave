# fix-fetch-gcm-credential-prompt · 代码审查报告

审查人：code-reviewer（7 维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践）
审查对象：分支 `fix/fetch-gcm-credential-prompt` 未提交改动
（`credentials.rs` / `remote.rs` / `repo_adapter.rs`），已核对 vendored
libgit2 1.9.7（winhttp.c / http.c）与 git2 0.20.4 源码验证错误传播路径。

## ✅ 优点

- **根因修得准**：补全 `fill → approve/reject` 协议闭环正是"每次都弹"
  的正确解法。`approve()` 以 `helper_answer()` 门控，只在 helper 真正
  给出过凭据时回写——用户取消（`Fill::Empty`）、公共匿名仓库（回调不
  触发）、URL 内嵌凭据（libgit2 先用 `apply_url_credentials`，不进
  回调）、SSH（trait 默认 no-op）都不会误存，测试明确覆盖。
- **FillOnce 门闩设计正确**：一次操作内至多一次 `fill`，401 重试与
  用户取消都锁存，对应"取消一次即终止本次操作""同一次 fetch 不二连弹"。
- **并发隔离天然成立**：每次 fetch/push/pull/clone 各建 provider，
  auto-refresh 与手动 fetch 的门闩互不影响；`Arc<Mutex<FillOnce>>`
  用法正确，无循环引用。
- **安全处理到位**：凭据只经 stdin 传递、不进 argv（不泄漏到进程列表）；
  三个子进程统一 `GIT_TERMINAL_PROMPT=0`，注释准确说明只抑制终端回退、
  不影响 GCM GUI；stdin 协议符合 git-credential(7)。
- **不动凭据存储的守卫方向正确**（libgit2 源码核实）：5xx / 证书错误 /
  重放超上限都不是 `GIT_EAUTH`，不会触发 reject，无误清风险。
- 测试策略明智：approve/reject 正路径不碰开发者真实凭据存储；注释风格
  与仓库一致；错误上报沿用 AppError 错误码体系。

## 🔴 严重问题（必须修复）——已修复

1. **回调错误码错误，`reject()` 不可达**（`credentials.rs` 原实现）
   - **描述**：门闩在 401 重试时返回 `git2::Error::from_str(...)`，
     raw code 为 `GIT_ERROR(-1)` → `ErrorCode::GenericError`。libgit2
     的 HTTP transport 把回调错误**原样上抛**（winhttp.c
     `acquire_credentials`：`else if (error < 0) return error;`），只有
     无回调时才置 `GIT_EAUTH`。后果：`remote.fetch()` 落入 `_ =>`
     分支——`creds.reject()` 是死代码（坏凭据永远被重放、无恢复路径），
     且认证失败被误报为 Network 类错误（`FETCH_FAILED` 而非
     `FETCH_AUTH_FAILED`）。
   - **修复**：新增 `auth_error()`，用
     `git2::Error::new(ErrorCode::Auth, ErrorClass::Http, msg)` 构造，
     三处回调错误（poisoned / rejected / no credentials available）全部
     改用；`Fill::Empty` 由 fall-through 改为直接返回认证错误（HTTPS
     transport 不会单独请求 USERNAME）。新增单测
     `auth_error_carries_auth_code_for_reject_wiring` 断言 `code() ==
     Auth`。proxy 407 同样会以 Auth 上抛并连带 reject——已在注释说明
     可接受（下次 fill 重新获取）。

## 🟡 一般问题（建议修复）——已修复

1. **`notify_helper` 静默吞错**：spawn / stdin 写入 / 非零退出全部无感，
   失败即无感退化为"每次都弹"却无线索。→ 失败点补 `tracing::warn!`
   （stderr 保持 `Stdio::null()` 防止误记敏感内容，以退出码判断）。
2. **stdin payload 无测试且两处重复拼接**：→ 提取纯函数
   `credential_request(url, user, pass)`，fill/approve/reject 共用；新增
   单测覆盖键序、空行终止、空 password 仍输出键（协议安全敏感，拼写
   错误即静默失效）。值含换行无法转义属 git CLI 同款固有限制，已在
   doc 注释声明。
3. **`Fill::Empty` fall-through 依赖"不可达"才安全**：→ 与 🔴-1 一并
   处理，`Empty` 直接返回认证错误，注释说明 HTTPS transport 不请求
   bare USERNAME。

## 🟢 优化建议（可选）——已全部落地

1. ~~approve/reject + 错误映射样板在 4 处重复~~ → 提取
   `run_with_credentials(provider, operation, auth_error, other_error)`
   骨架（credentials.rs）：成功 approve、Auth 失败 reject 并走 auth
   映射、其他失败不动存储；fetch / push / 删远端分支 / clone 四处
   调用点全部改造为传入 git2 操作闭包 + 两个错误构造闭包，proxy 407
   说明移入骨架 doc。配 `RecordingProvider` 单测锁定
   成功→approve、Auth→reject+Credential 类、其他→二者皆不动的契约，
   防未来第五个同步操作漏挂。
2. 持锁跨 helper 调用的并发不变量 → 已在 trait `callbacks` doc 声明
   （libgit2 回调串行、approve/reject 在操作返回后才调用）。
3. ~~payload 值含换行仅注释声明~~ → `credential_request` 升级为返回
   `Option`：url/username/password 含 `\n` / `\r` 时拒绝构造（协议无
   转义，防注入伪键；值本就来自 helper 的按行输出，属纵深防御），
   调用方跳过并 `tracing::warn!`，配控制字符拒绝单测。
4. ~~auto-refresh 与手动 fetch 并发可能各弹一次 GCM~~ →
   `use_cases::fetch` 按 workspace 串行化：`workspace_fetch_lock`
   （OnceLock 注册表，per-workspace `Arc<Mutex<()>>`，锁中毒取回继续）
   在 fetch 入口加守卫，同 workspace 的 fetch 排队执行；不同 workspace
   互不阻塞。配 per-workspace 锁复用单测。

## 📝 总体评价

方案方向正确、实现干净：fill/approve/reject 闭环配合"网络失败不动存储"
的守卫准确命中根因，approve 门控、provider 每操作独立、stdin-only 传凭据
等细节均正确。审查发现的 🔴（回调错误码为 GenericError 导致 reject 不可达、
认证失败误报为网络错误）与 🟡 三项已全部修复并补测试。

## 验证

`cargo fmt` / `clippy -D warnings` / `cargo test --all-targets` 全绿，
216 passed（含 8 个新增单测：FillOnce ×2、approve/reject 空答案 no-op、
auth_error 错误码、credential_request 协议格式、控制字符拒绝、
run_with_credentials 生命周期、workspace fetch 锁）。真机验收按 plan.md
"回归测试要点"清单执行。
