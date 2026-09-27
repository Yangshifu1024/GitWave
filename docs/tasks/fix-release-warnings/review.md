# Review: release warning cleanup

审查角色：`.agents/agents/code-reviewer.md`。审查范围：前端 React 状态 / ref / effect 重构、固定行高虚拟列表、Shiki 资源拆分、Vite 配置、ESLint 门禁及 Tauri CSP。对应计划：[plan.md](./plan.md)。

## ✅ 优点

- **正确性**：scope 变化的 render 状态重置均有变化条件；AI / Gitignore / Hooks / interactive rebase 对话框通过 key 隔离会话；旧异步响应保留取消或请求序列检查。分页切换查询时立即清空旧 cursor，避免 debounce 期间加载另一查询的后续页。
- **安全性**：高亮语法与 WASM 都来自应用内置资源，没有新增远程加载或 diff 外传。CSP 只增加同源连接和 WASM 编译权限，没有开启 JavaScript `unsafe-eval`。这与 [Tauri CSP 指引](https://v2.tauri.app/security/csp/) 的 WASM 要求一致。
- **性能**：高亮保留文件大小限制与语言懒加载；固定行高虚拟列表只输出可见范围和 overscan，滚动 / resize 通过外部存储订阅更新。主题异步高亮增加 generation 检查，晚完成的旧主题不会覆盖新主题。
- **可维护性**：移除 React lint 降级并用 `--max-warnings 0` 保护门禁；共享固定行高 hook 以不可变快照替代可变 virtualizer 实例，两个调用点遵循相同生命周期。
- **可读性**：状态 reset、DOM 生命周期与异步请求分别表达；字符高亮的 offset 计算移至映射前，不改变字符区间含义。
- **测试覆盖**：新增用例覆盖分页 debounce、拖拽顺序、AI 重开与仓库切换、认证字段清理、冲突轮询 scope、Split 方向以及 sidebar 刷新保留数据。tester 已完成固定行高 hook 的 7 项回归测试，覆盖 null → node 后定位、scroll / overscan、resize、count 变化和节点替换清理，全部通过。
- **最佳实践**：纯 render、条件状态调整、layout effect ref 同步及 `useSyncExternalStore` 的快照订阅边界清晰；未通过禁用 lint 规则或提高 bundle warning 阈值消除诊断。

## 🔴 严重问题（必须修复）

无未解决项。补审发现并已闭合：

- **位置**：`src-tauri/tauri.conf.json`、`src/lib/syntaxHighlighter.ts`、`vite.config.ts`。
  **描述**：原 `connect-src` 不允许新高亮 loader 获取同源 JSON / WASM，且没有 WASM 编译权限，生产 Tauri 环境可能静默降级为纯文本。
  **修复与复核**：增加 `connect-src 'self'` 和 `script-src 'self' 'wasm-unsafe-eval'`；核对产物 loader 使用 `/assets/<name>.grammar-<hash>.json`，仅加载打包的本地资源。
- **位置**：`vite.config.ts` 的 `build.rolldownOptions.output.codeSplitting`。
  **描述**：开发者执行生产浏览器 smoke 时，早期按 vendor `maxSize` 自动切割的配置触发 React / Shiki 初始化循环，实际 import 报 `n is not a function`；构建成功本身不足以保证可运行。
  **修复与复核**：改按依赖 package 分组，设置 `includeDependenciesRecursively: false`，并以更高 priority 单独提取 Vite runtime。审查者检查当前产物的 64 个 JavaScript 文件，其静态 import 图无环；高亮入口依赖独立 runtime 和 Shiki chunks，没有反向依赖 React 应用入口。最大 JavaScript 文件为 406,354 bytes。开发者在无头 Chrome 中 import 实际生产高亮模块，15 种语言 × 明暗主题全部通过，CSP 违规为 0；这些浏览器执行结果由开发者提供，审查者独立完成配置与产物依赖复核。

## 🟡 一般问题（建议修复）

无未解决项。审查期间以下问题已修复并复核：

- **位置**：`RemotesPanel`、`SubmodulesPanel`、`WorktreePanel`。
  **描述**：新增查询的 `historyEpoch` 变化曾令 `items` 临时变空，卸载 sidebar Disclosure 并重置用户折叠状态；操作错误也可能跨 scope 残留。
  **修复与复核**：仅在 workspace / repo 均相同时使用 previous query data，跨仓库不复用；scope 变化清除操作错误。
- **位置**：`useFixedVirtualizer`、`CommitGraph` locate effect。
  **描述**：初次挂载滚动容器时，effect 曾可能在 element state 就绪前将 locate 请求标为已处理，导致定位丢失。
  **修复与复核**：hook 接受显式 element；`CommitGraph` 在写入 `handledLocateSeq` 前检查 `scrollElement`，节点就绪后重试。订阅在节点替换 / 卸载时移除 scroll listener 并 disconnect ResizeObserver。

## 🟢 优化建议（可选）

无本次必须增加的优化。固定行高实现保持当前两个调用点的固定行高约束；以后引入动态行高时，应同时调整虚拟范围与定位算法。

## 📝 总体评价

代码审查通过，发现的 CSP、生产分包初始化循环、sidebar 刷新和初挂载定位问题均已闭合，没有未解决的阻断项。最终合入仍以本任务最终状态的 `make check` 与 `pnpm build` 结果为准；浏览器 smoke 与产物检查没有替代 Windows / Linux 原生 WebView 运行验证，也未重复声称执行了整套 Rust 门禁。

### Final gate evidence

Implementation owner completed `make check` using a fresh Rust target directory: zero warnings, 320 frontend tests and 385 Rust tests passed (2 existing ignored Rust tests). Production build passed with no warnings; no warning threshold was raised. Plain Chrome validated emitted highlighter resources with the application CSP; full desktop UI still requires a Tauri host and remains outside that browser smoke's scope.
