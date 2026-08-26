# GitWave · AGENTS.md

> 项目入口指南。

## 必读文档

按以下顺序阅读：

1. [docs/pm/core/README.md](./docs/pm/core/README.md) — PM 文档索引
2. [01-features.md](./docs/pm/core/01-features.md) — 产品应具备的功能与不做清单
3. [02-scope.md](./docs/pm/core/02-scope.md) — 优先级与版本范围
4. [03-roadmap.md](./docs/pm/core/03-roadmap.md) — 版本路线
5. [docs/pm/features/README.md](./docs/pm/features/README.md) — 功能提案流程
6. [docs/tech/README.md](./docs/tech/README.md) — 技术文档索引（架构 / 选型 / ADR）
7. [docs/tasks/README.md](./docs/tasks/README.md) — 任务追踪（plan / review）

## 核心约束（来自产品原则）

- **AI 是协作者不是替代者**：禁止自动 commit / push / merge
- **本地优先 + 隐私可控**：diff 默认不离开本机；HTTPS 凭证走 `git credential helper`；SSH 走配置的 key
- **Workspace 是第一入口**：单 active 仓库 + 多 Workspace 同时开 + Workspace-scoped AI
- **Workspace 是抽象概念**：在文件系统中无实体，不依赖任何 root 目录

## PM / 工程边界

技术选型（Tauri / SwiftUI / Electron 等）属于工程决策，**不在 PM 文档范围内**。工程团队应基于 PM 给出的用户可感知约束（性能、平台、隐私等）做架构文档。PM 不输出选型，工程不输出原则。

## 技术文档归属（`docs/tech/` vs `docs/tasks/`）

工程文档按"跨任务 vs 单任务"分两个目录，避免归属混乱：

| 目录 | 性质 | 一份文档对应 | 典型内容 |
|---|---|---|---|
| `docs/tech/` | 跨任务工程文档 | 可被多个任务 / PR 引用 | 系统架构、技术选型、ADR、系统设计、工程约定 |
| `docs/tasks/<feat\|fix>-<name>/` | 单任务执行产物 | 一个 PR / 一个任务 | `plan.md`、`review.md` |

判定规则：

- 长期有效、被多次任务复用的内容（架构图、选型记录、ADR、命名 / 工程约定）→ `docs/tech/`
- 与某个具体 PR / 任务强绑定的执行过程（实施计划、审查报告）→ `docs/tasks/<任务名>/`

`docs/tasks/<任务名>/plan.md` 中如需引用既有技术决策，按 `docs/tech/<分类>/<文档名>` 链接。

## Git Workflow

### 分支策略（GitHub Flow）

- `main` 为唯一常驻分支
- 新功能 / 修复从 `main` 拉分支：`feature/<name>` 或 `fix/<name>`
- 命名对齐 `docs/pm/features/F<编号>.md` 或 `docs/tasks/<feat|fix>-<name>/`

### Commit 约定（Conventional Commits）

`<type>(<scope>): <subject>`，type ∈ `feat` · `fix` · `docs` · `refactor` · `test` · `chore`

例：`feat(workspace): add lastActiveRepo persistence on workspace switch`

### PR 合并（Squash Merge）

所有 PR squash merge 合入 `main`，squash commit message 遵循 Conventional Commits。

### 关键约束

- **AI 代理禁止自动 commit / push / merge**（符合 P1）
- **main 分支保护**：禁止 force push；PR 必须经 code-reviewer 审查通过
- **每个 PR 关联 proposal 或 task**：描述引用 `docs/pm/features/F<编号>.md` 或 `docs/tasks/<任务名>/plan.md`
- **新需求 / 新问题必须使用新分支**：处理前 AI 代理提出分支名推荐值（`feature/<name>` 或 `fix/<name>`），由用户确认后创建

## 可用专门代理

| 代理 | 触发场景 |
|---|---|
| **product-manager** | 需求分析、PRD、用户故事、竞品分析、优先级排序 |
| **code-reviewer** | 代码审查（正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践） |
| **tester** | 测试用例设计、测试策略、缺陷分析、自动化建议 |

代理详细行为约定见 `.opencode/agents/<name>.md`

按场景调用对应的专门代理。

### 1. 需求流程（用户提新需求时）

触发：用户提出新需求 / 功能想法

1. 调用 `@.opencode/agents/product-manager.md`
2. PM 分析需求、必要时提问澄清
3. PM 整理为结构化需求文档，写入 `docs/pm/features/F<编号>-<短描述>.md`
4. 工程团队分析需求、生成技术方案
5. 技术方案写入 `docs/tasks/<feat-任务名>/plan.md`
6. 状态流转：提案 → 接受 / 拒绝 → 已合并

### 2. 缺陷流程（用户提问题 / bug 时）

触发：用户报告问题、bug 或异常行为

1. 调用 `@.opencode/agents/tester.md`
2. Tester 复现问题、分析根因
3. Tester 给出最佳修复方案（含修改建议、回归测试要点）
4. 修改方案写入 `docs/tasks/<fix-任务名>/plan.md`
5. 落地修改并验证

### 3. 代码审查流程（开发完成后）

触发：开发完成、新代码待入库

1. 自动调用 `@.opencode/agents/code-reviewer.md`
2. Reviewer 按 7 个维度审查：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践
3. 严重问题（🔴）必须修复后再合入
4. 审查报告写入 `docs/tasks/任务名/review.md`

## 术语约定

- commit / rebase / merge / conflict / provider / prompt / BYOK / worktree 等技术词保留英文
- 中文用于叙述与判断
- 引用外部资料必须带 URL