# feat-macos-native-menu · macOS 应用内菜单迁移系统菜单栏

对应提案：[F007](../../pm/features/F007-macos-native-menu.md)。分支：`feature/macos-native-menu`。

## 需求

macOS 上把应用内顶栏菜单（File / Workspace / Repository / Branch）迁到系统菜单栏原生菜单；Windows / Linux 保持现状。用户确认范围：仅迁移现有菜单，不追加 Edit / Window / Help 标准菜单。**（修订）**真机验证发现快捷键回归后，已按用户确认补入 predefined-only 的 Edit / Window 菜单，见 [F007 补遗](../../pm/features/F007-macos-native-menu.md)。

## 现状

- 应用内菜单：`src/components/AppMenuBar.tsx`，条目只发 `AppMenuAction`（`src/stores/uiStore.ts:9-26`），`ActionBar.tsx:521-594` 的 switch 拥有全部 handler。
- gating 状态：`noWorkspace / noRepo / detached` 来自 `useWorkspaceUiStore` + `useWorkingCopy`（`AppMenuBar.tsx:93-100`）。
- Tauri 2.11.5；`@tauri-apps/api` 2.11.1 自带 `menu` 模块；capabilities 无 `core:menu:*`；Rust 侧无菜单代码（macOS 现为 Tauri 默认菜单）。
- macOS 窗口为 Overlay titlebar + 红绿灯位移 + 自定义拖拽（`useMacTitlebarWindow`），菜单条移除后 toolbar 仍是拖拽区，无需改布局。

## 方案

1. **共享 spec** `src/lib/appMenuSpec.ts`：纯函数 `buildAppMenuSpec(gating)` 把四个菜单条目数据化（`textValue` / `label` / `action` / `enabled` / `destructive` / `shortcut`）；File 组（Settings / About / Exit）单独导出供 win/linux 用。gating 输入 `{ noWorkspace, noRepo, detached, hasSha, syncBusy }`，逐条对齐 `AppMenuBar.tsx:98-100, 257-298` 现状。
2. **重构 `AppMenuBar.tsx`**：消费 spec 渲染，1:1 映射行为不变；此后仅非 macOS 挂载。
3. **新增 `src/hooks/useNativeAppMenu.ts`**（核心）：
   - 仅 `isMacOS()` 激活；用 `@tauri-apps/api/menu` 构建：
     - 应用菜单「GitWave」：About GitWave / — / Settings… `CmdOrCtrl+,` / — / Hide、Hide Others、Show All（predefined）/ — / Quit（predefined `quit`，⌘Q，替代 Exit 的 `quitApp()`）
     - Workspace / Repository / Branch 子菜单，文案、enabled、Delete destructive 对齐应用内菜单
   - 点击：17 个 action → `requestMenuAction`；Settings → `setSettingsOpen(true)`；About → `setAboutOpen(true)`
   - gating 变化时重建菜单 + `setAsApplicationMenu()`；卸载 `menu.close()`
4. **`Toolbar.tsx`**：macOS 条件挂载 `NativeAppMenu` 桥接组件（内调 `useNativeAppMenu`），Windows / Linux 不产生 gating 订阅。（修订）⌘, keydown 最终为全平台注册——原生 accelerator 存在时系统先消费该键；安装失败时它兜底，`setSettingsOpen(true)` 幂等。
5. **权限**：`src-tauri/capabilities/default.json` 加 `core:menu:default`；Rust 侧零改动。（修订）review 后收敛为细粒度 `core:menu:allow-new` + `core:menu:allow-set-as-app-menu`。

> **方案修订（review 二轮后）**：原生菜单纯构建器抽至 `src/lib/nativeMenuBuild.ts`（含汇总函数 `buildNativeAppMenuOptions` 与 predefined-only 的 Edit / Window 菜单，见 F007 补遗）；`main.tsx` 在 React 挂载前调 `installNativeAppMenuEarly()` 早期安装，与 hook 共用模块级串行队列；菜单点击统一经 `dispatchAppMenuItem` 路由。

## 验证清单

- [x] `npm run typecheck && npm run lint && npm test`（106 passed）&& `npm run format:check`；`cargo check` 验证 capabilities
- [x] `npm run tauri dev`（macOS）应用启动运行正常（构建 + 运行 3 分钟无报错退出）
- [ ] 系统菜单栏人工核对（GUI 自动化因授权受限未完成，待用户验证）：
  - [ ] 菜单栏出现 GitWave / Workspace / Repository / Branch
  - [ ] 条目 enabled 随 workspace / repo 状态变化（无 workspace 时 Workspace 除 New/Import 全灰、无 repo 时 Repository/Branch 相应灰、detached / syncBusy 灰）
  - [ ] 各 action 打开与 ActionBar 相同的对话框（新建 workspace、clone、fetch、LFS、hooks、worktree、PR 等）
  - [ ] ⌘, 只触发一次 Settings；About 打开；⌘Q Quit 正常
  - [ ] ~~各文本输入框 ⌘C / ⌘V / ⌘A / ⌘Z 可用~~ → 首轮真机验证**失效**；已补 predefined-only Edit / Window 菜单（见 F007 补遗），**待复验** ⌘C / ⌘V / ⌘A / ⌘Z / ⌘M / ⌃⌘F
  - [ ] ⌘A 在变更面板全选文件（既有行为，predefined Select All 与组件 handler 共存性顺带确认，防日后误判为新缺陷）
  - [ ] 红绿灯位置与标题栏拖拽、双击 zoom 不受影响
- [x] 非 macOS 路径 no-op（`isMacOS()` 守卫），应用内菜单原样，CI 把关
- [x] 单测：`src/lib/appMenuSpec.test.ts` gating 矩阵 + dispatch 路由（8 用例）

## 约束

- AI 不 commit / push / merge（P1），由用户执行
- 菜单重建仅发生在 gating 变化时（此时菜单通常已关闭），替换 NSMenu 闪烁风险低
