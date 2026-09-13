# feat-open-in-editor · plan

关联提案：[F015-open-in-editor](../../pm/features/F015-open-in-editor.md)
分支：`feature/open-in-editor`

## 需求

ActionBar 左侧「文件」「终端」按钮旁新增「编辑器」按钮：检测本机安装的代码编辑器（Visual Studio Code / Zed / VSCodium），点击按钮弹出下拉列表，点击任意检测到的编辑器直接在当前活跃仓库根目录打开。列表按检测顺序排列（VS Code → Zed → VSCodium），第一个检测到的为默认；未检测到任何编辑器时按钮禁用并提示。

交互决策（用户确认）：点击弹出编辑器列表（与 WorkspaceDropdown 下拉模式一致）；默认固定为检测顺序第一个，不做持久化。

## 改动清单

### 后端（src-tauri/src/lib.rs）

| 改动 | 内容 |
|---|---|
| `DetectedEditor { id, name }` | serde struct，返回给前端的检测项 |
| `detect_editors()` | 按固定顺序探测三个编辑器，返回全部已安装者 |
| `list_editors()` command | 返回 `Vec<DetectedEditor>` |
| `open_in_editor(path, editor_id)` command | 校验目录 → 按 id 解析启动项 → 分离式 spawn（cwd + 目录参数） |
| `invoke_handler` 注册 | `generate_handler!` 中 `open_in_terminal` 之后注册两个新 command |
| `#[cfg(test)] mod tests` | id→name 映射与顺序、各平台 argv 构造纯函数、Windows 已知路径候选表 |

编辑器探测策略：

| 编辑器 | Windows（按序） | macOS（`/Applications` 与 `~/Applications`） | Linux（PATH） |
|---|---|---|---|
| vscode | `%LOCALAPPDATA%\Programs\Microsoft VS Code\Code.exe`、`%ProgramFiles%\Microsoft VS Code\Code.exe`、`%ProgramFiles(x86)%\Microsoft VS Code\Code.exe`，回退 PATH `code.cmd` | `Visual Studio Code.app` | `code` |
| zed | `%LOCALAPPDATA%\Programs\Zed\Zed.exe`，回退 PATH `zed.exe` | `Zed.app` | `zed` |
| vscodium | `%LOCALAPPDATA%\Programs\VSCodium\VSCodium.exe`、`%ProgramFiles%\VSCodium\VSCodium.exe`，回退 PATH `codium.cmd` | `VSCodium.app` | `codium` |

启动方式：

- Windows：`.exe` 直接 spawn（GUI 程序无控制台闪烁）；`.cmd` 回退经 `cmd /c` 启动并加 `CREATE_NO_WINDOW`
- macOS：`/usr/bin/open -a "Visual Studio Code"|"Zed"|"VSCodium" <dir>`（沿用终端检测的已知路径 + `open -a` 模式）
- Linux：直接 spawn 二进制，目录作为末位参数
- Unix 统一 `process_group(0)` 分离；所有平台目录同时设为子进程 cwd

### 前端

| 文件 | 改动 |
|---|---|
| `src/lib/api.ts` | `DetectedEditor` 接口、`listEditors()` / `openInEditor(path, editorId)` 包装 |
| `src/components/ActionBar.tsx` | 终端按钮后新增编辑器下拉控件：`DropdownMenu` 组合（仿 `WorkspaceDropdown`），lucide `SquareCode` 图标，`useQuery` 拉取检测列表，`!externalRepo \|\| editors.length === 0` 时禁用，菜单项点击经 `openExternal` 打开，失败走 `wc.setActionError` |
| `src/i18n/locales/en/workspace.json` | `workspace.openEditor`（label / title / noneDetected） |
| `src/i18n/locales/zh-CN/workspace.json` | 同步中文文案（parity 测试强制键位对齐） |

## 关键实现决策

- 检测与启动解析分离：`detect_editors()` 只做存在性探测供列表展示；`open_in_editor` 按 id 重新解析启动项（无状态，与终端检测模式一致）。
- 图标采用 lucide 通用图标 `SquareCode`（与 `SquareTerminal` 呼应；lucide 无品牌图标，语义靠 tooltip）。
- 触发按钮样式与 `ActionBarButton` 保持一致，通过 `DropdownMenuTrigger` 组合（必要时扩展 `ActionBarButton` 透传 trigger props）。
- 校验：`open_in_editor` 入参 path 必须是已存在的目录；未知的 `editor_id` 返回错误字符串。
- 纯只读操作，不涉及 git 状态修改。

## 验证

- `cargo test --manifest-path src-tauri/Cargo.toml --all-targets`（含新增单元测试）
- `cargo clippy --manifest-path src-tauri/Cargo.toml -- -D warnings`
- 前端 `pnpm lint` / `pnpm test`（含 i18n parity 测试）
- 手动（Windows，本机已装 VS Code）：按钮出现在终端右侧 → 下拉列出 VS Code → 点击后 VS Code 打开当前仓库目录；无活跃仓库时禁用
- macOS / Linux 探测与 argv 逻辑由单元测试覆盖（本环境无法手动验证）
