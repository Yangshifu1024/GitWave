# fix-ci-linux-tauri-deps

> 修复 `test.yml` 与 `lint.yml` 的 `Install build deps (Linux)` 步骤只装 `cmake` 导致 `cargo test --all-targets` / `cargo clippy --all-targets` 在 Linux runner 上缺 Tauri 系统库（`glib-2.0.pc` / `gio-2.0.pc` / `gobject-2.0.pc`）的问题。

## 状态

草案。**注**：此 bug 早在 Sprint 0（`feat-bootstrap-tau-app`）就存在；`fix/linux-ci-build`（Sprint 0 阶段一）只补了 `build.yml`，漏了 `test.yml` 与 `lint.yml`。`feat/windows-runner` 把 Linux 加进 `rust-test` 矩阵后立刻显形。

## 问题描述

CI 日志（来自 `feat/windows-runner` push 触发的 `rust-test (ubuntu-latest)`）：

```
cargo test --all-targets
  shell: /usr/bin/bash -e {0}
  ...
   Compiling glib-sys v0.18.1
   Compiling gobject-sys v0.18.0
warning: glib-sys@0.18.1:
error: failed to run custom build command for `glib-sys v0.18.1`
  ...
  pkg-config exited with status code 1
  > PKG_CONFIG_ALLOW_SYSTEM_CFLAGS=1 pkg-config --libs --cflags glib-2.0 'glib-2.0 >= 2.70'
  Package glib-2.0 was not found in the pkg-config search path.
  ...
Error: Process completed with exit code 101.
```

`gio-sys` / `gobject-sys` 报同样错误（同一 GLib 套件的另两个 `.pc`）。

来源：

- `.github/workflows/test.yml` 第 28–29 行：
  ```yaml
  - name: Install build deps (Linux)
    if: runner.os == 'Linux'
    run: sudo apt-get install -y cmake
  ```
- `.github/workflows/lint.yml` `rust-lint` job（新增 `ubuntu-latest` 维度后）：相同写法
- 参考 `.github/workflows/build.yml` 第 67–79 行（已正确安装完整 Tauri Linux 依赖）

## 根因

1. `cargo test --all-targets` 与 `cargo clippy --all-targets` 都会编整个 `src-tauri/` crate tree，其中 Tauri / wry / tao 在 Linux 上依赖 GTK + GLib 系（`libwebkit2gtk-4.1-dev` → `libglib2.0-dev` 传递）。`test.yml` / `lint.yml` 当前只装 `cmake`，远不够。
2. `fix-linux-ci-build` 阶段一修的是 `build.yml` 的 `Install Tauri Linux deps` 步骤（`build-linux` job），没同步把同一份 apt 列表同步到 `test.yml` / `lint.yml` 的 `Install build deps (Linux)`。这是该 fix 的疏漏。
3. **可行修复路径**：
   - (a) 把 `build.yml` 已验证过的完整 Tauri Linux apt 列表同步到 `test.yml` 与 `lint.yml`
   - (b) 用 `tauri-apps/tauri-action` 重构三个 workflow（统一入口），不在本任务范围
   - (c) `rust-test` / `rust-lint` 跑 `--no-default-features` 或 feature 切换避开 Tauri 依赖 —— 破坏现有测试覆盖
   
   选 (a)：与 `fix/linux-ci-build` 阶段一完全等价、零代码改动、最小 diff。

## 修复方案

**只改两个 workflow 文件**：把 `Install build deps (Linux)` 步骤的 apt 命令替换为 `build.yml` 中已验证过的完整列表。

### 替换前

`test.yml` 第 28–29 行 + `lint.yml` 同样写法：

```yaml
- name: Install build deps (Linux)
  if: runner.os == 'Linux'
  run: sudo apt-get install -y cmake
```

### 替换后

