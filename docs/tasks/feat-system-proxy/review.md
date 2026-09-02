# feat-system-proxy · 代码审查报告（F013）

- 审查人：code-reviewer 代理（7 维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践）
- 审查日期：2026-09-02
- 审查范围：`feature/system-proxy` 分支全部未提交改动（F013 系统代理）
- 结论：**初版「需修复后合入」→ 修复后通过，可合入**

## 审查认可项

- env bridge 架构（一处注入覆盖 AI / libgit2 / LFS / updater 四条路径）经
  依赖源码逐一核实成立（reqwest 构建期读 env、libgit2 `http_proxy_env`
  每操作读 env 且保留 no_proxy 粒度、git-lfs 读大写变量、updater 插件
  默认路径不禁用代理）
- macOS 探测与 hyper-util 0.1.20 `matcher.rs` mac 模块逐 API 一致
- Windows 解析器比 hyper-util 更完善（per-scheme 格式、scheme 补齐）
- reqwest 0.13 feature unification 手法正确（依赖图核实单份构建）
- 错误码 + i18n（en/zh-CN parity）链路完整、模块文档注释质量高

## 发现与处置

| 级别 | 问题 | 处置 |
|---|---|---|
| 🔴 R1 | `set_env_var` 跳过条件不查询所有权：Manual 切回 System 后旧手动代理残留生效，与 UI 承诺矛盾 | ✅ 已修复：跳过条件加 `!ours`（自己注入的值可被 System 模式覆盖），新增 `bridge_manual_to_system_replaces_stale_manual_proxy` 测试锁定该序列 |
| 🟡 Y2 | Manual 强制覆盖用户 env 原值后，Off 只会删除变量，原值无法恢复 | ✅ 已修复：`INJECTED_VARS` 改存 `(name, Option<OsString>)` 原值记录，清除时恢复原值而非删除；`bridge_manual_overrides_then_off_restores_user_env` 测试覆盖 |
| 🟡 Y3 | 裸格式 `socks5://host:port` 未被过滤，会让 reqwest（无 socks feature）构建 client 失败 | ✅ 已修复：`with_proxy_scheme` 仅接受 http/https 显式 scheme，其余丢弃；补测试 |
| 🟡 Y4 | plan.md 承诺「IP 通配展开」与实现（丢弃）不符；`loopbackHint` 文案过度承诺 | ✅ 已修复：plan.md / F013 措辞改为如实描述（通配丢弃 + 局域网 HTTP(S) git 服务器列为已知限制）；i18n 文案改为「GitWave 应用的代理始终跳过本地地址」 |
| 🟡 Y5 | app settings 双连接无 busy_timeout，保存可能撞 SQLITE_BUSY | ✅ 已修复：`sqlite::open()` 增加 `busy_timeout(5s)`，两条连接同时受益 |
| 🟡 Y6 | `get/set_proxy_settings` 零单测；env 全局性导致状态机难测 | ✅ 已修复：① 拆出无副作用的 `validate_and_store_proxy_settings` 并补 5 个测试（默认回退 / URL 归一化持久化 / 非法 URL 拒绝 / Manual 空 URL 允许 / Off-System 分支）；② env 交互抽象为 `EnvAccess` trait（`ProcessEnv` 生产实现 + `FakeEnv` 测试实现），状态机 4 个测试不经真实 env |
| 🟢 | 运行期 `set_var` 与外部读者在 POSIX 上的理论竞态 | 已在 `apply_to_env` 文档注释与 plan.md 已知限制中如实记录（dotenv 同水平风险，保存事件罕见；启动路径单线程） |
| 🟢 | unification 依赖在 updater 升级 reqwest 0.14 时静默失效 | ✅ Cargo.toml 注释已加升级检查提示 |
| 🟢 | 损坏 JSON 静默回退默认值 | ✅ `get_proxy_settings` 增加 `tracing::warn!` |
| 🟢 | 保存失败后 `saved` 标志未复位 | ✅ 失败分支显式 `setSaved(false)` |
| 🟢 | Manual 模式 + 空 URL 立即报错，首点「手动」有惊吓感 | ✅ 语义改为「Manual 空 URL = 尚未配置，视为无代理」，仅非空且非法才报错 |
| 🟢 | Linux 小写 env 变量、`build_no_proxy` O(n²) 去重、URL 内嵌凭证明文存储 | 接受现状：libgit2/git-lfs 均读大写；n 极小；明文存储属 F013 已声明决策，已在提案「明确不做」记录 |

## 修复后验证（全绿）

- `cargo fmt` / `cargo clippy --all-targets -- -D warnings`：零警告
- `cargo test --all-targets`：277 passed / 2 ignored（较初版 +9）
- `tsc --noEmit`：零错误；`vitest run`：147 passed（含 locale parity）
- `eslint .` / `prettier --check .`：通过；`vite build`：成功

## 遗留（非阻塞）

- 真机手动验收清单见 plan.md「验证」节（Clash 系统代理 / 手动 / 关闭三档
  热切换、本地 Ollama 不走代理、更新检查可达），需用户在 Windows 真机
  过一遍
