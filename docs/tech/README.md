# GitWave · Tech Docs

> 跨任务的工程文档索引：架构、技术选型、ADR、系统设计、工程约定。

## 与 `docs/tasks/` 的边界

工程文档按"跨任务 vs 单任务"分两个目录：

| 目录 | 性质 | 一份文档对应 | 典型内容 |
|---|---|---|---|
| `docs/tech/` | 跨任务工程文档 | 可被多个任务 / PR 引用 | 系统架构、技术选型、ADR、系统设计、工程约定 |
| `docs/tasks/<feat\|fix>-<name>/` | 单任务执行产物 | 一个 PR / 一个任务 | `plan.md`、`review.md` |

判定规则：

- 长期有效、被多次任务复用的内容（架构图、选型记录、ADR、命名 / 工程约定）→ `docs/tech/`
- 与某个具体 PR / 任务强绑定的执行过程（实施计划、审查报告）→ `docs/tasks/<任务名>/`

`docs/tasks/<任务名>/plan.md` 中如需引用既有技术决策，按 `docs/tech/<分类>/<文档名>` 链接。

## 目录结构

```
docs/tech/
├── README.md             ← 本文件（索引）
├── architecture/         ← 系统架构、模块划分、数据流、关键序列图
├── tech-selection/       ← 技术选型记录（框架、语言、关键库，含上下文 / 备选 / 决策 / 后果）
├── decisions/            ← ADR 风格的架构决策记录
├── engineering/          ← 工程约定（命名、目录、依赖、CI、安全、日志、错误处理等）
└── planning/             ← 版本开发计划（里程碑、依赖、验收、风险）
```

## 命名与格式

- **文件名**：小写字母与连字符（kebab-case）
- **选型 / 决策类文档**建议四段式：`## 上下文` · `## 备选` · `## 决策` · `## 后果`
- **文档头**建议标注：作者、状态（`草案` / `已采纳` / `已废弃`）、最近更新日期
- **索引**：新增文档后，请同步在本 README 的"当前文档"小节登记链接

## 当前文档

| 分类 | 文档 | 内容 |
|---|---|---|
| tech-selection | [tech-selection/00-overview.md](./tech-selection/00-overview.md) | 桌面框架 / 前端栈 / Git 后端 / 存储 / AI 集成的选型与理由 |
| architecture | [architecture/00-overview.md](./architecture/00-overview.md) | 进程拓扑、DDD 分层、IPC 边界、性能热点、多 Workspace 并行 |
| decisions | [decisions/00-overview.md](./decisions/00-overview.md) | ADR 汇总（框架 / Workspace / 凭证 / AI 双轨） |
| engineering | [engineering/00-overview.md](./engineering/00-overview.md) | 代码风格 / 测试策略 / 错误与日志 / CI/CD / 安全 |
| planning | [planning/roadmap-v0.2.md](./planning/roadmap-v0.2.md) | v0.2 开发计划：里程碑 M0–M3、依赖、验收、风险 |
| planning | [planning/roadmap-v0.3.md](./planning/roadmap-v0.3.md) | v0.3 开发计划：里程碑 M0–M4、依赖、验收、风险 |

## 关联

- `@AGENTS.md` 工程边界小节：技术选型归工程，PM 仅给用户可感知约束
- `@docs/pm/core/`：用户可感知约束（性能、平台、隐私等）的源头
- `@docs/tasks/`：单任务执行产物的落点，可反向引用本目录