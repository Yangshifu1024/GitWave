# fix-git-fetch-system-proxy · 代码审查报告

- 审查人：code-reviewer 代理（7 维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践）
- 审查日期：2026-09-03
- 审查范围：`fix/git-fetch-system-proxy` 分支全部未提交改动（`.gitignore` 为用户自有改动，不在范围）：`remote.rs`（+39）、`repo_adapter.rs`（+2）、`submodule.rs`（+9/-2）
- 结论：**通过，可合入**（0 🔴 / 1 🟡 建议修复 / 4 🟢 可选）

## 审查认可项

- **接线完整性**：全仓 grep `FetchOptions` / `PushOptions` / `SubmoduleUpdateOptions` / `RepoBuilder` / `connect(` 核实，libgit2 网络入口恰好五处（`fetch`、`push_with_options` 含隔离重试闭包、`delete_remote_branch`、`clone_with_creds`、`update_submodule` / `add_submodule`），全部挂上 `attach_auto_proxy`；无 `remote.connect` / 匿名 remote 等遗漏路径。`branch.rs:669` 的 `find_remote` 仅读配置不建网，LFS / credential helper 子进程路径由 F013 env 桥接覆盖——边界正确。
- **trait 设计与生命周期健全**：`ProxyOptionsCarrier<'cb>` 的 `'cb` 是结构性生命周期（镜像 git2 0.20 `FetchOptions<'cb>` / `PushOptions<'cb>` 的 bound），`ProxyOptions` 经 by-value move 进 options，之后无任何使用点——use-after-move / 悬垂由编译器排除（clippy -D warnings 全绿佐证）。`proxy.auto()` 返回 `&mut Self`，代码分两步 `let mut proxy = ProxyOptions::new(); proxy.auto();` 再 move，规避了临时借用陷阱，写法正确。`pub(super)` 可见性恰好覆盖同模块树内两个调用方，不外泄 API。
- **`GIT_PROXY_NONE → GIT_PROXY_AUTO` 语义恢复正确**：AUTO 下 libgit2 按 git CLI 顺序解析 `http.<url>.proxy` / `http.proxy` config → `HTTPS_PROXY` / `HTTP_PROXY` env，与 `docs/tasks/fix-git-fetch-system-proxy/plan.md` 声明一致。
- **F013 三档行为符合 plan.md**：Off → `resolve()` 返回 None，env 被还原，无 config 时直连；System/Manual → env 注入后 AUTO 生效；`NO_PROXY` 恒含 `localhost,127.0.0.1,::1`（`proxy.rs` `build_no_proxy`），loopback 豁免成立。SOCKS-only 丢弃、Windows IP 通配丢弃两条已知限制在 plan.md 如实记录，未过度承诺。
- **回归面控制良好**：`attach_auto_proxy` 只动 proxy 字段，不触碰 `RemoteCallbacks` —— 进度回调、取消标志（transfer_progress 返回 false 的中止 lever）、approve/reject 凭据生命周期均不受影响；push 重试闭包每次 attempt 新建 `PushOptions` 并重新挂代理，重试路径不漏。SSH 远程走 libssh2 传输，libgit2 代理选项仅作用于 HTTP(S) 传输，SSH 路径行为不变。credential helper 子进程继承进程 env，`git credential fill` 本身不建网，407 场景已被 `run_with_credentials` 文档注释覆盖。

## 发现与处置