```yaml
- name: Install Tauri Linux deps
  if: runner.os == 'Linux'
  run: |
    sudo apt-get update
    sudo apt-get install -y \
      build-essential \
      cmake \
      curl \
      wget \
      file \
      libssl-dev \
      libxdo-dev \
      libayatana-appindicator3-dev \
      libwebkit2gtk-4.1-dev \
      librsvg2-dev \
      patchelf
```

### 关键变更点

1. 步骤名 `Install build deps (Linux)` → `Install Tauri Linux deps`，与 `build.yml` 命名一致（PR review 时一眼能看出三处同步）
2. apt 列表与 `build.yml` 第 67–79 行逐字相同（11 个包），避免任何生态漂移
3. 显式 `sudo apt-get update`（`build.yml` 已含，CI runner 镜像每次启动 apt index 不会自动更新）
4. `if: runner.os == 'Linux'` gate 保留：macOS / Windows runner 不跑此步骤（macOS 用 Homebrew / Xcode CLT，Windows 用 MSVC + WebView2）
5. **不动** `build.yml`：它已经是对的

### 不动其他东西

- `package.json` / `package-lock.json`
- `src-tauri/Cargo.toml` / `Cargo.lock`
- 三个 workflow 的 trigger 条件 / 矩阵
- pre-commit hook / commitlint 配置
- `docs/tasks/feat-windows-runner/plan.md`（那是 v0.1 提前加 Windows 的记录）

## 回归验证

1. **CI `rust-test (ubuntu-latest)`** 在 push `fix/ci-linux-tauri-deps` 后：
   - `Install Tauri Linux deps` step 显示 11 个包 `Get` / `Setting up`
   - `cargo test --all-targets` 后续 `Compiling glib-sys` / `gobject-sys` / `gio-sys` 三个 build script 成功，无 `pkg-config exited with status code 1`
   - 所有现有 `#[test]` 通过
2. **CI `rust-lint (ubuntu-latest)`**（`feat/windows-runner` 加的 Linux 维度）：
   - `cargo clippy --all-targets -- -D warnings` 顺利通过（同 Tauri 编译链）
   - `cargo fmt -- --check` 不受影响
3. **CI `rust-test (macos-latest)` / `rust-test (windows-latest)`**：不受影响（apt step gated by `if: runner.os == 'Linux'`）
4. **CI `frontend-test` / `frontend-lint` 任一 OS**：不受影响（不跑 apt）
5. **CI `build-linux`**：本身已正确，无重复
6. **范围约束自检**：PR diff 仅含 `.github/workflows/test.yml` + `.github/workflows/lint.yml`，每处仅 `Install ... (Linux)` 步骤内容替换；零代码改动

## 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| `apt-get install` 多装 10 个包增加 ~30s 安装时长 | CI 单 job 时长微增 | apt 包来自 ubuntu-22.04 默认源，下载与解压都很快；与 `build-linux` 完全一致 |
| 未来 Tauri 2 升级到要求新 apt 包 | `build.yml` 升级时漏同步 `test.yml` / `lint.yml` | 后续 PM 任务：在 `docs/tech/engineering/` 增 CI 维护条目，明确「3 处 Linux dep 列表必须同步」；本次不做 |
| Swatinem/rust-cache 因新依赖无效 | cache 命中率短期下降 | rust-cache 键依赖 `Cargo.lock`，不是 apt 包；不受影响 |

## 关联

- `docs/tasks/fix-linux-ci-build/plan.md`（阶段一）：补了 `build.yml` 但漏了这两个，本任务是其延续
- `docs/tasks/feat-windows-runner/plan.md`：把 Linux 加进 `rust-test` 矩阵后立刻触发本 bug
- `AGENTS.md` Git Workflow：分支 `fix/ci-linux-tauri-deps`、Conventional Commits

## 参考链接

- Tauri 2 Ubuntu 22.04 prerequisites：<https://v2.tauri.app/start/prerequisites/#linux>
- Ubuntu `libglib2.0-dev`（提供 `glib-2.0.pc` / `gio-2.0.pc` / `gobject-2.0.pc`）：<https://packages.ubuntu.com/jammy/libglib2.0-dev>
