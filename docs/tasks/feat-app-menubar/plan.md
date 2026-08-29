# feat-app-menubar · TopBar 系统菜单 + ActionBar 精简

## 需求

顶栏左侧增加系统菜单（File / Workspace / Repository / Branch），动作逻辑与 ActionBar 完全一致；菜单接管入口后精简 ActionBar。用户补充决定：

- Repository 菜单含 Fetch（六项对齐原 Repository 组）
- **悬停切换**：菜单开着时悬停其他菜单标题即切换（原生 menubar 行为）
- ActionBar 精简：Workspace 整组移除；Repository 只留 Fetch/Hooks；Branch 只留 Pull/Push
- File 菜单：Settings / About / Exit，Settings 标注快捷键（`⌘,` / `Ctrl+,`）
- macOS 下经现有 `.app-toolbar--macos` 左 padding（78px）避让交通灯

## 方案

### 架构：store 转发 + ActionBar 同源处理函数

菜单不复制业务逻辑。`uiStore` 新增 `menuAction: { id, action }` 请求通道：`AppMenuBar` 只 dispatch（`requestMenuAction`），`ActionBar`（无条件挂载、保留全部弹框宿主职责）在 effect 中消费请求并调用与原按钮相同的处理函数——一致性由构造保证。

```
AppMenuBar --requestMenuAction("repo:clone")--> uiStore.menuAction
                                                    |
ActionBar useEffect 消费 → switch 映射到 openCreate/openExport/... 同名处理函数
```

### 关键点

- `AppMenuBar` 用受控 `Dropdown isOpen/onOpenChange` + 触发器 `onMouseEnter` 实现悬停切换：首次点击打开；有菜单开着时悬停即切换；点击外部 / Escape / 选中后收拢。
- 门控逐项复刻原 ActionBar 按钮 disabled 条件（`useWorkspaceUiStore` + `useWorkingCopy` 共享 TanStack 缓存，轮询不叠加）。
- Exit 为新能力：Rust `quit_app` 命令（`app.exit(0)`），前端 `quitApp()` 包装。
- `ui/DropdownMenu.tsx` 扩展 `placement` 与菜单项快捷键标注（HeroUI `Kbd`）。

## 改动文件

| 文件 | 改动 |
|---|---|
| `src/stores/uiStore.ts` | `AppMenuAction` 类型 + `menuAction` 通道 |
| `src/components/AppMenuBar.tsx` | 新建：四菜单 + 悬停切换 + 门控 |
| `src/components/ui/DropdownMenu.tsx` | `placement` prop + `shortcut` prop（Kbd） |
| `src/components/Toolbar.tsx` | 挂载 `<AppMenuBar onAbout={...} />` |
| `src/components/ActionBar.tsx` | 提取具名处理函数；消费 menuAction；删除 Workspace 组 / Repo 的 Init·Clone·Add·LFS / Branch 的 New·PR；弹框全保留 |
| `src/lib/api.ts` | `quitApp()` |
| `src-tauri/src/lib.rs` | `quit_app` command + 注册 |

## 验证

- `npm run build`（tsc）+ `cargo check`
- 手动：16 个菜单项行为与原按钮一致；悬停切换；ActionBar 精简后无死代码；Exit 退出进程
