# fix-auth-credential-not-persisted · 审查报告

> 审查对象：本分支未提交改动（credentials.rs / lib.rs / api.ts / useRemoteSync.ts / 双语 status.json / ADR-0003 / plan.md）
> 审查方式：code-reviewer 按 7 维度（正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践）

## 🔴 严重问题（1 项，均已修复）

| # | 位置 | 问题 | 处置 |
|---|---|---|---|
| 1 | `credentials.rs` `CredentialStorageSlot::take` | 实现为 clone 后返回，槽位永不清空：某次 `Failed` 记录会在**每一次**后续 sync 操作（含无关的 SSH 操作）结束时被重复 drain 并 emit，danger 提示永久粘滞、跨操作误报 | 改为 `slot.take()` 真正取走；新增测试 `storage_slot_take_clears_so_outcomes_do_not_leak_across_operations` 锁定生命周期 |

## 🟡 一般问题（处置记录）

| # | 问题 | 处置 |
|---|---|---|
| 2 | `store_in_vault` 不拒绝含换行的值 → 存入不可读回的载荷，兜底静默失效（复刻本任务要消灭的"看似保存实则没有"） | **已修**：写入点检查 `\n`/`\r` 返回 false（outcome 正确落为 Failed），与 `credential_request` 的协议防御对齐；新增测试 `store_in_vault_rejects_line_breaking_values` |
| 3 | helper 路径恒写 vault 是否越权（GCM GUI 弹窗供给的凭据没有 checkbox） | **判定不违规**：checkbox 只治理 inline 输入（`InlineCredentialProvider`）；approve 在 git 协议中本就是"请存储"，GCM 默认也落盘。但 plan.md/ADR-0003 原措辞"仅 helper 失败时兜底"已不符实 → **已更新** ADR-0003 与 plan.md：helper 为主、vault 恒镜像、reject 同步清除；对刻意用 `cache` helper 的用户会被持久化，记为已知取舍 |
| 4 | 全局 STORAGE_SLOT 并发串扰（不同 workspace 并行 sync 的 outcome 互相覆盖、事件不带 workspaceId） | **接受**：状态区本为 last-wins，修完 🔴1 后最坏后果是提示张冠李戴的 UI 噪音，不阻塞合入；后续迭代给事件加 workspaceId |
| 5 | reject 清除 vault 无测试（防坏凭据无限复活的核心路径） | **已补**：`helper_reject_also_erases_the_vault_fallback`（伪造 helper answer → reject → 断言 vault 条目被清除） |

## 🟢 优化项

- use 语句移到文件顶部 use 区 ✅ 已落地
- `entry()` 的 `https/` 前缀与 scheme 剥离说明 ✅ 已补注释
- `lib.rs` emit 失败补 `tracing::warn` ✅ 已落地
- `VAULT_TEST_LOCK` 毒化容忍：未采纳（现有测试集无 panic 路径，`VaultGuard` Drop 已保证恢复）

## 已核实无风险的关注点

- serde 契约：`tag="status"` + 单词变体 → `{"status":"stored"|"fallback"|"failed"}`，与前端 TS 联合类型一致。
- `vault_host` scp 形式（`git@host:22/path`）将 `22` 并入 host 的误判：SSH 远端走 `SshAgentCredential`，永不触达 vault，纯防御性代码。
- `FillOnce` Empty latch 时 reject 清不到 vault：Empty 意味着 helper 与 vault 均为空，无物可清，对称性成立。

## 验证

`cargo fmt` / `cargo clippy --all-targets -- -D warnings` / `cargo test --lib`（288 passed，含 10 个新增）/ `npx tsc --noEmit` / `npx eslint`（改动文件）全绿。

## 结论

🔴 已修复、🟡 按上表处置完毕，**可合入**（squash merge，关联本 plan.md）。
