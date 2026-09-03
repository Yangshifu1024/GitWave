# fix-auth-credential-not-persisted · 修复方案

## 现象

F012「Authentication required」弹窗中填写用户名 + PAT（勾选「Save to the
system keychain」）并成功后，随后的 fetch / pull / push 仍持续再次弹出认证
对话框——凭证像从未被保存。平台 Windows。

## 环境实证（本机 Windows + GCM）

- 系统级 `credential.helper=manager`（GCM），无其他 helper。
- Windows 凭据管理器当前**不存在** `git:https://github.com` 条目。
- 终端 `git credential fill` 对 github.com 可弹 GCM GUI（helper 链本身可用）。

## 根因（tester 分析）

GitWave 侧协议实现正确（与 git CLI 调用方式逐字段相同）：

```
F012 弹窗 → InlineCredentialProvider（当次直接用输入凭证 → 成功）
         → approve: git credential approve（remember=true 时）→ GCM 落盘 ✗
下次操作 → GitCredentialHelper: git credential fill → 空 → 再次弹窗
```

- **确定原因**：GCM 对程序化 `git credential approve` **静默不落盘**（凭据
  管理器为空即证据）。当次操作成功是因为 Inline 路径直接使用输入凭证，与
  存储无关；下一次 fill 为空必然再弹。
- **放大器**：`notify_helper` 任何失败（spawn 失败 / 非 0 退出 / 超时 /
  exit 0 但没存）都只 `tracing::warn`，UI 无感知，用户无从知晓保存失败。
- 已排除：remember 透传链（前端默认值/serde/入参）、approve 与 fill 的
  url 不一致（同取 `remote.url()`）、reject 误清（Inline reject 恒 no-op）、
  helper 解析被干扰（系统级仅 manager 一条）。

## 修复方案

确定性思路：不再依赖 GCM 是否配合落盘——GitWave 自己记住。

1. **`credentials.rs`：notify_helper 返回四态结果**
   `Stored / SpawnFailed / ExitFailed / TimedOut`（保留既有 warn 日志），
   调用方可感知 helper 落盘是否真正生效。
2. **`credentials.rs`：approve 路径 keyring 双写**
   `remember=true` 时在 `git credential approve` 之外同步写入应用 keyring
   （复用 `infrastructure::ai::secrets` 的 `SERVICE_REMOTE = "gitwave.remote"`
   预留命名空间；account = url→host 归一化，payload = username/password）。
   helper 的确认不可信（GCM 可能 exit 0 却不落盘），因此 helper 路径的
   approve（GCM GUI 弹窗供给的凭据）同样恒写 keyring——与 git 语义对齐
   （approve 本就是"请存储"）；已存同值跳过写入；凭据被远端拒绝时同步
   清除 keyring 对应条目。写入失败不影响原操作结果（best-effort）。3. **`credentials.rs`：query_helper fill 为空时降级读 keyring**
   helper 链路取不到凭据时回退读 keyring 兜底；命中则本次操作静默完成。
   凭据被远端拒绝（Auth reject）时同步清除 keyring 对应条目，防止坏凭据
   在缓存中无限复活。
4. **UI 提示（用户补充意见，已采纳）**：approve 结果写入进程内槽位，
   use_cases 在操作结束后取走并经 `on_storage` 回调透传；lib.rs 以 Tauri
   事件 `credential-storage` 发出；前端 `useRemoteSync` 监听写入状态区：
   - helper approve 成功（或已存）：静默，不打扰；
   - helper 失败但 keyring 已兜底：info「凭据已保存到应用钥匙串（系统
     credential helper 未生效），GitWave 将自动复用」；
   - 两者都失败：danger「凭证暂未能保存，下次操作可能需要重新输入」。
   i18n 补 zh-CN / en。
5. **补充修复（真机验收发现）**：非 github 类主机上，GCM 判定非交互后静默
   返回空，git 自身的终端提示链路会退到 `GIT_ASKPASS`/`SSH_ASKPASS`
   （PortableGit 自带 `git-askpass.exe`，即截图中的「Git for Windows」
   原生对话框；`GIT_TERMINAL_PROMPT=0` 拦不住它）。fill/approve/reject
   子进程现将 `GIT_ASKPASS` 与 `SSH_ASKPASS` 指向不存在的程序，交互回退
   立即失败返回空，随后由应用 keyring 兑底接管——凭证 UX 全部收敛到
   应用内（F012 弹窗 / 静默复用）。GCM 自行决定弹出 GUI 的场景仍保留
   （helper 主导，ADR-0003 语义不变）。

## 回归测试要点

单元测试（不触网、不触真实凭据存储）：

- url→host 归一化（scheme 剥离、userinfo 剥离、端口保留、大小写、尾路径）。
- keyring payload 编解码与含换行值拒绝。
- approve 结果映射：helper Stored → stored；helper 失败 + vault 写入成功 →
  fallback；helper 失败 + vault 失败 → failed。
- fill 为空降级读 vault 命中 / 未命中的决策逻辑。
- notify 子进程结果四态分类（纯函数映射）。

真机手动验收（Windows + GCM）：

- [ ] F012 勾选保存 → 操作成功；凭据管理器出现 `git:https://<host>` 或
      keyring 有 `gitwave.remote` 条目；状态区无警告（helper 正常时静默）。
- [ ] GCM 故意配坏（`credential.helper=!exit 1` 级别）→ keyring 兜底仍能
      记住，后续 fetch/push 静默成功，状态区出现 info 提示。
- [ ] GCM 不落盘且 keyring 写入失败（模拟）→ 状态区 danger 提示，下次操作
      重新弹窗（行为可解释，不再无感循环）。
- [ ] 取消勾选「Save to the system keychain」→ 不写 keyring、不 approve，
      下次操作重新弹窗。
- [ ] 重启应用后 fetch/pull/push 静默成功（keyring 持久化生效）。
- [ ] 换错密码触发 401 → keyring 旧条目被清除，重新输入新凭据后恢复。
- [ ] SSH 远端行为不变；LFS 走外部 git lfs 不受影响。

## 分支

`fix/auth-credential-not-persisted`（从 main 拉出）。

## 关联

- 前置：docs/tasks/fix-fetch-gcm-credential-prompt/（fill→approve/reject 闭环）
- 决策：docs/tech/decisions/00-overview.md ADR-0003（HTTPS 走系统
  credential helper，应用 Keychain 恒镜像兑底；审查后已同步更新措辞）  应用内兜底。
