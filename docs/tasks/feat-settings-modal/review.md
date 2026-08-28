# review: feat-settings-modal

审查人：code-reviewer 代理（2026-08-28），范围 = 本任务全部工作区改动（lib.rs / api.ts / Toolbar / SettingsModal / SshKeyManager / capabilities / public/app-icon.png）。

## 结论

🔴 1 项已修复并复验，🟡 3 项中 2 项已修复、1 项挂起为后续任务。静态验证全绿（cargo check / clippy / tsc / eslint / vitest 43 / vite build），**合入前仍需真机验证"数据目录"按钮**（ACL 是运行时检查，静态验证无法覆盖）。

## 审查发现与处置

### 🔴 已修复

- **`opener:default` 不含 `allow-open-path`，"数据目录"按钮运行时必失败**
  - 核对 tauri-plugin-opener 2.5.4 `permissions/default.toml`：`opener:default` 仅含 `allow-open-url`（mailto/tel/http/https scope）+ `allow-reveal-item-in-dir` + `allow-default-urls`
  - 处置：`src-tauri/capabilities/default.json` 追加 `"opener:allow-open-path"`；plan.md 决策表已同步更正
  - 参考：https://tauri.app/plugin/opener/

### 🟡 已修复

- **内联表单内按 Escape 会关掉整个 Settings 弹框**（内联化引入的行为回退）
  - 处置：Add/Test 表单容器 `onKeyDown` 拦截 Escape（`preventDefault` + `stopPropagation` + 关闭表单），Radix DismissableLayer 尊重 `defaultPrevented`，只关表单不关弹框
- **`role="radio"` 无 roving tabindex，键盘语义不完整**（palette 网格既有问题，Theme 组复制了它）
  - 处置：两组改为 `role="group"` + 按钮 `aria-pressed`，语义自洽且无需方向键实现
  - 参考：https://www.w3.org/WAI/ARIA/apg/patterns/radio/

### 🟡 挂起（后续任务）

- **核心组件零测试覆盖**：仓库现状无组件测试基础设施（无 @testing-library/react、无 jsdom 环境，43 个用例均为 `src/lib` 纯函数）。补 render 冒烟用例（齿轮开弹框 / 三区切换 / Add-Test 互斥与重置）需先搭基建，超出本任务范围，建议单开任务。

### 🟢 已采纳

- 重开弹框固定回到 Appearance 区（`useEffect` on `open` 重置 section）
- plan.md 决策表与 🔴 发现矛盾的两行已更正

### 🟢 知悉 / 后续可选

- 表单关闭时焦点掉到 body（未还给触发按钮）；SSH 区每次切入重挂载并 refetch `ssh-keys`（可设 staleTime）；`get_data_dir` 同步命令内含幂等的 `create_dir_all`（启动时已建目录，无害）

## 审查确认无问题项

- Rust 命令签名 / 注册 / `state_dir()` 复用正确
- 单层 Radix Dialog + 内联 DOM 表单，旧"需关闭多次"bug（嵌套 DismissableLayer 的 body pointer-events 锁）在此流程无法复现
- CSP/XSS 干净：图标走 `img-src 'self'`；GitHub URL 为硬编码 https 常量；错误文案均为 React 转义文本
- lucide `Palette` 图标与 `Palette` 类型的重名已用 `PaletteIcon` 别名解决，无其他遮蔽

## 修订记录

- **需求 2（快捷键）**：Toolbar 增加 window 级 `Ctrl+,` / `Cmd+,` 打开设置监听 + 按钮 title 快捷键提示。小而机械的增量（约 15 行，含既有跨平台约定复用），按 feat-sidebar-branch-dedupe 先例跳过独立 reviewer 代理轮，以 typecheck / lint / test / build 机械验证替代；PR 前整分支 diff 终审时一并覆盖。
- **需求 3（真机反馈：数据目录按钮 "Not allowed to open path"）**：证实 🔴 的修复不到位——opener 插件 `open_path` IPC 权限无可用默认 scope，追加裸 `opener:allow-open-path` 权限后路径仍被拒。最终方案：新增 Rust 命令 `open_data_dir` 用 `OpenerExt` 直接打开 `state_dir()`（不经 webview ACL），前端 api.ts 改为 `openDataDir()` invoke 封装，capabilities 回退为仅 `opener:default`（GitHub 的 openUrl 走 https scope 不受影响），移除前端对 `openPath` 的使用与 `get_data_dir` 命令。

## 验证记录

| 项 | 结果 |
|---|---|
| `cargo check` / `cargo clippy --all-targets` | ✅ |
| `npm run typecheck` / `npm run lint` / `npm test` (43) / `npm run build` | ✅ |
| capabilities JSON 解析 | ✅ |
| 真机：数据目录打开 `%APPDATA%\GitWave` | ⏳ 待用户验证 |
| 真机：GitHub 打开仓库、三区切换、SSH 内联表单 + Escape 行为 | ⏳ 待用户验证 |
