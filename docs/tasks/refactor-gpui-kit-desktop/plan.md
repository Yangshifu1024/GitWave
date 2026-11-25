# GitWave 桌面界面迁移到 GPUI Kit

状态：进行中。分支 `feature/gpui-kit-desktop`，基线 `004c9f4`（v0.11.0）。决策见 [ADR 0007](../../tech/decisions/00-overview.md#0007--桌面界面迁移到-gpui-kit)，选型依据见 [调研笔记](../../tech/tech-selection/gpui-kit-research.md)。

## 目标与验收基线

用 GPUI Kit 重写桌面界面，把 GitWave 改成不依赖 Tauri、WebView 与 Node.js 前端的纯 Rust 应用。Rust 后端的领域、应用、基础设施三层保留，抽成独立 crate，由界面在进程内直接调用。

以 [v0.11.0 实际交付的工作台](../../design/09-workbench-redesign.md)和 [PM 范围](../../pm/core/02-scope.md)为基线。每条用户流程覆盖正常、加载、空态、失败、确认、取消与恢复。保留 Workspace 无目录实体、多 Workspace 并行与单 active repo，repo 级草稿和错误，Git 写操作显式确认及 AI 本地优先。三端功能、数据、凭证、安装、更新与回退验收通过后切换默认入口，再移除 Tauri 与 Node.js 前端。

## 原则

上一次迁移（Go + NSGo）暴露的四个问题，这次逐条对应：

- **选型先留记录。** 决策与备选写入 ADR 0007，依据写入调研笔记。
- **最没把握的先做。** 构建与运行、后端抽离、发布链路排在界面铺开之前，不等做到中后期才发现问题。
- **三端 CI 从第一个里程碑开始。** 不再出现只在本地验证、合并时才第一次在托管环境运行的工作流。
- **每个里程碑单独成 PR。** 合并前跑完等价检查并经 code-reviewer 审查，不再把多个里程碑压进一个不可回退的大 PR。

## 工程结构

```
Cargo.toml                 # 根 workspace；迁移期间排除 src-tauri
crates/
├── gitwave-core/          # 领域、应用、基础设施与服务容器，不含界面框架类型
└── gitwave/               # GPUI Kit 应用：窗口、菜单、各功能的 model 与 view
src-tauri/                 # Tauri 应用，M8 之前继续发布；改为调用 gitwave-core 的薄封装
```

迁移完成后删除 `src-tauri/`、`src/`、`package.json`、`pnpm-lock.yaml`、`vite.config.ts` 与相关前端配置。

## 实施里程碑

| 阶段 | 实施内容 | 退出标准 |
|---|---|---|
| **M0 工程骨架** | 根 workspace 与 `crates/gitwave`；`src-tauri` 排除在 workspace 之外 | gpui-kit 应用在 macOS 构建并启动；`src-tauri` 的构建、锁文件与检查不受影响；根 workspace 的格式与 clippy 通过 |
| **M1 后端抽离** | `crates/gitwave-core`；服务容器与用例 API 去掉 Tauri 类型；Tauri 应用改为薄封装；tokio 运行时桥接 | Tauri 应用的全部现有检查通过；core crate 不含任何 Tauri 类型；三端 CI 构建通过 |
| **M2 应用壳** | 窗口、自绘标题栏、应用菜单、快捷键、主题、中英切换、状态栏、tokio 运行时接入 | 三端 CI 构建通过；菜单、快捷键、主题与语言切换可用；中文输入法在对话框与多行输入中实测 |
| **M3 只读工作台** | Workspace 与仓库标签、History 列表与提交图、提交详情、文件列表、Diff、Blame | 对照 v0.11.0 逐项核对布局与交互；冷启动、10k commit History 与 1 MB diff 达到性能预算 |
| **M4 Changes 与日常 Git** | 状态列表、Stage / Unstage、Commit / Amend、Discard、分支 / 标签 / 检出、Fetch / Pull / Push、克隆与初始化 | commit → push、切换 Workspace、操作中切仓、失败重试与外部 Git 竞态通过端到端验证 |
| **M5 高级工具** | Merge 与冲突编辑、Rebase（含交互式）、Cherry-pick / Revert、Stash、Reflog 恢复、Worktree、Remote、Submodule、LFS、Hooks、`.gitignore`、健康摘要 | 对照旧版校验暂停后重启、继续与取消及失败恢复后的仓库状态 |
| **M6 设置与 AI** | 设置各页、AI provider 与 Prompt 模板、命令面板、各 AI 入口 | 三端桌面交互与各 AI 用例通过；离线仅连接 loopback |
| **M7 发布链路** | 三端打包、签名与公证、带签名的应用内更新与重启、三端 CI 的构建与发布任务 | 干净系统可安装使用；从 v0.11.0 升级及回退可重现；应用与安装包通过各平台签名验证 |
| **M8 切换与清理** | 移除 Tauri 与 Node.js 前端；同步 README、官网、AGENTS、CONTRIBUTING、发布技能、CI 与 Makefile；废弃 ADR 0001 / 0005 / 0006 | 默认构建与发布不再需要 Tauri 与 Node.js；文档不再描述旧行为 |

## 测试门禁

- M8 之前，Tauri 应用的现有门禁保持通过：`make check`（格式、lint、类型检查、前端与 Rust 测试），自 M1 起同时覆盖两个 Cargo workspace。
- 根 workspace：`cargo fmt --all -- --check`、`cargo clippy --workspace --all-targets -- -D warnings`、`cargo test --workspace --all-targets`，在三端 CI 上运行（`.github/workflows/workspace.yml`）。
- `gitwave-core` 的守卫测试（`crates/gitwave-core/src/lib.rs`）：不依赖任何界面框架；三个平台都声明了 keyring 的原生存储后端；运行时拿到的后端不是 keyring 的进程内 mock。起因见 M1 执行记录。
- 界面测试用 gpui-kit 的 test-support 在无头窗口里渲染真实组件并模拟输入；局限是它不覆盖系统输入法组合、原生对话框与读屏器，这些在三端实机验收。
- 沿用[性能预算](../../tech/engineering/00-overview.md#性能预算)：Apple Silicon 冷启动 < 1 秒、10k commit History < 500 毫秒、1 MB diff < 200 毫秒。M3 起每个里程碑复测。
- 写操作退出后不得有意外文件或状态变更；与旧版使用相同 fixture 的独立副本比较 refs、index、工作区文件、stash、reflog 和操作状态。

## 风险与未决项

风险清单与证据见[调研笔记](../../tech/tech-selection/gpui-kit-research.md)的 H 节。影响最大的四项：

- 上游 API 每 1 到 2 周一次破坏性变更。版本精确钉死，升级作为独立任务。
- 打包、签名、公证、带签名更新没有框架级方案，M7 要整体自建。
- 冷启动、数万 commit 的提交图与 diff 在 gpui-kit 上没有测量数据，M3 复测。
- macOS 拼音候选窗在焦点移入对话框后残留（上游 #3605）、Windows 平台层的启动闪烁与实时缩放（上游 #3863）仍未关闭。

需要用户确认的未决事项：

- 打包与更新采用哪套工具，以及 macOS 上带签名更新的实现方式（M7 之前）。
- macOS 最低系统版本：调研发现文档写 15+、上游 12.0、Longbridge Pro 11+ 三个数字不一致。
- README 所写的「menu bar app mode」指窗口内的菜单栏，还是常驻系统菜单栏的托盘模式。现有代码只有前者，gpui-kit 没有后者的现成实现。
- dependabot 是否忽略 `gpui-kit` 的升级 PR。当前配置为忽略，理由与 ADR 0007 的「升级是独立任务」一致，代价是它的安全更新也不会自动提 PR。

## M2 设计输入

实现 M2 之前已经对照 gpui-kit 0.11.0 源码核实的接口与约束，避免下次重新查：

- **菜单**：同一份 `Vec<Menu>` 定义同时交给 `cx.set_menus`（macOS 原生菜单栏）和 `GlobalState::global_mut(cx).set_app_menus(..)`（标题栏内的 `AppMenuBar`，Windows 与 Linux 使用），随后调用 `app_menu_bar.update(cx, |bar, cx| bar.reload(cx))`。`MenuItem::action` 可以链式 `.checked()`、`.disabled()`；编辑菜单直接复用 `gpui_kit::component::input` 的 `Undo`、`Redo`、`Cut`、`Copy`、`Paste`、`Delete`、`SelectAll` 等内置动作。切换语言或主题后重建菜单以刷新勾选项。参考上游 `crates/story/src/app_menus.rs`。
- **主题**：`Theme::change(ThemeMode::Light | Dark, Some(window), cx)`，`cx.theme().mode.is_dark()`，`cx.observe_global::<Theme>` 监听变化。「跟随系统」的接线方式没有找到文档。
- **国际化**：组件库用 `rust-i18n` 4.2，应用侧需要自带 `rust_i18n::i18n!("locales", fallback = "en")`。现有 `src/i18n/locales/{en,zh-CN}/*.json`（25 个文件、1108 条文案）可以复用，但不能原样使用。文件名要改成 `域名.语言.json`，因为 `rust-i18n` 取文件名最后一段作为语言代码；插值从 `{{name}}` 改成 `%{name}`，共 296 处；`rust-i18n` 没有内置复数，18 组 `_one` / `_other` 要在 Rust 侧按数量选 key。两套应用并存期间需要确定唯一的文案来源与同步方式。
- **可调分栏**：面板尺寸按位置保存。面板要始终作为子节点存在，用 `resizable_panel().visible(bool)` 隐藏，不能增减子节点。
- **窗口**：`WindowOptions { ..TitleBar::window_options() }` 给出自绘标题栏所需的窗口选项；标题用 `window.set_window_title(..)` 设置。

## 执行记录

- 2026-11-25：完成调研（`docs/tech/tech-selection/gpui-kit-research.md`），用户决定不做前置原型、直接按里程碑重写。分支 `feature/gpui-kit-desktop` 自 `004c9f4` 创建，M0 开始。
- 2026-11-25 · M0 完成：根 workspace（`exclude = ["src-tauri"]`）与 `crates/gitwave`，gpui-kit 精确钉在 `=0.11.0`。本机首次构建 88 秒，含整棵依赖树的下载与编译；调试二进制 43 MB；启动后运行 5 秒无崩溃、无输出。`cargo metadata` 确认 `src-tauri` 仍是独立的 workspace 根、用自己的构建目录，`cargo check --all-targets` 通过，锁文件未改动。
- 2026-11-25 · M1 完成：
  - 搬移：`application`、`domain`、`infrastructure` 与迁移脚本以 `git mv` 迁入 `crates/gitwave-core`，共 66 个 `.rs` 和 4 个 SQL。`run_sync_op` 与 `local_git_task` 迁入 `application::sync_ops`，从 Tauri 运行时改用 tokio。终端与编辑器的宿主工具迁入 `host`，状态库、libgit2 所有者校验与代理环境桥接迁入 `bootstrap`。
  - Tauri 侧：`src-tauri/src/lib.rs` 由 2225 行减到 1822 行，注册的命令仍是 130 个，只剩命令薄封装、窗口装饰和发往 WebView 的事件。直接依赖只剩 `gitwave-core`、Tauri 及其插件、`tracing`、`serde_json`（`generate_context!` 展开 needs it）和 macOS 的 `objc2` 系列。`anyhow` 在任何文件里都没有被使用，随之移除。
  - 验证：Tauri 的锁文件逐包对比，427 个原有包无一增删，只有 `gitwave` 的依赖列表发生变化。CI 里关于 libgit2 TLS 的两条断言在本地成立。`make check` 的三部分全过：前端 367 个测试、Tauri 集成测试 1 个、core 396 个（394 通过，2 个原本就忽略）。
  - 过程中抓到并修复的问题：重写 core 的依赖清单时漏掉了 macOS 的 `keyring` `apple-native`。keyring 3.6.3 在没有原生后端时会静默退回进程内的 mock，编译、clippy、测试都不报错，存入的密钥在重启后丢失。这个漏项是人工核对锁文件差异发现的，已补回并加了上面三条守卫测试；反向验证确认去掉那一行后守卫会失败。另外，前端的语言包一致性测试按路径读取 Rust 错误码源文件，已改为新路径。
  - 新增：`.github/workflows/workspace.yml`（三端 rustfmt、clippy、测试与 TLS 断言）、根 `clippy.toml` 与 `rustfmt.toml`（与 `src-tauri` 的配置相同）、Makefile 把根 workspace 纳入格式、lint、测试四个目标。
  - 未完成：三端 CI 还没有在托管环境运行过（本机没有 actionlint）。`gitwave` 应用目前只有窗口壳：标题栏、三栏占位与状态栏。
- 2026-11-25 · M1 代码审查：code-reviewer 子代理独立审查，🔴 零条，🟡 八条，未发现改变 Tauri 应用外部行为的问题；它另行核实了依赖逐项一致、69 个搬移文件逐字节一致、130 个命令注册逐字一致。八条全部处理，逐条结果见 [审查报告](./review.md#处理结果)。其中影响最大的一条是 Tauri 应用自己的 CI 不再运行业务层的测试和 lint，现已通过 `cargo test -p gitwave-core` 与对应的 clippy 在 Tauri 的锁文件下恢复覆盖。同时把版本号改为由根 workspace 继承并纳入 `pnpm bump`，pre-commit 钩子与 dependabot 覆盖根 workspace。
  - 修复后的门禁：`make fmt-check`、`make lint`、`make test` 全过。前端 367 个测试；Tauri 集成测试 1 个；core 的 396 个测试（394 通过，2 忽略）在 Tauri 与根 workspace 两套锁文件下各跑一遍。
- 2026-11-25 · 首次托管 CI（PR #243）：Tauri 应用的 12 项检查首轮全部通过，新增的 `cargo test -p gitwave-core` 在三端都跑了。`workspace` 工作流首轮 macOS、Windows 通过，Ubuntu 在 `cargo clippy` 失败：`libdbus-sys` 需要 `libdbus-1-dev`，Tauri 的任务里它由 `libayatana-appindicator3-dev` 间接带入。补上该包后 15 项全部通过，用时 macOS 1 分 51 秒、Windows 2 分 58 秒、Ubuntu 11 分 48 秒。
