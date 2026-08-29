# feat-macos-native-menu · Code Review 报告

对应提案：[F007](../../pm/features/F007-macos-native-menu.md) · 方案：[plan.md](./plan.md) · 分支：`feature/macos-native-menu`

## 审查方式

code-reviewer 只读审查 `git diff main` 全部改动（含 6 个新增文件），按 AGENTS.md 七维度（正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践）执行；并对照 tauri 2.11.5 与 @tauri-apps/api 2.11.1 源码验证了 menu 生命周期行为（`set_as_app_menu` 会把 previous 以新 rid 重新入表、`ResourceTable::close` 对已关 rid 报错、`Menu.new` 单次 IPC 建整棵树）。

## 审查结论

**可合入**。未发现 🔴 阻断问题；spec 抽取使双端行为同构，gating 重建时机（useMemo 原始值依赖，避免 2s 轮询触发重建）与 StrictMode 双挂载清理经源码级验证正确，⌘, 单触发设计正确（NSMenu key equivalent 在系统层被消费）。

## 🟡 建议修复项及处置（首轮 review）

| # | 问题 | 位置 | 处置 |
|---|---|---|---|
| 1 | `setAsAppMenu` 整体替换 Tauri 默认菜单，默认 Edit / Window 项（Copy/Paste/Minimize 等）消失，macOS 文本快捷键 ⌘C/⌘V/⌘A/⌘Z 是否仍由 WKWebView 自行处理待确认 | `useNativeAppMenu.ts` | ✅ **已修复**（用户真机确认快捷键失效）：新增 `src/lib/nativeMenuBuild.ts` 补 predefined-only 的标准 Edit 菜单（Undo/Redo/Cut/Copy/Paste/Select All）与 Window 菜单（Minimize/Zoom/Bring All to Front），全部系统预定义项；F007 已记补遗 |
| 2 | 两次重建的 `set_as_app_menu` IPC 并发乱序，可能把旧 gating 的菜单装回 | `useNativeAppMenu.ts` | ✅ 已修复：install 队列串行化（模块级 `installQueue`，启动安装与 hook 重建共用）+ 安装后复查 stale |
| 3 | `core:menu:default` 超出最小权限集（放开 popup / remove 等全部命令） | `capabilities/default.json` | ✅ 已修复：改为 `core:menu:allow-new` + `core:menu:allow-set-as-app-menu` |
| 4 | 否定式 `isAppMenuAction` + dispatch 兜底 `else quitApp()`：将来加 special id 漏改会静默退出应用，且两处路由不对称 | `appMenuSpec.ts` / `AppMenuBar.tsx` | ✅ 已修复：`isAppMenuAction` 改穷尽 switch（default 分支赋值编译期拦截新增 special id）；新增 `dispatchAppMenuItem` 集中路由，应用内菜单与原生菜单共用，quit 显式传入 |
| 5 | Makefile（fc581e0）与本任务无关，混入同一分支 | 分支提交 | 该提交为用户在并行会话所加（AI 未做任何 commit）；如需拆分：`git rebase --onto main fc581e0 feature/macos-native-menu`（会丢该提交，Makefile 另行在 main 提交）；否则 PR 描述中单独说明 |

## 🟢 可选项及处置（首轮 review）

- ✅ 注释纠偏：destructive 标红仅应用内菜单支持，原生菜单不着色；F007 措辞已同步。
- ✅ `callbacks.current` 写入从 render 期移入 effect（声明在 install effect 之前，保证同 commit 内先更新）。
- ✅ ⌘, keydown 监听保持全平台注册：原生 accelerator 存在时系统先消费该键、webview 收不到；原生安装失败时它充当兜底，且 `setSettingsOpen(true)` 幂等无副作用。
- ✅ 单测补充 `dispatchAppMenuItem` 路由断言（含「每个 spec id 恰好路由一次」）。
- ✅ 纯构建器抽到 `src/lib/nativeMenuBuild.ts`（tauri api 仅 type-only，运行时零依赖）并补单测：菜单排序与 File 剔除、app 菜单结构（About / Settings ⌘, / predefined 尾部）、Edit/Window predefined 结构、entry→MenuItemOptions 映射（label/enabled 随 gating）、action 经 dispatch 到 handler。
- ✅ 非 macOS 双订阅消除：`Toolbar` 内建 `NativeAppMenu` 桥接组件按平台条件挂载，Windows / Linux 不再执行 hook 与 gating 订阅。
- ✅ 默认菜单闪现消除：`main.tsx` 挂载 React 前调用 `installNativeAppMenuOnce()`（macOS 守卫、与 hook 共用串行队列），窗口 `visible: false` 至 `activate_and_show` 才显示，默认菜单在窗口可见前已被替换；hook 挂载后以 live gating 重建接替。

