# chore: 后端 🟢 健壮性扫尾

状态：待实现（Batch 4，全量修复 4 批次之末；全部低风险，可与前批并行，无文件冲突）

分支：`chore/minor-hardening`（从 `main` 切出，squash merge）

## 背景

源码审查剩余 🟢 项：迁移并发竞态（理论）、解析失败静默丢弃、`AI.md` 无大小上限、
代理凭据明文落盘、`probe_ollama` 任意 URL、无 config 提交假身份无提示。
单项影响小，打包一批处理。

## 修复步骤

1. **迁移并发 + 降级检测**（`persistence/migrations.rs:54-79`）：`apply` 前加 `BEGIN IMMEDIATE`
   或单次重试规避双连接同时建表；未知 DB 版本（高于二进制已知）启动打 warn 日志；
   `filter_map(Ok)`（59）读 schema_version 失败打 warn 不静默。
2. **脏数据可见性**（`persistence/workspace_repo.rs:104/243/355`）：解析失败行加 warn 日志
   （不改丢弃语义）；未知 status 回落 Active 时同样 warn。行为不变，只让损坏可观测。
3. **`AI.md` 大小上限**（`ai/rules.rs:15`）：读前 `metadata` 限 1MB，超限截断 + warn 日志。
4. **代理凭据不落盘**（`app_settings_repo.rs:63` + `proxy.rs:185`）：持久化前剥 userinfo；
   内存/环境变量保持不变（子进程仍需认证）。
5. **`probe_ollama` 限 localhost**（`lib.rs:312-315`）：非 `127.0.0.1/localhost` base_url 直接拒，
   关闭内网扫描面。
6. **假身份可观测**（`git2_adapter.rs:57-62`）：无 git config 回退占位签名时打 warn 日志一次。
7. **顶层 `allow(dead_code)`**（`lib.rs:9-10`）：保持不动，标 wontfix 候选（收益低、触达面大）。

## 验证

- `cd src-tauri && cargo test --all-targets` + `cargo clippy` + `cargo fmt --check`
- 手动：旧版二进制打不开新版 DB 时日志有 warn；超大 `AI.md` 不驻留内存

## 实施偏差（以实现为准）

- 代理 userinfo **不剥离落盘**：运行时 env 桥需要它做代理认证，剥离会断掉认证代理；
  改为保存/解析时 warn 一次（不记密钥内容）。DB 明文问题由 Batch2 的 unix 0600 覆盖。
- `use_cases.rs` 本批零改动（poison 统一已由 Batch2 覆盖，避免 PR 互冲）；
  `proxy.rs:242` expect 同理归 Batch3。
- 顶层 `allow(dead_code)` 保持（wontfix）。
