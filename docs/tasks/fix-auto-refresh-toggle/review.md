# fix-auto-refresh-toggle · 代码审查报告

> 审查人：code-reviewer（按 7 维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践）
> 审查对象：分支 `fix/auto-refresh-toggle` 全部未提交改动
> 结论：**无 🔴 严重问题，可合入**（🟡 ×1 已修复，🟢 ×4 采纳 1 项、其余保持现状）
> 实测：`npm run typecheck` / `npm run lint` / `npm test`（20 文件 133 用例）全绿

## ✅ 优点

- **修复正确且完整**。消费方已穷尽确认：全仓仅有 `App.tsx:60`（`useAutoRefreshLoop`，reader）与 `SettingsModal.tsx:124`（`useAutoRefresh`，writer）两处消费偏好；`Toolbar.tsx:7` 仅用 `useRefreshRepo`（手动刷新不受开关控制，属有意设计）。开关变化时：SettingsModal 调 store action → App 订阅重渲染 → `useAutoRefresh.ts` effect 依赖 `[autoRefresh, refreshRepo]` 变化 → cleanup `clearInterval` + 重跑守卫。无遗漏消费方、无残留写入点。
- **无虚假的 effect 重跑**。`refreshRepo` 为 `useCallback([queryClient])`，`QueryClient` 实例跨渲染稳定，interval 不会被无关重置；toggle off→on 时 60s 倒计时重新起算，行为合理。
- **防御层次合理**。tick 时用 `useAutoRefreshStore.getState()` 实时复核，无订阅开销、杜绝过期闭包回归；plan.md 已记录为有意双保险。
- **对外接口不变，改动面最小**。`{ autoRefresh, setAutoRefresh }` 签名保持，App / SettingsModal / Toolbar 零改动；localStorage key 不变，既有偏好无缝迁移。
- **与既有模式一致**。`create<AutoRefreshState>()(...)` 对齐 `uiStore.ts`；localStorage best-effort try/catch 对齐 `workspaceStore.ts` 的持久化约定。
- **测试覆盖关键行为且清理干净**：默认 off、模块加载读取持久化值、setAutoRefresh 双写、持久化抛异常时内存值仍生效；`afterEach(vi.unstubAllGlobals)` 防止全局 stub 泄漏。
- **stub 方案实测可靠**：先 `vi.stubGlobal` 后动态 import 的顺序正确；即便裸 node 下 `window` 未定义，`readStoredAutoRefresh` 的 try/catch 也会降级为 `false`，模块导入不崩溃。
- **plan.md 根因分析与代码逐条吻合**（含 e56d440 引入溯源、范围外轮询的显式排除）。

## 🔴 严重问题（必须修复）

无。

## 🟡 一般问题（建议修复）

- **位置**：`docs/tasks/fix-auto-refresh-toggle/plan.md`（决策记录「单测」行）
- **描述**：原表述「对齐 `uiStore.test.ts` 模式」不准确——`uiStore.test.ts` 是静态 import 且无需 stub；本测试因 store 在模块求值期读 localStorage，必须 `vi.stubGlobal` + `vi.resetModules()` + 动态 import，会误导后来者。
- **处理**：✅ 已修复（plan.md 措辞已改为准确描述机制差异）。

## 🟢 优化建议（可选）

1. **`vite.config.ts` 显式声明 vitest 环境**（原建议：测试注释依赖未固化的 node 默认值）→ **✅ 已采纳**：新增 `test: { environment: "node" }` 并附注释说明 store 测试自行 stub localStorage。
2. **store 模块求值期读 localStorage 为本仓库 store 首例**（其余 store 多为惰性读取或 `typeof` 守卫）→ **保持现状**：审查确认 Tauri WebView 中 localStorage 于任何脚本执行前同步可用，try/catch 已兜底，无实际风险。
3. **测试中 `STORAGE_KEY` 字面量重复三次**（store 未导出常量）→ **保持现状**：测试绑定真实持久化契约，key 变更会被测试捕获，属特性而非缺陷。
4. **hook 层行为（effect 清除/重建定时器、tick 复核）无自动化测试**（测试栈无 testing-library，plan.md 已如实声明）→ **记为后续项**：未来引入 `@testing-library/react` 后，用 fake timers 补「toggle off 后 interval 不再触发 refreshRepo」的 effect 级测试。

## 📝 总体评价

最小化修复质量高：根因诊断准确（组件级 useState 割裂 App 与 SettingsModal 状态），方案（zustand store 共享 + 接口不变 + tick 防御复核）正确完整且与代码库既有约定一致，store 层测试覆盖全部关键行为并实测全绿。无阻塞合入的问题。合入前按 plan.md 冒烟清单完成手动验证（关闭开关等待 > 1 分钟、重启后偏好保持）。
