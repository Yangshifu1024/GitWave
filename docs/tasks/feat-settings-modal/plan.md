# feat: 完整设置弹框（外观 / SSH Keys / 关于）

状态：进行中

## 需求来源

用户 2026-08-28（会话内口述五点）：

1. 溢出菜单图标（⋯）改为设置图标
2. 点击不再出现下拉框，而是设置弹框
3. 设置弹框包含外观、SSH Key、关于三个区块
4. 外观与 SSH Key 沿用 / 改造现有功能
5. 关于页显示应用图标、版本、slogan、两个按钮（数据目录 / GitHub）

## 需求 2（2026-08-28 追加）

设置弹框支持应用内快捷键打开：Windows/Linux `Ctrl+,`、macOS `Cmd+,`。

- 落点：`Toolbar.tsx` window 级 `keydown` 监听（`(metaKey || ctrlKey) && key === ","`，跨平台写法对齐 `CommitMessageBox.tsx:45` 既有约定），齿轮按钮 title 显示对应快捷键
- 全局监听为首个 window 级快捷键，无既有 hook 可复用

分支：`feature/theme-design`（用户确认留在本分支）。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 布局 | 左侧竖向导航 + 右侧内容区 | 用户指定（初版横向 Tabs 被否）；`ui/Tabs` 是横向下划线式，不启用，导航用组件内 useState |
| GitHub URL | https://github.com/Yangshifu1024/GitWave | 与 git remote 一致；用户原话 Yangshifu/GitWave 经确认为笔误 |
| Slogan | "Local-first Git client with AI collaboration." | README/Cargo.toml 既有事实 slogan，用户确认 |
| 外观区内容 | 主题模式（light/dark/system）+ 调色板 | 均为既有能力（useTheme / usePalette），ThemeToggle 保留在 Toolbar |
| SSH 嵌入 | SshKeyManager 原组件内联化 | 无 props 可直接嵌入；顺带修复"弹框需关闭多次"bug（嵌套 Radix dialog 层 → 内联视图） |
| 数据目录按钮 | Rust 命令 `open_data_dir`（OpenerExt）+ 前端 invoke | 初版前端 `openPath` 真机被拒（"Not allowed to open path"）：opener 插件的 open_path IPC 权限无可用默认 scope，capability 加 `allow-open-path` 也不放行路径；改为 Rust 侧直接调用，不经 webview ACL，capabilities 维持 `opener:default` |
| 打开方式 | GitHub：前端 `openUrl`；数据目录：Rust `open_data_dir` | openUrl 命中 opener:default 的 https scope；open_path 全程 Rust 侧 |
| 按钮文案 | "数据目录" → "App Data" | 用户 2026-08-28 调整（与 GitHub 按钮统一为英文） |

## 改动清单

- `src-tauri/src/lib.rs`：`open_data_dir` 命令（Rust 侧 OpenerExt 打开 `state_dir()`）+ generate_handler 注册 + import
- `src/lib/api.ts`：`getDataDir()` 封装
- `src/components/Toolbar.tsx`：删 DropdownMenu / SshKeyManagerModal / sshOpen / version；齿轮按钮
- `src/components/SettingsModal.tsx`：重写为左竖导航（Appearance / SSH Keys / About）
- `src/components/SshKeyManager.tsx`：去内部标题；Add/Test 子 Modal → 内联表单区
- `public/app-icon.png`：自 `src-tauri/icons/128x128.png` 复制

## 验证

- `cargo check`、`npm run typecheck / lint / test / build`
- 手动：齿轮开弹框、三区切换、SSH 添加/测试内联流程、数据目录打开 `%APPDATA%\GitWave`、GitHub 打开仓库页
