# feat-app-menubar · Code Review

- 分支：`feature/app-menubar`（未提交，未合入）
- 审查方式：code-reviewer 代理，对照 `git diff` + HeroUI 3.2.4 / react-aria-components 安装源码核验
- 结论：**需修复后合入** → 两处 🟡 已当场修复，复验（vitest 60 passed / tsc / eslint / cargo check）全绿

## 审查认可项

- store 转发架构（`menuAction: {id, action}` → ActionBar 同源处理函数）消除双入口逻辑分叉；`App.tsx:124` ActionBar 无条件挂载保证请求必有消费者
- AppMenuBar 16 项门控表达式与被删除的 ActionBar 按钮 `disabled` 逐项比对全部一致（含 `!wc.data?.sha`）
- pointermove 悬停切换实现正确：仅打开期间注册、effect 按 `openMenu` 重挂载无闭包过期、跳过当前菜单、相邻触发器 gap 无重叠区
- 无死代码（被删按钮的 import/state 已清理；保留的弹框与 mutation 均为菜单宿主职责）
- `useWorkingCopy()` 双观察者经 queryKey 去重，2s 轮询不叠加
- `quit_app` Rust 实现规范（`AppHandle` 注入 + `generate_handler!` 注册）
- 受控 `isOpen/onOpenChange`、`placement="bottom start"`、`textValue`、`Kbd` 用法均与官方一致

## 发现与处置

| 级别 | 发现 | 处置 |
|---|---|---|
| 🟡 | `DropdownMenuTrigger` 包裹自定义 `Button` 产生嵌套 `<button>`：无效 HTML、双重 Tab 停靠、读屏重复播报 | ✅ 已修：`Button` 改为 `DropdownMenu` 直接子元素（HeroUI 官方 canonical 模式，PressResponder 合并 menuTriggerProps），移除手写 `aria-haspopup`/`aria-expanded` 与 `DropdownMenuTrigger` import |
| 🟡 | `data-[open=true]:*` 是死样式（RAC/HeroUI Button 无 `data-open` 属性），打开态高亮从未生效 | ✅ 已修：改由受控 `isOpen` 计算 `className`（`isOpen && "bg-bg-elevated text-text-primary"`） |
| 🟢 | `void quitApp()` 异常路径会产生 unhandled rejection | ✅ 已顺手修复：`quitApp().catch(() => undefined)` |
| 🟢 | 路由 switch 新增 `AppMenuAction` 成员漏路由时静默 no-op | ✅ 已顺手修复：`default` 分支 `never` 穷举保护，漏路由编译期报错 |
| 🟢 | pointermove 监听可加 `passive` | ✅ 已顺手修复 |
| 🟡 | menuAction 通道缺最小单测 | ✅ 已补 `src/stores/uiStore.test.ts`（重复 dispatch id 递增可重触发；clearMenuAction 只清匹配 id 不误清新请求） |
| 🟢 | `"(detached)"` 魔法字符串全仓第 7 次出现 | 遗留：建议后续导出 `DETACHED_BRANCH` 常量收敛（跨 6 处既有代码，超出本任务范围） |
| 🟢 | 门控 disabled 表达式是"手工镜像"而非构造共享 | 遗留：可抽 `useActionGating()` 共享（当前注释已声明镜像意图） |
| 🟢 | Exit 无进行中操作守卫（clone 中退出留半成品目录） | 遗留：产品决策，可作后续增强 |
| 🟢 | 菜单打开时不支持 ←/→ 键盘横向切换菜单标题 | 遗留：增强项 |
| 🟢 | Kbd 传纯字符串偏离官方 `Kbd.Abbr` 组合写法 | 遗留：渲染正确且与真实快捷键 `(metaKey\|\|ctrlKey)+,` 一致，可后续对齐 |
| 🟢 | AppMenuBar 承载整套 `useWorkingCopy()`（只用三个值） | 遗留：可抽轻量 `useWorkingCopySnapshot` |

## 复验记录

- `npm run test`：9 文件 60 用例全过（含新增 uiStore 2 例）
- `npx tsc --noEmit`：零错误
- `npm run lint`：零告警
- `cargo check`（src-tauri）：通过
- 浏览器冒烟（vite + 合成事件）：菜单渲染/打开/Kbd 标注/门控禁用态、悬停切换（File 开 → 悬停 Workspace → 切换且禁用态正确）、菜单项 "New" 点击后 ActionBar 的 New Workspace 弹框打开（store 转发端到端）

## 测试期发现并已修复的问题

1. AppMenuBar 根容器漏 `pointer-events-auto`——Toolbar 内容层是 `pointer-events-none`，漏掉会导致真实窗口里点菜单变成拖动窗口（Playwright actionability 检查抓到）
2. 菜单开着时 HeroUI 将 `#root` 置 `inert`，兄弟触发器收不到任何指针事件，`onMouseEnter` 悬停切换方案不可行——改为 document 级 `pointermove` + 触发器 rect 手动命中测试（事件 retarget 到 `<body>`，document 监听仍可达）
