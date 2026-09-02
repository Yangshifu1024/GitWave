# GitWave · Features

本目录用于跟踪功能提案与变更请求。每条记录独立成文件，便于评审与追溯。

## 文件命名

`F<编号>-<短描述>.md`

- `F` = Feature
- 编号 = 三位数字，按登记顺序递增
- 短描述 = 小写字母与连字符

例：`F001-ai-commit-multicontext.md`

## 文件模板

```markdown
# F<编号> · <标题>

## 背景

<为什么需要这个功能？解决什么问题？>

## 提议方案

<具体做什么？>

## 影响

- 涉及模块：
- 影响版本：
- 是否破坏向后兼容：

## 决策

- 状态：提案 / 接受 / 拒绝 / 已合并
- 决策人：
- 决策日期：
- 关联决策：
```

## 状态

| 状态 | 含义 |
|---|---|
| **提案** | 已登记，待评审 |
| **接受** | 评审通过，待纳入 |
| **拒绝** | 评审未通过，已记录原因 |
| **已合并** | 已纳入产品（同步至 [核心文档](../core/)） |

## 当前条目

| 编号 | 标题 | 状态 | 关联 |
|---|---|---|---|
| [F001](./F001-workspace-crud.md) | Workspace CRUD | 接受 | Sprint 1 |
| [F002](./F002-repo-ingestion.md) | Repo Ingestion + SSH Key Management | 接受 | Sprint 2 |
| [F003](./F003-history-diff-blame.md) | History Graph + File Diff + Blame + Branch Ops | 接受 | Sprint 3 |
| [F004](./F004-safe-branch-switch.md) | Safe branch switch（双击切换 + 脏工作区弹窗） | 接受 | v0.1 |
| [F005](./F005-repo-tab-drag-reorder.md) | Repository Tab 拖动排序 | 接受 | v0.2.x |
| [F006](./F006-font-settings.md) | 字体设置（UI 字体 + Mono 字体） | 接受 | v0.2.x |
| [F007](./F007-macos-native-menu.md) | macOS 原生系统菜单 | 接受 | v0.3.x |
| [F008](./F008-website.md) | 项目官网（GitHub Pages + 自定义域名） | 接受 | v0.4.x |
| [F009](./F009-auto-update.md) | 应用内检查更新与自动更新 | 接受 | v0.5.0 |
| [F010](./F010-i18n.md) | 完整国际化（UI 中英双语 + AI 回复语言） | 接受 | v0.6.0 |
| [F011](./F011-commit-context-menu.md) | History 提交右键菜单（参考 Fork） | 接受 | v0.7.x |
| [F012](./F012-checkout-remote-branch.md) | 双击远程分支：创建同名本地分支并切换（DWIM） | 接受 | v0.7.x |
| [F013](./F013-system-proxy.md) | 支持系统代理（网络设置节：跟随系统 / 手动 / 关闭） | 接受 | v0.7.x |

## 相关文档

- [核心文档](../core/README.md)：功能列表 / 范围与优先级 / 版本路线