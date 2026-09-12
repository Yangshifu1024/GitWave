# Review · chore-minor-hardening（f1f509c + 修复提交）

> 最终态审查记录。初审报告中的问题若已在最终代码修复，标注「已修复」；
> 本提交（gate 下沉 + warn 去重 + 文档约束）为初审遗留项的收尾。

## ✅ 优点

- `migrations.rs` 单事务原子性正确：`BEGIN IMMEDIATE → apply_inner → COMMIT/ROLLBACK`，并行首开串行化，降级 warn 条件（`max_applied > latest_known`）精确，5 个既有测试语义兼容。`COMMIT` 失败分支同样尝试 `ROLLBACK`（`migrations.rs:60-63`）。
- `workspace_repo.rs` 三处 warn 只加日志不改行为；`status` 未知值仍回落 `Active`，语义不变。
- `ai/rules.rs` 用 `File::take(MAX+1)` 先读后判，**无 metadata-then-read TOCTOU**（初审报告此处描述失实，特此更正），超限整体拒绝为更保守的 fail-closed。
- `proxy.rs` warn 为纯静态文本，不记录密钥内容。
- `plan.md` 偏差节诚实：代理不剥离（附 env 桥原因）、`allow(dead_code)` 保持，均与 diff 一致。

## 已修复记录

| 初审发现 | 位置 | 修复方式 |
|---|---|---|
| 🔴 probe 限域手工解析可被 `#`/`?` 绕过 | `lib.rs` | f1f509c 终版已改 `reqwest::Url::parse` + `host_str()`，并带 12 断言回归测试（含 `evil.com#@127.0.0.1`、`?x=`、userinfo 三种绕过） |
| 🟡 gate 只在命令层，`use_cases::probe_ollama` 是无 gate 旁路 | `use_cases.rs:606` | 本提交：`ollama_probe_allowed` 下沉至 use case 入口，命令层变薄；新增「非回环在 use case 层即拒」测试 |
| 🟡 `commit_signature` 回退告警刷屏 | `git2_adapter.rs:57` | f1f509c 终版已用 `AtomicBool::swap` 每进程一次 |
| 🟡 proxy 凭据 warn 跟随每次 resolve 触发 | `proxy.rs:199` | 本提交：`AtomicBool` 去重为每进程一次 |
| 🟡 `apply` 事务约束仅存于行内注释 | `migrations.rs:48` | 本提交：doc 注释显式写明「调用方不得有未决事务」约束 |

## 遗留（不阻断，可后续处理）

- 🟡 `BEGIN IMMEDIATE` 的并发竞态路径（两连接同库一方持锁）无自动化用例——需要真实并发，单测模拟价值有限，建议观察 CI。
- 🟢 `rules.rs` 恰在 1 MiB 边界劈开多字节 UTF-8 时 `read_to_string` 报错被 `.ok()?` 静默忽略（fail-closed、概率极低）；可加多字节边界用例。
- 🟢 `0.0.0.0`、十进制 `127.0.0.1` 写法一律拒绝，属安全方向误伤，当前接受。

## 📝 总体评价

改动整体低风险、方向正确，日志只观测不改行为。初审唯一的 🔴（probe 限域绕过）已在最终代码修复并补测试；本轮收尾把限域 gate 下沉到 use case 层消除旁路，并完成告警去重与事务约束文档化。无阻断项。
