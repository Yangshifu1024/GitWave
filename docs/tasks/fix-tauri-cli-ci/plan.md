# fix-tauri-cli-ci

> 修复 GitHub Actions `build.yml` 中 `build-macos` job 报 `error: no such command: tauri` 的问题。

## 状态

草案。

## 问题描述

`build-macos` job 失败日志：

```
cargo tauri build
  shell: /bin/bash -e {0}
  env:
    CARGO_HOME: /Users/runner/.cargo
    CARGO_INCREMENTAL: 0
    CARGO_TERM_COLOR: always
    CACHE_ON_FAILURE: false
error: no such command: `tauri`

help: a command with a similar name exists: `miri`

help: view all installed commands with `cargo --list`
help: find a package to install `tauri` with `cargo search cargo-tauri`
Error: Process completed with exit code 101.
```

来源：`.github/workflows/build.yml` 第 31–33 行：

```yaml
- name: tauri build
  working-directory: src-tauri
  run: cargo tauri build
```

## 根因

1. **`cargo tauri build` 调用的是 `cargo-tauri` 这个 Cargo 子命令**（来自 `cargo install cargo-tauri` 或 `cargo install tauri-cli`），CI runner 上的 Rust toolchain 默认不装它。
2. 仓库里已经有 **`@tauri-apps/cli@^2`**（`package.json` 第 51 行 devDependencies），但 CI 没有用 npm 路径，而是直接调 `cargo tauri`。
3. bootstrap plan `feat-bootstrap-tau-app/plan.md` §3「前端工具链」只写了「引入 `@tauri-apps/cli`」（npm 版），没有约定 CI 必须 `cargo install tauri-cli`；实际 CI 写了 `cargo tauri build` 但没装 cargo 子命令，是 plan / CI 实现不一致的 bug。

### 可选修复路径

| 路径 | 改动 | 成本 | 风险 |
|---|---|---|---|
| (a) 改 CI 调 npm 版的 `@tauri-apps/cli` | 改 `build.yml` + `build-macos` / `build-linux` 步骤 | 0 安装时间，复用已锁的 devDep | 行为应与 cargo-tauri 一致 |
| (b) 在 CI 里 `cargo install tauri-cli --locked` | 仅新增步骤 | 增加 ~5–10 min / job | 多一次编译，需锁版本 |
| (c) 切到 `tauri-apps/tauri-action` | 重写两个 job | 0 编译时间，但重构 | 与 Swatinem/rust-cache 协作方式需评估 |

选 (a)：**改动最小、与项目意图（用 npm CLI）一致、零额外 CI 时间**。

## 修复方案

**只改 `.github/workflows/build.yml`**：

1. `build-macos` 与 `build-linux` 的 `tauri build` 步骤改为：
   ```yaml
   - name: tauri build
     run: npm run tauri -- build
   ```
   （去掉 `working-directory: src-tauri`，从仓库根运行；`@tauri-apps/cli` 与 `cargo-tauri` 一样会自动向上查找 `src-tauri/tauri.conf.json`，并以 `src-tauri/Cargo.toml` 为 Rust 入口。）
2. `npm ci` 步骤必须**先于** `tauri build` 执行。当前 `build-macos` 已经先 `npm ci` 后 `tauri build`，`build-linux` 也是，无需调整顺序。
3. **不引入** `cargo install tauri-cli`，避免无谓 CI 时间膨胀。

### 替换前（build.yml 第 31–33 行）

```yaml
- name: tauri build
  working-directory: src-tauri
  run: cargo tauri build
```

### 替换后

```yaml
- name: tauri build
  run: npm run tauri -- build
```

`build-linux` 同位置的步骤（build.yml 第 66–68 行）同步替换。

### 为什么从仓库根跑 `npm run tauri -- build` 是对的

- `npm run tauri` 在 `package.json` 第 10 行定义为 `"tauri": "tauri"`，npm 会从仓库根的 `node_modules/.bin/tauri` 启动 `@tauri-apps/cli`
- `@tauri-apps/cli` 在 CWD 为仓库根时，会自动定位 `src-tauri/tauri.conf.json`（沿父目录向上查找，且 `--config` 默认指向相对路径 `src-tauri/tauri.conf.json`）
- `tauri build` 内部 `beforeBuildCommand: "npm run build"` 与 `cargo build --release` 都按 `src-tauri/tauri.conf.json` 的相对路径解析，CWD 影响已收敛在 tauri.conf.json 内部
- 这与本地开发体验一致：`pnpm tauri build` 从仓库根跑能成功，CI 也从仓库根跑同样的命令

### 不动其他东西

- `package.json`（`@tauri-apps/cli@^2` 已在）
- `src-tauri/Cargo.toml` / `src-tauri/tauri.conf.json`
- `lint.yml` / `test.yml`
- `.pre-commit-config.yaml`
- `Swatinem/rust-cache` 配置

## 回归验证

1. **CI `build-macos` job**：步骤序列
   - `npm ci`
   - `tauri build`（= `npm run tauri -- build`）
   - `actions/upload-artifact`
   - 预期：cargo 从 src-tauri 编译 release 成功，产物在 `src-tauri/target/release/bundle/macos/*` 与 `dmg/*`，artifact `gitwave-macos` 上传。
2. **CI `build-linux` job**：与 `build-macos` 同样的步骤序列 + 前置的 `Install Tauri Linux deps`（已在前序 `fix-linux-ci-build` 修复），artifact `gitwave-linux` 包含 `bundle/deb/*` 或 `bundle/appimage/*`。
3. **本地**：`npm run tauri -- build` 从仓库根跑通，与 `cargo tauri build` 行为等价（这点已在前序 `feat-bootstrap-tau-app` 验证过；本地 dev 用 `npm run tauri -- dev` 走的就是这条路径）。
4. **范围约束自检**：`.github/workflows/build.yml` 之外，PR 不应有其他改动。

## 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| `@tauri-apps/cli` 与 `cargo-tauri` 在某条 flag 上行为不一致 | build 产物或 bundle 配置差异 | 两个 CLI 是同一个 monorepo 的不同分发（npm vs cargo），CLI flag 兼容；如未来出现差异，由 `tauri-apps/tauri-action` 切回统一入口 |
| `npm run tauri -- build` 在 CWD=仓库根时找不到 `tauri.conf.json` | build 失败 | `@tauri-apps/cli` 向上查找已稳定多年；如出问题可加 `--config src-tauri/tauri.conf.json` 显式指定 |
| Swatinem/rust-cache 因 CLI 路径变化命中率下降 | CI 变慢 | 缓存键仅依赖 `Cargo.lock` / `Cargo.toml` / 编译 env，不依赖前端 CLI 路径，命中率不受影响 |
| `macos-latest` runner 上 npm 20 默认锁行为变化 | 偶发 | 已用 `actions/setup-node@v4` + `node-version: 20` 锁定版本 |

## 关联

- `docs/tasks/feat-bootstrap-tau-app/plan.md`：约定用 npm 版 CLI，但 CI 实现偏离
- `docs/tasks/fix-linux-ci-build/plan.md`：前序 Linux 依赖修复（同一 job 链路）
- `AGENTS.md` Git Workflow：分支 `fix/tauri-cli-ci`、Conventional Commits

## 参考链接

- `@tauri-apps/cli`（npm 包，与 `cargo-tauri` 同源）：<https://www.npmjs.com/package/@tauri-apps/cli>
- `tauri-apps/tauri-action`（备选方案，本任务未采用）：<https://github.com/tauri-apps/tauri-action>
- Tauri CLI 命令列表（npm 与 cargo 版等价）：<https://v2.tauri.app/reference/cli/>
