# fix-prettier-formatting

> 修复 `lint.yml` 中 `prettier check` 步骤报「72 files have formatting issues」并 exit 1 的问题。

## 状态

草案。

## 问题描述

`lint.yml` 的 `frontend-lint` job 跑 `npx prettier --check .`，报：

```
[warn] .commitlintrc.json
[warn] .github/ISSUE_TEMPLATE/bug_report.yml
... (省略 70 行) ...
[warn] vite.config.ts
Code style issues found in 72 files. Run Prettier with --write to fix.
Error: Process completed with exit code 1.
```

来源：

- `.github/workflows/lint.yml` 第 43–44 行：`npx prettier --check .`
- `.prettierrc.json`：`singleQuote: false`、`printWidth: 100`、`tabWidth: 2`、`useTabs: false`、`semi: true`、`trailingComma: "all"`、`arrowParens: "always"`、`endOfLine: "lf"`

## 根因

1. **Sprint 0（`feat-bootstrap-tau-app`）只把 `prettier@^3` 装进 devDependencies + 配置 `.prettierrc.json`**，但从未对仓库已有文件执行过 `prettier --write`。结果是：仓库内的 `src/` / `src-tauri/` / `docs/` / 配置文件 / `.opencode/agents/*.md` / `AGENTS.md` 等 72 个文件全部用「手写风格」写就，与 prettier 现行规则的格式化输出不一致。
2. **仓库约定**：
   - **Tauri / Rust 侧**（`src-tauri/**`）由 Cargo + 各自 formatter（`cargo fmt`、`clippy`）管理
   - **Markdown 文档与 agent prompt**（`*.md`）的换行 / 引号风格由团队约定维护，不交给 prettier
   - **结构化配置文件**（`*.yml` / `*.yaml` / `*.json`）由编辑器 / lint 工具 / 包管理器各自维护；prettier 的引号 / 换行风格与手调配置、package.json 的键、lockfile 等冲突
3. **可行修复路径**：
   - (a) 在 `.prettierignore` 里把上述 4 类文件全部排除 + 对剩余的源文件跑 `prettier --write`
   - (b) 把 `.prettierrc.json` 改成符合手写风格（`singleQuote: true` 等）
   - (c) 只在 `lint.yml` 改 `--check` → 加 ignore 路径（不改源码）

   (a) 是「让代码服从 config，但把不该管的交给各自 formatter」的标准做法；选 (a)。

## 修复方案

**两个动作**：

1. **新增 `.prettierignore`**：
   ```
   # Markdown — not driven by Prettier; the team uses its own conventions
   # for docs/agent prompts and prettier's wrapping/quote style conflicts.
   *.md

   # Tauri / Rust side — Cargo handles its own formatting, Tauri config
   # and capabilities JSON are intentionally not driven by Prettier.
   src-tauri/**

   # Structured config files — managed by their respective ecosystems
   # (YAML by editor/lint tooling, JSON by package managers / config tools).
   # Prettier's quote/wrap style conflicts with hand-tuned configs and
   # package.json keys / lockfiles.
   *.yml
   *.yaml
   *.json

   # Node / Vite / build output
   node_modules/
   dist/
   dist-ssr/
   out/

   # Lockfiles (managed by their respective package managers)
   package-lock.json
   pnpm-lock.yaml
   yarn.lock
   Cargo.lock
   ```

2. **运行 `prettier --write .`** 一次，把剩余 29 个源文件（`.ts` / `.tsx` / `.css` / `.js` / `.html`）格式化到符合 `.prettierrc.json` 的输出。

3. **不修改 `.prettierrc.json`**：保留 `singleQuote: false` 等真实状态；与 Sprint 0 plan 文档漂移由单独文档任务修复，不与本格式化任务混。
4. **不修改 `.github/workflows/lint.yml`**：保留 `npx prettier --check .`。

## 回归验证

1. **CI `frontend-lint` job**：步骤
   - `npm ci`
   - `prettier check`（=`npx prettier --check .`）：输出 `Checking formatting... All matched files use Prettier code style!`，exit 0
   - `eslint`、`typecheck` 不受影响
2. **CI 其他 job**（`rust-lint` / `frontend-test` / `build-macos` / `build-linux`）：不受影响
3. **本地**：`npm ci && npm run format:check` exit 0
4. **后续加入新文件**：非 `.md` / 非 `*.json` / 非 `*.yml` / 非 `*.yaml` / 非 `src-tauri/**` 的新文件应按 `.prettierrc.json` 配置走 prettier；pre-commit hook 的 `prettier --check`（`.pre-commit-config.yaml` 第 26–28 行）继续生效
5. **范围约束自检**：PR 仅含 `.prettierignore` 与 29 个源码文件的 prettier 自动格式化差异；不包含任何手写逻辑变更

## 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| `prettier --write` 一次性改 29 个文件，diff 中等 | code review 工作量 | 单次 PR；reviewer 可聚焦「这是 prettier 自动改的，非手写逻辑」；不影响功能正确性 |
| `*.md` / `*.json` / `*.yml` / `*.yaml` 整体忽略后，未来这些格式的排版无 prettier 兜底 | 风格漂移 | 由 markdownlint / JSON schema / yamlfmt 等生态工具兜底（若团队后续引入）；本任务不引入新工具 |
| `src-tauri/**` 整体忽略后，未来加入的 `src-tauri/frontend_dist/` 等非 Rust 文件可能也被跳过 | 误忽略 | 当前 `src-tauri/` 仅含 Rust + Tauri 配置；如未来加入新类型文件需重新评估 ignore |
| Sprint 0 plan 文档漂移（plan 写 `singleQuote: true`，实际 `false`） | 文档不一致 | 不在本任务范围；后续文档清理任务处理 |

## 关联

- `docs/tasks/feat-bootstrap-tau-app/plan.md`：Sprint 0 引入了 prettier 但未跑过 `--write`
- `docs/pm/core/04-sprint-v0.1.md`：v0.1 工程门禁要求 lint job 全绿
- `AGENTS.md` Git Workflow：分支 `fix/prettier-formatting`、Conventional Commits

## 参考链接

- Prettier `--write` / `--check`：<https://prettier.io/docs/cli#--write>
- Prettier `.prettierignore`：<https://prettier.io/docs/ignore>
- Prettier `.gitignore` integration：<https://prettier.io/docs/ignore#ignoring-files-gitignore>
