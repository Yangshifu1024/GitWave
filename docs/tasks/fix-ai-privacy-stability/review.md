# Review · fix-ai-privacy-stability（d2666e2 + 测试补充提交）

> 最终态审查记录。本文件为分支内初审报告的刷新版：初审所列问题已逐项核对
> d2666e2 最终代码，绝大多数已在最终态修复；下文「已修复记录」给出逐项结论，
> 并登记合并前遗留清单。

## ✅ 优点

- `client()` 双重检查锁正确：读→写→复查，`into_inner` 恢复毒锁，构建失败返回 `network_with(AI_CLIENT_BUILD)` 且不污染单例；`generate_text` 将 `client()?` 提到循环外。
- `Debug` 脱敏完备：`AiGenerateRequest` / `ProviderAttempt` / `ResolvedAiProvider` 均手写、`api_key → "***"`；`http_error`/`anthropic_no_text_error` 的 message 与 content 解耦（log 只剩 `stop_reason` + 200 字截断）。
- `redacted_url` 正确：`rsplit_once('@')` 抗密码内 `@`、`path` 中 `@` 保留、scp 原样；`repo_adapter`/`submodule` 闭包内单次调用。
- 凭据多账号：`vault_key = host + \x1f + user` 不透明不可逆；迁移/删除均校验用户名一致性，他账号不受影响。
- `cherry_pick_commit` 冲突先 `reset(Hard)→HEAD` 再 `cleanup_state`，顺序正确且有回归测试；`rebase` 改读 `inmemory_index` 对症，`abort` 仍执行。
- 同步锁统一为 `workspace_sync_lock`，fetch/pull/push/删远端分支加锁顺序一致；`clone` 守卫先验后删。
- 含约 20 个新增单测：scrubber 形态与误杀反例、https 强制、状态码映射、Debug 脱敏、vault 迁移与共存、大输出死锁、ssh 注入拒绝等，覆盖真实回归路径。

## 已修复记录（初审 🟡 → 最终态核对）

| 初审发现 | 最终态结论 |
|---|---|
| `asia` 裸词误杀 | 已修复：移除裸词匹配并补误杀反例测试 |
| `token = "x"`（等号带空格）漏杀 | 已修复：nospace 副本命中带空格形态 |
| 整行丢弃 vs plan 承诺的片段替换 | 已申报偏差：整行 `[REDACTED]` 替代片段替换，plan 偏差节如实记录 |
| `should_failover` 注释与行为矛盾 | 已修复：注释同步为 fail-fast 语义（400/422 → Protocol 不重试） |
| `vault_host` IPv6 端口丢失 | 已修复：IPv6 保留端口（有单测） |
| `clone_repo` 守卫无 `.git` 例外 | 已申报偏差：失败残留同样走确认流程，比 plan 更保守（合理） |
| 成功路径 `join()` 无上限 | 已修复：`join_drain` 加 5s 上限（正向补强） |
| `ssh-add -L` 无超时 | 已修复：`-l`/`-L` 均加 15s 超时（超出 plan 的正向偏差） |
| `proxy.rs:242` 残留 expect | 已修复：生产代码 `.expect`/`unwrap` 清零 |
| `ResolvedAiProvider` 派生 `Serialize` | 已修复：不再携带明文 `api_key` 序列化面 |

## 本提交新增（测试债收尾）

- `tracing.rs`：`should_prune` 纯函数化 + 策略单测（名字/年龄双门控）+ 临时目录集成测试（过期删、现存留、异名留）；`prune_old_logs` 改为委托纯函数。
- `sqlite.rs`：`#[cfg(unix)]` 权限位回归测试——`state.db` 与 `-wal` 被 `lock_down_db_files` 收敛为 0600（Windows 无权限位语义，不编译）。
- `provider.rs`：抽出 `build_client()`（行为不变），新增「坏 proxy env → `Err(AI_CLIENT_BUILD)` 不 panic」回归测试。

## 遗留（不阻断，建议合并后跟进）

1. 🟡 scrubber 对「独行高熵裸密钥」（无 keyword 同行的 base64 PAT 等）有结构性漏检缺口——建议后续加 ≥32 字符 `[A-Za-z0-9+/=_-]` 启发式（宁可误杀）。
2. 🟢 `redacted_url` 对「密码内含 `/`」的不合法输入会整条原样返回；RFC 3986 要求 percent-encode，可按「authority 整段 rsplit」加固。
3. 🟢 unix-only 路径（chmod/prune 权限位、client 坏 env 在 unix 的具体行为）需 CI 在 unix 目标上确认通过；本机为 Windows。
4. 🟢 增量日志（umask 继承）与 WAL 新建文件的权限收敛——plan 偏差已申报，保持跟踪。
5. 🟢 `abort_merge` 现删除 `ORIG_HEAD`（git 自身是恢复为 merge 前值）；当前无依赖方，可接受。
6. 🟢 vault 用户名大小写变体（`Alice`/`alice`）可能形成两个条目；行为安全，仅存储冗余。

## 📝 总体评价

隐私面收敛与稳定性目标达成，plan 9 项全部落地、偏差全部如实申报，无阻断项。本提交补齐 plan 承诺的三项测试债后，`cargo test --all-targets`、`clippy -D warnings`、`fmt --check` 全绿（unix-only 用例标注为 CI 待办）。
