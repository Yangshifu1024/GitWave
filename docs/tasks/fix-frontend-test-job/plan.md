# fix-frontend-test-job

> 修复 GitHub Actions `test.yml` 中 `frontend-test` job 因为仓库内无 vitest 测试文件而失败的问题。

## 状态

草案。

## 问题描述

CI 日志：

```
> gitwave@0.1.0 test
> vitest run

 RUN  v2.1.9 /home/runner/work/GitWave/GitWave

include: **/*.{test,spec}.?(c|m)[jt]s?(x)
exclude:  **/node_modules/**, **/dist/**, **/cypress/**, **/.{idea,git,cache,output,temp}/**, **/{karma,rollup,webpack,vite,vitest,jest,ava,babel,nyc,cypress,tsup,build,eslint,prettier}.config.*

No test files found, exiting with code 1
Error: Process completed with exit code 1.
```

来源：`.github/workflows/test.yml` 第 45–46 行：

```yaml
- name: vitest
  run: npm test
```

`package.json` 第 15 行：

```json
"test": "vitest run",
```

实际状态：

- `vitest@^2` 已在 `devDependencies`（`package.json` 第 63 行）
- `frontend-test` job 已在 `.github/workflows/test.yml` 注册
- 但仓库内**无任何 `*.test.ts(x)` / `*.spec.ts(x)` 文件**（`src/` 下无 tests 目录、`src/**/*` 也无匹配）
- `cargo test --all-targets` 不受影响：Rust 侧 `domain/` `application/` `infrastructure/` 各层已有不少 `#[cfg(test)] mod tests`（`src-tauri/src/domain/{workspace,history,branch,blame,diff,error}.rs`、`infrastructure/git/*`、`infrastructure/persistence/*`、`infrastructure/observability/tracing.rs`、`infrastructure/ssh/keys.rs`、`application/use_cases.rs`），`rust-test` job 当前是绿的

## 根因

1. **Sprint 0（`feat-bootstrap-tau-app`）的完成定义要求「`cargo test` + `vitest run` 全绿」**，但 plan §2 「前端工具链」只引入 `vitest`，并未在 Sprint 0 范围内产出前端单元测试；前端测试是 Sprint 1+（W1 Workspace CRUD）才需要。
2. **`test.yml` 的 `frontend-test` job 提前在 Sprint 0 落地**，但 `vitest run` 默认行为是「找不到任何测试文件 → exit 1」，导致 CI 红，与 PM 文档意图（CI 矩阵存在但 frontend-test 此时是空集）不符。
3. **可行的修复路径**有四种，按改动量从小到大：
   - (a) `npm test` 加 `--passWithNoTests` CLI 标记
   - (b) 在 `vite.config.ts` 加 `test: { passWithNoTests: true }` 配置
   - (c) `test.yml` 用 `if: hashFiles(...)` 条件跳过
   - (d) 删 `frontend-test` job

   (a) 改动最小（1 行 package.json），语义最清晰（"允许空测试集通过"），且 vitest 官方推荐；选 (a)。

## 修复方案

**只改 `package.json` 的 `test` 脚本**，加 `--passWithNoTests`。

### 替换前

```json
"test": "vitest run",
```

### 替换后

```json
"test": "vitest run --passWithNoTests",
```

### 为什么是 CLI flag 而不是配置文件

- 改动最小：1 行
- 不污染 `vite.config.ts`（vite 配置只服务于 dev/build，不应该被 vitest 的存在所影响）
- 后续 Sprint 1+ 加测试时无需移除/改动任何配置：vitest 检测到测试文件后 `--passWithNoTests` 自动失效（vitest 行为：只在零测试时生效）
- 与 vitest 官方推荐一致：<https://vitest.dev/config/#passwithnotests>

### 不动其他东西

- **不改** `.github/workflows/test.yml`（保持 Sprint 0 设定的 CI 矩阵形状）
- **不改** `vite.config.ts`
- **不动** Rust 测试相关任何文件
- **不创建** 占位测试文件（避免假数据随 Sprint 1 真测试一起腐烂）

## 回归验证

1. **本地**：`npm install && npm test` 退出码 0，输出形如 `No test files found, exiting with code 0`。
2. **CI `frontend-test` job**：推送后 step `vitest` exit 0，job 绿。
3. **CI `rust-test` job**：不受影响（macOS + Ubuntu 双平台仍绿）。
4. **CI `lint` / `build` job**：不受影响。
5. **后续 Sprint 加测试的兼容性**：在 `src/` 任一子目录加一个 `.test.ts` 文件后，`npm test` 自动检测到并执行，行为不依赖任何额外配置改动。
6. **范围约束自检**：`package.json` 之外，PR 不应有其他改动。

## 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| Sprint 1+ 添加第一个前端测试时忘记删除 `--passWithNoTests` | 无实际影响：flag 在有测试时是 no-op | 可在 Sprint 1 review 时清理；不必预先删除 |
| `passWithNoTests` 与 vitest 版本不兼容 | `npm test` 仍 exit 1 | 当前锁的 `vitest@^2` 从 1.x 起就支持；如升级到 vitest 3 行为不变（<https://vitest.dev/config/#passwithnotests>） |
| 把"没有测试"当成 CI 绿 | 假阴性 | `lint.yml` 的 eslint + prettier check、`rust-test` 的 cargo test 已覆盖实际代码质量；前端测试缺位由 Sprint 1 单独建任务补齐 |

## 关联

- `docs/tasks/feat-bootstrap-tau-app/plan.md`：Sprint 0 的 CI 矩阵在此落地
- `docs/pm/core/04-sprint-v0.1.md` Sprint 1：「单元测试覆盖 SQLite 适配 + libgit2 适配」（前端测试正式交付时间点）
- `AGENTS.md` Git Workflow：分支 `fix/frontend-test-job`、Conventional Commits

## 参考链接

- Vitest config: `passWithNoTests`：<https://vitest.dev/config/#passwithnotests>
- Vitest CLI: `vitest run --passWithNoTests`：<https://vitest.dev/guide/cli.html>
