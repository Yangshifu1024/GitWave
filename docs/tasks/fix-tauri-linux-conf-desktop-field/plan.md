# fix: tauri.linux.conf.json 使用非法字段 `bundle.linux.desktop` 导致 Linux CI 全线失败

## 现象

GitHub Actions 所有 Linux job 在 src-tauri build script 阶段即失败（`test.yml` 的 `cargo test --all-targets`、`lint.yml` 的 clippy/fmt、`build.yml` 的打包均受影响）：

```
cargo:rerun-if-changed=/home/runner/work/GitWave/GitWave/src-tauri/tauri.conf.json
cargo:rerun-if-changed=/home/runner/work/GitWave/GitWave/src-tauri/tauri.linux.conf.json
unknown field `desktop`, expected one of `appimage`, `deb`, `rpm`
```

macOS 本机构建不受影响（平台配置 `tauri.linux.conf.json` 不在 macOS 解析），故问题只在 Linux CI 暴露。

## 根因

commit `4da67ed`（chore(release): Linux packaging metadata and CI hardening）新建 `src-tauri/tauri.linux.conf.json`，在 `bundle.linux` 下写入了 `desktop` 对象（name/genericName/comment/categories），意图是给 Linux 包写 .desktop 元数据。

但 `tauri.linux.conf.json` 与 `tauri.conf.json` 共用同一严格 schema（`deny_unknown_fields`）。锁定的 `tauri-utils 2.9.3` 中 `LinuxConfig` 仅有 `appimage` / `deb` / `rpm` 三个字段（`~/.cargo/registry/.../tauri-utils-2.9.3/src/config.rs`），`desktop` 不是合法字段，反序列化直接报错。

桌面入口元数据的合法通路：

- `bundle.category` / `bundle.shortDescription`（`BundleConfig` 顶级字段）→ 经 tauri-bundler 注入模板变量 `categories` / `comment`
- `bundle.linux.deb.desktopTemplate` / `rpm.desktopTemplate`（`DebConfig` / `RpmConfig` 均有 `desktop_template` 字段）→ 自定义 .desktop 模板

tauri-bundler 默认模板（`crates/tauri-bundler/src/bundle/linux/freedesktop/main.desktop`）不包含 `GenericName` 行，故需自定义模板才能保留 genericName 元数据。

## 修复方案

1. `src-tauri/tauri.linux.conf.json`：删除非法的 `bundle.linux.desktop`；元数据迁移到合法字段：
   - `bundle.category: "Development"`、`bundle.shortDescription` 保留 comment 意图
   - `linux.deb.desktopTemplate` / `linux.rpm.desktopTemplate` 指向新模板
2. 新建 `src-tauri/linux/gitwave.desktop`：基于 tauri-bundler 默认模板，增加 `GenericName=Git Client`；`name`/`categories`/`comment`/`exec`/`icon` 使用 bundler 注入的 Handlebars 变量（`StartupWMClass={{exec}}` 与默认模板一致，保证任务栏图标分组）。

未采用 `cargo update` 升级 tauri 系列依赖：`desktop` 在 Tauri 2.x schema 中本就不是合法字段，升级无法解决，反而引入依赖漂移。

## 验证

- `cd src-tauri && cargo test --all-targets`：本地回归通过（注意 macOS 不解析 linux conf，Linux 侧正确性依据：新配置字段与 `tauri-utils 2.9.3` 源码逐一比对 + 模板变量与 tauri-bundler `freedesktop/mod.rs` 的 `DesktopTemplateParams` 比对，最终以 push 后 CI 为准）
- JSON 语法校验（jq）

## 回归影响面

`test.yml`（cargo test）、`lint.yml`（clippy/fmt）在 Linux 上的 build script 失败随本修复消除；`build.yml`（tag 触发）Linux 打包的 deb/rpm 将使用自定义 .desktop 模板（含 GenericName），AppImage 仍用默认模板生成桌面入口（AppImageConfig 无 desktopTemplate 字段）。