## 验证记录

- `npm run typecheck` / `lint` / `test`（106 passed）/ `format:check` 全绿；`cargo check` 通过（capabilities 细粒度权限合法）。
- `npm run tauri dev`（macOS）构建成功、应用运行正常（约 3 分钟后正常退出，exit 0）。
- 系统菜单栏逐项人工核对未完成：ZCode Computer Use 缺 Accessibility 与屏幕录制授权（`request_access` 返回 denied）。待用户按 plan.md 验证清单核对。

## 第二轮 review（2026-08-29，复查修复落地）

审查方式：对照 tauri 2.11.5 / @tauri-apps/api 2.11.1 / muda 0.19.3 源码（含项目构建产物 acl-manifests.json）逐项验证，重跑 14 个菜单单测。结论：**可合入**，无 🔴。

首轮处置核实：5 项 🟡 全部真实落地——Edit/Window 走 muda predefined 选择器（`undo:`/`cut:`/`copy:`/`paste:`/`selectAll:`/`performMiniaturize:`/`performZoom:`）转发首响应者，快捷键恢复路径成立；模块级队列在 E1（早期安装）→ H1a/H1b（StrictMode 双挂载）→ restore 重建交错下最终态正确；`Menu.close()` 走 `core:resources:default` ⊂ `core:default`，最小权限集经 acl-manifests.json 确认充分；`set_as_app_menu` 将 previous 以新 rid 重入表，双 close 不冲突；桥接组件无 hooks 规则问题。

第二轮 🟡 处置：

| # | 问题 | 处置 |
|---|---|---|
| 1 | `installNativeAppMenuOnce` 冷启动读 workspaceStore 是「死读」（store 初始恒 null，真实恢复在 App 挂载后 IPC） | ✅ 已修复：改用同步 `readLastActive()`（localStorage 持久化的真实冷启动值），并删除误导性注释 |
| 2 | 相比 main 丢失 Enter Full Screen（⌃⌘F）且未记录；Close Window / Services 省略未补记 | ✅ 已修复：Window 菜单补 `predefined("Fullscreen")`；F007 补遗记录 ⌘W / Services 的有意省略及理由；plan.md 验证清单加入 ⌃⌘F |

第二轮 🟢 处置：

- ✅ 队列毒化防护：`enqueueInstall` 改 `then(install, install)`，单个失败不再使队列永久失效。
- ✅ 注释纠偏：predefined 项标题为 muda 提供（仅 Hide/Quit/About 本地化，其余英文，与 Tauri 默认菜单一致），不再称 system-localized。
- ✅ 文档漂移：plan.md 需求行与 Toolbar 方案行加修订标记；测试计数更正为 106。
- ✅ `installNativeAppMenuOnce` 更名 `installNativeAppMenuEarly`（无 once 守卫，命名贴合语义）。
- ⏸ 队列串行化 / stale 复查 / cleanup 交互的 hook 级单测：项目无 React hook 测试设施（无 testing-library），引入超出本任务范围；队列逻辑已由二轮源码级推演覆盖，记为后续可选项。
- ⏸ ChangesPanel ⌘A 全选文件与 predefined Select All 共存：main 默认菜单同样含 Select All，属既有行为非本分支回归；已加入 plan.md 顺带验证清单。

## 备注

- 全程未执行 commit / push / merge（P1）；分支上 fc581e0 为用户自行提交的 Makefile。
