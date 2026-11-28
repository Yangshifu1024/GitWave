# F020 · 终端与编辑器选择

**状态**　已合入 v0.12.2

终端和编辑器各有一个纯文字入口。首次点击展开已安装工具列表。选中一项会在当前仓库根目录启动它；启动成功后按钮显示该工具的名称。再次点击名称直接启动，点击旁边的箭头可以改选。当前选择在菜单内以文字标记。

两类工具分别在本机记住最近一次成功选择，跨仓库、工作区和应用重启生效。启动失败保留原选择。上次的工具已卸载时恢复选择菜单。无活动仓库或仓库缺失时禁用入口；有仓库但未检测到工具时，打开菜单显示提示。

| 平台 | 终端候选 |
|---|---|
| macOS | iTerm2、系统终端、Ghostty、Warp、WezTerm、kitty、Alacritty |
| Windows | Windows Terminal、WezTerm、Alacritty、PowerShell 7、Windows PowerShell、命令提示符、Git Bash |
| Linux | GNOME Terminal、Konsole、Xfce Terminal、GNOME Console、Ptyxis、Ghostty、kitty、Alacritty、foot、WezTerm、Tilix、XTerm |

去重后共 20 款终端。Linux 的可执行 `$TERMINAL` 优先，已识别的别名只显示一次。编辑器包括 Visual Studio Code、Zed 和 VSCodium。

[任务计划](../../tasks/feat-terminal-picker/plan.md)
