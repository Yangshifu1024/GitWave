# fix-updater-restart · Code Review

审查分支：`fix/updater-restart`。范围为本任务的全部未提交改动与新增测试；依据 [计划](./plan.md) 和 `.agents/agents/code-reviewer.md`，未修改产品代码。

## ✅ 优点

- 沿用 Tauri 2.12.2 的官方 macOS restart 修复，没有引入额外脚本或自定义启动实现。`Cargo.lock` 仅变更 `tauri` 的版本和 checksum。
- 重启失败保持 `ready`、目标版本和 release notes，重试直接重新请求 restart；跨 hook 的重复请求由共享 store 防护。失败提示与 pending 按钮标签均提供中文和 English。
- 真实隔离 `.app` fixture 调用 `tauri::process::restart`，测试旧实例退出并关闭输出 pipe 后，新实例仍能输出、拥有独立 process group，并保留参数；fixture 不会重启已安装的 GitWave。

| 维度 | 审查结果 |
|---|---|
| 正确性 | 依赖修复与 macOS 退出后没有新实例的现象一致；前端失败分支保留已安装状态、可重试且可重新显示隐藏的 modal。 |
| 安全性 | 延续已授权的 process plugin 与 capability，没有增加 shell 拼接、权限、数据外传或凭证接触。临时 bundle 路径隔离且 canonicalize 后交给 Tauri 的 executable 安全检查。 |
| 性能 | store 新增常量成本的布尔状态；没有轮询或额外网络请求。fixture 的等待有 15 秒上限。 |
| 可维护性 | 版本下限与 lockfile 同步；重启状态集中在既有 store，产品文档将改动明确登记为待发布。 |
| 可读性 | 失败与安装状态区分清晰，注释说明 retry 不会再次下载安装。 |
| 测试覆盖 | hook 覆盖失败重试、多个实例重复点击、非 ready 防护、隐藏 modal 后失败；组件覆盖 error、版本保留及按钮禁用。真实 fixture 补足 mock 无法证明的进程存活性。 |
| 最佳实践 | 使用官方 patch、既有 hooks/store/双语体系和真实进程回归；保持用户明确要求的本地修复范围。 |

## 🔴 严重问题（必须修复）

无。

## 🟡 一般问题（建议修复）

无未解决的一般问题。

审查中发现并已修复：`src-tauri/tests/updater_restart.rs` 原先由父进程在 spawn 后读取子进程的 process group，子进程可能先执行 restart 并退出，导致 `ps` 找不到 PID。最终代码改为首次 fixture 子进程在 restart 前自行写入 `first-group`；父进程不再查询这一 PID，移除了调度顺序依赖。

## 🟢 优化建议（可选）

- 正式发布验收时，使用签名 GitWave bundle 验证下载 → 安装 → restart 完整流程。隔离 fixture 已覆盖本次进程问题，但不覆盖签名、真实 updater 安装替换和已安装应用的环境。

## 📝 总体评价

修改范围集中，产品代码未发现阻断交付的问题，fixture 的进程时序问题已修复并复查。本地完整质量门禁已通过，发布结论仍应保留真实签名更新链路的验证边界；旧客户端使用旧进程执行的首次更新后 restart 仍可能需要手动打开一次。

### 验证证据与边界

- reviewer 独立执行 `node_modules/.bin/vitest run src/hooks/useUpdater.test.tsx src/components/UpdateModal.test.tsx src/stores/updaterStore.test.ts`：3 files、14 tests 全部通过。
- reviewer 执行 `git diff --check`：通过；静态核对本机依赖源中的 process plugin → `AppHandle::request_restart` → `tauri::process::restart` 路径与 2.12.2 的 LaunchServices / stdio / process group 实现。
- 主代理报告同一初始 fixture 的真实 macOS 对照：Tauri 2.12.1 在旧 pipe 关闭后未收到新实例 report，15.13 秒失败；2.12.2 新实例成功完成 report，0.22 秒通过。fixture 时序修正后已随完整 `make check` 再次通过（0.22 秒）。
- 主代理执行最终 `make check`：Prettier / rustfmt、Tauri npm/Rust 兼容、ESLint、clippy `-D warnings`、TypeScript、cargo check 全部通过；Node 4 tests、Vitest 53 files / 337 tests、Rust 385 tests + 1 真实重启测试通过，2 个既有 Rust 测试 ignored。
- 本审查没有执行 Windows / Linux 运行验证、远程 CI 或签名 GitWave 的实际更新链路，也没有提交、推送或重启用户应用。
