# F014 · Repository 标题栏外部打开按钮

## 背景

用户在 GitWave 中浏览某个仓库时，经常需要在**系统文件管理器**中查看该仓库目录（拖拽文件、查看未跟踪文件等），或在**终端**中对仓库执行命令。当前必须手动复制路径、打开外部工具再粘贴，路径长时非常繁琐。

## 提议方案

在 repository tab 标题栏（tab strip 右侧）增加两个图标按钮：

1. **文件管理器按钮**：使用系统默认文件管理器图标语义（macOS Finder / Windows Explorer / Linux 各 DE 文件管理器），点击后在文件管理器中打开**当前活跃仓库**的根目录。
2. **终端按钮**：使用系统默认终端图标语义，自动检测并使用用户机器上的终端模拟器，点击后在该终端中打开当前活跃仓库根目录，工作目录即为仓库根目录。检测顺序：
   - Windows：Windows Terminal（`wt`），不可用时回退 `cmd.exe`
   - macOS：iTerm2 → Terminal.app
   - Linux：`$TERMINAL` 环境变量 → 常见终端（GNOME Terminal / Konsole / xfce4-terminal / kitty / alacritty / foot / wezterm 等）

### 行为细节

- 按钮作用于**当前活跃（选中）的 repo tab**，tooltip 显示动作含义；无活跃仓库或仓库缺失（missing）时按钮禁用。
- 打开动作由 Rust 侧执行，不经过 webview 权限面（与现有 `open_data_dir` 约定一致）。
- 纯只读操作：不涉及 git 状态修改，不违背「AI 不自动 commit/push/merge」原则。
- 打开失败（终端未检出等）向用户展示错误提示。

## 影响

- 涉及模块：repository tab strip（前端）、Tauri command（后端）、i18n（en / zh-CN）
- 影响版本：0.x
- 向后兼容：纯新增功能，无破坏性变更
