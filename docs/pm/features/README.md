# GitWave · Features

本目录用于跟踪功能提案与变更请求。每条记录独立成文件，便于评审与追溯。

## 文件命名

`F<编号>-<短描述>.md`

- `F` = Feature
- 编号 = 三位数字，按登记顺序递增
- 短描述 = 小写字母与连字符

例：`F001-workspace-crud.md`

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

截至 v0.9.3，F001–F017 全部为 **已合并**，F018 为 **接受**（实现中）。编号 F012 被两条提案共用（检出远程分支 / 应用内凭证弹窗），文件名保持原样，不重编号。

| 编号 | 标题 | 状态 | 关联 |
|---|---|---|---|
| [F001](./F001-workspace-crud.md) | Workspace CRUD | 已合并 | Sprint 1 |
| [F002](./F002-repo-ingestion.md) | Repo Ingestion + SSH Key Management | 已合并 | Sprint 2 |
| [F003](./F003-history-diff-blame.md) | History Graph + File Diff + Blame + Branch Ops | 已合并 | Sprint 3 |
| [F004](./F004-safe-branch-switch.md) | Safe branch switch（双击切换 + 脏工作区弹窗） | 已合并 | v0.1 |
| [F005](./F005-repo-tab-drag-reorder.md) | Repository Tab 拖动排序 | 已合并 | v0.2.x |
| [F006](./F006-font-settings.md) | 字体设置（UI 字体 + Mono 字体） | 已合并 | v0.2.x |
| [F007](./F007-macos-native-menu.md) | macOS 原生系统菜单 | 已合并 | v0.3.x |
| [F008](./F008-website.md) | 项目官网（GitHub Pages + 自定义域名） | 已合并 | v0.4.x |
| [F009](./F009-auto-update.md) | 应用内检查更新与自动更新 | 已合并 | v0.5.0 |
| [F010](./F010-i18n.md) | 完整国际化（UI 中英双语 + AI 回复语言） | 已合并 | v0.6.0 |
| [F011](./F011-commit-context-menu.md) | History 提交右键菜单（参考 Fork） | 已合并 | v0.7.x |
| [F012](./F012-checkout-remote-branch.md) | 双击远程分支：创建同名本地分支并切换（DWIM） | 已合并 | v0.7.x |
| [F012](./F012-in-app-credential-prompt.md) | 应用内凭证弹窗（认证失败原地重试） | 已合并 | v0.7.x |
| [F013](./F013-system-proxy.md) | 支持系统代理（网络设置节：跟随系统 / 手动 / 关闭） | 已合并 | v0.7.x |
| [F014](./F014-open-external-tools.md) | ActionBar 外部打开按钮（文件管理器 / 终端） | 已合并 | v0.7.12 |
| [F015](./F015-open-in-editor.md) | ActionBar 编辑器按钮（VS Code / Zed / VSCodium） | 已合并 | v0.7.x |
| [F016](./F016-image-diff.md) | DiffView 图片 diff（左右分栏对比） | 已合并 | v0.7.x |
| [F017](./F017-repo-tab-wheel-scroll.md) | Repository Tab 溢出时滚轮横向滚动 + 激活 tab 入视野 | 已合并 | v0.9.x |
| [F018](./F018-getting-started.md) | 新手引导（应用内空状态 + 官网中英双语上手教程） | 接受 | v0.9.x |

## v0.8.8 起的登记方式

从 v0.8.8 起，新增功能直接落在 `docs/tasks/<feat-任务名>/plan.md`，不再为每个功能单独开提案文件；本目录保留 F001–F017 的历史提案，作为早期评审与追溯的记录。

例外只有两处：F017（仓库标签栏滚轮横向滚动）与 F018（新手引导）。F018 是因为用户明确要求「先出提案与实现计划，确认后再动代码」，而且这一次跨了应用界面、官网与文档三类改动，值得留一份提案。

理由写实：从 v0.8.8 到 v0.9.3 交付的 10 项功能与依赖升级（自动刷新全部仓库、一次添加多个本地仓库、stash 面板重做、提交列表滚动性能、时间格式统一、更新清单改用普通下载链接，以及 Vite 8.3.0、TypeScript 6、eslint js 10、lucide 1.x 等升级）都没有对应的提案文件（F017 仓库标签栏滚轮横向滚动、F018 新手引导是两处例外）。与其事后补一批走过场的提案，不如把约定写清楚。

## 相关文档

- [核心文档](../core/README.md)：功能列表 / 范围与优先级 / 版本路线
