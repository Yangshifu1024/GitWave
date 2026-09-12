## ✅ 优点

- `client()` 双重检查锁正确：读→写→复查，`into_inner` 恢复毒锁，构建失败返回 `network_with(AI_CLIENT_BUILD)` 且不污染单例；`generate_text` 将 `client()?` 提到循环外。
- `Debug` 脱敏完备：`AiGenerateRequest` / `ProviderAttempt` / `ResolvedAiProvider` 均手写、`api_key → "***"`，`fallbacks` 经已脱敏的 `ProviderAttempt` 打印；`http_error`/`anthropic_no_text_error` 的 message 已与 content 解耦（log 只剩 `stop_reason` + 200 字截断）。
- `redacted_url` 正确：`rsplit_once('@')` 抗密码内 `@`、`path` 中 `@` 保留、scp 原样；`repo_adapter`/`submodule` 闭包内单次调用。
- 凭据多账号：`vault_key = host + \x1f + user` 不透明不可逆；迁移/删除均校验用户名一致性，他账号不受影响；`use_cases:822` 的 `Option::expect`（非 poison）未被误动。
- `cherry_pick_commit` 冲突先 `reset(Hard)→HEAD` 再 `cleanup_state`，顺序正确且有回归测试；`rebase` 改读 `inmemory_index` 对症，`abort` 仍执行。
- 同步锁统一为 `workspace_sync_lock`，fetch/pull/push/删远端分支加锁顺序一致（先 sync 锁再 workspaces 锁），锁内无嵌套持锁调用；`clone` 守卫先验后删。

## 🔴 严重问题（必须修复）

无。

## 🟡 一般问题（建议修复）

- **位置**：`ai/scrubber.rs`、`LINE_KEYWORDS`/`TOKEN_PREFIXES`
  - **描述**：`asia` 作为裸子串匹配，任何含 "Asia" 的正常行（如注释、地名）都会整行丢弃，误杀明显；`token = "x"`（等号带空格）不含 `token=` 子串而漏杀；裸密钥值独占一行（无关键字同行）同样漏杀。实现是整行丢弃，与 plan 2.1 承诺的"正则片段替换、保留行结构"不符。
  - **建议**：`asia` 改为带长度/字符集的 `ASIA[0-9A-Z]{16}` 判定；补 `token\s*[:=]` 含空格形态；偏差节补记"片段替换降级为整行丢弃"。
- **位置**：`application/use_cases.rs:669-678` 注释 vs `:811-819` 代码
  - **描述**：注释称"400/422 → Protocol 会 fail over 到下一 provider"，但 `should_failover` 仅匹配 `Network`，`Protocol` 走 `:819` 直接返回，注释与行为矛盾；偏差节"注释已同步"不诚实。
  - **建议**：二选一并改注释：若要 failover 则 `should_failover` 放行 `Protocol` 中 400/422；若要停止则把注释改为"停止整条链"。
- **位置**：`git/credentials.rs`、`vault_host` IPv6 分支
  - **描述**：`[::1]:2222` 被剥括号后按 `]` 切分得 `::1`，端口丢失；而 IPv4 `github.com:8443` 保留端口。同一主机不同端口的 IPv6 凭据会共用一键。
  - **建议**：保留 `]` 后 `:port` 后缀（`::1:2222` 仍有歧义，更稳妥是 `[::1]:2222` 原样小写保留）；同步更新单测期望。
- **位置**：`application/use_cases.rs`、`clone_repo` 守卫
  - **描述**：plan 2.7 要求"失败 clone 残留（无有效 `.git`）可直接清理"，实现对一切非空目录一律 `CLONE_DEST_NOT_EMPTY`，无 `.git` 例外；偏差节未提及。
  - **建议**：补 `.git` 有效性检查或在偏差节如实记录行为收紧。
- **位置**：`infrastructure/process.rs`、`wait_with_output_timeout` 成功路径 `join()`
  - **描述**：子进程已退出但 grandchild 继承管道未关时，`read_to_end` 永不到 EOF，`join` 永久阻塞（超时路径已正确 detach 成功路径无保护）。
  - **建议**：成功路径也用超时 `join`，超限则 detach 并返回已收集部分；或文档注明该局限。
- **位置**：`infrastructure/ssh/keys.rs`、`list_identities_best_effort`
  - **描述**：`ssh-add -L` 用阻塞 `.output()` 无超时，agent 挂起会卡住调用方；与 `-l` 的按索引对齐仅 best-effort（已诚实注释）。
  - **建议**：复用 `wait_with_output_timeout` 加 5-10s 超时；失败回退逻辑保持不变。
- **位置**：`infrastructure/proxy.rs:242`、`INJECTED_VARS.lock().expect(...)`
  - **描述**：plan 列出的 poison 点已清零，但同类 panic 在此残留一处（静态锁 poison 即崩）。
  - **建议**：顺手改为 `unwrap_or_else(PoisonError::into_inner)`。
- **位置**：`application/use_cases.rs`、`ResolvedAiProvider` 派生 `Serialize`
  - **描述**：`api_key` 明文可序列化，若未来经 IPC 回传前端即泄漏（当前链内使用似未外发）。
  - **建议**：确认永不外发，或 `#[serde(skip_serializing)]` / 手写 `Serialize` 掩码。

## 🟢 优化建议（可选）

- **位置**：`ai/provider.rs`、`require_https_url`
  - **描述**：loopback 例外仅 `localhost/127.0.0.1/::1`，与 plan 一致；`10./192.168.` 内网明文、`::ffff:127.0.0.1` 不在例外内，行为偏严但可接受。
  - **建议**：维持现状，如需放行内网网关再扩展并补单测。
- **位置**：`ai/scrubber.rs`、`TOKEN_PREFIXES`
  - **描述**：Slack 前缀缺 `xoxe-/xoxt-/xoxq-` 等少见变体；`npm_` 可能误伤 `npm_install` 类标识（方向为宁可误杀，符合本模块注释）。
  - **建议**：补全 Slack 变体；`npm_` 可收紧为 `_authToken=npm_` 上下文，后续项。
- **位置**：`git/rebase.rs`、`if let Ok(idx) = rebase.inmemory_index()`
  - **描述**：错误被静默吞掉后冲突列表为空，但 `abort` 照常执行，安全无虞。
  - **建议**：加一行 `tracing::debug!` 记录吞掉的错误，便于排障。

## 📝 总体评价

实现质量高：隐私止血（脱敏 Debug、错误体截断、userinfo strip）与稳定性（去 panic、DCL、锁统一、超时与冲突恢复）均对症且单测覆盖到位。最需优先处理的是 `should_failover` 注释与行为矛盾及 `asia` 误杀两处；`review.md` 所列其余多为边界收紧与偏差诚实度补记，无阻止合并项。
