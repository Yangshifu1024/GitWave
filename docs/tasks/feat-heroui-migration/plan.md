# feat: HeroUI v3 完整替换 ui/ 手写组件

状态：进行中（不提交，等用户审查）

## 目标

按 [HeroUI React Quick Start](https://heroui.com/en/docs/react/getting-started/quick-start) 把 `src/components/ui/` 里所有**通用手写 / Radix 包壳**换成 HeroUI v3，**3-pane 布局与几何尺寸不变**。对外导出路径、组件名、props 签名保持兼容，业务层零改动（除 `App` 挂载 Toast 区域）。

## 需求来源

用户 /goal（2026-08-28）：https://heroui.com/en/docs/react/getting-started — 使用 HeroUI 完整替换现有所有手写组件，不修改布局，给出完整方案后直接开工，不提交。

官方文档入口实际为 [`/docs/react/getting-started/quick-start`](https://heroui.com/en/docs/react/getting-started/quick-start)（无 `/getting-started` 单页）。

## 技术事实（以 `@heroui/react@3.2.4` 类型为准）

- 包：`@heroui/react` + `@heroui/styles` + `tailwind-variants`；基于 **React Aria Components**（非 Radix）
- Tailwind v4 强制；接入 = `@import "tailwindcss";` 后立刻 `@import "@heroui/styles";`（顺序敏感）
- **无 Provider**（v2 `HeroUIProvider` 已废弃）；动效纯 CSS
- 暗色：`<html data-theme="dark">`；我们原有 token 仍走 `.dark` / `prefers-color-scheme`
- Button：`variant` primary/secondary/tertiary/outline/ghost/danger；`size` sm/md/lg；`isDisabled` / `onPress`
- Input 无校验；校验走 `TextField` + `FieldError`；`onChange` 在 TextField 上是 `(value: string) => void`
- Modal 为复合组件 + `useOverlayState({ isOpen, onOpenChange })`
- Toast.Provider 是 **viewport**（不包应用树）；命令式 API 为 `toast.success/danger/info/warning`
- **v3 无 ContextMenu 组件**：用 Popover（光标锚点）+ Menu 复刻原 API

## 方案原则

1. **wrapper 保 API**：`@/components/ui/*` 文件名、导出、props 不变，内部换成 HeroUI。
2. **几何留给 wrapper**：h-7 / px-2 / max-w-* / gap-* 继续输出；皮肤色走 HeroUI BEM + token 桥接。
3. **布局壳不动**：`ThreePaneLayout` / `Split` 是分栏拖拽，HeroUI 无 splitter。source-list 行用 `Surface`（transparent），不用 `ListBox.Item`，以便行内嵌套 Button。
4. **领域组件只换内部原语**：`PathInput` / `ErrorAlert` / `CommitMessageBox` / `SyncButtons` / `WorkingCopyModal` / `FileListItem` 不改结构。
5. **不提交**，等审查。

## 组件映射

| 文件 | HeroUI | 映射要点 | 消费者 |
|---|---|---|---|
| `Button.tsx` | `Button` | secondary→**outline**（默认仍是 secondary）；primary/danger/ghost 同名；`disabled`→`isDisabled`；保留原生 `onClick`（ListItem 内需 `stopPropagation`）；保留 h-7/h-8 | 大量 |
| `Input.tsx` | `TextField` + `InputGroup` + `FieldError` | 保留 `(value: string) => void`；`error`→`isInvalid`；`variant=search` 走 `InputGroup.Prefix`；外壳锁 h-8 | 多处 |
| `Modal.tsx` | `Modal.*` + `useOverlayState` | `open/onOpenChange/title/description/size` 原样；宽度继续用 `sizeClasses`；不传 HeroUI `size` 以免抢宽度 | 大量 |
| `Tooltip.tsx` | `Tooltip.Root/Trigger/Content` | `delayDuration`→`delay`；`side`→`placement`；Trigger 用 `inline-flex` 避免撑破 toolbar | ThemeToggle / SectionAction / ActionBar |
| `DropdownMenu.tsx` | `Dropdown.*` + `Separator` | 同名导出；`onSelect`→`onAction` | 0 直接消费者 |
| `Tabs.tsx` | `Tabs.Root/List/Tab/Panel` | `value`→`selectedKey`；Trigger/Content 的 `value`→`id` | 0 消费者 |
| `Toast.tsx` | `Toast.Provider` + `toast.*` | 保留 `ToastProvider` / `useToast({ title, description, variant, duration })`；Provider 渲染 viewport | 需在 App 挂载 |
| `KeyHint.tsx` | `Kbd` | 每个 key 一个 `Kbd`，保留 pill 几何类 | Toolbar |
| `EmptyState.tsx` | `EmptyState` | 只换根节点，内部结构/class 原样 | CommitGraph / Workspace / Repo / App |
| `StatusBadge.tsx` | `Chip` | 保留 git 语义色 class（active/missing/ahead/behind/conflict） | BranchList / RepoList |
| `ContextMenu.tsx` | `Popover` + `Menu` | 保留 Root/Trigger(asChild)/Content/Item/Label/Separator；右键坐标锚点打开；`onSelect`→`onAction` | ChangesPanel / BranchList / RepoList |
| `SidebarSection.tsx` | `Disclosure` | 折叠时仍卸载 children；标题 11px caps 与 chevron 几何不变 | App / Workspace / Repo / Branch |
| `SectionAction.tsx` | `Button` ghost | 覆盖为 h-auto / 10px uppercase，避免走默认 h-7 | Fetch / Pull / Push |
| `StatusIcon.tsx` | `Chip` | 锁 w-5 h-5 + mono 字母 | FileListItem |
| `BranchIndicator.tsx` / `SyncButtons.tsx` | `Chip` | ahead/behind 计数胶囊 | WorkingCopy |
| `CommitMessageBox.tsx` | `TextField` + `InputGroup.TextArea` + `Button` | textarea 与 feat/fix 类型芯片都走 HeroUI；外壳走 InputGroup 与 Input 一致 | ChangesPanel |
| `FileListItem.tsx` | `Button` + `Chip` | stage 开关改为 Button，图标走 StatusIcon | ChangesPanel |
| `ListItem.tsx` / `FileListItem.tsx` | `Surface` variant=transparent | 行内仍可嵌套 Button；几何 class 原样 | 侧栏 / Working Copy |
| `Select.tsx` | `Select` + `ListBox` | 受控 `value/onChange/options`；Trigger 默认 h-8，几何可被 className 覆盖 | AI provider / merge / pull / rebase |
| `Checkbox.tsx` | `Checkbox.*` | 受控 `checked/onChange`；Content+Control+Indicator，行高与原来的 checkbox+label 一致 | pull / push / delete branch |
| `Label.tsx` | `Label` | 默认 12px secondary 字段标题；`htmlFor` 原样 | 表单对话框 |
| `PathInput.tsx` / `ErrorAlert.tsx` / `WorkingCopyModal.tsx` | 间接 | 继续组合我们的 wrapper（ErrorAlert 走 Modal，保留右上角关闭；PathInput 仍是输入框 + 旁边浏览按钮，不收成 `InputGroup.Suffix`） | — |
| `Split` / `ThreePaneLayout` | **保留** | GitWave 布局壳，HeroUI 无 splitter；`Split` 目前无业务消费者 | App |
| gitignore 范围选择 | `RadioGroup` + `Radio`（无 Control） | 卡片式选项，不渲染圆点以免改布局；压掉 RadioGroup 默认 `mt-4` | ChangesPanel |
| Settings 主题 / 色板卡片 | `RadioGroup` + `Radio`（无 Control） | 保持卡片网格与选中描边；`display:grid` 覆盖 RadioGroup 默认 flex | SettingsModal |
| Diff 分段控件 | `RadioGroup` + `Radio`（无 Control） | 保留 iOS 胶囊外壳与字号；不用 ButtonGroup | DiffViewer |
| ActionBar 组分隔 | `Separator` vertical | 锁 `h-8 w-px mx-1`，避免 `self-stretch` 把整列撑开 | ActionBar |
| CommitGraph 行 | `Surface` transparent | 替代 `div role=button`；保留 `border-l-2` 与 `ROW_H` | CommitGraph |
| CommitGraph 分支/ref 标签 | `Chip` + `Chip.Label` | HEAD / tag / 当前分支 / lane 色本地/远程分支；几何 class 原样 | CommitGraph |
| BranchList 操作通知条 | `Alert` | 保留 success/danger 条形色与 `border-b` | BranchList |
| Conflict / rebase 多行输入 | `TextField` + `InputGroup.TextArea` | Conflict 编辑器仍 `border-0` 铺满面板 | ConflictPanel / InteractiveRebaseDialog |

## 主题桥接（已落地，本轮补系统主题）

`tokens.css` 已 `@import "@heroui/styles"`，末尾 unlayered 把 HeroUI 变量 alias 到 GitWave token。`useTheme.applyTheme` 已写 `dataset.theme`。

缺口：`main.tsx` 在偏好为 `system` 时未设 `data-theme`，首屏 HeroUI 会落到默认 light。补：按 `prefers-color-scheme` 写入 `data-theme`，**不**给 `<html>` 加 `.dark`（我们的 token 仍走 media query）。

## 依赖

保留：`@heroui/react` `@heroui/styles` `tailwind-variants` `tailwind-merge@3`

卸载：全部 `@radix-ui/*`（含 context-menu）。ContextMenu 已用 HeroUI Popover + Menu 重写。

`class-variance-authority` 仍给 Button / StatusBadge 的几何变体用。

## 布局不变清单（审查对照）

- Toolbar 40px；sidebar / inspector 拖拽宽度；Working Copy bar
- Button sm=h-7、md=h-8；Input h-8；Modal 90vw + max-w 480/640/800/1200
- ListItem 左 3px accent 条；SidebarSection 11px caps 标题
- Tooltip 不得把 icon 按钮变成块级（Trigger `inline-flex`）

## 验证

- `npm run typecheck` / `lint` / `test` / `build`
- `rg "@radix-ui" src` 为空；`package.json` 无 Radix
- Modal 关闭：`CloseTrigger` / ESC / 点 backdrop 都必须把受控 `open` 置 false（`isOpen`/`onOpenChange` 挂在 `Modal.Backdrop` 上）
- 用户真机对照布局（不提交）

## 执行进度

- [x] 调研 HeroUI v3 接入
- [x] 依赖安装（含 tailwind-merge ^3）
- [x] tokens.css import + 桥接
- [x] 完整方案（本文）
- [x] data-theme 系统主题初始化
- [x] Button / Input / Modal / Tooltip
- [x] DropdownMenu / Tabs / Toast / KeyHint / EmptyState / StatusBadge
- [x] CommitMessageBox textarea
- [x] App 挂载 ToastProvider；卸载全部 Radix（ContextMenu 改 HeroUI Menu）
- [x] SidebarSection / SectionAction / StatusIcon / BranchIndicator / FileListItem / SyncButtons
- [x] ListItem / FileListItem → Surface transparent
- [x] 业务层剩余原生 button / textarea / select / checkbox / label / 进度条换成 Button · TextArea · Select · Checkbox · Label · ProgressBar
- [x] Commit / conflict / rebase 多行输入改 `InputGroup.TextArea`；gitignore 范围改 Radio 卡片（无圆点）
- [x] Settings 主题/色板卡片、Diff 分段控件改 Radio（无圆点）；ActionBar 分隔改 Separator
- [x] CommitGraph 行改 Surface；BranchList 通知条改 Alert
- [x] typecheck / lint / test / build
- [x] Vite 空状态：3-pane / Toolbar / Modal 开合 / Input / Disclosure 折叠（无 Tauri 后端）
- [x] `cargo run` 启动 Tauri 窗口（复用已有 Vite `localhost:1420`，编译通过无运行时错误）
- [ ] 用户真机（Tauri）对照布局（不提交）
