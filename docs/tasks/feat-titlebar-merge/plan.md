# feat-titlebar-merge · 操作栏并入自绘标题栏

> 状态：已完成
> 需求（用户）：把 toolbar 移到自绘标题上，注意避开 macOS 的交通灯和 Windows 的 controls。
> 澄清：用户选择 `ActionBar` + `AppMenuBar` 合并为一行——菜单靠左、操作靠右、中间为拖拽区与居中的状态区。

## 目标

- 顶部只保留一条 40px 自绘标题栏（`TitleBar`），移除原先 Toolbar（仅菜单/拖拽）与
  ActionBar（操作条）两行结构。
- 系统窗口控件不被遮挡、不被覆盖：macOS 红绿灯（左）、Windows / Linux 窗口控制按钮（右）。
- 状态区弱化，与标题栏背景融为一体。

## 改动清单

### 1. 标题栏外壳（新增 `src/components/TitleBar.tsx`，删除 `src/components/Toolbar.tsx`）

`TitleBar` 为纯展示外壳，接收 `children`（操作控件）与 `overlay`（窗口居中的状态层）：

- 拖拽区 `absolute inset-0 z-0` 位于内容**下方**；内容层 `pointer-events-none`，
  交互分组各自 `pointer-events-auto` —— 空白处可拖动、按钮可点击（沿用原 Toolbar 的模式）。
- 菜单：Windows / Linux 渲染 `AppMenuBar`（栏内左）；macOS 走原生系统菜单（`useNativeAppMenu`）。
- `SettingsModal` / `AboutModal` 与 `⌘,` / `⌘R` 快捷键随 Toolbar 一并迁入。

### 2. 平台留白（复用既有 CSS，不改动）

- macOS：`.app-toolbar--macos` 预留 `padding-left: max(78px, --tauri-plugin-decoration-left-clearance)`。
- Windows / Linux：`.app-toolbar` 预留 `padding-right: max(8px, --tauri-plugin-decoration-right-clearance)`
  （插件用 HTML 覆盖窗口控制按钮并发布右留白）。
- 全屏时两个 clearance 归 0，由插件隐藏控件。

### 3. `src/components/ActionBar.tsx`

- 行容器改为 `TitleBar`；Workspace 下拉、外部工具、Changes/Stash/Fetch、Pull/Push 作为
  `children`，交互分组加 `pointer-events-auto`。
- 状态区经 `overlay` 传入，绝对居中于**整个窗口**（不受标题栏左右留白影响）。
- 内容层加 `@container`；次级外部工具文字在窄栏（`@6xl` 以下）折叠为图标，避免
  Windows / Linux 菜单 + 操作按钮在 1024px 最小宽度下互相挤压。

### 4. 状态区弱化（`src/components/SyncStatusArea.tsx`）

- 去掉卡片容器：透明、无边框、直角、宽度贴合文字（`w-auto max-w-[60vw]`），纯文字融入标题栏。
- 2px 状态底线仅在非 idle（同步中 / 最近结果）时渲染；idle（仅分支名）不显示。

### 5. 文档

`docs/design/03-layout.md`：§1 全局 shell、§2 Toolbar → TitleBar、§6.1 ActionBar 更新为单行标题栏。

## 测试

- `make check`：prettier / eslint / tsc / clippy 全绿；前端 183 项、Rust 349 项测试通过。
- 手动冒烟要点（三平台）：
  - macOS：红绿灯不被 Workspace 下拉或菜单遮挡；空白标题栏可拖动、双击缩放。
  - Windows / Linux：栏尾窗口控制按钮（最小化/最大化/关闭）可见可点，未被操作按钮压住；
    最大化的 Snap Layout 悬停可用。
  - 窄窗口（1024px）：外部工具文字折叠为图标，无横向裁切；状态文字居中不压按钮。
  - 同步中：状态区显示进度条与取消按钮，标题栏按钮仍可点击（状态层穿透）。
