# feat-windows-runner

> 将 Windows runner 加入 CI 矩阵（build / test / lint 三 job 全部覆盖）。

## 状态

草案。**注：本任务把 `docs/pm/core/03-roadmap.md` 中规划在 v0.2 才加入的 Windows runner 提前到 v0.1**，由 PM 决策确认。

## 范围调整（与 PM 计划对比）

| 阶段 | 计划平台 | 来源 | 当前 |
|---|---|---|---|
| v0.1 | macOS + Linux | `docs/pm/core/03-roadmap.md` 第 10 行 | + Windows（**本任务提前**） |
| v0.2 | + Windows | `docs/pm/core/03-roadmap.md` 第 10 行 | — |
| v0.3 | 三平台稳定 | `docs/pm/core/03-roadmap.md` 第 47 行 | — |

**提前理由**：本地开发 + CI 已具备 Windows 适配能力（WebView2 + MSVC + `npm run tauri -- build`），无新增技术风险；把 v0.2 工作前置可让 Sprint 0–4 的 14 个 must-have 在三端即时拦截（特别是 WebView2 vs WebKit 行为差异）。

## 改动清单

仅改三个 workflow 文件，不改源码、不改依赖。

### 1. `.github/workflows/build.yml`：新增 `build-windows` job

参考现有 `build-macos` / `build-linux`，Windows runner 特点：

- `runs-on: windows-latest` 已预装 MSVC build tools、WebView2 Runtime、PowerShell
- 不需要 `apt-get install`（Windows 用 chocolatey 或 PowerShell），Tauri / Rust / Node 由各 step 显式安装
- 默认 Rust target 是 `x86_64-pc-windows-msvc`，无需显式声明
- Tauri build 产物路径：`src-tauri/target/release/bundle/msi/*` 与 `bundle/nsis/*`

替换位置：紧跟 `build-linux` 之后，作为第三个 job。

### 2. `.github/workflows/test.yml`：`rust-test` 矩阵加入 `windows-latest`

当前：

```yaml
matrix:
  os: [macos-latest, ubuntu-latest]
```

改为：

```yaml
matrix:
  os: [macos-latest, ubuntu-latest, windows-latest]
```

`Install build deps (Linux)` 步骤已有 `if: runner.os == 'Linux'`，Linux-only 的 `cmake` 安装只在 Ubuntu 跑，不会影响 Windows。`cargo test --all-targets` 在 Windows / MSVC 下正常工作。

### 3. `.github/workflows/lint.yml`：`rust-lint` 加入 Windows 维度

当前 `rust-lint` 只在 `macos-latest` 跑（runner 固定）。

两条路径：

- **路径 A（最小化）**：把 `runs-on: macos-latest` 改为 matrix，加入 `windows-latest`
- **路径 B（与 test.yml 对齐）**：matrix `[macos-latest, ubuntu-latest, windows-latest]`

选 **A**。理由：`cargo fmt` + `cargo clippy` 在 macOS 上已能覆盖格式与 lint；加 Windows 仅作为「WebView2 / MSVC 特定 clippy lint」的兜底，避免一次性把三平台 lint 全开导致 CI 时长翻倍。

替换前：

```yaml
rust-lint:
  name: Rust lint
  runs-on: macos-latest
  steps: ...
```

替换后：

```yaml
rust-lint:
  name: Rust lint (${{ matrix.os }})
  runs-on: ${{ matrix.os }}
  strategy:
    fail-fast: false
    matrix:
      os: [macos-latest, windows-latest]
  steps: ...
```

`frontend-lint` 维持 `ubuntu-latest`（prettier / eslint / tsc 与 OS 无关，单平台足够）。

## 替换前 / 替换后（具体 diff）

### `build.yml` 新增段（追加到文件末尾 `build-linux` 之后）

```yaml
  build-windows:
    name: Build (Windows)
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      - uses: dtolnay/rust-toolchain@stable
      - uses: Swatinem/rust-cache@v2
        with:
          workspaces: src-tauri -> target
      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm
      - name: npm ci
        run: npm ci
      - name: tauri build
        run: npm run tauri -- build
      - uses: actions/upload-artifact@v4
        with:
          name: gitwave-windows
        path: |
          src-tauri/target/release/bundle/msi/*
          src-tauri/target/release/bundle/nsis/*
        if-no-files-found: warn
```

### `test.yml` `rust-test` 矩阵 diff

```diff
       matrix:
         os: [macos-latest, ubuntu-latest]
+        os: [macos-latest, ubuntu-latest, windows-latest]
```

（注：实际替换只一行：`os: [macos-latest, ubuntu-latest]` → `os: [macos-latest, ubuntu-latest, windows-latest]`）

### `lint.yml` `rust-lint` diff

```diff
   rust-lint:
-    name: Rust lint
-    runs-on: macos-latest
-    steps:
+    name: Rust lint (${{ matrix.os }})
+    runs-on: ${{ matrix.os }}
+    strategy:
+      fail-fast: false
+      matrix:
+        os: [macos-latest, windows-latest]
+    steps:
       - uses: actions/checkout@v4
       - uses: dtolnay/rust-toolchain@stable
         with:
           components: clippy, rustfmt
       - uses: Swatinem/rust-cache@v2
         with:
           workspaces: src-tauri -> target
       - name: cargo fmt
         working-directory: src-tauri
         run: cargo fmt -- --check
       - name: cargo clippy
         working-directory: src-tauri
         run: cargo clippy --all-targets -- -D warnings
```

