# fix-linux-ci-build

> 修复 GitWave 在 GitHub Actions `build-linux` job（`ubuntu-22.04`）上 `glib-sys` / `gio-sys` / `gobject-sys` build script 失败的问题。

## 状态

草案。

## 问题描述

`build-linux` job 执行 `cargo tauri build` 时，`glib-sys` / `gio-sys` / `gobject-sys`（Tauri / `wry` / `tao` 在 Linux 上的 GTK + GLib 绑定）的 build script 通过 `pkg-config` 探测系统库失败，日志如下：

```
PKG_CONFIG_ALLOW_SYSTEM_CFLAGS=1 pkg-config --libs --cflags glib-2.0 'glib-2.0 >= 2.70'
Package glib-2.0 was not found in the pkg-config search path.
...
The system library `glib-2.0` required by crate `glib-sys` was not found.
The file `glib-2.0.pc` needs to be installed and the PKG_CONFIG_PATH environment variable must contain its parent directory.
The PKG_CONFIG_PATH environment variable is not set.
```

`gobject-2.0`（`>= 2.70`）和 `gio-2.0` 报同样错误（同一个 GLib 套件下的不同 `.pc`）。三个 `.pc` 文件都来自同一个 apt 包（见根因分析），缺失意味着 `libglib2.0-dev` 未被安装到 runner 上。

工作流当前安装的依赖（`.github/workflows/build.yml`，第 55–63 行）：

```yaml
- name: Install Tauri Linux deps
  run: |
    sudo apt-get update
    sudo apt-get install -y \
      cmake \
      libwebkit2gtk-4.1-dev \
      libappindicator3-dev \
      librsvg2-dev \
      patchelf
```

## 根因

### 1. 三个 `.pc` 文件的归属

`glib-2.0.pc`、`gobject-2.0.pc`、`gio-2.0.pc` 都由同一个包提供：**`libglib2.0-dev`**（Ubuntu 22.04 jammy 上 `2.68.4-1ubuntu1`，满足 `>= 2.70` 版本要求的 `>= 2.67.1` 间接约束）。

参考：

- Debian/Ubuntu package tracker: <https://packages.ubuntu.com/jammy/libglib2.0-dev>
- GLib upstream: <https://gitlab.gnome.org/GNOME/glib>

### 2. 为什么 `libwebkit2gtk-4.1-dev` 没把它们带上来

理论上 `libwebkit2gtk-4.1-dev`（jammy 上版本 ≥ `2.44.x`）的 `Depends:` 字段里有 `libglib2.0-dev (>= 2.67.1)`，`apt-get install` 应该把 GLib dev headers 一并装上。**但 CI runner 上观察到的行为不是这样**，可能由以下一个或多个因素叠加造成：

1. **当前安装列表本身就是 Tauri 1 时代的最小集**，缺了一堆 Tauri 2 必须用到的包（见下表）。当依赖树不完整时，build script 探测仍然按设计路径失败，暴露底层问题。
2. **`libappindicator3-dev` 是 GNOME/Canonical 已废弃的旧库**（transitional package），Tauri 2 官方推荐的是它的活跃 fork **`libayatana-appindicator3-dev`**。两者都提供 `appindicator3.pc`，但装错分支会让链接期再出一次错（被 Tauri 2 链接成旧 ABI）。
3. **GitHub Actions 提供的 `ubuntu-22.04` runner image**（`runs-on: ubuntu-22.04`）apt index 会随 runner 镜像版本漂移；某些镜像快照里 `libwebkit2gtk-4.1-dev` 的依赖图对 `libglib2.0-dev` 的引用形态不稳定，建议显式声明而非依赖传递解析。
4. 工作流里**没有 `build-essential`**（cc / make / pkg-config 的元包），CI 上探测系统库之前最好确保编译器、链接器、`pkg-config` 都可用（pkg-config 二进制通常在 build-essential 的传递依赖里被带入，但显式声明更稳）。

