# F007 · macOS 原生系统菜单

## 背景

当前 GitWave 在顶栏内自绘菜单（File / Workspace / Repository / Branch，见 `docs/tasks/feat-app-menubar/plan.md`）。macOS 用户习惯应用菜单位于屏幕顶部系统菜单栏（Apple 菜单右侧），应用窗口内的自绘菜单条不符合 macOS 平台惯例，也与红绿灯、全屏、系统菜单交互（如帮助搜索）不兼容。

## 提议方案

- 仅 macOS：应用内顶栏菜单迁移为系统菜单栏原生菜单（Tauri 2 menu API），窗口内不再渲染菜单条。
- 系统菜单结构：
  - 应用菜单「GitWave」：About GitWave、Settings…（⌘,）、Hide / Hide Others / Show All、Quit GitWave（⌘Q）
  - Workspace / Repository / Branch 三个子菜单：条目、可用状态（gating）与现有应用内菜单逐项对齐（Delete 的破坏性标红为应用内菜单样式；系统菜单不支持着色，仅条目对应）
  - 现有 File 菜单在 macOS 移除（其 Settings / About / Exit 三项归入应用菜单，Exit ≡ Quit）
- Windows / Linux：维持应用内菜单现状不变。
- 菜单项点击复用现有 `AppMenuAction` 请求总线（ActionBar handler 零改动），保证菜单与应用内按钮行为一致（P1 同源）。

## 影响

- 涉及模块：前端菜单（`AppMenuBar` / `Toolbar`）、新增原生菜单 hook、`@tauri-apps/api/menu`（已有依赖）、`capabilities/default.json` 加 `core:menu:default`；Rust 侧零改动
- 影响版本：v0.3.x
- 是否破坏向后兼容：否（Windows / Linux 行为不变；macOS 菜单入口能力等价迁移）

## 补遗（2026-08-29 真机验证后修订）

真机验证发现：整体替换 Tauri 默认菜单后，文本编辑快捷键（⌘C / ⌘V / ⌘A / ⌘Z）与窗口快捷键（⌘M）失效——Webview 文本控件依赖系统 Edit 菜单角色转发。经用户确认，补入 **predefined-only** 的标准 Edit / Window 菜单（Edit：Undo / Redo / Cut / Copy / Paste / Select All；Window：Minimize / Zoom / Enter Full Screen（⌃⌘F）/ Bring All to Front）。全部为系统预定义项：无自维护条目、无 gating、无新增 dispatch，仅恢复 macOS 系统行为；原「不追加 Edit / Window」的范围决定据此修订。

同时记录默认菜单中被有意省略的项：Close Window（⌘W）——主窗口即应用，关闭窗口等同退出，避免与 ⌘Q 语义重叠；Services 子菜单——GitWave 无系统服务消费场景。如后续需要，二者均可按同模式补 predefined 项。

## 决策

- 状态：接受
- 决策人：用户（yangzhenbiao）
- 决策日期：2026-08-29
- 关联决策：范围确认为「仅迁移现有菜单」，不追加 Edit / Window / Help 标准 macOS 菜单；分支 `feature/macos-native-menu`；关联 [F005](./F005-repo-tab-drag-reorder.md) 同类平台适配经验