## 不动的东西

- 所有 Rust 源码 / Cargo.toml / Cargo.lock
- 前端 `src/` / `package.json` / `package-lock.json`
- `.github/workflows/` 三个文件之外的任何 `.github/` 文件
- `docs/` 任何文档（roadmap 调整由 PM 单独任务处理）
- `.prettierignore` / `.prettierrc.json`

## 回归验证

1. **CI `build-macos` / `build-linux` / `rust-test` (macos + ubuntu) / `rust-lint` (macos)**：维持现有行为，不因本任务变绿/变红。
2. **CI `build-windows` job**：
   - Runner 选 `windows-latest`，预期：npm ci 通过、cargo 自动选 MSVC target、`npm run tauri -- build` 产出 `.msi` 或 `.nsis`，artifact `gitwave-windows` 上传（路径 `bundle/msi/*` 或 `bundle/nsis/*`）
   - 失败兜底：`if-no-files-found: warn` 保证即使某个 bundle 缺失也不让整个 job 红
3. **CI `rust-test (windows-latest)`**：`cargo test --all-targets` 通过；现有 `Install build deps (Linux)` step 已用 `if: runner.os == 'Linux'` gate，不会在 Windows 跑（apt-get 也不存在）
4. **CI `rust-lint (windows-latest)`**：`cargo fmt -- --check` 与 `cargo clippy --all-targets -- -D warnings` 通过；Swatinem/rust-cache 在 Windows 下需确认 cache key 兼容（应兼容，已知在 windows runner 上工作）
5. **范围约束自检**：PR diff 仅含 `.github/workflows/{build,test,lint}.yml`，无任何源码变更
6. **CI 总时长**：
   - `build-windows` job 单跑约 ~10–15 min（cargo build release + tauri bundle 第一次冷启 ~ 8 min，后续命中 cache ~3 min）
   - `rust-test (windows-latest)` 新增 ~3–5 min
   - `rust-lint (windows-latest)` 新增 ~3–5 min
   - 总增量：~15–25 min（与现有 macos + linux 总时长叠加，不替换）

## 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| Windows runner 首次构建冷启慢（无 cache） | CI 时长增加 | Swatinem/rust-cache 跨 platform cache key 已知兼容；第二次起命中 |
| WebView2 在 windows-latest 上的 preinstalled 版本与本地开发机不同 | UI 渲染差异 | 在 PR description 标注「CI uses WebView2 version X」；后续若发现差异再 pin |
| `npm run tauri -- build` 在 Windows 下因 PowerShell 路径解析报路径相关错误 | build 失败 | Tauri CLI 2 在 windows runner 上已 GA；如失败，先 `npm run tauri -- build --debug` 调试 |
| Swatinem/rust-cache 在 Windows 上的 `workspaces: src-tauri -> target` 路径映射 | cache miss 或路径错误 | 该 action 维护者已声明 Windows 支持（参考 [Swatinem/rust-cache#cache-details](https://github.com/Swatinem/rust-cache#cache-details)）；如出错，回退到「不指定 workspaces，使用默认 cache key」 |
| `windows-latest` 镜像升级后 MSVC / WebView2 版本变化 | 偶发 CI 失败 | 用 GitHub 维护的 [windows-2022](https://github.com/actions/runner-images) 锁定大版本（`runs-on: windows-2022`），与 `windows-latest` 等价但更稳 |
| `v0.2 Windows 适配` 任务失去意义 | roadmap 漂移 | 单独 PM 任务调整 `docs/pm/core/03-roadmap.md`，把 v0.2 的 Windows 适配内容移走；本任务不修改 PM 文档 |
| cargo clippy 在 Windows / MSVC 上报 platform-specific lint（如 `clippy::unused_async` for FFI） | rust-lint 红 | 该类 lint 已是 clippy 默认；如出现，单独处理 |
| `frontend-test` / `frontend-lint` 仍只在 Ubuntu 跑 | Windows 下前端问题不立即拦截 | 与现有 plan 一致；前端 OS-agnostic，无需三平台 |

## 关联

- `docs/pm/core/03-roadmap.md` 第 10 行（v0.2 + Windows）
- `docs/pm/core/04-sprint-v0.1.md` 第 178 行（v0.1 CI 仅 macOS + Linux）
- `docs/tech/decisions/00-overview.md` 第 21 行（v0.1 仅 macOS、v0.2 扩 Windows、v0.3 加 Linux）
- `docs/tasks/feat-bootstrap-tau-app/plan.md` 第 13 行（Sprint 0 CI 矩阵 + Windows runner v0.2 加入）
- `AGENTS.md` Git Workflow：分支 `feat/windows-runner`、Conventional Commits

## 参考链接

- Tauri 2 Windows prerequisites：<https://v2.tauri.app/start/prerequisites/#windows>
- GitHub Actions `windows-latest`：<https://github.com/actions/runner-images#available-images>
- `Swatinem/rust-cache` Windows 兼容性：<https://github.com/Swatinem/rust-cache#cache-details>
- Tauri Windows bundle 类型（`msi` / `nsis`）：<https://v2.tauri.app/distribute/bundles/>