综合判断：**问题不是「`libwebkit2gtk-4.1-dev` 没把它带上来」这一个孤立现象，而是当前 apt 列表整体不匹配 Tauri 2 在 Ubuntu 22.04 上的官方依赖清单**。最小化补全到 Tauri 2 官方列表即可彻底解决，无需修改 `src-tauri/Cargo.toml`、前端、`build.rs`。

### 3. Tauri 2 在 Ubuntu 22.04 上的官方推荐依赖

来源：<https://v2.tauri.app/start/prerequisites/>（Debian 小节，截至 2026-08 当前文档）：

```bash
sudo apt install libwebkit2gtk-4.1-dev \
  build-essential \
  curl \
  wget \
  file \
  libxdo-dev \
  libssl-dev \
  libayatana-appindicator3-dev \
  librsvg2-dev
```

对照当前工作流：

| 包名 | Tauri 2 官方 | 当前 `build.yml` | 备注 |
|---|---|---|---|
| `libwebkit2gtk-4.1-dev` | ✅ | ✅ | 已装 |
| `build-essential` | ✅ | ❌ | 缺：提供 `cc` / `gcc` / `g++` / `make` / `pkg-config` |
| `curl` / `wget` / `file` | ✅ | ❌ | 缺：build script 在探测外部资源时调用 |
| `libxdo-dev` | ✅ | ❌ | 缺：`tao` 在 X11 下做键盘模拟需要 |
| `libssl-dev` | ✅ | ❌ | 缺：`openssl-sys` / `git2` / `reqwest` 等需要 |
| `libayatana-appindicator3-dev` | ✅ | ❌（用了 `libappindicator3-dev`）| **需替换** |
| `librsvg2-dev` | ✅ | ✅ | 已装 |
| `cmake` | ❌（官方未列） | ✅ | 当前有就保留；Tauri 2 当前不强制，但 `librsvg` 升级或某些依赖会要 |
| `patchelf` | ❌（官方未列） | ✅ | `cargo tauri build` 在打 `.deb` / `.AppImage` 时调用，需保留 |

补充建议（基于社区 CI 实践 / 防御性写法，**非 Tauri 官方强制**）：如果希望更稳妥，可以再显式列出 GTK / GLib / libsoup 的一组 dev 包，避免对 webkit2gtk-4.1-dev 传递依赖的隐式依赖：

- `libgtk-3-dev`
- `libglib2.0-dev`
- `libsoup-3.0-dev`
- `libjavascriptcoregtk-4.1-dev`

加这几行后，构建不再依赖 webkit2gtk-4.1-dev 的传递依赖图，与本任务「重跑 build-linux 即可成功」的最低修复目标一致。

## 修复方案

**仅修改 `.github/workflows/build.yml` 中 `build-linux` job 的 `Install Tauri Linux deps` 步骤**。`build-macos` job、前端、`src-tauri/Cargo.toml`、`src-tauri/build.rs` 全部不动。

### 替换前（第 55–63 行）

```yaml
- name: Install Tauri Linux deps
  run: |
    sudo apt-get update
    sudo apt-get install -y \
      cmake \
      libwebkit2gtk-4.1-dev \
      libappindicator3-dev \
      librsvg2-dev \
      patchelf
```

### 替换后（推荐 / 最小 Tauri 2 官方列表）

```yaml
- name: Install Tauri Linux deps
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

### 替换后（防御性 / 显式声明 GLib / GTK / libsoup —— 工程团队可二选一）

如果 CI 后续因为 webkit2gtk 依赖图变化再次飘到 GLib 相关错误，建议在上面的列表上追加 4 个包：

```yaml
- name: Install Tauri Linux deps
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
      libglib2.0-dev \
      libgtk-3-dev \
      libsoup-3.0-dev \
      libjavascriptcoregtk-4.1-dev \
      libwebkit2gtk-4.1-dev \
      librsvg2-dev \
      patchelf
