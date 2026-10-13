# fix-updater-restart · 更新后的 macOS 重启

## 问题与证据

用户报告：macOS 更新安装完成后点击「立即重启」，旧实例退出，但新实例没有打开，需手动启动。

- `src/hooks/useUpdater.ts` 调用 process plugin 的 `relaunch()`；后端已注册插件，main window capability 已允许 `process:allow-restart`，没有退出拦截。
- 修复前锁定 Tauri 2.12.1。其 macOS bundle 重启直接 spawn executable，继承旧实例的 stdio / process group 后退出。
- [Tauri 2.12.2 官方记录](https://tauri.app/release/tauri/all-versions/#2122) 已修复这一缺陷：通过 LaunchServices 启动 bundle，fallback 也隔离 stdio / process group。该机制与用户现象吻合；现有日志不足以独立确认用户这一次失败的进程原因。
- 前端原先没有捕获 relaunch rejection；即使 IPC 失败，也没有可见错误与重试反馈。

## 修改

1. 将 Tauri 最低版本提高到 2.12.2，并只更新 lockfile 的 Tauri 条目，沿用官方修复。
2. 重启请求进行中禁用按钮，并通过 store 防止多个 hook 重复提交。
3. 捕获请求失败，显示中英错误提示；保留 ready、版本与 release notes，再次点击只重试重启，不重新检查或下载安装。
4. 添加 hook / 组件回归测试与真实 macOS 临时 `.app` 重启测试。fixture 调用项目实际使用的 `tauri::process::restart`，验证旧实例退出、输出管道关闭后，新实例仍能写 stdout / stderr、位于独立 process group，且启动参数被保留。

## 验证

- 对照同一隔离 fixture 在 Tauri 2.12.1 与 2.12.2 下的结果。
- 前端测试覆盖失败后重试、跨 hook 重复点击、未安装时不重启、隐藏 modal 后失败重新显示，以及按钮 / 错误提示。
- `make check`：格式、Tauri 版本兼容性、lint、typecheck、Rust check、前后端全部测试。
- code-reviewer 审查七个维度，结果记录于 `review.md`。

## 已执行验证

- 同一真实 macOS bundle fixture：Tauri 2.12.1 在旧实例退出、关闭输出管道后没有成功报告，测试失败（15.13 秒）；Tauri 2.12.2 完成重启并通过全部断言（0.22 秒）。
- 前端相关 3 个测试文件、14 个测试通过。
- 最终代码的 `make check` 全部通过：Prettier / rustfmt、Tauri npm/Rust 版本兼容、ESLint、clippy `-D warnings`、TypeScript、cargo check；Node 4 个测试、Vitest 53 个文件 / 337 个测试、Rust 385 个测试 + 1 个真实重启测试通过（另有 2 个既有 ignored 测试）。最终 fixture 已修复 process group 记录竞态，随全量门禁重跑通过（0.22 秒）。
- code-reviewer 七维审查通过，无未解决问题；独立运行 14 个相关前端测试通过。详见 [审查报告](./review.md)。

## 发布与验证边界

本任务是本地修复，按用户要求完成本地提交；不升 GitWave 版本、不推送、不发布。README 与官网的 v0.10.1 仍指向现有发布，scope / roadmap 的已发布版本记录保留；产品功能页登记待发布修复。

隔离 bundle 验证进程重启机制；它不等同于已发布签名 GitWave 的下载 → 安装 → 重启完整验收。旧客户端用旧进程执行升级后的第一次重启，仍可能需要手动打开一次；新重启机制在运行修复后的版本后生效。
