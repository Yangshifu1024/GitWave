# review: fix-console-flash(提交 b07f532)

审查方式:code-reviewer 子代理因模型提供者未配置(builtin:bigmodel-coding-plan)无法运行,
由主代理按同样维度复核;改动范围小(1 个新文件 + 3 处调用点),已逐行审读完整 diff。

## 结论

**通过** — 无 🔴 问题。

## 七维度复核

### 正确性(✅)

- `CREATE_NO_WINDOW = 0x08000000` 与 Windows SDK 定义一致;`creation_flags` 语义为"子进程不分配控制台",
  对 GUI(subsystem: windows)应用 spawn 控制台程序正是所需,与 `CREATE_NEW_CONSOLE` 等标志无冲突。
- `#[cfg(windows)]` 覆盖常量与 `CommandExt` 导入;非 Windows 平台上 `hide_console_window` 是空操作,
  行为与修复前完全一致(之前的代码本来就无此标志)。
- `hidden_command` 返回 `Command` 保持链式风格,`credentials.rs` / `keys.rs` 的
  `.args().stdin().stdout().stderr().spawn()/.output()/.status()` 均正常,`cargo check` 通过。
- 全仓库 Spawn 点核实:仅 `credentials.rs::query_helper`(git credential fill)与 `keys.rs`
  (ssh-add ×3、ssh -T)共 4 处,全部已改走 `hidden_command`;push 本身经 vendored libgit2,无其它 spawn。

### 安全(✅)

- 所有进程 stdin/stdout/stderr 仍显式设置(pipe/null),`CREATE_NO_WINDOW` 不影响管道行为;
  无新增命令注入面,命令与参数未变。

### 性能(✅)

- 仅多一次 `creation_flags` 调用,无影响。

### 可维护性(✅)

- 统一入口 `hidden_command` 沉淀为团队约定,未来新增 spawn 点有明确落点;`hide_console_window`
  亦 pub 可复用(暂无外部调用,保留作为扩展点,🟢)。

### 可读性(✅)

- 注释解释了"GUI 不闪控制台"意图;常量命名与 Windows API 对齐,阅读成本低。

### 测试覆盖(🟡)

- `process.rs` 无单元测试:CARGO 目标平台是 Windows-only 行为,难以在 CI(Linux)断言,🟢 级可选;
- 现有 `credentials.rs` / `keys.rs` 测试不受影响(`query_helper_returns_none_for_unconfigured_helper`
  在 Linux CI 上同样通过——修复前即存在,未改变其运行环境)。

### 最佳实践(✅)

- 使用 `std::os::windows::process::CommandExt::creation_flags` 是 Windows 下官方推荐做法;
  `cfg(windows)` 条件编译将平台差异限制在单文件内,架构上正确。

## 风险提示

- 修复需重新构建(build 出的 exe / tauri dev)后才生效;若用户此前运行旧二进制,弹窗仍会复现,属预期。
- 若用户使用 SSH 远端:libssh2 内置传输不 spawn 系统 ssh,本修复不涉及该路径;UI 的 ssh key 管理
  面板(ssh-add -l / ssh -T)已由本提交覆盖。
