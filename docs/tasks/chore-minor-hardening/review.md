## ✅ 优点

- `migrations.rs` 单事务原子性正确：`BEGIN IMMEDIATE → apply_inner → COMMIT/ROLLBACK`，并行首开串行化，降级 warn 条件（`max_applied > latest_known`）精确，5 个既有测试语义兼容。
- `workspace_repo.rs` 三处 warn 只加日志不改行为；`status` 未知值仍回落 `Active`，语义不变。
- `proxy.rs` warn 为纯静态文本，不记录密钥内容。
- `plan.md` 偏差节诚实：代理不剥离（附 env 桥原因）、`use_cases.rs` 零改动、`allow(dead_code)` 保持，均与 diff 一致。

## 🔴 严重问题（必须修复）

- **位置**：`src-tauri/src/lib.rs`，`cmd_probe_ollama` 手工 host 解析
- **描述**：先 `split('/')` 取 authority、再 `rsplit('@')` 取 host，全程未剥离 query/fragment。`http://evil.com#@127.0.0.1` 与 `http://evil.com?x=@127.0.0.1` 会被误判 host 为 `127.0.0.1` 而放行，但 reqwest 实际请求的是 `evil.com`——限域形同虚设，内网扫描面仍在。
- **建议**：用 `reqwest::Url::parse`（`proxy.rs` 已有先例）取 `host_str()` 比对；或先按 `?`/`#` 截断再解析；补 `evil.com#@127.0.0.1` 被拒的回归测试。

## 🟡 一般问题（建议修复）

- **位置**：`src-tauri/src/infrastructure/git/git2_adapter.rs:57`
- **描述**：无 config 时每次回退都 warn；`commit_signature` 有约 10 处调用方（merge/rebase/stash/working_copy/tag…），受影响用户每次提交刷屏。
- **建议**：用 `AtomicBool`/`OnceLock` 每进程只 warn 一次。

- **位置**：`src-tauri/src/infrastructure/ai/rules.rs:19`
- **描述**：`metadata` 预检与 `read_to_string` 之间存在 TOCTOU（竞态下仍可能读入大文件）；fail-closed，影响低。
- **建议**：先读后按 `bytes.len()` 判定，或同一 fd 上 `File::metadata`。

- **位置**：`src-tauri/src/infrastructure/persistence/migrations.rs:54-63`
- **描述**：`BEGIN IMMEDIATE` 在调用方已处事务中会直接报错；`COMMIT` 失败分支未尝试 `ROLLBACK`；幂等重跑每次拿写锁（启动期多进程轻微争用）。
- **建议**：`COMMIT` 失败也尝试 `ROLLBACK`；注释约束 `apply` 须在自动提交连接上调用。

- **位置**：`src-tauri/src/infrastructure/proxy.rs:199`
- **描述**：warn 跟随 `normalize_manual_url`，如校验路径（如保存前前端校验）也调它，可能重复刷屏。
- **建议**：确认调用频率，必要时只在真正入库保存时 warn 一次。

## 🟢 优化建议（可选）

- **位置**：`src-tauri/src/lib.rs` probe 解析
- **描述**：`0.0.0.0`、十进制/十六进制 `127.0.0.1` 写法一律拒绝，属安全方向误伤，可接受；切 `Url` 解析后可顺手放行 `::ffff:127.0.0.1`。
- **建议**：切 `Url` 解析时一并决策，补大小写/无 scheme/`[::1]`/userinfo 用例（正常 `127.0.0.1:11434` 当前未被误伤，已验证）。

- **位置**：`src-tauri/src/infrastructure/ai/rules.rs` 测试
- **描述**：`absurd_file` 用例用纯 ASCII（字节数 == 字符数），多字节边界未覆盖。
- **建议**：加 1MiB+1 中文用例。

## 📝 总体评价

改动整体低风险、方向正确，日志只观测不改行为，偏差节与实现一致。唯一阻拦项是 probe 限域可被 `#`/`?` 绕过，修复建议明确（复用 `Url::parse`）；其余为 warn 频率与边缘健壮性问题，不阻拦合并。