```

### 关键变更点

1. **`libappindicator3-dev` → `libayatana-appindicator3-dev`**：Tauri 2 官方在 Ubuntu 22.04 上使用 Ayatana fork。两者在 jammy 上都可装，但 `libappindicator3-dev` 已是 transitional package，Tauri 2 的链接路径与 `libayatana-appindicator3-dev` 的 ABI 一致。
2. **新增 `build-essential`**：补齐 `cc` / `gcc` / `g++` / `make` / `pkg-config`。
3. **新增 `curl` / `wget` / `file`**：build script 探测外部资源用。
4. **新增 `libssl-dev`**：`openssl-sys` / `git2-rs` / `reqwest` 链接 OpenSSL 需要。
5. **新增 `libxdo-dev`**：`tao` 在 X11 下做键盘模拟需要。
6. **（可选）新增 `libglib2.0-dev` / `libgtk-3-dev` / `libsoup-3.0-dev` / `libjavascriptcoregtk-4.1-dev`**：显式声明，消除对 `libwebkit2gtk-4.1-dev` 传递依赖的隐性假设。

### 不需要手动处理 Swatinem/rust-cache 缓存

`Swatinem/rust-cache@v2` 的缓存键由以下输入哈希而成（参考 <https://github.com/Swatinem/rust-cache#cache-details>）：

- `Cargo.lock` / `Cargo.toml` 的内容
- `rust-toolchain` / `rust-toolchain.toml` 内容（若有）
- `.cargo/config.toml` 内容（若有）
- 默认匹配的 env vars：`CARGO` / `CC` / `CFLAGS` / `CXX` / `CMAKE` / `RUST*`

**apt 安装的系统包不在缓存键计算范围内**。新增 apt 包不会让 rust-cache 自动失效，因此不需要额外动作。但需要注意：之前 `build-linux` 的失败运行并不会污染 target 缓存（失败发生在 build script 阶段，目标 crate 还没产出 `rlib` / 可执行文件），所以修复后的第一次跑会从 rust-cache 的历史有效缓存继续命中，仅 `glib-sys` / `gio-sys` / `gobject-sys` 三个 build script 会重新执行（这正是我们想要的）。

## 回归验证

测试 / 工程 owner 执行以下检查，全部通过后才能合入。

### 1. `build-linux` job 必须绿

- 在 PR（`fix/linux-ci-build`）上触发 workflow，定位到 `build (Linux)` job。
- 关键步骤预期：
  - `Install Tauri Linux deps`：新增包都 `Get` / `Setting up`，无 `Unable to locate package`。
  - `tauri build`：`Compiling glib-sys vX.Y.Z` / `gobject-sys` / `gio-sys` 三个 build script 成功，`Compiling wry` / `Compiling tao` / `Compiling tauri` 顺利完成，无 `pkg-config exited with status code 1`。
  - 最后上传 artifact：`gitwave-linux` 包含 `src-tauri/target/release/bundle/deb/*` 或 `src-tauri/target/release/bundle/appimage/*` 至少之一（取决于哪个 target 在 jammy 上成功）。
- 失败判定：仍出现 `Package glib-2.0 was not found` 或 `gobject-2.0` / `gio-2.0` 同样的报错 → 说明 apt 列表仍未生效，回到修复方案排查。

### 2. `build-macos` job 不受影响

- 同一个 workflow run 里 `build (macOS)` job 必须仍为绿（绿色 ✓），因为本次只改了 `build-linux` job 内的步骤。
- macOS job 没有 `Install Tauri Linux deps` 步骤，未触及其依赖列表。

### 3. 范围约束自检

以下文件在 PR diff 中**不应有任何改动**：

- `src-tauri/Cargo.toml` / `src-tauri/Cargo.lock`
- `src-tauri/build.rs` 及 `src-tauri/tauri.conf.json`
- 前端任何文件（`src/` / `package.json` / `package-lock.json` / `vite.config.ts` 等）
- `lint.yml` / `test.yml`（若存在）

唯一改动应在 `.github/workflows/build.yml` 的 `build-linux` job 内 `Install Tauri Linux deps` 步骤。

### 4. 缓存行为回归（可选，但建议）

- 在 PR 上跑一次后，再 push 一个空的 commit 重跑同一 workflow：
  - `Swatinem/rust-cache` 应命中 `cache-hit` 或部分命中，`Compiling` 的 crate 数量应明显少于首次运行（绝大多数直接 `Compiling` 跳到链接）。
  - 没有出现 `error: linker 'cc' not found` 或 `cannot find -lglib-2.0` 等回归。

## 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| `libayatana-appindicator3-dev` 在某个未来 runner 镜像版本不可用 | `apt-get install` 报错 | 退路：换成 `libappindicator3-dev`，但需关注 Tauri 2 ABI 一致性（多数情况下 jammy 上两者并存） |
| 显式追加 `libgtk-3-dev` / `libglib2.0-dev` / `libsoup-3.0-dev` 与 `libwebkit2gtk-4.1-dev` 冲突 | apt 报版本冲突 | 这 4 个包在 jammy 上由 webkit2gtk-4.1-dev 直接 Depends，正常情况下不会出现版本冲突；若冲突，多半是镜像被 pin，需到 GitHub Actions runner image 维护团队排查 |
| 升级到 `ubuntu-24.04` runner 后 webkit2gtk 路径变化 | 再次出现 `.pc` 缺失 | 本任务先固定在 `ubuntu-22.04`；迁移 runner 的工作在独立任务里做（参考 `docs/pm/core/03-roadmap.md`） |
| Swatinem/rust-cache 在新依赖加入后行为异常 | 命中率下降 / 编译变慢 | 缓存键与 apt 无关，理论上不受影响；若发现异常，按 rust-cache README 的 `Debugging` 段开 debug 日志排查 |

## 关联

- `docs/tasks/feat-bootstrap-tau-app/plan.md`：Sprint 0 已假设 Linux CI 跑通；本任务为其后续修复
- `docs/pm/core/03-roadmap.md`：v0.1 里程碑内的 CI 缺陷
- `AGENTS.md` Git Workflow：分支命名 `fix/linux-ci-build`（建议）、Conventional Commits、PR 需 code-reviewer 通过

## 参考链接

- Tauri 2 Prerequisites（官方 Debian / Ubuntu 列表）：<https://v2.tauri.app/start/prerequisites/>
- Ubuntu Package: `libglib2.0-dev`（提供 `glib-2.0.pc` / `gio-2.0.pc` / `gobject-2.0.pc`）：<https://packages.ubuntu.com/jammy/libglib2.0-dev>
- Ubuntu Package: `libayatana-appindicator3-dev`：<https://packages.ubuntu.com/jammy/libayatana-appindicator3-dev>
- Ubuntu Package: `libappindicator3-dev`（transitional，Tauri 1 时代）：<https://packages.ubuntu.com/jammy/libappindicator3-dev>
- Ubuntu Package: `libwebkit2gtk-4.1-dev`：<https://packages.ubuntu.com/jammy/libwebkit2gtk-4.1-dev>
- `glib-sys` / `gobject-sys` / `gio-sys`（gtk-rs GLib 绑定）：<https://gtk-rs.org/gtk-rs-core/stable/latest/docs/glib_sys/> · <https://gtk-rs.org/gtk-rs-core/stable/latest/docs/gio/>
- `wry`（Tauri 底层 WebView 抽象）：<https://github.com/tauri-apps/wry>
- `tao`（Tauri 窗口管理）：<https://github.com/tauri-apps/tao>
- `tauri-apps/tauri-action`（另一种 CI 方案，本项目暂未采用）：<https://github.com/tauri-apps/tauri-action>
- `Swatinem/rust-cache`（缓存键规则）：<https://github.com/Swatinem/rust-cache#cache-details>
- PKG_CONFIG_PATH 文档：<https://people.freedesktop.org/~dbn/pkg-config-guide.html>