| 级别 | 问题 | 处置 |
|---|---|---|
| 🟡 Y1 | **无测试锁定代理接线**（测试覆盖）。`attach_auto_proxy` 是否真的把 kind 置为 `GIT_PROXY_AUTO`，现有 277 个测试都无法感知——本地 fetch/push/clone/submodule 集成测试在 `GIT_PROXY_NONE` 与 `AUTO` 下行为一致（无代理可用时两者都直连），若未来重构漏掉一处挂接或 helper 回归，测试全绿但缺陷复现（正是本次修复的 bug 形态）。**已尝试 raw 断言护栏：git2 0.20.2 的 `Binding` trait 与 `raw` 模块均私有（docs.rs/git2/0.20.2 crate 根导出清单核实），无法在测试中访问；master 分支已公开（src/proxy_options.rs 的 `Binding for ProxyOptions`），git2 升级包含该提交后即可补上 `kind == GIT_PROXY_AUTO` 断言** | 建议（不阻塞合入）：当前以单一代入点（`attach_auto_proxy`）+ plan.md 真机 Clash 验收兜底；git2 升级后补 raw 断言测试 |
| 🟢 Z1 | `update_submodule` 的注释「`.fetch` implies allow_fetch(true)」依据是 git2-rs 0.20 源码行为（`fetch()` 同时置 `allow_fetch = true`；即便不置，`SubmoduleUpdateOptions` 的该字段默认即 `true` 且无人关闭），语义等价成立。但该断言属对依赖内部行为的记忆性依赖，建议注释补 git2-rs 源码引用（URL）以便后续升级 0.21+ 时复核 | 可选：补引用 |
| 🟢 Z2 | 行为变化未在用户可感知层面记录：AUTO 模式会开始尊重用户 repo/global config 里的 `http.proxy`（此前 `GIT_PROXY_NONE` 一律直连）。这与 git CLI 语义一致、plan.md 已声明「config > env 属预期」，但对「global config 里有陈旧 `http.proxy`」的用户，fetch/push 将从「能直连」变为「按陈旧代理失败」。建议合并后 release note 提一句 | 可选：记录于用户可见变更 |
| 🟢 Z3 | `ProxyOptionsCarrier` 抽象（trait + 2 impl + 泛型函数）对仅两实现、单一调用形态的场景略重；两个直调函数（fetch/push 各一）等价且更短。trait 胜在文档化意图与可扩展（未来 `RepoBuilder` 外的 options 类型），属风格取舍，无问题 | 可选：保持现状即可 |
| 🟢 Z4 | 每次操作新建 `ProxyOptions` 的开销可忽略（结构体初始化 + FFI 拷贝），无性能问题；仅备注 | 无需处理 |

## 验证

- 采信开发侧报告（本审查未重复运行套件）：`cargo fmt` / `cargo clippy --all-targets -- -D warnings` 零警告；`cargo test --all-targets` 277 passed / 2 ignored；`tsc --noEmit` 零错误；`vitest run` 148 passed。前端零改动，与 diff 一致。
- 审查后开发侧补充：Y1 raw 断言护栏测试已尝试落地，因 git2 0.20.2 `Binding` trait / `raw` 模块私有无法访问（E0433/E0603），已撤回；待 git2 升级后重试（见 Y1 处置与「遗留」）。
- 静态核查（本审查执行）：diff 全量逐行审阅；全仓 grep 确认五处接线无遗漏、无其他 libgit2 网络构造点；`attach_auto_proxy` 三个调用文件（`remote.rs:204/339/555`、`repo_adapter.rs:96`、`submodule.rs:101/149`）挂接位置均在 `remote_callbacks` 之后、操作调用之前，字段互不干扰。
- 待用户执行：plan.md「验证」节真机验收（Windows + Clash 系统代理，fetch/push 在代理工具连接日志可见）——同时补上 F013 review 遗留验收项。

## 遗留（非阻塞）

- Y1 护栏测试待 git2 升级（`Binding`/`raw` 公开后）补上；真机三档热切换验收（跟随系统 / 手动 / 关闭）见 plan.md，需用户在 Windows 真机过一遍。
- plan.md 已知限制（SOCKS-only 不适用 git 路径、`192.168.*` 类通配被丢弃致局域网 HTTP(S) git 服务器被代理）沿袭 F013，待 libgit2 / hyper-util 能力变化后再评估。
