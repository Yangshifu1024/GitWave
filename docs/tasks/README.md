# GitWave · Tasks

本目录记录每次 agent 执行产生的技术方案与审查报告。

## 流程

1. **需求沟通**：用户提出需求并与 PM 沟通完成后，PM 写入 `@docs/pm/features/`（参考 `docs/pm/features/README.md` 模板与命名规范）
2. **方案分析**：对需求进行技术分析，生成技术方案，写入 `docs/tasks/<feat-任务名>/plan.md`
3. **缺陷修复**：对问题进行分析，生成修改方案，写入 `docs/tasks/<fix-任务名>/plan.md`
4. **代码审查**：code-reviewer 的结论报告，写入 `docs/tasks/任务名/review.md`

## 目录命名

`<feat|fix>-<任务名>/`

- `feat-`：新功能需求
- `fix-`：缺陷修复
- `<任务名>`：小写字母与连字符；与 `docs/pm/features/` 中对应条目编号对齐

例：

```
docs/tasks/
├── feat-ai-commit-multicontext/
│   ├── plan.md
│   └── review.md
└── fix-clone-ssh-failure/
    └── plan.md
```

## 文件结构

每个任务目录至少包含以下文件之一：

| 文件 | 产出方 | 内容 |
|---|---|---|
| `plan.md` | 工程团队 / architect | 技术方案或修改方案，含目标、步骤、验证 |
| `review.md` | code-reviewer | 审查结论报告（7 维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践） |

可选附加：

- `notes.md`：实施过程记录
- `testing.md`：tester 输出的测试用例或缺陷分析

## 关联

- `@AGENTS.md` 工作流程小节：触发与执行规范
- `@docs/pm/features/`：需求与变更源头
- `@docs/tech/`：跨任务的架构与选型文档（本目录 plan / review 可引用其中条目）