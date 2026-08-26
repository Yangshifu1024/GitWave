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

## 相关文档

- [核心文档](../core/README.md)：功能列表 / 范围与优先级 / 版本路线