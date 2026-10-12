# Native menu dialog dispatch review

Reviewed the native menu construction, replacement and resource disposal changes, plus the native-menu and real Modal regressions. Source verification used the installed Tauri 2.12.1 Rust menu implementation and JavaScript API. Related task: [plan.md](./plan.md).

## ✅ 优点

- **正确性**：显式创建子资源避免 inline payload 临时包装析构时移除 action channel；每代 UUID 避免旧菜单清理删除新菜单回调。逻辑 action 保留在闭包，菜单 ID 的变化不改变业务路由。
- **安全性**：不增加权限、网络传输或敏感数据暴露；变更沿用本地菜单与 UI store。
- **性能**：同一模块生命周期中，每次成功替换关闭旧 root 和子资源，只保留当前一套资源；菜单大小固定，额外 IPC 有限。
- **可维护性**：安装队列明确承担替换和释放职责。部分构建失败与 setAsAppMenu 失败均清理本次资源，并保留原安装状态。
- **可读性**：注释解释了 Tauri channel 所有权与 ID 命名空间原因，和实际依赖源码一致。
- **测试覆盖**：ID registry 回归验证旧 ID 删除后 Settings/workspace action 仍可用；hook 回归覆盖启动安装、StrictMode 和 gating 替换；真实 HeroUI Modal 测试覆盖打开、关闭和再次打开。
- **最佳实践**：显式 Resource 生命周期符合 Tauri API。setAsAppMenu 返回的 previous 是新 resource-table entry，和旧 installedMenu 的原始 entry 分别关闭合理。异步安装串行化防止 gating 更新乱序。

## 🔴 严重问题（必须修复）

无。未发现阻断此修复的正确性、安全或生产资源生命周期问题。

## 🟡 一般问题（建议修复）

- **位置**：`src/hooks/useNativeAppMenu.ts` 的模块级 installedMenu / installedResources / installQueue。
  **描述**：若开发环境热更新重新执行此模块，而原 webview 的 native resource table 仍存活，模块变量会重置。下一次安装只能关闭 setAsAppMenu 返回的旧 root entry，不能关闭丢失引用的原始 root entry 和显式子资源；旧资源可能保留至应用退出。此风险限于开发时模块重建，不影响普通 gating/language 更新或生产安装流程。
  **建议**：若需要支持这种热更新路径，用 `import.meta.hot.data` 保存资源所有权与安装队列，或明确要求修改菜单 hook 后重启开发进程。验证时区分 gating rebuild、模块热更新和完整进程重启。

## 🟢 优化建议（可选）

- **位置**：`src/hooks/useNativeAppMenu.test.tsx`。
  **描述**：主成功路径的回归充分，但创建中途失败和 setAsAppMenu 失败暂未由故障注入覆盖。
  **建议**：分别让 Item.new 与 setAsAppMenu 拒绝，确认本次已创建资源关闭、原菜单的 Settings channel 仍可用，后续安装不被 rejected queue 阻断。
- **位置**：`installMenu` 中的 Promise.allSettled 清理。
  **描述**：清理失败被吞掉，避免破坏安装，但真实资源释放异常缺少诊断信息。
  **建议**：如未来排查资源泄漏，可仅记录 rejected disposal 的资源类型和错误，保持最佳努力清理。

## 📝 总体评价

修复覆盖原生菜单无法打开弹框的两个独立原因，普通启动、StrictMode 与菜单重建的资源所有权清晰。用户已在完整重启的开发版确认 Settings 和添加仓库恢复；主任务报告最终 `make check` 通过（331 Vitest、4 Node、385 Rust passed / 2 ignored），生产 build 通过，新增 hook 测试已纳入检查。生产路径可以接受；开发时模块重建的资源滞留已在 plan 记录，保持为后续改进点。

References: [Tauri menu plugin](https://github.com/tauri-apps/tauri/blob/tauri-v2.12.1/crates/tauri/src/menu/plugin.rs), [Tauri resource Drop](https://github.com/tauri-apps/tauri/blob/tauri-v2.12.1/crates/tauri/src/menu/mod.rs).
