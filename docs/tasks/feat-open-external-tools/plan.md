# feat-open-external-tools · plan

关联提案：[F014-open-external-tools](../../pm/features/F014-open-external-tools.md)
分支：`feature/external-open`

## 需求

Repository 标题栏区域新增两个按钮（应用户要求，最终位置为 ActionBar 工作区选择器右侧，样式与「变更 / 贮藏」等 ActionBarButton 一致）：

1. 在系统文件管理器中打开当前活跃仓库根目录
2. 在检测到的终端模拟器中打开当前活跃仓库根目录

无活跃仓库或仓库 missing 时禁用。

## 改动清单

### 后端（src-tauri/src/lib.rs）

| 改动 | 内容 |
|---|---|
| `open_in_file_manager(app, path)` command | 校验 path 存在后走 opener 插件 `open_path`（沿用 `open_data_dir` 绕过 webview ACL 的约定） |
| `open_in_terminal(path)` command | 按平台检测终端并 spawn，cwd = 仓库根目录 |
| `invoke_handler` 注册两个新 command | lib.rs `generate_handler!` 列表 |

终端检测策略：

- **Windows**：`wt.exe -d <path>`（Windows Terminal）；失败回退 `cmd.exe`（`CREATE_NEW_CONSOLE`，cwd = path）
- **macOS**：存在 `/Applications/iTerm.app` 时 `open -a iTerm <path>`，否则 `open -a Terminal <path>`
- **Linux**：`$TERMINAL` 环境变量 → 依序探测 `gnome-terminal`（`--working-directory`）/ `konsole`（`--workdir`）/ `xfce4-terminal` / `kgx` / `kitty` / `alacritty` / `foot` / `wezterm`（后五种直接以 cwd spawn）

### 前端

| 文件 | 改动 |
|---|---|
| `src/lib/api.ts` | 新增 `openInFileManager(path)` / `openInTerminal(path)` 包装 |
| `src/components/ActionBar.tsx` | 工作区选择器（WorkspaceDropdown）右侧新增两个 `ActionBarButton`（lucide `FolderOpen` / `SquareTerminal`，样式同 Changes/Stash），作用于活跃 repo，missing/无选中时禁用，失败经 `wc.setActionError` 提示 |
| `src/i18n/locales/en/workspace.json` | `workspace.openFileManager` / `openTerminal`（label + title） |
| `src/i18n/locales/zh-CN/workspace.json` | 同步中文文案 |

## 关键实现决策

- 复用 opener 插件（已在依赖中）打开文件管理器，不新增 shell 插件；终端 spawn 用 `std::process::Command`，detached（windows `CREATE_NEW_CONSOLE`，unix `process_group(0)` + `spawn`）。
- 图标采用 lucide 通用图标（lucide 无品牌图标），以 tooltip 表达语义。
- 校验：命令入参 path 必须是已存在的目录，否则返回错误字符串。

## 验证

- `cargo check`（src-tauri）
- 前端 `pnpm lint` / `pnpm test`（含 i18n parity 测试）
- 手动：Windows 下验证资源管理器与 Windows Terminal 打开
