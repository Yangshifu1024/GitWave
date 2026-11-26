# 代码审查：M0 工程骨架 + M1 后端抽离

> **作者**：AI code-reviewer 子代理，未经人工复核
> **状态**：草案
> **最近更新**：2026-11-25

| 项 | 内容 |
|---|---|
| 分支 | `feature/gpui-kit-desktop`，尚未提交的工作区改动 |
| 基线 | `004c9f4`（v0.11.0，`main`） |
| 计划与决策 | [plan.md](./plan.md) · [ADR 0007](../../tech/decisions/00-overview.md#0007--桌面界面迁移到-gpui-kit) |
| 环境 | macOS arm64，rustc 1.100.0，cargo 1.100.0 |
| 方式 | 只读。除本文件外未新建、修改或删除任何文件；涉及锁文件的 cargo 命令一律带 `--locked` |

**审查对象**：非搬移的改动，即依赖拆分、`src-tauri/src/lib.rs` 的改写、`gitwave-core` 新增的 `lib.rs` / `bootstrap.rs` / `host.rs` 与 `sync_ops.rs` 的新增部分、根 workspace 与 `crates/gitwave`、`workspace.yml`、`Makefile`、文档。70 个 `git mv` 的搬移文件只做逐字节比对，不逐文件审查。

**总体结论**：没有发现改变 Tauri 应用外部行为的问题。需要处理的是门禁与流程没有跟上「两个 workspace」的新结构，以及 `workspace.yml` 与骨架代码里几处尚未被任何工具验证的地方。

## 静默回归核对

| 核对项 | 方法 | 结果 |
|---|---|---|
| 依赖的版本、feature、平台条件 | 逐项对照 `git show HEAD:src-tauri/Cargo.toml` 与 `crates/gitwave-core/Cargo.toml`、`src-tauri/Cargo.toml` | 一致。`git2`（含 `vendored-openssl`）、`reqwest 0.12`、`tokio full`、三组 target 表的 `keyring` 原样进了 core；`objc2` 系列与 `reqwest 0.13`（更新器代理统一）留在 `src-tauri`；只移除了 `anyhow`，全仓无使用 |
| `src-tauri/Cargo.lock` | name + version 集合 diff | 571 → 572，仅新增 `gitwave-core 0.11.0`；其余条目逐字节未变 |
| 搬移文件 | 69 个文件与 HEAD 原路径逐字节比较 | 全部一致，只有 `sync_ops.rs` 有改动 |
| 注册的命令 | `generate_handler!` 块逐字 diff | 130 个，含顺序一致；`#[tauri::command]` 130 → 130 |
| 命令函数体、事件名、错误码 | `git diff HEAD -- src-tauri/src/lib.rs` 的新增行归类 | 除模块头注释 5 行外，新增行只有三类：re-export 与 import、19 处调用点加 `sync_ops::` 前缀、`host::` 与 `bootstrap::` 的调用。`.emit(` 与 `codes::` 行未被触及 |
| `run_sync_op` | 与 HEAD 版 `lib.rs` 逐行对比；调用点所在函数 | 逻辑一致。`tokio::time::timeout` 原实现就在用，说明调用处本来就在 tokio 运行时内；19 个调用点全部在 `async fn cmd_*` 中，改动前后相同 |
| `bootstrap.rs` | 与 HEAD 版 `run()` 逐行对比 | 日志文案、回退顺序、`expect` 文案、调用顺序一致 |
| `host.rs` | 与 HEAD 版终端、编辑器实现机械 diff | 只剩可见性（`pub`）与参数 `String` → `&str` |
| 编译期环境 | grep `env!` / `option_env!` / `cfg(dev)` / `cfg(feature` | core 内无使用，`CARGO_PKG_VERSION`、`CARGO_MANIFEST_DIR` 的变化不产生影响 |
| 按旧路径读文件的消费者 | 全仓 grep `src-tauri/src/{application,domain,infrastructure}`、`src-tauri/migrations` | 只有 `src/i18n/locales/parity.test.ts`，已改 |
| core 的依赖图 | `cargo tree -p gitwave-core -e normal` | 143 个包，无 `tauri`、`gpui` |
| `keyring` 的 feature 解析 | 两个 workspace 的 `cargo metadata` | 两边都是 `keyring 3.6.3`，feature 为三平台并集，实际后端由 target 条件选择，与改动前相同 |

## ✅ 优点

- **搬移纪律好。** 69 个文件逐字节一致，唯一有改动的 `sync_ops.rs` 的 diff 只有模块文档、一个 import 和新增的两个函数及其测试。审查面因此收敛到少数几个文件。
- **`lib.rs` 的改写是纯机械的。** +38 / −441 行，没有顺手重构；`run_sync_op`、`local_git_task`、终端与编辑器、启动逻辑都逐行对应原实现，日志文案和错误码没有漂移。
- **keyring 的漏项被抓住并补了守卫。** 守卫分两层（清单文本与运行时后端）。读了 keyring 3.6.3 源码确认：三个原生后端在构造 `Entry` 时都不做 I/O，mock 下 `Entry::new` 恒为 `Ok` 且 Debug 输出含 `MockCredential`，所以这三条守卫在三端 CI 上不会误报。
- **`sync_ops.rs` 的新测试覆盖到位。** `run_sync_op` 与 `local_git_task` 各自的 Ok、超时、panic 路径都有用例；超时用例用轮询加 deadline 等待注销，没有「sleep 一段再断言」的竞态。
- **`workspace.yml` 遵循了仓库惯例。** action 版本、runner 矩阵、`concurrency` 写法、缓存方式、Chrome apt source 的处理都与 `test.yml` / `lint.yml` 一致，并多了 `--locked`，fmt 只在一端跑。YAML 可解析，6 个脚本块通过 `bash -n`。
- **文档改动面广，核对过的数字大多属实。** ADR 的 2.6 万行 Rust、2.45 万行 TypeScript、5.9 千行测试，任务目录 152 个，测试数 367 / 1 / 396，`lib.rs` 2225 → 1822 行，66 个 `.rs` + 4 个 SQL，锚点 `#0007--桌面界面迁移到-gpui-kit` 的 4 处引用都能对上。plan 如实记录了「三端 CI 还没有在托管环境运行过」。

## 🔴 严重问题（必须修复）

未发现。

已按上表逐项核对依赖拆分、命令注册与函数体、事件名与错误码、`run_sync_op` 的调度方式、启动逻辑、编译期环境和按路径读文件的消费者，没有找到会让 Tauri 应用行为发生变化的改动。唯一已知的静默回归（macOS 的 `keyring` `apple-native` 被漏掉）已在本次改动中修复并有守卫覆盖；去掉那一行后守卫测试 2 会失败（已用 `sed` 模拟验证）。

## 🟡 一般问题（建议修复）

### 1. Tauri 应用（当前发布版本）的 CI 对业务代码的覆盖静默缩水

- **位置**：`.github/workflows/test.yml:55-57`（`rust-test`）、`.github/workflows/lint.yml:57-62`（`rust-lint`）、`docs/tech/engineering/00-overview.md:63-66`、[plan.md](./plan.md) 第 36、77 行
- **描述**：`src-tauri` 的 `cargo test --all-targets` 现在只运行 1 个测试：lib 单元测试 0 个、main 0 个、`updater_restart` 1 个，且后者是 `#![cfg(target_os = "macos")]`，Linux 与 Windows runner 上是 0 个。改动前同一命令还会运行业务层的全部单元测试和 `editor_tests`（按文件内容推算约 393 个，未检出 HEAD 实测）。`rust-lint` 同理：在 `src-tauri/` 里只 lint 壳的 1900 行。
- 业务层的 396 个单元测试和 lint 移到了 `workspace.yml`，对应的是根 `Cargo.lock` 与 `resolver = "3"`；Tauri 应用实际发布用的是 `src-tauri/Cargo.lock` 与 resolver 1。两套锁的 376 个共同包里有 174 个版本不同。core 的直接依赖只有 tokio 不同（1.53.3 / 1.54.1），差异集中在传递依赖：rustls 0.23.43 / 0.23.46、hyper 1.11.1 / 1.13.0、libssh2-sys 0.3.2 / 0.3.4、openssl-sys 0.9.117 / 0.9.118、libz-sys 1.1.29 / 1.1.33、dbus-secret-service 4.1.0 / 4.2.0。
- **为什么是问题**：M8 之前 Tauri 版仍是发布版本，而它的 CI 对 93% 的后端 Rust 代码（core 24283 行 / 26183 行）在交付配置下是零测试、零 lint，CI 仍是绿色。plan 第 36 行的 M0 退出标准「`src-tauri` 的构建、锁文件与检查不受影响」中，构建与锁文件属实，检查的覆盖面不是。第 77 行记了「Tauri 集成测试 1 个」，但没有说明它的含义。
- **建议**：
  1. 文档如实说明 `test.yml` / `lint.yml` 的 Rust job 现在只覆盖 Tauri 壳，业务层的测试与 lint 在 `workspace.yml`。
  2. 在 plan 的「风险与未决项」或 ADR 0007 的后果里记下「两套锁独立解析，交付配置下不再运行业务测试」。在现有结构下无法让 `src-tauri` 的 CI 跑 core 的单元测试（依赖包的 `#[cfg(test)]` 不会编译），只能选择定期对齐两套锁并接受剩余风险，或者将来改回单一 workspace。这应当是一个显式决定，而不是默认结果。
- **验证**：已验证（测试数为实跑结果，锁文件差异为逐包比对）。改动前的测试数是推算。

### 2. pre-commit 的 Rust 钩子没有覆盖根 workspace

- **位置**：`.pre-commit-config.yaml:12-24`、`CONTRIBUTING.md:19`
- **描述**：`cargo-fmt` 与 `cargo-clippy` 两个钩子的 entry 都带 `--manifest-path src-tauri/Cargo.toml`，触发条件是 `types: [rust]`。提交 `crates/**` 下的 `.rs` 文件会触发钩子，但它们只处理 `src-tauri/` 的文件。改动前所有 Rust 代码都在 `src-tauri/` 下，钩子是全覆盖的。
- **为什么是问题**：`CONTRIBUTING.md:19` 写着本地质量门禁 "All run automatically via `pre-commit` hook"，本次改动更新了同一段落，却让这句话不再成立。AGENTS.md 把格式或 lint 不通过归因为「跳过了这一步」，现在照做也会漏。
- **建议**：两个钩子各加一条根 workspace 的命令（`cargo fmt --all`；`cargo clippy --workspace --all-targets -- -D warnings`），`src-tauri` 的保留；同步修正 CONTRIBUTING 的那句话。
- **验证**：已验证（读了配置）。没有运行 `pre-commit run`，因为它的 fmt 钩子会改写文件。

### 3. dependabot 没有覆盖根 workspace 的 `Cargo.lock`

- **位置**：`.github/dependabot.yml:13-15`
- **描述**：cargo 生态只配置了 `directory: /src-tauri`。根 `Cargo.lock`（864 个包）不在 dependabot 范围内。
- **为什么是问题**：core 的传递依赖在根锁里是独立解析的。`src-tauri` 那边收到安全更新时，根 workspace 不会同步，上条提到的版本漂移会持续扩大。
- **建议**：增加 `package-ecosystem: cargo` / `directory: /`。同时决定是否允许 dependabot 更新 GPUI 的传递依赖：ADR 0007 钉死的只有 `gpui-kit = "=0.11.0"`（连同它精确依赖的 `gpui-pre`），`gpui-base`、`gpui-component` 等是 `0.11` 的 caret 约束，patch 更新仍会进入。如要冻结，用 dependabot 的 `ignore` 并在 ADR 里写明。
- **验证**：已验证。

### 4. 版本号 bump 流程没有覆盖两个新 crate

- **位置**：`crates/gitwave-core/Cargo.toml:3`、`crates/gitwave/Cargo.toml:3`、`scripts/bump-version.mjs:22-44`、`.agents/skills/gitwave-release/SKILL.md:38,55`、`README.md:81,125`
- **描述**：两个新 crate 各自硬编码 `version = "0.11.0"`，根 `[workspace.package]` 没有 `version`。`pnpm bump` 只改 `package.json`、`src-tauri/tauri.conf.json`、`src-tauri/Cargo.toml`，再刷新 `src-tauri/Cargo.lock` 里的 `gitwave` 条目；release skill 的残留检查范围相同。
- **为什么是问题**：下一次发版后 `gitwave-core` 会停在 0.11.0，两套锁里的 `gitwave-core` 条目与 `src-tauri` 的版本对不上。对 Tauri 应用没有行为影响（core 里没有任何 `CARGO_PKG_VERSION` 之类的引用），但 GPUI 应用一旦开始读版本号就会显示旧值。AGENTS.md 的 Documentation maintenance 要求同步发布技能登记的位置，这次新增了两处版本号却没有登记。
- **建议**：根 `[workspace.package]` 加 `version = "0.11.0"`，两个 crate 改为 `version.workspace = true`；bump 脚本多改根 `Cargo.toml` 并刷新两套锁；`SKILL.md` 的同步点与残留检查加上 `Cargo.toml` 与 `crates/`。最低限度也要把这两个文件加进脚本与残留检查。
- **验证**：已验证（脚本内容；全仓 grep 确认 core 无编译期版本引用）。

### 5. 文档写了不存在的命令 `cargo fmt --workspace`

- **位置**：`README.md:92`、`AGENTS.md:86`
- **描述**：两处说根 workspace 的命令是「同样的命令加 `--workspace`」。`cargo fmt` 没有这个参数，实测返回 `error: unexpected argument '--workspace' found`（exit 2）。
- **为什么是问题**：AGENTS.md 是 agent 和新贡献者的入口，按字面执行会报错。`CONTRIBUTING.md` 与 `Makefile` 用的 `--all` 是对的。
- **建议**：两处改成「`cargo fmt` 用 `--all`（或不带参数在仓库根运行），`cargo clippy` / `test` / `check` 用 `--workspace`」。
- **验证**：已验证（实际运行；`cargo fmt -v -- --check` 在仓库根不带 `--all` 时列出了两个成员的入口文件）。

### 6. `workspace.yml` 的 libgit2 TLS 断言，注释与脚本逻辑相反

- **位置**：`.github/workflows/workspace.yml:90-108`（沿用自 `.github/workflows/test.yml:66-85`）
- **描述**：注释写「缓存里陈旧的 libgit2 构建目录会掩盖丢失的 feature，所以扫描每一个 header」，脚本却是遇到第一个含 `GIT_HTTPS` 的 header 就 `exit 0`。缓存里留下的陈旧目录恰恰是带 HTTPS 的旧构建，当前构建丢了 `https` 时，它们会让断言通过。脚本实际防的是「盲目挑到一个不带 HTTPS 的旧目录而误报」，注释把它说成了「防止漏报」。本机 `target/` 下现在就有两个 `libgit2-sys` 构建目录，多目录并存是常态。
- **为什么是问题**：实际影响有限。第一层断言（`cargo tree -e features`）对「清单里丢了 feature」是确定性的；第二层的增量价值只在「依赖图里有 feature、构建产物里没有」时才体现，而那正是会被陈旧目录掩盖的场景。注释会让后来者高估这条断言的保障。
- **建议**：二选一，并把注释改回与实现一致（`test.yml` 的同一段可一并处理）：
  - 在 rust-cache 恢复之后、构建之前执行 `rm -rf target/debug/build/libgit2-sys-*`，断言改成「必须找到 header 且含 `GIT_HTTPS`」；
  - 或改成「所有 header 都必须含 `GIT_HTTPS`」，代价是不带 HTTPS 的陈旧目录会让某次运行失败一次。
- **验证**：已验证脚本逻辑与注释（读了两个工作流，并按 workspace.yml 的脚本在本机原样执行）。「陈旧目录掩盖丢失的 feature」是按脚本逻辑的推演，没有在 CI 上复现。

### 7. `workspace.yml` 的 Linux 依赖清单与依赖树对不上，且未经托管环境验证

- **位置**：`.github/workflows/workspace.yml:55-75`
- **描述**：[plan.md](./plan.md) 第 80 行已如实记录「三端 CI 还没有在托管环境运行过」。对照依赖树，这个清单有几处疑点：
  - 注释说其余的包是「GPUI 与原生音频 / 图形 crate 链接的」，但根 `Cargo.lock` 里没有任何音频 crate 的系统绑定，`libasound2-dev` 找不到使用者。
  - `vulkan-validationlayers` 只是验证层，CI 不启动窗口。
  - gpui-pre-linux 的 X11 后端用的 `x11rb 0.13` 没有启用 `dl-libxcb`，要靠 pkg-config 找系统库。Zed 自己的 Linux 构建清单里有 `libxcb-shape0-dev` 和 `libxcb-xfixes0-dev`，这里没有。
  - `libasound2-dev` 在较新的 Ubuntu 上是过渡包名。
- **为什么是问题**：M1 的退出标准是「三端 CI 构建通过」，plan 的原则也要求「三端 CI 从第一个里程碑开始」。清单里的多余包无害；缺的包是只出 warning 还是让构建失败，决定了第一次托管运行的结果。
- **建议**：补上 `libxcb-shape0-dev libxcb-xfixes0-dev`；去掉音频与验证层包，或把注释改成与依赖树一致。首次跑通后把实际需要的清单回写到 plan 的执行记录。
- **验证**：推测。依据是 `gpui-pre-linux-0.3.14/Cargo.toml` 与根 `Cargo.lock` 的包列表，没有在 Linux 上构建过。

### 8. 侧栏显隐会让三栏宽度串位

- **位置**：`crates/gitwave/src/workbench.rs:74-96`（`h_resizable("workbench-panes")` 的子节点）
- **描述**：gpui-base 0.11.0 把面板尺寸存在一个按索引对齐的 `Vec` 里（`gpui-base-0.11.0/src/resizable/mod.rs:194-204`），面板数量变化时只在尾部追加或截断。这里在 `sidebar_visible` 为 false 时不插入 sidebar 面板。隐藏侧栏后，索引 0、1 的状态（原本属于 Sidebar 和 History）落到 History 和 Diff 上；再显示时索引 2 是新追加的最小尺寸，Diff 被压到最小宽度，Sidebar 与 History 各继承后一栏的宽度。
- **为什么是问题**：Cmd-B 是 M0 骨架里仅有的两个交互之一，往返一次就能看到宽度跳变。
- **建议**：三个面板始终作为子节点存在，侧栏用 `resizable_panel().visible(self.sidebar_visible)` 隐藏（`gpui-base-0.11.0/src/resizable/panel.rs:277`），状态里会保留它的尺寸。
- **验证**：推测。依据是 gpui-base 源码的阅读，没有在运行中的应用里实际按 Cmd-B 验证。

## 🟢 优化建议（可选）

- **守卫测试的稳健性**（`crates/gitwave-core/src/lib.rs:23-62`）：
  - `native_credential_store_is_compiled_in` 依赖 Debug 文本里的类型名。上游若调整 Debug 输出，`!contains("MockCredential")` 会恒真而静默失效；`Err => return` 是一个空通过分支（在 keyring 3.6.3 的三个原生后端上不可达）。可以改用 `keyring::default::default_credential_builder().persistence()` 并断言不等于 `CredentialPersistence::EntryOnly`。
  - 前两条守卫是对清单文本的子串匹配：第一条会被注释里的 `tauri` / `gpui` 字样误触发；第二条不检查 feature 是否挂在 `keyring` 上、是否在正确的 target 段。目前够用。
  - M1 退出标准写的是「core crate 不含任何 Tauri 类型」，守卫只覆盖直接依赖的清单文本。可以把 `cargo tree -p gitwave-core -e normal | grep -E 'tauri|gpui'` 应为空加成 CI 步骤，覆盖传递依赖。
- **`#![allow(dead_code)]` 与 `#![allow(unused_imports)]`**（`crates/gitwave-core/src/lib.rs:10-11`）：注释说明是继承来的、打算逐模块解除。建议排在 M2 之前，用来发现抽离后不再被任何 presentation 层使用的 API。
- **测试缺口**：`run_sync_op` 缺一条「操作返回 `Err` 时原样透传」的用例；`bootstrap.rs` 的两个内存回退分支没有测试，抽成 pub 函数后已可测。
- **重复模式**：`src-tauri/src/lib.rs:383`、`:1429`（`cmd_list_repos`、`cmd_list_remotes`）仍是手写的 `tauri::async_runtime::spawn_blocking` 加 `AppContext::new(workspaces, app_settings)`，与 `sync_ops::local_git_task` 是同一模式。给后者加错误码参数后可以统一。
- **`Makefile:5`**：`WORKSPACE := --workspace --all-targets` 不带 `--locked`，而 `workspace.yml` 的 clippy 与 test 都带。AGENTS.md 把 `make check` 称为 CI 等价门禁，本地通过后 CI 仍可能因根 `Cargo.lock` 过期而失败。给 `WORKSPACE` 加上 `--locked` 即可。
- **`workspace.yml` 的触发范围与耗时**：`paths` 是白名单，`crates/**` 下的 `.md` 改动也会触发三端编译，可用 `!crates/**/*.md` 排除。`[profile.dev.package."*"] opt-level = 1` 让 CI 的全部依赖以 opt-level 1 编译，而 CI 不运行界面，可以评估是否只在本机开启。
- **过时的路径与数字**：
  - `crates/gitwave-core/src/infrastructure/persistence/migrations.rs:3` 与 `docs/tech/tech-selection/00-overview.md:61` 仍写 `src-tauri/migrations/`。
  - `crates/gitwave-core/src/domain/error_codes/cmds.rs:1` 的「the Tauri command layer (`lib.rs`)」指代变得含糊。
  - [plan.md](./plan.md) 第 77 行的「427 个原有包」：实测 HEAD 锁文件有 571 个 `[[package]]`，按平台统计（macOS 328、Linux 394、Windows 317）也得不出 427。结论「无一增删」是对的，数字建议改成实测值或注明口径。
  - `README.md:106` 的 `make lint` 一行没有提两个 workspace，其余三行都提了。
- **包名与 lint 配置**：根 workspace 的 `gitwave`（GPUI 应用）与 `src-tauri` 的 `gitwave`（Tauri 应用）同名，仅靠 workspace 隔离，`cargo tree` 与残留检查的输出里无法区分，可考虑改包名、保留二进制名。`src-tauri/Cargo.toml` 用 `[lints.rust] linker_messages = "allow"` 压掉 MSVC 链接器的噪音，根 workspace 没有对应配置，Windows 上 `cargo test --workspace` 会出现同样的警告（不带 `-D warnings`，不会失败；推测，未在 Windows 上运行）。
- **提交前需要确认的事项**：
  - 70 个 rename 已在暂存区，但 `crates/gitwave-core/{Cargo.toml,src/lib.rs,src/bootstrap.rs,src/host.rs}`、根 `Cargo.toml` / `Cargo.lock` / `clippy.toml` / `rustfmt.toml`、`workspace.yml`、`crates/gitwave/`、`docs/tech/tech-selection/gpui-kit-research.md` 与本任务目录都还未跟踪，`src-tauri` 的三个文件、`sync_ops.rs` 的后续修改、前端两个文件、`Makefile`、`.gitignore` 与文档都未暂存。此时直接提交会得到一个无法编译的提交。
  - `workspace.yml` 用 `--locked`，根 `Cargo.lock` 不入库则第一步就失败。
  - `native/.nsgo/test/screenshots/macos/native-history-split.png`（243 KB，11 月 23 日）是已放弃的 Go + NSGo 尝试留下的未跟踪文件，没有被 `.gitignore` 覆盖，`git add -A` 会把它带进来。

## 📝 总体评价

抽离本身做得很干净：搬移逐字节一致，`lib.rs` 的改写是机械的，没有找到改变 Tauri 应用行为的改动，两个 workspace 的门禁在本机全绿。需要优先处理的是门禁与流程还按单 workspace 配置：Tauri 作为当前发布版本，它的 CI 与 pre-commit 对业务代码的覆盖已经缩水到接近零而没有任何信号（🟡 1、2），两套锁会各自漂移（🟡 1、3、4）。合并前还建议让 `workspace.yml` 在托管环境实际跑一次，并修正文档里的 `cargo fmt --workspace`。

## 验证记录

本机实际执行的只读命令与结果。

| 命令 | 结果 |
|---|---|
| `cargo metadata --locked --format-version 1 --no-deps`（根） | 通过。成员 `gitwave 0.11.0`、`gitwave-core 0.11.0`，`rust-version 1.92` |
| `cargo metadata --locked ... --manifest-path src-tauri/Cargo.toml` | 通过。workspace root 是 `src-tauri`，构建目录 `src-tauri/target`，路径依赖 `gitwave-core` |
| `git diff HEAD -- src-tauri/Cargo.lock` | 只有 `gitwave` 的依赖列表变化，新增 `gitwave-core` 条目 |
| 两份 `src-tauri/Cargo.lock` 的 name + version 集合 diff | 571 → 572，仅 `gitwave-core 0.11.0` |
| 70 个搬移文件：`git show HEAD:<旧路径> \| cmp -s - <新路径>` | 69 个一致，1 个不同（`sync_ops.rs`） |
| `generate_handler!` 块逐字 diff | 130 个命令，含顺序一致 |
| `cargo tree --locked -p gitwave-core -e normal \| grep -i -E 'tauri\|gpui'` | 0 行（依赖图 143 个包） |
| `cargo fmt --all -- --check`（根） | 通过 |
| `cargo clippy --workspace --all-targets --locked -- -D warnings` | 通过 |
| `cargo test --workspace --all-targets --locked` | `gitwave` 0 个；`gitwave-core` 396 个，394 通过，2 个原本就忽略 |
| `cargo fmt --manifest-path src-tauri/Cargo.toml -- --check` | 通过 |
| `cargo clippy --manifest-path src-tauri/Cargo.toml --all-targets --locked -- -D warnings` | 通过 |
| `cargo test --manifest-path src-tauri/Cargo.toml --all-targets --locked` | lib 0 个、main 0 个、`updater_restart` 1 个，全部通过 |
| `pnpm test` | 61 个文件、367 个测试通过 |
| `workspace.yml` 的两条 TLS 断言，在根目录原样执行 | `git2 feature "https"` 命中；`target` 下 2 个 `git2_features.h` 都含 `GIT_HTTPS` 与 `GIT_SECURE_TRANSPORT` |
| `workspace.yml` 用系统 ruby 解析 YAML，各 `run` 块过 `bash -n` | 解析成功，6 个脚本块语法通过（本机无 actionlint，未安装） |
| `cargo fmt --workspace -- --check` | 报错 `unexpected argument '--workspace' found`（exit 2） |
| 守卫测试 2 的反向验证：`sed` 去掉 `apple-native` 那一行后模拟 `contains` | 命中数 1 → 0 |
| `make -n check` | 展开的 12 条命令覆盖两个 workspace |
| 阅读本机 registry 的 `keyring-3.6.3` 源码 | 三个原生后端的 builder 都不做 I/O；`MockCredential` 的 Debug 输出含类型名；mock 下 `Entry::new` 恒为 `Ok` |
| 审查前后的 `git status` 与 `git diff HEAD --stat` | 已跟踪文件无变化 |

**没有验证的部分**：

- 没有在 Linux、Windows 或任何托管环境运行过；`workspace.yml` 的 Linux 依赖清单与 Windows 上的 `linker_messages` 警告都是推测。
- 没有启动 Tauri 应用做端到端的 fetch / pull / push，`run_sync_op` 的等价性是静态对比加编译与单元测试的结果。
- 没有运行 GPUI 应用并按 Cmd-B，侧栏宽度串位是源码推断。
- 没有检出 HEAD 实测改动前的测试数，该数字是按文件内容推算的。

---

## 处理结果

记录人：主会话（实现方），2026-11-25。逐条对应上面的 🟡。

| # | 问题 | 处理 | 状态 |
|---|---|---|---|
| 1 | Tauri 的 CI 对业务代码覆盖缩水 | 审查认为现有结构下无法让 `src-tauri` 的 CI 跑 core 的单元测试，这一点不成立：`src-tauri` 目录下 `cargo test -p gitwave-core --lib` 实测可运行（396 个，394 通过，2 忽略），用的就是 Tauri 应用的锁文件。据此在 `test.yml` 的 `rust-test` 加 `cargo test -p gitwave-core --all-targets`，在 `lint.yml` 的 `rust-lint` 加 `cargo clippy -p gitwave-core --all-targets -- -D warnings`，`Makefile` 的 `test` 与 `lint` 同步。core 的测试与 lint 现在在两套锁文件下各跑一遍 | 已修复 |
| 2 | pre-commit 的 Rust 钩子不覆盖根 workspace | 两个钩子改为同时处理 `src-tauri` 与根 workspace | 已修复；本机没有安装 pre-commit，只验证了 YAML 可解析 |
| 3 | dependabot 不覆盖根 `Cargo.lock` | 新增 `directory: /` 的 cargo 条目；对 `gpui-kit` 设了 `ignore`，理由与 ADR 0007 一致（版本精确钉死，升级是独立任务） | 已修复；是否忽略 `gpui-kit` 待用户确认 |
| 4 | 版本号 bump 不覆盖新 crate | 根 `[workspace.package]` 加 `version`，两个 crate 改为继承；`scripts/bump-version.mjs` 同时改根 `Cargo.toml` 并刷新两套锁；发布技能、README、AGENTS.md 同步。实测 bump 到 0.11.1 时四处版本与两套锁的 `gitwave`、`gitwave-core` 条目全部更新，随后逐字节还原 | 已修复 |
| 5 | 文档写了不存在的 `cargo fmt --workspace` | AGENTS.md 与 README 改为「`cargo fmt` 用 `--all`，其余用 `--workspace`」 | 已修复 |
| 6 | TLS 断言的注释与脚本相反 | 注释改回与实现一致：第一层断言负责发现清单里丢了 feature，第二层只在多个构建目录并存时避免误报 | 已修复（仅注释） |
| 7 | Linux 依赖清单有疑点 | 采纳低风险的补全：加上 `libxcb-shape0-dev`、`libxcb-xfixes0-dev`，保留上游安装脚本的全部条目，注释改为与来源一致 | 已修改；首次托管运行时 Ubuntu 另缺 `libdbus-1-dev`，补上后三端通过 |
| 8 | 侧栏显隐让三栏宽度串位 | 三个面板始终存在，侧栏用 `resizable_panel().visible(..)` 隐藏 | 已修复，clippy、构建、启动通过；未手工验证往返 |

处理过程中另外修正的两处：

- 通读差异时发现 `docs/tech/engineering/00-overview.md` 里 `frontend-lint` 一行被误改成 `tsc --no-ese`，已恢复为与 `main` 一致的 `tsc --noEmit`。
- 验证 bump 脚本时有两次操作失误，都把版本文件改到了 0.11.1：一次是备份步骤在 zsh 下没有生效，一次是误在后台启动了脚本。两次都已还原，并用逐包比对确认 Tauri 锁文件相对 `main` 仍只有预期的差异，两个 workspace 的 `--locked` 解析通过。

修复后的验证：`make fmt-check`、`make lint`、`make test` 全部通过；前端 367 个测试，Tauri 集成测试 1 个，core 在两套锁文件下各 396 个（394 通过，2 忽略）。

---

# 代码审查：M1b 移除旧应用

> **作者**：AI code-reviewer 子代理，未经人工复核
> **状态**：草案
> **最近更新**：2026-11-25

| 项 | 内容 |
|---|---|
| 分支 | `chore/remove-tauri-app`，审查时尚未提交的工作区改动 |
| 基线 | `8466d55`（`main`，PR #243 合并后） |
| 计划与决策 | [plan.md](./plan.md) · [ADR 0007](../../tech/decisions/00-overview.md#0007--桌面界面迁移到-gpui-kit) |
| 环境 | macOS arm64，rustc 1.100.0，cargo 1.100.0，bash 3.2.57 |
| 结论 | 没有必须修复才能合并的问题；`make check` 全绿，文案转换逐字保真，文档里的数字都有依据。合并前建议处理 🟡 的前两条 |

审查范围是整组未提交的改动，重点核实了九件事：删除的完整性、文案转换的保真度、`locales.rs` 的测试、`scripts/bump-version.sh`、配置与文档的一致性、`gitwave-core` 的改动是否只有注释、`rust-version` 的变更、依赖与锁文件、CI。审查者独立跑了命令并做了变异实验，下面的报告是它的原文。

## ✅ 优点

- **文案转换逐字保真。** 50 对文件一一对应，2216 条叶子的键集合、顺序、文本全部一致，686 处占位符只从 `{{name}}` 变成 `%{name}`。审查者用独立的对比脚本和真实的 rust-i18n 4.2.4 各验证了一遍，后者经 `t!` 取回 2216 个键，0 处不符。
- **`gitwave-core` 的改动严格限于注释。** 80 条变更行里 78 条是 `//` 注释，2 条是 `Cargo.toml` 的 `#` 注释，没有代码行。顺手修正了 `migrations.rs` 里指向 `src-tauri/migrations/` 的陈旧路径。
- **文档里的数字都能复现。** 测试数 84 / 306 / 3 / 3 / 4、文案 1108 条、占位符 343 处、复数 9 组、后端 2.4 万行与 2.6 万行、前端 2.45 万行与 5.9 千行、10 个无引用的 `cmds.*` 错误码，逐项对上了（见验证记录）。
- **测试的核心保证是有效的。** 错误码与文案双向校验、非字符串值拒绝、空消息拒绝都成立；审查者在 `/tmp` 副本上做的 22 个变异里有 15 个被抓到。
- **文档对现状是诚实的。** `architecture` 与 `tech-selection` 总览明确写了「还没有接入 `gitwave-core`」，工程总览写明日志初始化没有调用方、目前不写日志，ADR 表把 0001 / 0005 标为已废弃。
- **删除干净。** 17 个图标与原文件 blob 哈希逐一相同；`site/` 只引用自己目录里的资源；`Cargo.lock` 相对 `main` 只多两行依赖边。
- **MSRV 的变更理由属实，且恰好。** 依赖图 864 个包里 MSRV 高于 1.92 的第三方包正好是那三个，`1.95` 就是全图最高值。

## 🔴 严重问题（必须修复）

无。

## 🟡 一般问题（建议修复）

### 1. `proxy.rs` 的注释多了一行重复

- **位置**：`crates/gitwave-core/src/infrastructure/proxy.rs:251-252`
- **描述**：第 252 行 `/// loaders use; libgit2/reqwest read the env from other unsynchronized` 是第 251 行后半句的重复。读起来是半句接半句，句子断了。这是编辑事故，不影响行为。
- **建议**：第 251 行改为 `/// dotenv loaders use; libgit2/reqwest read the env from other unsynchronized`，删除第 252 行。
- **核实**：读文件；`grep -c 'loaders use; libgit2/reqwest'` 当前为 2，`main` 上为 1。

### 2. 两处把「尚未接线」写成了现状

- **位置**：`README.md:50`（"It is called in process, with no IPC."）；`docs/tech/tech-selection/00-overview.md:16`（「界面在进程内直接调用 `gitwave-core` 的用例」）
- **描述**：`crates/gitwave` 的 `Cargo.toml` 不依赖 `gitwave-core`，`bootstrap.rs` 与全部用例目前没有任何调用方。同一份 README 的第 51 行和 `architecture/00-overview.md:48` 写的是「还没有接入」，前后矛盾。上一轮审查指出过同类问题。
- **建议**：改成将来时，例如 "It is meant to be called in process, with no IPC; the application does not link it yet."
- **核实**：`cat crates/gitwave/Cargo.toml`；`cargo tree --locked -p gitwave -i gitwave-core` 报 "did not match any packages"；全仓 grep `bootstrap::` 无结果。

### 3. `scripts/bump-version.sh` 在误用时会改错文件或改错行，并且照样报成功

- **位置**：`scripts/bump-version.sh:15`（`cd "$(git rev-parse --show-toplevel)"`）、`:18-34`（按「第一个以 `version = "` 开头的行」定位）
- **描述**：两个问题，在 `/tmp` 临时仓库里都复现了。
  - 在非 git 目录里运行不会中止：`git rev-parse` 失败后变成 `cd ""`，脚本留在当前目录。审查者在一个无关的 Rust 项目目录里运行，它的 `Cargo.toml` 从 `1.0.0` 被改成了 `9.9.9`。
  - 不校验匹配到的行是否在 `[workspace.package]` 段内。在该段之前插入一行 `version = "9.9.9"` 后，脚本改的是那一行，真正的版本和 `Cargo.lock` 都没动，输出仍是 "[bump] Cargo.lock refreshed, every crate -> 0.12.0"。
  - 旧脚本 `bump-version.mjs` 有「替换后内容必须变化」的自检，新脚本没有。
- **建议**：`toplevel=$(git rev-parse --show-toplevel) || exit 1`；定位时限定 `[workspace.package]` 段；改完后用 `cargo metadata --locked --no-deps` 读回 workspace 版本，与目标值比较，不一致就回滚并退出非零。
- **核实**：在 `/tmp/gw-bump-lab`（`git init`，复制 `Cargo.toml`、`Cargo.lock`、两个 crate 的清单）里运行脚本，对比 `git diff`。

### 4. `scripts/bump-version.sh` 失败时不回滚，会留下改坏的 `Cargo.toml`

- **位置**：`scripts/bump-version.sh:10`（正则）、`:24-38`
- **描述**：带前导零的版本（`0.012.0`、`00.12.0`、`0.12.00`）能通过校验并被写入 `Cargo.toml`，随后 `cargo metadata` 以 101 退出，文件留在无效状态。`cargo` 不在 `PATH` 时同理（exit 127，`Cargo.toml` 已改而 `Cargo.lock` 未改）。
- **建议**：正则改为 `0|[1-9][0-9]*`；改写前备份，`trap` 在失败时恢复。
- **核实**：同上，逐个传入非法参数并检查 `git status --porcelain` 与退出码。其它非法输入（空、`0.12`、`abc`、`1.2.3.4`、`0.12.0+build`、大写 `V`、全角数字）都在写文件之前被拒。

### 5. `scripts/bump-version.sh` 的「只改版本号」有前提，脚本不检查也不披露

- **位置**：`scripts/bump-version.sh:36-39`
- **描述**：注释写 "nothing else moves"，输出写 "every crate -> x.y.z"。这只在 `Cargo.lock` 的起点与清单一致时成立。审查者先把一个依赖需求改掉再运行，脚本顺带把 `base64 0.23.1` 改成了 `0.22.1`，输出没有提示。旧版 `SKILL.md` 要求 bump 前工作区干净、事后核对变更文件，新版没有这道人工检查。
- **建议**：改版本之前先跑 `cargo metadata --locked`，起点不干净就中止。
- **核实**：同上；bump 前 `cargo metadata --locked` 退出码 101，bump 退出码 0，`git diff -U0 -- Cargo.lock` 多出 base64 两行。

### 6. `locales.rs` 的 `merge` 对跨文件重复键静默覆盖

- **位置**：`crates/gitwave/src/locales.rs:126-137`（`merge`），用于 `:115-124` 的 `errors()`
- **描述**：两个 `errors-*.json` 定义同一个键时，后者直接覆盖前者，四条测试全部通过。rust-i18n 加载时也是静默覆盖，最终显示哪条取决于目录遍历顺序。当前没有重复键，所以这是防线缺口而不是现存缺陷。
- **建议**：`merge` 里遇到「已存在的叶子」时 `assert!` 并报出文件名与键；`errors()` 遇到没有顶层 `errors` 键的文件也应当报错，而不是跳过。
- **核实**：在 `/tmp` 副本上往 `errors-infra.*.json` 里塞入 `errors.git.raw`，4 passed；在探针 crate 里复现 rust-i18n 的覆盖行为。

### 7. 改了语言文件后，本地增量构建仍内嵌旧文案

- **位置**：`crates/gitwave/src/main.rs:9`（`rust_i18n::i18n!(…)` 在编译期读文件）；`crates/gitwave/src/locales.rs:195-202`
- **描述**：cargo 的指纹不追踪过程宏自己读的文件。实测只改 `locales/app.en.json` 后 `cargo run`，二进制里还是旧值 `Health`，`touch main.rs` 之后才更新。后果有两个：
  - 本地改文案后 `make dev` 看不到变化。
  - 测试 3（`the_messages_load_through_rust_i18n`）在本地跑的是陈旧的内嵌表，而测试 1、2、4 读的是磁盘上的新文件。

  另外，测试 3 把 `Health` / `健康` 写死为期望值，任何对这两个文案的合法修改都会让它失败。CI 是全新构建，不受影响。
- **建议**：三选一。
  - `build.rs` 里对 `locales/` 算摘要并输出 `cargo:rustc-env`，让文件变化触发重编。
  - 在 `locales.rs` 模块注释和 `CONTRIBUTING.md` 里写明「改了语言文件要 `cargo clean -p gitwave`」。
  - 测试 3 的期望值改为从 JSON 读出。
- **核实**：`/tmp/gw-probe`（依赖 `rust-i18n = "=4.2.4"`，`--offline` 构建）三步实验。

### 8. MSRV 提到 1.95 的决策没有记录，而且与依赖自己的注释矛盾

- **位置**：`Cargo.toml:8`；`crates/gitwave-core/Cargo.toml:51-53`
- **描述**：改法本身恰当。合成实验证实 cargo 的 MSRV 检查是构建期的硬错误，并且只针对参与当前 target 构建的包，所以 `main` 上的 `1.92` 在 Windows 上本来就自相矛盾，只是 CI 用 stable 没有暴露。问题在决策依据：
  - 把 MSRV 推到 1.95 的唯一原因是 `windows-registry = "0.100"`。
  - 这个依赖上一行的注释说，版本范围是为了与 hyper-util 的可选依赖一致，好让依赖图解析出单一共享构建。
  - hyper-util 的实际需求是 `">=0.3, <0.7"`，0.100 不在其中。锁文件里 `windows-registry` 有 0.4.0、0.6.1、0.100.0 三个版本并存。
  - 0.6.1 的 MSRV 是 1.82。
- **建议**：在 ADR 0007 或提交说明里记一句 MSRV 的来由。同时澄清 0.100 是有意的选择还是笔误，并修正那句注释。这句话在 `main` 上就有，不是本次引入。
- **核实**：`cargo metadata --locked`；`cargo tree --locked --target <triple> -i windows-registry@0.100.0`；读 registry 里三个 crate 的 `Cargo.toml`；`/tmp/gw-msrv` 合成实验。

## 🟢 优化建议（可选）

- **`locales.rs` 测试的其余盲区**（实证 4 passed）：
  - 占位符出现次数不同（比较的是集合）。
  - 新增 `app.ja.json` 而未列入 `LANGUAGES`（硬编码）。
  - 非 `errors` 域文件里出现孤儿 `errors.*`（只扫 `errors*` 域）。
  - `%{ detail }` 与未闭合的 `%{`。
  - 反方向：`error_codes/*.rs` 的注释里出现 `pub const X: &str = "git.fake"` 会被当成常量，测试 4 失败。
- **`locales.rs` 模块注释**："the keys that end in `_one` and `_other` are chosen in code" 是现在时，但生产代码里没有任何 `t!` 调用，`workbench.rs` 里的 "Sidebar" / "History" / "Diff" / "No repository selected" 仍是硬编码英文。README 的 "interface text lives in rust-i18n catalogues" 同理，建议注明尚未接线。
- **`main.rs`**："defines the `t!` macro at the crate root" 不够精确。`i18n!` 展开出的是 `pub(crate) use __rust_i18n_t as _rust_i18n_t`，`t!` 由 `rust_i18n` crate 导出并转发到 `crate::_rust_i18n_t!`。结论（必须放在 crate 根）是对的，实验证实放进子模块会报 `E0433`。
- **`remote.rs`**：沿袭下来的 "Empty when constructed; always stamped before emit." 与事实不符。`SyncProgress` 全仓只有一个构造点，那里直接填入调用方的值。
- **`use_cases.rs`、`cmds.rs`**：把 "the presentation layer…" 写成了现状，建议改成将来时或「设计约定」的语气。
- **`proxy.rs`、`remote.rs`**：注释仍提到 updater 插件，它已随 Tauri 应用删除。
- **`.github/PULL_REQUEST_TEMPLATE.md`**：被注释掉的提示文字里还有 "CI green (lint / test / build)"，是已删除的工作流名。
- **`docs/tech/engineering/00-overview.md`**：`*.md` 应为 `**.md`；只列了一条 TLS 断言，实际有两条。`plan.md` 说三个命令「在三端 CI 上运行」，fmt 只在 Ubuntu 上跑。
- **`plan.md` 与 README、工程总览**：后两者说界面的无头窗口测试「随 M2 起」，但计划的 M2 定义里没有这一项。
- **`.gitattributes`**：`package-lock.json`、`yarn.lock`、`composer.lock` 已是模板残留。
- **`scripts/bump-version.sh`**：多余参数被静默忽略；awk 中断时残留的 `Cargo.toml.tmp` 不在 `.gitignore` 里。
- **`.pre-commit-config.yaml`**：clippy 没带 `--locked`，CI 带。
- **`.github/dependabot.yml`**：`keyring` 的注释属实，但直接依赖写的是 `"3"`，本来就解析不到 4.x，这条 ignore 目前不起作用。
- **合并后的收尾**：GitHub 上还开着 4 个针对 npm 生态的 dependabot PR（#238、#239、#240、#241），建议关闭。
- **`branches.irebase.*`**：键名里的 `irebase` 是旧实现就有的拼写，转换如实保留；接线时注意别写成 `rebase`。

## 📝 总体评价

这次改动的机械部分质量很高：文案转换、图标搬移、核心库的注释改写都逐字可查，没有发现偏差；文档里的每个数字审查者都能用命令复现。最需要优先处理的是三类「说的和做的不一致」：`proxy.rs` 那行重复注释，README 与选型总览里两处把「将要进程内调用」写成「正在进程内调用」，以及 MSRV 决策与 `windows-registry` 注释之间的矛盾。`bump-version.sh` 在文档写明的用法下是正确且幂等的，但它比旧脚本少了自检和回滚；在 M7 之前它处于休眠状态，建议等发布链路落地时一并补上。语言文件不触发重编译这一点会让后续里程碑里接线 `t!` 的人困惑，值得现在就在文档里留一句。

## 验证记录

以下两小节是审查者列出的、它亲自运行过的命令和结果，以及它没有验证的内容。行号对应审查时的工作区，后续修复后有移动。

### 验证过的

**环境与范围**

- `git status --short | wc -l` → 383（D 325、M 37、R 17、RM 1、未跟踪 3 条目 / 52 个文件）；审查结束时仍是 383，`Cargo.lock` 哈希前后一致。
- `git diff main -- crates/gitwave-core` → 80 条变更行，按前缀分类：`//` 78、`#` 2、其它 0。
- `git diff main -- Cargo.lock` → 只有 `+ "rust-i18n",`、`+ "serde_json",`。

**文案保真度**

- 独立脚本把 `main:src/i18n/locales/<语言>/<域>.json` 与 `crates/gitwave/locales/<域>.<语言>.json` 展平后对比：文件 50 / 50；叶子 2216 / 2216；占位符 686 / 686；键集合差异 0；规范化后文本差异 0；空白差异 0；非字符串值 0；重复 JSON 键 0；含点的键名 0；展平冲突 0；跨文件重复键 0（每种语言 1108 个键）。
- 占位符审计：64 个不同的名字，字符只有字母与下划线；没有 `$t(`、`${`，占位符之外没有孤立的花括号；没有 `%{` 之外的 `%`。
- 旧实现 `src/i18n/index.ts`：`fallbackLng: "en"`、`escapeValue: false`；`resources.ts` 把四个 `errors-*.json` deep-merge 进同一个 `errors` 子树。
- 四个错误文件的二级前缀：`errors-cmds` → `cmds`（10 条）；`errors-git` → `git`（94 条）；`errors-infra` → `ai` / `persist` / `ssh`（40 条）；`errors-usecases` → `usecases`（49 条）。合并后 193 条，对应 `error_codes/` 下 `cmds.rs` 10、`git.rs` 94、`infra.rs` 40、`usecases.rs` 49 个常量。
- 复数键：9 对 `_one` / `_other`，没有 `_zero` / `_two` / `_few` / `_many`。
- 新文件：50 个都是 LF、无 BOM、恰好一个结尾换行。

**rust-i18n 4.2.4 的行为**

- 读了 registry 里 `rust-i18n-support`、`rust-i18n-macro`、`rust-i18n` 4.2.4 的源码（`load_locales`、`flatten_keys`、`merge_value`、`i18n!` 的展开与回退链、`t!` 的展开、`replace_patterns`）。
- 确认：语言取自 `file_stem()` 按 `.` 切分的最后一段；缺键时先走 `zh-CN` → `zh` 的子标签链，再查 `fallback`；缺键返回键名，不报错；没有复数；`t!` 对键没有编译期校验。
- `/tmp/gw-probe`（真实 rust-i18n，50 个文件复制过去）：`available_locales = ["en", "zh-CN"]`；2216 个键经 `t!` 取回与 JSON 逐字一致；`zh-TW` 回退到 `"Health"`；缺键返回 `"does.not.exist"`；`diff.toolbar.filesChanged` 返回键名（复数键不会被自动选择）。
- `/tmp/gw-pos`：`i18n!` 放进子模块 → `error[E0433]: cannot find _rust_i18n_t in the crate root`；放在 crate 根时子模块可用。
- `/tmp/gw-probe` 重编译实验：只改 JSON 后内嵌值仍是 `"Health"`，`touch main.rs` 后才变。

**`locales.rs` 测试的变异实验**（`/tmp/gw-mut`，基线 4 passed）

| 变异 | 结果 |
|---|---|
| 删除 zh-CN 的一个键 | 抓到（3 条失败） |
| zh-CN 改占位符名 | 抓到 |
| en 加数字叶子 | 抓到 |
| errors 文件加无对应 code 的消息 | 抓到 |
| `error_codes` 加无文案的常量 | 抓到 |
| 某个 code 从 `pub const` 改为 `const` | 抓到 |
| 空消息 | 抓到 |
| 顶层 `errors` 键改名 | 抓到 |
| code 值前缀写错；删除一个 code 常量；键名含点 | 抓到 |
| 删除整个 `errors-cmds.zh-CN.json`；文案换成数组；文案换成空对象 | 抓到 |
| 两个 errors 文件定义同一个键 | 未发现 |
| 占位符出现次数不同 | 未发现 |
| 新增 `app.ja.json` 未列入 `LANGUAGES` | 未发现 |
| 非 errors 域文件里的孤儿 `errors.*` | 未发现 |
| `%{ detail }`；未闭合的 `%{` | 未发现 |
| 源码注释里出现 `pub const X: &str = "git.fake"` | 误报 |

**`scripts/bump-version.sh`**（`/tmp/gw-bump-lab`，`CARGO_NET_OFFLINE=true`）

- 正常升级：`exit=0`；`Cargo.toml` 只改 `version` 一行；`Cargo.lock` 只改 `gitwave` 与 `gitwave-core` 的两个 `version`。
- 幂等：提交后重跑同一版本，`git status --porcelain` 为空，无残留文件，权限保持。
- `v0.13.0-rc.1`：正常。
- 14 种非法输入：除前导零外全部 `exit=1` 且不改文件；全角数字在 `C`、`en_US.UTF-8`、`zh_CN.UTF-8` 下都被 bash 拒绝。
- 清单变体：行尾注释、无空格、尾随空格、CRLF、单引号 → 都是 `exit=1`，文件不变。
- 仓库外、非 git 目录 → 改写了当前目录的无关 `Cargo.toml`。
- `PATH` 里没有 `cargo` → `exit=127`，`Cargo.toml` 已改而 `Cargo.lock` 未改。
- 锁文件起点过期 → 顺带改写 `base64`。

**MSRV**

- `cargo metadata --locked`：864 个包，583 个声明了 `rust_version`；高于 1.92 的第三方包是 `windows-strings` 0.100.0、`windows-result` 0.100.0、`windows-registry` 0.100.0，都是 1.95；其余最高是 `oo7` 0.6.0 的 1.92。
- 读 registry 源码：三个 crate 的 `Cargo.toml` 都是 `rust-version = "1.95"`；`hyper-util` 对 `windows-registry` 的需求是 `">=0.3, <0.7"`；`windows-registry` 0.6.1 的 MSRV 是 1.82。
- `cargo tree --locked --target <triple> -i <包>@0.100.0`：macOS 与 Linux 上都是 "nothing to print"；`x86_64-pc-windows-msvc`、`aarch64-pc-windows-msvc` 上链路是 `gitwave-core → windows-registry 0.100.0 → windows-result` / `windows-strings 0.100.0`。
- `/tmp/gw-msrv` 合成实验：声明 `rust-version = "1.200"` 的本地包作普通依赖时，`cargo build` 报 "rustc 1.100.0 is not supported by the following package"；放进 `cfg(target_os = "windows")` 依赖后，宿主 `cargo build` 不报，`cargo check --target x86_64-pc-windows-msvc` 才报。

**依赖与锁文件**

- `crates/gitwave/Cargo.toml`：`rust-i18n.workspace = true` 被 `main.rs` 的宏使用；dev `serde_json = "1"` 被 `locales.rs` 使用。
- `rust-i18n` 4.2.4 在 `main` 上已由 `gpui-component` 引入。
- `cargo build --locked -p gitwave`：Finished。

**门禁**

- `make check` 经注入 `--locked` 的 cargo 包装脚本运行：`exit=0`。`cargo fmt --all -- --check` 无差异；clippy `Finished`；`gitwave` 4 passed；`gitwave-core` 394 passed、0 failed、2 ignored。
- `cargo clippy --locked --workspace --all-targets -- -D warnings -W clippy::all`：警告与错误 0 行。
- `make -n check`、`make -n build`、`make help`：解析正确。
- `cargo tree --locked -e features | grep -c 'git2 feature "https"'` → 1；按 `ci.yml` 的脚本在本机找到 `#define GIT_SECURE_TRANSPORT 1`。

**删除完整性**

- 排除历史记录目录后，在 183 个文件里按 38 个关键词搜索旧路径与旧工具链，命中项都属于：已标注的旧实现对照小节、ADR 的废弃记录、调研快照、说明「已移除」的文字、合法的同名内容（`health.rs` 里的 `"node_modules"`、`Cargo.lock` 里的 `tauri-winrt-notification`），以及上面列出的几处。
- 17 个重命名图标的 blob 哈希与 `main` 逐一相同；`src-tauri/icons/` 下未带走的只有 `android/`（17 个）与 `ios/`（18 个）。
- `site/index.html` 的本地引用只有 `/favicon-32.png`、`/icon.png`，都存在。
- `pages.yml` 相对 `main` 无改动，只依赖 `site/`，与 `ci.yml` 的 `paths-ignore` 互补。
- 非历史文档的相对链接检查：116 处有效，0 处断开。

**文档里的数字与名称**

| 陈述 | 实测 |
|---|---|
| 每种语言 1108 条文案 | 1108 / 1108 |
| 占位符 343 处（每种语言） | 686 / 2 = 343 |
| 9 组 `_one` / `_other`，每种语言 18 个 | 9 对 |
| 单元 84、集成 306、`host` 3、守卫 3、界面 4 | `--list` 分类：domain 29 + application 55 = 84；infrastructure 306；host 3；lib.rs 3；gitwave 4 |
| `cmds.*` 10 个错误码无引用 | 10 个常量，`cmds::` 0 处引用 |
| core 约 2.4 万行 | 24293 |
| v0.11.0 后端约 2.6 万行 | tag `v0.11.0` 的 `src-tauri/src`：69 个文件，25948 行 |
| 前端约 2.45 万行，测试 5.9 千行 | 24531 行；5925 行（61 个文件） |
| 日志初始化没有调用方 | 0 处；旧调用方在 `main:src-tauri/src/lib.rs:1667` |
| 「应用设置只有代理一项」 | `app_settings.rs` 只有 `ProxyMode`、`ProxySettings` |
| `#![allow(dead_code)]`、`#![allow(unused_imports)]` | `lib.rs` |
| keyring 4.x 改了 feature 名 | 3.6.3：`apple-native`、`windows-native`、`sync-secret-service` 等；4.0.0：无；4.2.0：`cli`、`default`、`v1` |
| Go 分支托管 CI 只运行一次、三端全败 | `go-desktop.yml` 共 1 次运行，3 个 job 都是 failure |
| 架构总览列出的类型、模块、函数 | `Workspace`、`RepoRef`、`FileChange`、`BranchInfo`、`StashEntry`、`clone_repo`、`generate_commit_message`、`repo_adapter::init` / `clone_remote`、`SqliteWorkspaceRepo`、`SqliteAppSettingsRepo`、`ai::provider` / `secrets`、`AppContext` 全部存在 |
| 「domain 层只依赖 serde」 | domain 下的 `use` 只有 serde 与 crate 内 |
| 「GPUI 不依赖 tokio」 | `cargo tree --locked -p gitwave -i tokio` → nothing to print |
| README 的 Linux 依赖清单 | 16 个包与 `ci.yml` 的 apt 列表逐项一致 |
| 「一个测试会启动 `git --version`」 | `process.rs` |
| 「运行时调用系统 `git`」 | `credentials.rs`；`lfs.rs` |
| 迁移脚本截至 0004 | 4 个 SQL 文件 |

**CI 与仓库外围**

- `ci.yml`、`pages.yml`、`dependabot.yml`、`.pre-commit-config.yaml`、`.vscode/extensions.json`、`SKILL.md` 的 frontmatter：都能解析。
- `ci.yml` 的 6 个 `run` 块：`bash -n` 全部通过。
- 徽章：无。旧工作流名只出现在历史记录目录、`docs/pm/features/F009-auto-update.md`、工程总览的说明文字和 PR 模板的注释里。
- GitHub 只读查询：`main` 没有经典分支保护（HTTP 404）；规则集 `Main` 只有 `deletion`、`non_fast_forward`，没有必需的状态检查；基线 `8466d55` 在 `main` 上三个旧工作流都是 success。

### 没有验证的

- **三端托管 CI 没有跑过。** 本分支当时还没有 PR；本机只有 macOS arm64。
- **Linux 依赖清单**：包名相对 `main` 没有变化，基线在三端是绿的，这是间接证据。`ci.yml` 注释说它「镜像 GPUI Kit 的 `script/install-linux.sh`」，但发布到 crates.io 的包里没有 `script/` 目录，无法核对。
- **`pre-commit` 钩子没有实际运行**：本机没有安装。`actionlint` 也没有运行。
- **bash 5 下 `cd ""` 的行为**：本机只有 bash 3.2。
- **「367 个前端测试」无法复现**：Node 工具链已删除。依据是 M1 的执行记录加上粗数（331 处 `it(` 加 4 处 `it.each`），量级吻合。
- **ADR 0007 里「2026-11-23 至 25 日」的起始日**：那个分支已删除，只能确认运行日期是 11-25。
- **「Dependabot 对 cargo 的更新在 `keyring` 上失败」**：这是历史事件，没有查；只核实了 feature 名确实变了。
- **`gpui-kit` 要求 Rust 1.92**：来自其官方文档，crate 本身没有声明 `rust-version`。
- **`plan.md` 的「基线 `004c9f4`（v0.11.0）」**：tag `v0.11.0` 实际指向 `04e8b6c`，`004c9f4` 在它之后 37 个提交。这半句在 `main` 上就有。
- **审查中的假设与偏离**：
  - 历史记录目录（`docs/tasks/`、`docs/design/`、`docs/pm/`、`docs/tech/planning/`、`.specify/`、`.claude/`）按任务说明排除在残留检查之外。
  - `Makefile` 里的 cargo 命令不带 `--locked`，所以 `make check` 是通过注入 `--locked` 的包装脚本运行的。
  - 有一条命令没有带 `--locked`：`cargo test --workspace --all-targets --no-run`，是创建包装脚本时出错导致的。事后核对 `Cargo.lock` 相对 `main` 仍只多那两行。
  - 任务说不需要联网；审查者做了上面列出的 GitHub 只读查询，都是 GET，没有改变任何状态。

---

## 处理结果：M1b

记录人：主会话（实现方），2026-11-25。🟡 八条全部处理；🟢 逐条给出结论；最后是处理过程中实现方自己另外发现并更正的几处，以及修复后的验证。

### 🟡 一般问题

| # | 问题 | 处理 | 状态 |
|---|---|---|---|
| 1 | `proxy.rs` 的注释多了一行重复 | 删掉重复的那一行。它是实现方上一轮编辑留下的 | 已修复 |
| 2 | 两处把「尚未接线」写成了现状 | README 与技术选型总览改成「设计上在进程内调用，新应用尚未接入」；`use_cases.rs` 里 `AppContext` 的注释、`cmds.rs` 的模块注释同样改成设计约定的语气；README 的技术栈一行写明窗口壳还不读取这些文案 | 已修复 |
| 3 | `bump-version.sh` 在非 git 目录不中止，不校验所在段，没有自检 | 重写。依次确认在仓库内、`cargo` 可用、`Cargo.lock` 与清单一致；只在 `[workspace.package]` 段内定位版本行；改完读回两个文件，锁文件里除版本行外有任何变化就还原并退出非零；多余参数被拒绝；临时目录建在仓库外并自动清理 | 已修复 |
| 4 | 带前导零的版本被写入后才失败，且不回滚 | 正则拒绝前导零；改写前备份，之后任何一步失败都把 `Cargo.toml` 与 `Cargo.lock` 放回原样 | 已修复 |
| 5 | 「只改版本号」有前提却不检查 | 改动前先跑 `cargo metadata --locked`，起点不干净就中止；改动后比对锁文件，只允许版本行从旧值变成新值 | 已修复 |
| 6 | `merge` 对跨文件重复键静默覆盖；没有 `errors` 键的文件被跳过 | 两处都改成断言并报出文件与键 | 已修复 |
| 7 | 改了语言文件不触发重新编译；测试 3 写死期望值 | 新增 `crates/gitwave/build.rs`，把语言文件的摘要作为给 rustc 的环境变量放进 crate 的指纹；测试改为把每条文案经 `t!` 读回与文件比对 | 已修复 |
| 8 | MSRV 提到 1.95 的决策没有记录，且与 `windows-registry` 的注释矛盾 | 从根上处理，见下 | 已修复；Windows 上的编译由 PR #245 的 CI 验证通过 |

第 3 到 5 条的验证：重写后的脚本在 `/tmp` 的一次性仓库里跑了 26 次。正常升级只改 `Cargo.toml` 一行与 `Cargo.lock` 里两行版本；同一版本重跑报告「已经在该版本」且不动文件；`v` 前缀与预发布号可用；9 种非法调用（前导零三种、`abc`、`1.2.3.4`、带 `+build`、空串、参数过多、无参数）都在动文件之前被拒；在无关项目的目录里运行时，那个项目的 `Cargo.toml` 原样未动；`[workspace.package]` 之前另有一行 `version` 时改的仍是正确的那一行；锁文件落后于清单时拒绝执行；`PATH` 里没有 `cargo`、清单的版本行带行尾注释、没有空格、用单引号、以 CRLF 结尾，这五种情况都拒绝且不改文件；用一个假的 `cargo` 模拟了「解锁文件时多写了一行」「那一步失败」「没有刷新锁文件」，三种都还原了两个文件并以非零退出。macOS 自带的 bash 3.2 下全部可运行。

第 6、7 条的验证：在隔离副本上重做了变异实验，基线 5 个测试全过。

| 变异 | 结果 |
|---|---|
| 同一个错误键出现在两个 `errors-*.json` 里 | 抓到：`two message files define errors.git.raw` |
| 新增未列入 `LANGUAGES` 的 `app.ja.json` | 抓到 |
| 非 errors 文件里出现 `errors` 树 | 抓到：`app.en.json carries an errors tree` |
| 占位符名字带空格 `%{ detail }` | 抓到 |
| 占位符没有闭合 | 抓到 |
| 源码注释里出现 `pub const …` | 不再误报，5 个全过 |
| `errors-*` 文件丢了顶层 `errors` 键 | 抓到 |
| 只改 JSON、二进制没有重新编译 | 抓到（副本里去掉了 `build.rs`） |
| zh-CN 的某条文案变成数字 | 抓到 |

重编译本身在真实仓库里验证：只改 `app.en.json`、不碰 `main.rs`，`cargo test` 重新编译了 `gitwave`，而那条逐条比对的测试通过，说明内嵌值已更新。只 `touch` 而内容不变时也会重新编译一次，这是 cargo 在 build script 重跑后的固有行为，代价是这个很小的 crate 多编译一次。

第 8 条的处理。读了 registry 里 `windows-registry` 0.6.1 与 0.100.0 的源码：调用处（`proxy.rs` 探测系统代理）用到的 `CURRENT_USER`、`Key::open`、`get_u32`、`get_string` 在两个版本里签名相同，`open`、`get_u32`、`get_string` 的实现没有变化，0.100.0 只有新增（`get_bytes`、WOW64 选项等）和随新版 `windows` crate 改名的类型别名。于是把 `gitwave-core` 在 Windows 上的直接依赖从 `"0.100"` 改成 `"0.6"`：改后 `gitwave-core`、`hyper-util`、`gpui-pre-windows` 三者共用 0.6.1，锁文件少了 `windows-link`、`windows-registry` 0.100.0、`windows-result` 0.100.0、`windows-strings` 0.100.0 四个包，全图声明最高的最低版本回到 1.92（`oo7` 0.6.0，GPUI Kit 的 Linux 平台层带进来的），workspace 的 `rust-version` 保持 `1.92`。那句注释改为事实。本机无法编译 Windows 目标，编译结果由本分支的 Windows CI 给出；失败则退回 0.100 与 1.95。

### 🟢 优化建议

| 建议 | 处理 | 状态 |
|---|---|---|
| 占位符出现次数不同 | 没有改。同一条文案里重复提到同一个名字是合法的，沿袭自 i18next 版的测试比较的也是名字的集合 | 不改 |
| 新增语种未列入 `LANGUAGES` | 新增一条测试：磁盘上的语种必须与 `LANGUAGES` 完全一致 | 已修复 |
| 非 errors 域文件里的孤儿 `errors.*` | `errors()` 遍历所有域名：只有 `errors-*` 文件可以带 `errors` 树，而且必须带 | 已修复 |
| `%{ detail }` 与未闭合的 `%{` | 占位符名只能是字母、数字、下划线，且必须闭合 | 已修复 |
| 注释里的 `pub const` 被当成常量 | 只认以 `pub const ` 开头的行 | 已修复 |
| `locales.rs` 与 README 把未接线的文案写成现状 | 改为「要由代码选择」「窗口壳还不读取」 | 已修复 |
| `main.rs` 关于 `t!` 的注释不精确 | 改为「`t!` 转发到这次调用在 crate 根定义的宏」 | 已修复 |
| `remote.rs` 的 "Empty when constructed; always stamped before emit." | 删掉这句与事实不符的话 | 已修复 |
| `use_cases.rs`、`cmds.rs` 的时态 | 见 🟡 第 2 条 | 已修复 |
| `proxy.rs`、`remote.rs` 提到 updater 插件 | 模块注释改为只列现存的三条出站路径；另一处改为 "only the reqwest clients honored them" | 已修复 |
| PR 模板注释里的 "lint / test / build" | 改为 "CI green" | 已修复 |
| 工程总览的 `*.md` 与 TLS 断言条数；计划里「三端 CI 上运行」的说法 | 改为 `**.md`、两条断言；计划写明 rustfmt 只在 Ubuntu 上检查 | 已修复 |
| 「无头窗口测试随 M2 起」但 M2 的定义里没有这一项 | 写进 M2 的实施内容 | 已修复 |
| `.gitattributes` 里的 `package-lock.json` 等模板残留 | 没有改。这几行是通用模板，从来不是本项目专用的；只删掉了指向本项目已删除文件的 `pnpm-lock` 两行 | 不改 |
| `bump-version.sh` 忽略多余参数、可能残留 `.tmp` | 见 🟡 第 3 条 | 已修复 |
| pre-commit 的 clippy 没带 `--locked` | 加上，与 CI 一致 | 已修复 |
| dependabot 对 `keyring` 的 ignore 目前不起作用 | 配置保留。计划里「Dependabot 在 `keyring` 上失败」这句在 GitHub 上找不到任何证据（没有相关的 PR 或运行），改写成能核实的说法：4.x 改了 feature 名，当前清单要求 `3`，这条忽略只是声明意图 | 已修改 |
| 合并后关闭 4 个 npm 的 dependabot PR | 合并后先看 Dependabot 是否自动关闭 | 待办 |
| `branches.irebase.*` 的拼写 | 写进计划的 M2 设计输入。核对过：新旧文件里各 20 个这样的键，集合相同 | 已记录 |

### 处理过程中另外发现并更正的

- `ci.yml` 的注释把 `libxcb-shape0-dev` 与 `libxcb-xfixes0-dev` 归为「Zed 的 Linux 构建清单」。核对上游：Zed 的 `script/linux` 里没有这两个包；锁文件里 860 个 crate 没有一个提到 xcb 的 shape 或 xfixes。它们是 M1 审查建议补上的，当时写明是推测。去掉这两个包，注释改为与 GPUI Kit 的 [`install-linux.sh`](https://github.com/longbridge/gpui-kit/blob/main/script/install-linux.sh) 逐项对应并附上链接，README 的清单同步。PR #245 的 Ubuntu 任务在没有它们的环境里装依赖、编译并链接通过。
- 发布技能里「README and the site describe v0.11.0 and stay as they are」与重写后的 README 不符，已改。
- 计划状态行「基线 `004c9f4`（v0.11.0）」：`v0.11.0` 标签指向 `04e8b6c`，`004c9f4` 在其后 37 个提交。改为「v0.11.0 发布后的 `main`」。
- 「Rust 1.92 受 GPUI Kit 约束」这句话，实现方一度因为找不到依据而删掉。核对上游后确认依据存在，只是在依赖树里而不在文档里：GPUI Kit 自己不声明最低版本，仓库里也没有工具链文件，它依赖树里声明最高的是 Linux 平台层带进来的 `oo7` 0.6.0（1.92）。工程约定按这个说法重写。

### 对「没有验证的」的回应

- 三端托管 CI、Linux 依赖清单是否够用：PR #245 三端全部通过，结果回写到了 plan 的执行记录。
- Linux 清单注释引用的上游脚本：已核对并附链接。
- `pre-commit`、`actionlint` 没有运行：实现方这边同样没有安装，没有擅自安装。`make check` 跑的是钩子里那两条命令。
- bash 5 下 `cd ""`：脚本现在显式检查 `git rev-parse` 的结果，这个问题不再存在。
- ADR 0007 的起始日 11-23：依据是本地归档标签 `archive/nsgo-native-ui` 里的 `7991e57`（2026-11-23，NSGo history workbench preview）和 `74d42aa`（2026-11-25，迁移提交）。
- `gpui-kit` 要求 Rust 1.92：上游没有这样的声明，见上一节最后一条。
- 审查过程中那次没带 `--locked` 的命令：知悉。它没有改动 `Cargo.lock`；这之后锁文件又被有意修改过一次，见 🟡 第 8 条。

### 修复后的验证

`make check` 通过：`cargo fmt --all -- --check` 无差异，clippy 没有输出，`gitwave` 5 个测试，`gitwave-core` 394 个通过、2 个忽略。改动过的 12 份 Markdown 里 61 处相对链接与锚点全部有效。`ci.yml`、`dependabot.yml`、`.pre-commit-config.yaml` 可解析。`bump-version.sh` 通过 `bash -n`。应用构建并启动 8 秒无崩溃。

---

# 代码审查：M2 切片二（语言切换、文案、日志）

> **作者**：AI code-reviewer 子代理，未经人工复核
> **状态**：草案
> **最近更新**：2026-11-26

| 项目 | 内容 |
|---|---|
| 分支 | `feature/gpui-kit-app-shell` |
| 基线 | `37e9059`（改动全部未提交：27 个已跟踪文件修改，1 个新文件 `crates/gitwave/src/logging.rs`；`1017 insertions(+), 109 deletions(-)`） |
| 环境 | rustc 1.100.0 (0ef1bc386 2026-11-09)、cargo 1.100.0 (3acc79c0c 2026-11-05)、macOS 27.2.1 (26B131)、arm64 |
| 审查时间 | 2026-11-26 |
| 审查方式 | 只读。仓库内只运行了 fmt、clippy 和测试编译。变异与探针实验都在 `/tmp` 的副本里做，副本已删除。下文路径相对仓库根；第三方源码路径相对 `~/.cargo/registry/src/rsproxy.cn-e3de039b2554c837/`。 |

下面「验证记录」之前是审查者的报告原文，行号对应审查时的工作区；之后是实现方的处理结果。

## ✅ 优点

- **core 的语言设置与代理设置写法一致。** `get_language_settings` 与 `get_proxy_settings` 的回落逻辑相同，`set_language_settings` 沿用 `protocol_with` / `unknown_with` 的构造方式。16 个针对规则的变异里 15 个被测试发现（见验证记录）。
- **同步保存的实现干净。** `locales.rs:199-211` 先改全局状态，再在同一帧的 effect flush 之前写库，没有后台任务或排队的残留。写库失败只记日志，界面保持新语言。
- **语言锁解决的是真实问题。** 副本里去掉 7 处锁后，100 次运行失败 41 次；原样运行 60 次全部通过。
- **文案改动面克制。** 脚本比对确认旧内容零改动，只有 `menu.*.json` 各一行加了行尾逗号。两种语言各新增 19 条，叶子键 1127 对 1127，占位符一致，无重复键。
- **菜单重建链路实测可用。** 切换语言后不手工重建，`cx.get_menus()` 与 `GlobalState::app_menus()` 的菜单名都变成 `["GitWave", "编辑", "显示", "窗口"]`。
- **文档里的数字大多能复现。** 测试数 16 / 400 / 2、文件 25 个、文案 1127 条、复数 9 组、文件名 `app.log.YYYY-MM-DD`、两线程运行时都与代码一致。没有把「后台保存」写成现状的地方；`plan.md:135` 是过程记录。
- **没有为了测试进入正式代码的东西。** `lock_language` 与 `LANGUAGE_LOCK` 都在 `#[cfg(test)]` 下。

## 🔴 严重问题（必须修复）

- **位置**：`crates/gitwave/src/workbench.rs:177-179`（新增的 `SwitchLanguage` 监听器），以及同一元素上的 `164-185` 行；`crates/gitwave/src/menus.rs:112-121, 147`。
- **描述**：所有应用级动作都挂在 `div#workbench` 上。设置对话框打开后焦点在对话框里（`gpui-component-0.11.0/src/root.rs:409`），对话框渲染在 Root 的 `dialog_layer`（`root.rs:245`），是工作台的兄弟分支。gpui 的 `Window::dispatch_action` 从当前焦点向上派发（`gpui-pre-0.3.14/src/window.rs:2915-2928`）。macOS 菜单动作走的是同一条路径（`gpui-pre-0.3.14/src/platform/app_menu.rs:351-357`，`app.rs:3049-3055`）。
  后果是设置或关于对话框打开时，菜单栏的「显示 › 语言」不会生效。`Quit`、`Hide`、`Minimize` 等挂在同一元素上，按同样机制也到不了，所以 ⌘Q 很可能也失效；这一点是推断，没有逐个验证。
  根因在切片一的挂载位置，本切片新增的动作沿用了它。文档写的「入口在「显示 › 语言」菜单」（`docs/tech/tech-selection/00-overview.md:28`）在这个状态下不成立，现有测试也测不到。
- **建议**：用 `App::on_action`（`app.rs:2858`，不依赖焦点）注册应用级动作，并补一个「对话框打开时动作仍生效」的测试。合并前在实机上确认一次。
- **核实**：副本里加了探针测试。没有对话框时 `SwitchLanguage` 与 `SwitchThemeMode` 都生效；设置对话框打开时两个都不改变状态。把 `SwitchLanguage` 改成 `App::on_action` 后，对话框打开时语言切换生效，原有 16 个测试仍通过。真实 macOS 菜单栏没有验证（受「不启动应用」约束）。

## 🟡 一般问题（建议修复）

**Y1 文档说组件库文字只有英文，与事实相反**
- **位置**：`docs/tech/tech-selection/00-overview.md:30`。
- **描述**：gpui-component 0.11.0 自带 8 种语言，zh-CN 的 122 条全覆盖。它与 `gitwave` 共用 rust-i18n 的进程级语言，查找宏读的是 `rust_i18n::locale()`（`rust-i18n-macro-4.2.4/src/tr.rs:415`），所以内置文字会跟着切换到简体中文。
- **建议**：改成「组件库自带多语言，随 GitWave 的语言切换」，并在实机上抽查一个带内置文字的组件。
- **核实**：副本里调用 `gpui_component::_rust_i18n_available_locales()` 得到 `["en", "it", "ja", "pt-BR", "ru", "zh-CN", "zh-HK", "zh-TW"]`；`Dialog.cancel` 在 zh-CN 下是「取消」。`locales/ui.yml` 逐语言计数：en / ja / pt-BR / ru / zh-CN / zh-HK 各 122，zh-TW 85，it 46。

**Y2 「切换后菜单重建、视图重绘」没有测试**
- **位置**：`crates/gitwave/src/workbench_tests.rs:139-187`；`menus.rs:50-61`、`workbench.rs:70-71`。
- **描述**：菜单测试在切换后直接调用 `menus::build_menus(cx)`，绕开了观察者。副本里删掉菜单的语言观察者、工作台的语言观察者、`update()` 里的 `cx.set_menus` 与标题栏 `reload()`，16 个测试都照常通过。实现方也写明没有人工验证系统菜单栏，所以这条链路目前没有任何一层验证。
- **建议**：切换后断言 `cx.get_menus()` 和 `GlobalState::global(cx).app_menus()`。测试平台会保存 `set_menus` 的内容（`gpui-pre-0.3.14/src/platform/test/platform.rs:754-760`）。
- **核实**：变异实验与上面的探针。探针用的正是这两个读数。

**Y3 大部分标签的翻译没有被测试钉住**
- **位置**：`crates/gitwave/src/menus.rs:90-156`；`workbench.rs:96-136, 237-241`。
- **描述**：把下列标签改回硬编码英文，测试全部通过：编辑与窗口两个菜单名、「设置…」、最小化与缩放、浅色与深色、设置对话框标题、关闭按钮、关于对话框标题。测试只断言了 View 菜单名、语言子菜单和单选项。
- **建议**：加一个测试，在两种语言下遍历 `build_menus(cx)` 的全部菜单与菜单项，与预期文案比对。
- **核实**：8 个变异全部存活（见验证记录）。

**Y4 core 的 ASCII 约束没有被测试独立验证**
- **位置**：`crates/gitwave-core/src/application/use_cases.rs:3321-3340`；规则在 `crates/gitwave-core/src/domain/app_settings.rs:67-78`。
- **描述**：把 `is_ascii_alphanumeric()` 改成 `is_alphanumeric()` 后，core 的 400 个测试全部通过。拒绝列表里的 `"中文"` 是被「首段以 ASCII 字母开头」拒绝的，不是被 ASCII 约束拒绝的。
- **建议**：在拒绝列表里加 `"en-中文"` 或 `"zh-é"`。
- **核实**：变异实验。另加探针测试确认：原规则下这四个输入都被拒；变异规则下 `normalize_language_tag("en-中文")` 返回 `Ok(Some("en-中文"))`。

**Y5 两个测试在锁外读界面文字**
- **位置**：`crates/gitwave/src/workbench_tests.rs:90, 219`；`docs/tech/engineering/00-overview.md:31`。
- **描述**：`#[gpui_kit::test]` 在测试体返回后会再 `run_until_parked()`（`gpui-pre-macros-0.3.14/src/test.rs:262-265`）。两个打开设置对话框的测试此时窗口仍是 dirty，会再画一帧，对话框构造闭包在锁释放后读 `t!`。目前无害，读到的值不参与断言；但文档的「整段都要持有」对这类测试无法字面成立。
- **建议**：把 guard 存进 `TestAppContext` 的全局状态，让它在 `drop(cx)` 时才释放（未验证）。或在工程文档里写明这个边界。
- **核实**：给每个读写入口加「必须持锁」断言，150 次运行每次触发 2 次。`RUST_BACKTRACE=1` 显示调用栈是 `run_test`、`run_until_parked`、`flush_effects`、`Window::draw`、`dialog_layer`、`open_settings::{closure#0}`。

**Y6 占位符数量过时**
- **位置**：`docs/tech/tech-selection/00-overview.md:31`，写的是「343 处」。
- **描述**：新增的两条错误文案各带一个占位符，现在是 345。
- **建议**：改成 345。
- **核实**：脚本统计 en 与 zh-CN 各 345。

**Y7 代码注释与新约定矛盾**
- **位置**：`crates/gitwave/src/runtime.rs:23-26`。
- **描述**：注释写「Use it for anything that touches a repository or the database, so the main thread never waits on it」。`docs/tech/architecture/00-overview.md:58` 的新约定是保存设置在主线程同步做。该文件不在本次改动里，约定变了但注释没同步。
- **建议**：把注释改成「耗时的或时长不可预估的仓库、数据库操作」，并指向架构文档的约定。

**Y8 设置对话框的语言单选组没有标题**
- **位置**：`crates/gitwave/src/workbench.rs:117-120, 248-265`。
- **描述**：对话框里只有三个单选项，没有说明它们是「界面语言」。`RadioGroup` 不支持 `aria_label`，读屏器也听不到组名。
- **建议**：在组上方加可见标题，复用已有的 `settings.general.uiLanguage`（两种语言都已存在，目前没有代码引用）。
- **核实**：`grep -rn "uiLanguage" crates/gitwave/src` 无结果；`gpui-component-0.11.0/src/radio.rs:290-371` 的 `RadioGroup` 没有实现 `InteractiveElement`。

**Y9 正常退出时不保证刷新日志**
- **位置**：`crates/gitwave/src/logging.rs:9-21`。
- **描述**：guard 存成全局状态足以让它活到应用结束，但 macOS 上退出走 `[NSApp terminate:]`（`gpui-pre-macos-0.3.14/src/platform.rs:581-598`），不保证运行析构。`WorkerGuard::drop` 负责发关闭信号并等待写出（`tracing-appender-0.2.5/src/non_blocking.rs:282-301`），不运行的话退出瞬间缓冲里的日志可能丢失。这是源码推断。
- **建议**：用 `cx.on_app_quit`（`app.rs:2949`）在退出前 flush，并在实机上确认退出时的日志能落盘。
- **核实**：只读了源码，没有运行验证。

**Y10 文档措辞**
- **位置**：`docs/tasks/refactor-gpui-kit-desktop/plan.md:129`；`docs/tech/architecture/00-overview.md:58`；`docs/tech/tech-selection/00-overview.md:28`。
- **描述**：
  - `plan.md:129` 写「一到八个字母或数字」，漏了 ASCII。
  - 架构文档把「只有耗时操作才放后台」这层边界一并归为「用户 2026-11-25 定下的」。用户原话定的是同步做、界面等待；判据是实现方补的。
  - 两处都写「其余落到英文」，省略了「按偏好顺序取第一个支持的」。`["fr", "zh-Hans-CN"]` 的结果是 zh-CN。
  - 约定没写保存失败时的行为：界面已切换，只记日志，下次启动回到旧值。
- **建议**：逐处补全。

## 🟢 优化建议（可选）

- **O1 重复选择也会重写。** `locales.rs:199` 对「选择没变」没有短路。菜单点击已勾选项会重写设置并重建菜单、重绘窗口。对话框的单选不受影响，因为基座只给未选中项挂点击处理（`gpui-base-0.11.0/src/radio.rs:230`）。
- **O2 注释的因果不成立。** `workbench.rs:246-247` 说 id 是序号「也是」辅助技术得知第几项的方式。实际上这来自 `position_in_set` 与 `size_of_set`（`gpui-component-0.11.0/src/radio.rs:419-420`），与 id 无关。无障碍语义本身是完整的。
- **O3 冗余调用。** `workbench.rs:257` 的 `.accessibility_label(name.to_string())` 可删，组件默认用 label 文本（`radio.rs:188-191`）。
- **O4 `set_language_settings` 的细节。** `use_cases.rs:435` 的变量名 `refused` 指的其实是待校验的输入。最终英文消息是「invalid language tag: not a language tag: …」，有重复。`LANGUAGE_SERIALIZE_FAILED` 的分支不可达，保留只是为了与代理设置对称。
- **O5 `logging.rs` 与 `tracing.rs`。**
  - `logging.rs:4` 导入的模块名 `tracing` 遮蔽了同名 crate，这个文件里写不了 `tracing::warn!`。
  - `logging.rs` 没有测试，把 guard 改成函数返回即丢弃也不会被发现。
  - `try_init()` 失败被忽略、重复调用仍返回新 guard，都是既有行为，非本次引入。
  - `init_is_idempotent` 调用真实的 `init()`，本地或 CI 跑 `cargo test` 会写用户真实的日志目录。这也是既有行为，本次改了这个模块但没有处理。
- **O6 回退语言有两处定义。** `locales.rs:53` 的 `FALLBACK` 与 `main.rs:17` 的 `fallback = "en"`。
- **O7 `LanguageChoice::Fixed(&'static str)` 允许非法值。** 例如 `Fixed("ja")` 会让 `name_of` panic。改成携带 `Language` 更稳。
- **O8 文档可补充的事实。**
  - 「跟随系统」只在启动时读一次（`locales.rs:94-96, 127-129`）。
  - 日志目录建不出来时回落到 stderr（`tracing.rs:28-31, 54-60`）。
  - `docs/tech/architecture/00-overview.md:63` 的路径少了 `infrastructure::`。
- **O9 旧语言偏好不会迁移。** 旧实现存在 `localStorage["gitwave.language"]`（`git show 870e6ff~1:src/i18n/index.ts`）。建议在迁移计划里写明迁移或不迁移。
- **O10 英文大小写不一致。** `common.en.json:37` 是 "Follow System"，既有的 `settings.en.json:47` 是 "Follow system"。
- **O11 约定在磁盘库下的前提。** `app_settings` 连接是 WAL、5 秒 `busy_timeout`（`crates/gitwave-core/src/infrastructure/persistence/sqlite.rs:48-55`）。切到磁盘库后建议实测一次写入耗时，再决定约定是否要加时长上限。
- **O12 键盘可达性。** `RadioGroup` 里每个单选项都是 Tab 停靠点，这是上游设计。
- **O13 `review.md` 已存在。** `docs/tasks/refactor-gpui-kit-desktop/review.md` 是 M1b 的已提交报告，放入本报告前要决定追加还是替换。

## 📝 总体评价

实现干净、与既有模式一致，门禁全过，文档数字基本可信。最需要优先处理的是 R1：对话框打开时应用级动作到不了工作台，修法已在副本里验证可行。其次是 Y2 与 Y3 暴露的测试缺口：这个切片最核心的可见行为（切换后菜单、视图、各处标签真的变了）大多没有测试守护。

## 验证记录

### 验证过的

**门禁**
1. 仓库内 `cargo fmt --all -- --check`：退出码 0。
2. 仓库内 `cargo clippy --locked --workspace --all-targets -- -D warnings`：退出码 0。
3. 仓库内 `cargo test --locked --workspace --all-targets --no-run`：退出码 0，`--locked` 未报错。
4. 直接运行编译好的两个测试二进制，`HOME` 指向临时目录：
   - `gitwave`：16 passed。
   - `gitwave-core`：400 passed、2 ignored（`commit_log_empty_repo`、`merge_empty_repo_errors`），9.77 秒。
5. 副本内 `HOME=<临时目录> make check CARGO="cargo --locked"`：退出码 0。fmt 无差异，clippy 0 条警告，测试 16 与 400+2，耗时 2 分 50 秒。
6. 稳定性：未改动的 `gitwave` 测试二进制连跑 60 次，60 次通过（15 核并行）。

**文案**
7. Python 脚本比对 `git show HEAD:` 与工作区：
   - 每个文件只有新增，无删除、无改动；行尾加逗号的只有 `menu.en.json` 与 `menu.zh-CN.json` 的 `"settings"` 行。
   - 新增键：common 3、errors-usecases 2、menu 14，两种语言各 19。
   - 叶子键 1127 / 1127，单边键 0，占位符不一致 0，重复键 0。
   - 占位符出现次数 345 / 345；`_one` 与 `_other` 各 9；文件 25 / 25。
8. 两个新错误码与文案双向对应，由既有测试 `every_error_code_has_a_message_and_every_error_message_matches_a_code` 覆盖，运行通过。

**测试干扰**
9. 副本里去掉 7 处锁后连跑 100 次：41 次失败。
   - `switching_the_appearance_updates_the_view_menu` 26 次。
   - `starting_and_switching_follow_the_stored_choice_and_the_system` 19 次。
   - `switching_the_language_relabels_the_menus_and_stores_the_choice` 2 次。
   - `every_message_loads_through_rust_i18n_as_written` 1 次。
10. 其余 9 个测试不依赖当前语言：7 个只读文件或调用纯函数；`runtime.rs` 的 2 个只调用 `gpui_kit::init`，而 `gpui_component::init`（`gpui-component-0.11.0/src/lib.rs:131-151`）不调用 `set_locale`。core 的测试在另一个测试二进制进程里，不涉及 rust-i18n。

**变异实验（副本，每次变异后跑全部测试）**
11. core，16 个变异，15 个被发现：首段字母开头、段数上下限、长度上下限、trim、空白跟随系统、下划线、空段、保存原值、存错键、全部拒绝、读取恒空、大小写、空白被拒。存活 1 个：ASCII 改成 Unicode 字母数字（Y4）。
12. `gitwave`，28 个变异，14 个被发现：完整标签比较、只看首选项、下划线、`showing` 大小写、忽略已存选择、不保存、跟随系统恒英文、顺序颠倒、恒选第 0 项、勾选恒假或恒真、不处理动作、不响应点击、启动不读已存选择。
13. 存活 14 个：不转小写（影响很小，系统语言通常已是小写）、Y2 的 4 个、Y3 的 8 个、日志 guard 不进全局状态（O5）。

**行为探针（副本）**
14. 菜单重建链路见「优点」。有一次探针读到标题栏菜单栏仍是英文，之后同一二进制与重编后的二进制共 286 次都读到中文，未能复现，原因不明。
15. 组件库翻译表见 Y1。
16. 对话框打开时动作可达性见 R1。
17. 点击单选后不手工绘制，标签已是 `["跟随系统", "English", "简体中文"]`，对话框会自行更新。
18. `App::global_mut` 只推入一个 effect（`app.rs:2662-2669`），观察者在 `flush_effects` 里调用（`app.rs:2245`、`2399`）。`Workbench::new` 里的观察者只调用 `cx.notify()`，渲染不改语言状态，没有循环。
19. `window.open_dialog` 的构造闭包签名第三个参数是 `&mut App`，每次渲染对话框层时调用（`root.rs:262`）。
20. `ElementSnapshot::label()` 读的是无障碍标签（`gpui-base-0.11.0/src/test_support.rs:124-127`）。

**依赖、日志、文档**
21. `git diff Cargo.lock` 只有 `gitwave` 依赖列表新增 `"sys-locale"` 与 `"tracing"` 两行，没有新包。两个依赖都被用到（`locales.rs:128` 的 `sys_locale::get_locales`，`locales.rs:175, 209` 的 `tracing::warn!`）。锁文件里依赖 `sys-locale` 的包是 `cosmic-text`、`gpui-base`、`gitwave`。
22. `sys-locale 0.3.2`：`get_locales()` 返回迭代器（`lib.rs:85`），macOS 读 `CFLocaleCopyPreferredLanguages`（`apple.rs:143`）。
23. `dirs 7.0.0` 在 macOS 上用 `$HOME/Library/Application Support`，不认 `XDG_DATA_HOME`（`mac.rs:7, 12`）。
24. `tracing::init` 的 `file_name` 是 `"app.log"`（`tracing.rs:65`），真实日志目录里的文件名也是 `app.log.YYYY-MM-DD`；`should_prune` 按 `app.` 前缀匹配，新文件名能被清理。
25. 测试数：
    - core 的 `#[test]` 属性：domain 29、application 61、infrastructure 306、`host.rs` 4、`lib.rs` 3；本次只新增 5 个。
    - 基线 `37e9059` 是 398 个属性，`main`（`870e6ff`）是 397。
    - `gitwave`：`locales.rs` 9、`workbench_tests.rs` 5、`runtime.rs` 2。
26. 文档逐条核对相符：README 第 51、57、59、75 行；`docs/tech/architecture/00-overview.md` 第 11-13、50、54、57、59、61、62 行；`docs/tech/engineering/00-overview.md` 第 23-25、51、93-104、120 行；`docs/tech/tech-selection/00-overview.md` 第 11、12、16、27、29、33、63 行；`plan.md` 第 3、38、48、71、81、85-93、127-136 行。旧实现确实用 `PredefinedMenuItem`（`git show 870e6ff~1:src/hooks/useNativeAppMenu.ts`）。

**收尾**
27. 仓库状态未变：HEAD `37e9059`，`git status --short` 28 行，diff stat 相同，暂存区为空，无 stash。真实日志目录仍是 18 个文件，mtime 与大小哈希不变。`~/.cargo` 下只有 `.global-cache` 被更新，没有新增或修改任何 crate 缓存。实验脚本留在 `/tmp/gitwave-review-scripts/`。

### 没有验证的

- **真实应用行为**（受「不启动应用」约束）：
  - macOS 系统菜单栏切换后是否换成中文。
  - R1 在真实菜单栏与 ⌘Q 上的表现；除 `SwitchLanguage`、`SwitchThemeMode` 外的动作是按同一元素推断的。
  - 退出时日志是否真的丢失（Y9 只读了源码）。
  - `plan.md:133` 所说的日志增长 1628 字节、单选的外观、中文输入法。
- **环境**：Windows 与 Linux（标题栏内菜单栏、`sys-locale` 的环境变量路径、非 mac 的退出路径）；三端 CI。
- **`make check` 的运行方式**：没有在仓库里原样运行，因为 Makefile 的 cargo 不带 `--locked`，且 core 的 `init_is_idempotent` 会写真实日志目录。
- **Y5 与 Y9 的修法**：只给了方向，没有在副本里验证。
- **文档里无法核对的事实**：用户原话的日期 2026-11-25；切片二的记录日期（部分文件修改于 11-26 00:01–00:02）。
- **审查依赖的假设**：测试平台上 `Window::dispatch_action` 的表现能代表 macOS 平台（R1 的关键假设，由 `app.rs:3049` 的调用链支撑，未实机确认）。

---

## 处理结果：M2 切片二

记录人：主会话（实现方），2026-11-26。🔴 一条、🟡 十条全部处理；🟢 十三条里十一条处理、一条转为待用户决定、一条不处理。每条写明实现方自己是怎样核实的。

| 项 | 处理 | 核实 |
|---|---|---|
| R1 对话框打开时动作到不了工作台 | 已修复。先写复现测试（失败），再把十个应用级动作从工作台元素移到 `App::on_action`。实现中踩到两处：`Context` 有一个同名的窗口级 `on_action`，把 `App` 的遮住了，要写成 `let app: &mut App = cx`；监听器是在「为派发动作而更新窗口」的过程中被调用的，不能再更新同一个窗口，所以窗口操作用 `cx.defer` 推迟到派发结束。测试因此按真实时序改写：按键的那次更新结束后才渲染 | 复现测试由失败转为通过，18 个测试全过。真实 macOS 菜单栏与 ⌘Q 仍未验证 |
| Y1 组件库文字只有英文（写反了） | 已更正。实现方当初只看到 `locales/` 下只有一个 `ui.yml`，没打开看内容 | 自己核实：`ui.yml` 每条文案带 8 种语言，简体中文 122 条 |
| Y2 菜单重建没有测试 | 已补 `both_sets_of_menus_follow_the_language_and_leave_nothing_in_english`：直接读 `cx.get_menus()` 与 `GlobalState::app_menus()`，不调用 `build_menus` | 反向验证：临时删掉菜单的语言观察者，只有这条新测试失败，其余照常通过 |
| Y3 标签翻译没有被钉住 | 同一条测试把两套菜单的每一项文字与写死的英文表、中文表比对，包括子菜单 | 同上。**没有覆盖的**：设置对话框的标题、关闭按钮、关于对话框的标题。对话框标题没有可供测试查找的 id，测试读不到，这几处只有代码审查 |
| Y4 ASCII 约束没有测试 | 拒绝列表里加了 `"en-中文"` 和 `"zh-é"` | 反向验证：把规则放宽成任意字母数字后测试失败，还原后通过 |
| Y5 测试在锁外读界面文字 | 已修。渲染窗口的测试改调 `locales::take_language_turn(cx)`，锁存进测试的应用，应用销毁才释放 | 探针：在对话框读语言的地方加「必须持锁」断言。新做法 25 次运行 0 次命中；把两个对话框测试改回旧做法，25 次里 14 次命中 |
| Y6 占位符数量 | 已改为 345，并写明其中 2 处来自新增文案 | — |
| Y7 `runtime.rs` 注释与约定矛盾 | 已改：只说耗时或时长不可预估的操作才用它，并指向架构总览的约定 | — |
| Y8 单选组没有标题 | 已加可见标题，用的是已有的 `settings.general.uiLanguage` | 测试仍通过。标题本身没有测试 |
| Y9 退出时不保证刷新日志 | 已实现：`on_app_quit` 时把写线程的 guard 丢掉，它会在停止前把缓冲写完 | 测试 `the_writer_lives_until_the_application_quits`；反向验证：「不保存 guard」与「退出时不丢弃」两处变异都让它失败。实机退出时是否落盘仍未验证 |
| Y10 文档措辞 | 四处逐条补全：ASCII；约定的归属（同步做是用户定的，「看耗时」是实现方补的）；按偏好顺序取第一个能对应的语言；保存失败时的行为 | — |
| O1 重复选择也重写 | 已加短路：选的是当前语言时直接返回，不存、不重绘 | 测试里观察者的通知次数不增加 |
| O2 注释的因果不成立 | 已改 | — |
| O3 冗余的 `accessibility_label` | 已删 | 删后测试仍能读到标签 |
| O4 命名与重复的消息 | 变量改名为 `requested`，消息不再重复。序列化失败的分支保留，与代理设置对称 | — |
| O5 日志模块的几处 | 导入改名为 `subscriber`，不再遮蔽 `tracing` 这个 crate；core 新增 `tracing::init_in(dir)`，日志可以写到指定的目录；core 的 `init_is_idempotent` 改用临时目录；`logging.rs` 有了测试 | 整套 `make check` 前后，对真实日志目录逐文件比对大小、修改时间与内容哈希，完全一致 |
| O6 回退语言定义了两处 | 加了一条测试：`main.rs` 里 `i18n!` 的回退语言必须与 `FALLBACK` 一致 | — |
| O7 `Fixed` 允许非法值 | 已改：`Fixed(&'static Language)` 只能引用确实提供的语言，会崩溃的 `name_of` 删除 | — |
| O8 可补充的事实 | 已补：启动时读一次系统语言；日志目录建不出来时回落到 stderr；路径补上 `infrastructure::`。另外自己发现并补上：日志文件名里的日期按 UTC 算（本地 11-26 凌晨 1 点的日志写进了 `app.log.2026-11-25`） | — |
| O9 旧语言偏好不迁移 | **没有处理，列为待用户决定**：旧实现把语言选择存在 WebView 的 localStorage 里，新应用读不到，v0.11.0 的用户要重新选一次 | — |
| O10 英文大小写 | 已改成 "Follow system"，与语言文件里已有的写法一致 | — |
| O11 约定在磁盘库下的前提 | **没有处理，列为待办**：换成磁盘数据库后实测一次写入耗时，再决定约定要不要加时长上限 | — |
| O12 单选项都是 Tab 停靠点 | 不处理，这是上游的设计 | — |
| O13 `review.md` 已存在 | 追加，不替换 | — |

审查者报告里有一处没有解释的观察（验证记录第 14 条）：一次探针里，切换后标题栏的菜单读到的是英文，之后 286 次都没有再现。实现方新增的菜单测试在 60 次运行里没有遇到。原因不明，已记入任务计划的实机验证项。

修复后的验证：`make check` 通过，零警告，`gitwave` 19 个测试、`gitwave-core` 400 个通过、2 个忽略；应用的测试二进制连跑 60 次全部通过；应用在 macOS 上启动并运行 7 秒，无输出，日志正常写出。改动过的 12 份 Markdown 里 64 处相对链接与锚点全部有效。仍然没有验证的：macOS 系统菜单栏、⌘Q、退出时日志落盘、设置对话框里单选的外观、中文输入法，以及 Windows、Linux 和三端 CI。

**2026-11-26 补记**：R1、Y9 和「修复后的验证」三处写着仍未验证的 macOS 实机项（系统菜单栏、⌘Q、退出时日志落盘、设置对话框里单选的外观、中文输入法），以及三端 CI，都已验证，五项都通过，结果见[任务计划](./plan.md)同日的实机验证记录。Y9 另有一个实测结论：退出前的日志确实落盘，但对照显示这不靠退出时的那段刷新；留着它，是为了不把结果交给调度。实机验证另发现一条，不在上表里：工作台根元素有 id、跟踪焦点，但没有 role，所以不生成无障碍节点。无障碍接口激活后，每次焦点回到工作台，gpui 都记一行 INFO 级别的日志；按 gpui 的说法，这时读屏器念的是整个窗口。已给它 `Role::Group` 并加测试，审查与处理见下一节。

---

# 代码审查：M2 实机验证后的无障碍修复（工作台根元素的 role）

> **作者**：AI code-reviewer 子代理，未经人工复核
> **状态**：草案
> **最近更新**：2026-11-26

| 项目 | 内容 |
|---|---|
| 分支 | `feature/gpui-kit-app-shell`（PR #246） |
| 基线 | `6dc6ee3`（改动全部未提交：5 个已跟踪文件修改，`42 insertions(+), 3 deletions(-)`） |
| 环境 | rustc 1.100.0 (0ef1bc386 2026-11-09)、cargo 1.100.0 (3acc79c0c 2026-11-05)、macOS 27.2.1 (26B131)、arm64 |
| 审查时间 | 2026-11-26 |
| 审查范围 | 仅工作区未提交的改动：`crates/gitwave/src/workbench.rs`、`workbench_tests.rs`、`docs/tasks/refactor-gpui-kit-desktop/plan.md` 与 `review.md`、`docs/tech/engineering/00-overview.md`。已提交并审过的部分没有重审 |
| 审查方式 | 只读。仓库内只运行了 fmt、clippy、测试和 `cargo build -p gitwave`，都带 `--locked`，只写 `target/`。没有做变异实验，想法写在「没有验证的」里。另外只读地统计了当天应用日志的行数与时间戳，没有输出日志内容。下文路径相对仓库根；第三方源码路径相对 `~/.cargo/registry/src/rsproxy.cn-e3de039b2554c837/` |

下面是审查者的报告原文，行号对应审查时的工作区。

## ✅ 优点

- **根因找对了，修在源头。** `div#workbench` 是本仓库唯一一处 `track_focus`（`workbench.rs:219`）。无障碍激活时 gpui 为它登记 focus（`gpui-pre-0.3.14/src/elements/div.rs:2417-2436`），节点不存在才走到 `note_focus_without_node`（`src/window/a11y.rs:244-255`）；节点不存在是因为 `a11y_role()` 返回 `None`（`src/element.rs:436-450`）。日志能对上：`app.log.2026-11-25` 到 18:15:42Z 共 109 行，其中 78 条都是 `it has an id but no role`。相邻两条的中位间隔 3.5 秒，没有小于 50 毫秒的，确实是每次 focus 变化记一条，不是每帧一条。
- **修复后的证据成立。** `target/debug/gitwave` 构建于 02:52:47（本地时间），晚于 `workbench.rs` 的最后修改 02:52:35；`cargo build -p gitwave --locked` 报告产物已是最新。日志里 18:52:48Z 那次启动在无障碍激活后，直到文件末尾都没有 `a11y:` 行。
- **`.test_support()` 没有把测试带进正式构建。** 没开 `test-support` 时它原样返回元素（`gpui-base-0.11.0/src/observe.rs:27-36`）。`cargo tree -e no-dev,features -i gpui-base` 里没有这个 feature。生产二进制里找不到观察模块的字符串（`test_support requires an existing ElementId`、`Registered paths` 等都是 0 处，测试二进制里都有）。同文件的 `status-bar`、`settings-note` 是先例，组件库自己的 dialog 宿主也是同样的链式顺序（`gpui-base-0.11.0/src/dialog.rs:558-574`）。
- **测试钉住的是充分条件。** gpui 只在 `a11y.is_active()` 时建树（`src/window.rs:4087-4090`），测试平台从不激活它（`src/platform.rs:1524` 的 `a11y_init` 是空的默认实现），所以测试没法直接断言「不再记日志」。它退一步断言 role 和 focus，而 `a11y.rs:244` 只看「拿到 focus 的元素有没有节点」，role 一在，结论就成立。`focused()` 依赖打开窗口时画的首帧（`src/app.rs:1665-1670`），这是 gpui 有意保证的行为。
- **本仓库没有同类遗漏。** `crates/gitwave/src` 里带 id 的元素只有三个：`workbench`、`settings-note`（`Role::TextInput`）、`status-bar`（`Role::Status`）。
- **「退出时的日志」那条记录的因果站得住。** `tracing-appender-0.2.5/src/worker.rs:33,44` 每取到一行就 `write_all`，`rolling.rs:227-235` 直接写 `File`，没有攒批。
- **文档里的数字都能复现。** 测试 20 个（`workbench_tests.rs` 8 个）、`gitwave-core` 400 个通过、2 个忽略，与本地一致；78 / 109 与日志一致。

## 🔴 严重问题（必须修复）

无。

## 🟡 一般问题（建议修复）

**Y1 状态行把没有提交、没有过 CI 的改动也算进了「三端 CI 通过」，「实机验证已做完」也说满了**
- **位置**：`docs/tasks/refactor-gpui-kit-desktop/plan.md:3`。
- **描述**：两点。
  - role 修复和新测试还在工作区，PR #246 的三端 CI 跑的是 `6dc6ee3`，不含它们。状态行紧接着写「切片一、切片二已提交，三端 CI 通过」，读起来是当前全部改动都过了 CI。
  - 新状态行去掉了「合并前还有需要实机验证的项」，改成「macOS 上的实机验证已做完」。M2 的退出标准（`plan.md:38`）是「菜单、快捷键、主题与语言切换可用；中文输入法……实测」。实机记录（154-167 行）里没有主题（浅色 / 深色）切换，快捷键只记了 ⌘Q。主题这一项从切片二的审查起就没进过实机清单（`plan.md:139,149`，`review.md:750-761`）。
- **建议**：状态行注明实机验证中发现的这一处修复尚未提交，以提交后的 CI 为准。主题切换做过就补进记录，没做就列进「仍然没有验证的」，状态行相应改成「除主题切换外已做完」。提交时同步 PR #246 的变更列表与测试计划，并把记录里的 `6dc6ee3` 换成修复所在的 commit。

**Y2 「焦点」这条设计输入现在是不完整的，照着写会再出同样的问题**
- **位置**：`plan.md:86`（本次没有改）；`docs/tech/engineering/00-overview.md:19-32` 的「测试策略」一节。
- **描述**：设计输入只写「视图要用 `.track_focus(..)` 并在打开时 `window.focus(&handle, cx)`」，没有提 role。gpui 要求 id 和 role 两者都有（`gpui-pre-0.3.14/src/_accessibility.rs:63-66`；日志原文也是 "Give it both an `.id(...)` and a `.role(...)`"，`src/window/a11y.rs:196-200`）。M3 起每个视图都走这条路，每漏一个 role，无障碍激活时就多一种警告。这是跨任务复用的约定，按 `AGENTS.md` 的归属规则该在 `docs/tech/`，现在只埋在执行记录里。
- **建议**：`plan.md:86` 补上「元素要有 id 和 role，容器用 `Role::Group`」。同时写明 `Role::GenericContainer` 不算：gpui 会把它过滤掉，调试构建下 `role()` 直接断言失败（`src/elements/div.rs:1357-1364`、`2007-2013`）。`docs/tech/engineering/00-overview.md` 加一条同样的约定。

**Y3 review.md 的补记有一处写成了事实，一处会过期，一处指代不准**
- **位置**：`docs/tasks/refactor-gpui-kit-desktop/review.md:800`。
- **描述**：
  - 「读屏器也只会念整个窗口」写成了事实。实机只用 System Events 读了树，没有用 VoiceOver 听过。`plan.md:160` 的写法是「按 gpui 的说法」，两处不一致。
  - 「gpui 都记一行警告」：这条日志的级别是 INFO（`a11y.rs:196`）。`plan.md:160` 的「INFO 警告」同样把级别和性质挤在一起，读着别扭。
  - 「这一处改动没有经过审查」在本报告并进来以后就不成立了。
  - 「上面两处『仍未验证』」：字面带「仍未验证」的是 771 行（R1：菜单栏与 ⌘Q）和 780 行（Y9：日志落盘），括号里的五项来自 798 行的「仍然没有验证的」。
- **建议**：改成「按 gpui 的说法，读屏器会念整个窗口」；两处统一写成「一行 INFO 级别的提示」；并入本报告时把最后一句改成指向本节；「上面两处」改成「R1、Y9 和『修复后的验证』三处」。

**Y4 实机记录没有说清「修复后的二进制」是哪份代码，无障碍还剩什么没验证也没有列**
- **位置**：`plan.md:154`、`160`、`167`。
- **描述**：
  - 记录开头写「`6dc6ee3` 的调试二进制」，而 `6dc6ee3` 就是 HEAD，不含 role 修复。「修复后」的那次用的是工作区代码构建的二进制。
  - 「仍然没有验证的」只列了 Windows、Linux 和菜单读数那一项。无障碍还有三处没有验证：VoiceOver 的实际朗读；Windows 上映射成的 UIA Group；Linux 上映射成的 AT-SPI Panel。`plan.md:51` 和工程文档第 30 行都把读屏器列为要实机验收的内容，用 System Events 读树不算。
  - 日志里有一次归属不明的启动：18:48:21Z 启动（激活在 18:48:26Z，本地 02:48），之后仍有 3 条同种警告，早于 `workbench.rs` 的最后修改 02:52:35。从日志看不出它跑的是哪份二进制。
- **建议**：记录里写明「修复后的二进制由 `6dc6ee3` 加上未提交的 role 修复构建」；把上面三处列进「仍然没有验证的」；确认那一次跑的是修复前的二进制（按「修复前的二进制两次」推测是，但日志证不了）。如果不是，要回头查还有哪条路径会触发。

## 🟢 优化建议（可选）

**O1 `Role::Group` 选得对，理由没有留下来**
- **位置**：`crates/gitwave/src/workbench.rs:220-223`；`plan.md:160`。
- **描述**：候选 role 经 accesskit 0.24.1 映射后的结果：

  | role | macOS | Windows（UIA） | Linux（AT-SPI） |
  |---|---|---|---|
  | **Group** | `AXGroup`，子角色 `AXApplicationGroup` | Group | Panel |
  | Pane | `AXUnknown` | Pane | Panel |
  | Region / Main | `AXGroup` 加地标子角色 | Group | Landmark |
  | Application | `AXGroup`，子角色未知 | Pane | Embedded |
  | Window | `AXGroup`，不可聚焦 | Window | Frame |
  | GenericContainer | 不进树 | 不进树 | 不进树 |

  出处：`accesskit_macos-0.26.3/src/node.rs:41-180,256,288-290`、`accesskit_windows-0.34.0/src/node.rs:62-211`、`accesskit_atspi_common-0.19.1/src/node.rs:97-291`。

  - Pane 在 macOS 上是 `AXUnknown`。
  - Region 和 Main 是地标，ARIA 要求有名称，会出现在读屏器的地标列表里。Main 还表示「主要内容」，而工作台包含标题栏、侧栏和状态栏。
  - Application 会改变读屏器的按键处理，AT-SPI 上还是语义不对的 Embedded。
  - Window 是 gpui 树根用的 role，macOS 上不可聚焦，正好破坏这次要达到的效果。
  - Group 三端都是中性的容器。组件库自己也用它表示没有专门语义的容器（`gpui-component-0.11.0/src/input/group.rs:211`、`gpui-base-0.11.0/src/accordion.rs:61`、`toolbar.rs:276`）。

  与外层不重复。外面那层 group 是 gpui 每个窗口无障碍树的根（`a11y.rs:481-487`，`Role::Window`；macOS 上映射成 `AXGroup`，`accesskit_macos-0.26.3/src/node.rs:178-180`），不是组件库的 `div#root`。后者没有 role 也没有 `track_focus`，不进树（`gpui-base-0.11.0/src/root.rs:283-298`）。Windows 和 Linux 上是「窗口 › 组」。macOS 上两层 `AXGroup` 相套，多出的一层来自 gpui 的根，与选哪个 role 无关。

  可以对照的是上游自己的示例：拿到 focus 的根容器用 `Role::Application` 加 `aria_label`（`gpui-pre-0.3.14/examples/a11y.rs:65-68`）。对整个窗口内容来说，Group 的声明更弱，三端更一致，这里的偏离是合理的。
- **建议**：在 `workbench.rs` 的注释或 `plan.md:160` 留一句「为什么是 Group，不是 Application 或 Pane」，下次不用再查一遍。

**O2 group 现在没有名称**
- **位置**：`workbench.rs:216-223`。
- **描述**：macOS 适配器故意不让树根带窗口标题，注释说带了 VoiceOver 会出问题（`accesskit_macos-0.26.3/src/node.rs:306-313`），所以树里没有一个念得出「GitWave」的容器。focus 落回工作台时，读屏器面对的是一个没有名称的 group。
- **建议**：可以加 `aria_label`（应用名或「工作台」）。朗读效果要先在实机上用 VoiceOver 听过再定，不要凭推断加。

**O3 测试可以多钉一件事，注释可以说清边界**
- **位置**：`crates/gitwave/src/workbench_tests.rs:90-106`。
- **描述**：触发日志的路径是「dialog 关掉以后 focus 回到工作台」，测试只看了窗口刚打开时的状态。两者走的是同一段 gpui 代码，不影响结论，但前者没有被任何测试钉住。另外，测试名说的是 "exposed to assistive technology"，断言的其实是两个属性，注释没有说明为什么不断言日志。
- **建议**：在同一测试里打开再关掉设置 dialog，再断言一次 `focused() == Some(true)`（关闭按钮 `close-settings` 可以直接点，`Button` 自带 `.test_support()`，`gpui-base-0.11.0/src/button.rs:48`）。注释补一句「测试平台不激活无障碍树，所以断言的是 gpui 建节点的前提」。两条 `assert_eq!` 加上消息。语言锁不用动：这个测试自己不读界面文字，但 `open` 会重置进程级的语言，必须持锁，现在的写法与同文件其它测试一致。

**O4 `workbench.rs` 的注释会随 M3 失效**
- **位置**：`workbench.rs:220-221`。
- **描述**："It holds the focus whenever no dialog does" 现在成立，是因为工作台里还没有别的可聚焦元素。M3 加进列表和输入框以后这句话就不对了。
- **建议**：改成不依赖这个前提的说法，例如 "It takes the focus when the window opens and whenever a dialog closes"。

**O5 「对话框的无障碍节点没有名称」是已知上游缺口的实机确认**
- **位置**：`plan.md:163`。
- **描述**：调研笔记已经把它记为上游 #3595（`docs/tech/tech-selection/gpui-kit-research.md:22,151`）。归因也核实了：`gpui-component-0.11.0/src/dialog/dialog.rs` 的公开方法里没有 `accessibility_label`，377 行构造底层 dialog 时也没有设置，应用侧确实改不了。
- **建议**：这一条后面补一句引用，并注明「0.11.0 上实测仍在」。它就从一个观察变成了对上游缺口的确认。

**O6 日志落盘的新结论没有回到 review.md 的 Y9**
- **位置**：`review.md:780`、`800`；`plan.md:157`。
- **描述**：Y9 当初的担心是「不刷新，退出瞬间的日志可能丢」。`plan.md:157` 的对照实验说明丢不丢与退出时是否刷新无关，但补记只写了「五项都通过」。只读 review.md 的人会以为 Y9 的修复是实机通过的原因。
- **建议**：在 Y9 那一行的「核实」列或补记里补一句「实机确认落盘；对照显示不靠这段刷新，保留它是为了不把结果交给调度」。

**O7 同一份计划里菜单名有两种写法**
- **位置**：`plan.md:127-128`（本次没有改）。
- **描述**：切片一的记录把 View 菜单写成「查看」，文案里是「显示」（`crates/gitwave/locales/menu.zh-CN.json:8`），新记录用的是「显示」。
- **建议**：顺手统一成「显示」。

## 📝 总体评价

代码改动是对的，而且很小：一行 role、一行 `.test_support()`、一个断言两个前提的测试。门禁全过，生产构建里没有留下任何测试痕迹。需要处理的全在文档上：状态行把未提交的改动说成了已过 CI，「焦点」这条设计输入还停在会让人重犯的版本。合并前最该做的是 Y1 和 Y2。

## 验证记录

### 验证过的

**门禁**
1. 仓库内 `cargo fmt --all -- --check`：退出码 0。
2. 仓库内 `cargo clippy --locked --workspace --all-targets -- -D warnings`：退出码 0，以 `warning` 或 `error` 开头的输出 0 行。
3. 仓库内 `cargo test -p gitwave --locked`：20 passed，含 `the_workbench_is_exposed_to_assistive_technology`。
4. 仓库内 `make check CARGO="cargo --locked"`：退出码 0，11 秒（产物已是最新，没有重新编译）。`gitwave` 20 passed；`gitwave-core` 400 passed、2 ignored，9.77 秒。
5. 稳定性：直接运行编译好的测试二进制，`HOME` 指向临时目录，连跑 60 次，60 次通过。
6. 跑测试前后比对真实日志目录 `~/Library/Application Support/GitWave/logs`（18 个文件）的大小与修改时间：一致。临时 `HOME` 下没有生成日志文件。

**生产构建里没有 `Observed`**
7. `cargo tree -e no-dev,features -i gpui-base --locked`：`gpui-base` 只有 `default`。换成 `-e all`，`gpui-base feature "test-support"` 由 `gpui-kit feature "test-support"` 启用。
8. `cargo build -p gitwave --locked`：Finished，`target/debug/gitwave` 没有重编（修改时间 02:52:47，50,266,056 字节）。
9. `grep -a -c` 查字符串，生产二进制 / 测试二进制：`test_support requires an existing ElementId` 0 / 1，`focus binding was not observed for` 0 / 1，`ambiguous ElementId` 0 / 1，`Registered paths` 0 / 2。

**日志（只读，只取计数与时间戳；时间都是 UTC）**
10. `app.log.2026-11-25` 共 171 行。`has no accessibility node` 86 条，全部是 `it has an id but no role`，`it has no element id` 0 条。`Accessibility activated` 7 次，没有其它以 `a11y:` 开头的行。
11. 到 18:15:42Z 为止 109 行，其中警告 78 条。78 条相邻间隔的中位数 3.455 秒，最小 2.170 秒，最大 173.620 秒，小于 50 毫秒的 0 对。
12. 每次激活之后的警告条数：18:15:02 → 78；18:45:37 → 2；18:46:00 → 1；18:46:12 → 2；18:47:04 → 0；18:48:26 → 3；18:52:50 → 0，文件到此为止。
13. 修改时间（本地）：`workbench_tests.rs` 02:50:59，`workbench.rs` 02:52:35，测试二进制 02:52:36，`target/debug/gitwave` 02:52:47，`review.md` 与工程文档 02:54:17，`plan.md` 02:56:43。

**源码**
14. 触发路径：`div.rs:2417-2436`（`track_focus` 的元素在无障碍激活时 `set_focusable`，拿到 focus 时 `set_focus`）→ `a11y.rs:232-256`（有节点就设 focus 并清掉去重，没有就记日志）。节点在 `element.rs:436-450` 里建，前提是有 id 且 `a11y_role()` 不是 `None`。
15. 去重：`a11y.rs:193-202`，同一个 `FocusId` 只记一次，直到有一次 focus 落在有节点的元素上。
16. 测试平台不激活无障碍：`gpui-pre-0.3.14/src/platform.rs:1524` 是空的默认实现，测试平台与 `gpui-pre-platform-0.3.14` 都没有覆盖；`TestAppContext` 只有 `disable_accessibility`（`app/test_context.rs:210-212`）。
17. `Observed` 每帧多做的事，只在开了 `test-support` 的构建里：`prepaint` 建一个 accesskit `Node` 并登记（`gpui-base-0.11.0/src/test_support.rs:484-536`），`paint` 再算一次样式（`553-556`）。布局、绘制、命中测试、`a11y_role`、`write_a11y_info` 都原样转发，没有额外的布局节点和 hitbox。
18. 辅助技术请求 Focus 时，gpui 的内置处理是把 focus 给对应的句柄（`src/window.rs:7616-7622`）。对这个元素来说就是工作台本来默认的 focus，没有新的副作用。
19. 打开窗口时会画一帧：`app.rs:1665-1670`。
20. `crates/gitwave/src` 里的 `track_focus`、`focus_handle`、`.id(`、`.role(`、`test_support` 只在 `workbench.rs:189-192`、`217-223`、`270-273` 和 `workbench_tests.rs:102` 出现，其余六个文件里没有元素。
21. 组件库里有不带 id 的 focus 锚点（`gpui-base-0.11.0/src/dialog.rs:414`、`gpui-component-0.11.0/src/dialog/dispatch_anchor.rs:34`）。它们拿到 focus 时会触发另一种原因的同一条日志。注释写明它们不进 Tab 顺序、不主动取 focus，`DialogClose::activate` 只派发动作、不改 focus；日志里这一种是 0 条。
22. `tracing-appender`：见优点。

**文档**
23. 测试数：`workbench_tests.rs` 的 `#[gpui_kit::test]` 8 个，`locales.rs` 9 个，`runtime.rs` 2 个，`logging.rs` 1 个。
24. `plan.md:3` 的两个链接目标存在（ADR 0007 的标题在 `docs/tech/decisions/00-overview.md:249`），`review.md:800` 的 `./plan.md` 存在。锚点本身是既有链接，没有逐个核对 GitHub 的生成规则。
25. `gpui-base` 的 `Dialog::accessibility_label` 是公开的（`dialog.rs:496`），`gpui-component` 的 `Dialog` 没有透传。
26. 仓库状态没有变：HEAD `6dc6ee3`，`git status --short` 仍是那 5 个修改文件，暂存区为空，没有 stash，`Cargo.lock` 没有差异。

### 没有验证的

- **真实应用行为**（受「不启动应用」约束）：
  - VoiceOver、Narrator、Orca 的朗读，包括无名的 `AXGroup` 加 `AXApplicationGroup` 子角色念出来是什么。本机没有找到 AppKit 头文件里对这个子角色的说明。
  - 无障碍树的实际结构，以实现方用 System Events 的读数为准。
  - Windows 与 Linux 上的映射。
- **新测试在 Windows 与 Linux 上的运行**：三端 CI 跑的是 `6dc6ee3`，不含它。断言的两个属性与平台无关，预期一致。
- **变异实验**：没有做。四个想法，预期结果是：
  - 去掉 `.role(Role::Group)`：`role()` 为 `None`，第一条断言失败。实现方做过。
  - 把 `.test_support()` 挪到 `track_focus` 之后：`Observed` 没有记下 focus，`focused()` 返回 `None`，第二条断言失败。
  - 改成 `Role::GenericContainer`：调试构建下 `role()` 断言失败，测试 panic。
  - 删掉 `Workbench::new` 里的 `window.focus(&focus, cx)`：`focused()` 返回 `Some(false)`。这一条值得跑，它能证明第二条断言不是空转。
- **实机记录里其余几项**（系统菜单栏、⌘Q、设置 dialog 的外观、中文输入法）和两项测量（截图上的像素位置、仓库外的日志对照工程），以实现方为据。
- **日志里 18:48:21Z 启动的那一次**用的是哪份二进制。
- **主题切换**是否在实机上验证过：记录里没有。
- **PR #246 的描述**是否需要同步：没有查看。

## 处理结果：无障碍修复

| 编号 | 发现 | 处理 | 核实 |
|---|---|---|---|
| 🔴 | 无 | — | — |
| Y1 状态行说过头 | 没提交的修复被算进「三端 CI 通过」；「实机验证已做完」漏了主题切换，快捷键也只记了 ⌘Q | 状态行按提交后的事实重写。主题切换先补了一层测试证据：新增 `switching_the_appearance_repaints_the_window`，读一帧实际画出的背景，切换后不得残留旧主题的颜色；⌘B 新增 `the_sidebar_shortcut_hides_the_sidebar_and_brings_it_back`。随后屏幕解锁，窗口层面的实机检查补做完了（`ef0ef3a` 构建的二进制）：主题、⌘B、⌘M、⌘H、「缩放」、「关于」和「设置」菜单项、输入框的撤销、重做、全选，都通过，结果记在任务计划。没在实机上试的是 ⌥⌘H、剪切、复制、粘贴和「前置全部窗口」 | 变异：让工作台把背景色缓存起来，重绘测试失败在「something is still painted in the light theme」；把 ⌘B 的绑定改掉，侧栏测试失败 |
| Y2 「焦点」的设计输入没提 role | 规则只埋在执行记录里，照着旧的写法会再犯 | 任务计划的设计输入补上「id 与 role 要同时有，容器用 `Role::Group`」。技术文档新增「界面的无障碍」一节：规则、为什么是 Group、测试里读元素的前提 | 自己核对 gpui-pre（`role()` 的调试断言、`a11y_role()` 的过滤）和 accesskit 三端适配层的映射 |
| Y3 补记的措辞 | 一处写成了事实，一处级别与性质混在一起，一处会过期，一处指代不准 | 四处都按建议改了 | — |
| Y4 二进制的归属与未验证清单 | 记录没说清「修复后」的二进制是哪份代码；一次启动的归属不明；无障碍的未验证项没列 | 记录写明「修复后」的二进制是 `6dc6ee3` 加 `ef0ef3a`。18:48:21Z 那次启动用的是修复前的二进制：它构建于 02:11:11，之后到 02:52:47 才重新构建。那次的 3 条警告对应 3 次关闭对话框（点 ×、点「关闭」、点遮罩）；点面板空白处那次对话框没关，也就没有警告。未验证项补上 VoiceOver 的实际朗读、Windows 与 Linux 上的映射 | 对照实现方自己的操作记录 |
| O1 Group 的理由 | 选得对，理由没有留下来 | 写进了技术文档和代码注释 | — |
| O2 group 没有名称 | 读屏器面对的是一个无名的 group | **不处理**。按审查者的建议，等用 VoiceOver 听过再决定加不加名称，不凭推断加 | — |
| O3 测试可以多钉一件事 | 「对话框关闭后焦点回到工作台」这条真实路径没有测试；断言没有消息；注释没说清边界 | 测试打开再关闭设置对话框后重新断言；两条断言加了消息；注释写明测试平台不激活无障碍树 | 变异：删掉 `Workbench::new` 里的 `window.focus(&focus, cx)`，测试失败（`left: Some(false)`）。审查者列的另外三个变异：去掉 role 的已做；把 `.test_support()` 挪到 `track_focus` 之后的，开发时实际出现过，`focused()` 报「focus binding was not observed」；换成 `GenericContainer` 的没做，依据是 `role()` 源码里的调试断言 |
| O4 注释会随 M3 失效 | "whenever no dialog does" 依赖工作台里没有别的可聚焦元素 | 改成「打开窗口时取得焦点，每次对话框关闭时拿回来」 | — |
| O5 对话框节点没有名称 | 是已知上游缺口的实机确认 | 记录里补了对调研笔记里上游 #3595 的引用，注明 0.11.0 上实测仍在 | 自己核对调研笔记与组件库源码：`gpui-base` 的 `Dialog::accessibility_label` 是公开的，`gpui-component` 的 `Dialog` 没有透传 |
| O6 日志的新结论没回到 Y9 | 只读审查报告的人会以为 Y9 的修复是实机通过的原因 | 上一节的补记里加了 | — |
| O7 菜单名两种写法 | 切片一的记录写「查看」，文案是「显示」 | 统一成「显示」 | — |

### 修复后的验证

`make check` 通过，`gitwave` 22 个测试、`gitwave-core` 400 个通过、2 个忽略。四个变异各自让对应的测试失败，每次都从备份还原文件并比对校验和。`ef0ef3a` 构建的二进制在 macOS 上又确认了一次：无障碍激活后打开、关闭对话框多次，日志零条；对话框打开时 ⌘Q 照常退出。推送后的 CI 一次通过：Ubuntu 1 分、Windows 3 分 10 秒、macOS 1 分 28 秒，新增的三个测试在三端都跑了，测试数与本地一致。实机补做的主题与快捷键检查记在任务计划同日的记录里。

仍然没有验证的：⌥⌘H、剪切、复制、粘贴和「前置全部窗口」在实机上的效果；VoiceOver、Narrator、Orca 的实际朗读；Windows 与 Linux 上的实机表现。
---

# 代码审查：M3 切片一（只读工作台第一步）

> **作者**：AI code-reviewer 子代理，未经人工复核
> **状态**：草案
> **最近更新**：2026-11-27

| 项目 | 内容 |
|---|---|
| 分支 | `feature/gpui-kit-read-only-workbench`（自 `8578952` 切出），未提交的工作区改动：34 个已修改文件，5 个未跟踪文件 |
| 环境 | macOS arm64；rusqlite 0.40.2 / SQLite 3.53.2（与 `Cargo.lock` 一致） |
| 方式 | 只读。实验都在自建的临时目录里做，已清理；没有运行应用，没有访问 `~/Library/Application Support/` 下 GitWave 的目录 |

下面是审查者的报告原文，行号对应审查时的工作区；之后是实现方的处理结果。

**结论先行（针对第 1 条）**：没有找到让开发版改写正式版 `state.db` 或已存在的 `state.db-wal` 的路径，也没有找到会让正式版打不开它的路径。找到的是三件相关的事：只读连接会在原库目录里留下一个 0 字节的 `state.db-wal` 和一个 `state.db-shm`，与文档“只有 `-shm`”的说法不符（Y1）；开发版的日志仍写进正式版的 `GitWave/logs/`（Y2）；Windows 上这批改动编译不过（R1）。

## ✅ 优点

- **副本的读法是稳的。** 我独立复核了只读 `VACUUM INTO`：在写入方开着、崩溃后只剩 `-wal`、`-wal` 与 `-shm` 都在、已完全回填的 `-wal` 残留这四种状态下，原库的 `state.db` 和 `-wal` 的字节数与 mtime 都不变，变的只有 `-shm`。与实现方的结论一致。
- **替换副本的顺序是对的。** 先删旧的 `-wal` / `-journal` 再 rename，注释解释了原因（旧 WAL 会被重放到新库上）；staging 加 rename 是原子替换；失败时保留上一次成功的副本，有测试。
- **路径处理经得起特殊字符。** 引号翻倍正确；实测目标路径同时含 `'`、中文、`?`、`#` 时复制成功。
- **没有引用环，`.detach()` 的任务无害。** `CommitList` 持的是 `WeakEntity<History>`，打断了 History → ListState → CommitList → History 的环。视图销毁或窗口关闭后，任务跑完 `await`，`let _ = this.update(..)` 吞掉错误就结束。第 3 条没有发现问题。
- **代数判断有两道防线，补测的过程如实记录。** `repos_arrived` 既比代数，又拒绝别的 Workspace 的仓库列表；A → B → A 的场景有测试；变异暴露的两处缺口写进了执行记录。
- **事故处理得诚实。** 测试往副本里写过一次这件事记在 plan 里，并加了守卫；`state_dir_path()` 只算路径、不建目录。
- **`Pages` 是纯状态机。** 逐条核对了 `push` / `fail` / `retry` / `ask_for_more` / `clear` 的组合，没有走不通或自相矛盾的状态。
- **错误码经桥接原样保留。** `runtime::use_case` 取代 `runtime::blocking`，`error_text::show` 按错误码取文案并填参数，两条路径都有测试。
- **`ShellSettings` 的边界都考虑到了。** `#[serde(default)]`、空白 id 归一、损坏数据不报错，各有测试。
- **文档里的数字全部属实。** 逐项核对见「验证记录」。

## 🔴 严重问题（必须修复）

### R1. Windows 上编译不过：`lock_down_db_files` 的非 unix 版本仍是私有的

- **位置**：`crates/gitwave-core/src/infrastructure/persistence/sqlite.rs:100-101`（非 unix 版本）、`:81`（unix 版本）；`crates/gitwave-core/src/infrastructure/persistence/snapshot.rs:17`、`:142`；`.github/workflows/ci.yml:35`（`windows-latest`）
- **描述**：这次只给 `#[cfg(unix)]` 版本加了 `pub(super)`。`snapshot` 是 `sqlite` 的兄弟模块，无条件 `use super::sqlite::lock_down_db_files` 并在 `:142` 调用。Windows 上解析到的是私有函数，报 `E0603`。
- **为什么是问题**：CI 在 Windows 上跑 `cargo clippy --workspace --all-targets` 和 `cargo test --workspace --all-targets`，两者都会失败。`main` 的保护规则要求 CI 通过。`make check` 只在 macOS 上跑，发现不了。
- **建议**：`sqlite.rs:101` 也加 `pub(super)`。修完先让三端 CI 跑一次再继续 M3：`snapshot.rs` 的 6 个测试和 `workbench_tests.rs` 的 git2 夹具都还没有在 Windows 上跑过。
- **验证**：已验证。用同样的模块关系和 cfg 写了一个最小程序，`rustc` 在“非 unix”分支报 `error[E0603]: function 'lock_down_db_files' is private`，在"unix"分支通过。本机没有安装 Windows target，没有对仓库做交叉 `cargo check`。

## 🟡 一般问题（建议修复）

### Y1. 只读连接会在原库目录里留下 0 字节的 `state.db-wal` 和 32 KB 的 `state.db-shm`，文档说“只有 `-shm`”

- **位置**：`snapshot.rs:5-9`（模块注释）、`:92`（`refresh` 的文档注释）、`:94-104`（`refresh`）；`README.md:51`、`:77`；`docs/tasks/refactor-gpui-kit-desktop/plan.md:111`；`docs/tech/tech-selection/00-overview.md:65`
- **描述**：实测原库正常关闭后目录里只有 `state.db`。只读连接打开、`VACUUM INTO`、关闭之后，目录里多了 `state.db-wal`（0 字节）和 `state.db-shm`（32768 字节），关闭后不清理。
- **对正式版无害**：随后打开、读、写、关闭都正常；关闭时它照常把两个文件删掉。
- **副作用**：`Stamp::of` 把 `-wal` 算在内。`refresh` 在复制之前取 stamp（`wal: None`）并记录；复制之后原库目录里多了一个 `-wal`。下一次启动读到 `wal: Some(0 字节)`，与记录不等，于是再复制一次。正式版每运行一次（关闭时删掉这两个文件），开发版的下一次启动又多复制一次。与“原库变了才重新复制”不符，只是多做无用功。
- **为什么是问题**：第 1 条的答案取决于这些表述准不准。README 同一段先说 "never opens the original"，两句之后又说 "the original is read through a read-only connection"，自相矛盾。实机验证记的是 `state.db` 和 `state.db-wal` 的哈希不变：当时正式版如果开着，就测不到这个状态；如果没开，`-wal` 是从不存在变成 0 字节，比哈希看不出来。
- **建议**：
  1. 四处表述改成与实测一致：只读连接不改 `state.db` 和已存在的 `-wal`；会读或创建 `-shm`；没有 `-wal` 时创建一个空的。
  2. `Stamp` 把“长度为 0 的 `-wal`”与“没有 `-wal`”视为相同。
  3. 补一条测试：原库的写入方关闭后连续 `refresh` 两次，断言第二次是 `Current`。现有 6 个 snapshot 测试里写入方都开着，没有覆盖这个最常见的状态。
- **验证**：`-wal` 的出现是实测（下面实验的场景 2、6、8、9）。第二次 `refresh` 返回 `Taken` 是按 `snapshot.rs:94-104` 和实测结果推演的，没有直接调用 `refresh`。

### Y2. 开发版的日志仍写进正式版的 `GitWave/logs/`

- **位置**：`crates/gitwave/src/logging.rs:21`；`crates/gitwave-core/src/infrastructure/observability/tracing.rs:76-80`；`README.md:75`
- **描述**：`logging::init` 调 `subscriber::init()`，日志落在 `dirs::data_dir()/GitWave/logs/app.log.<日期>`。副本方案只管了 `state.db`。两个进程往同一个按天滚动的文件追加写，日志里没有字段区分来源；开发版启动时还会对正式版的日志目录跑一遍 `prune_old_logs`。
- **为什么是问题**：这是第 11 条要找的那类遗漏，同一类问题漏了一处。正式版的日志里混进开发版的行，排障时分不开。README:75 的描述是准确的，所以不算文档不实。
- **建议**：core 已有 `init_in(dir)`（测试在用）。应用启动时把日志目录定在 `GitWave-dev/logs/`，日志字段带上 `build = "dev"`。
- **验证**：已验证（读了调用链；按约束没有去看那个目录里的实际文件）。

### Y3. 离开再回到同一个 Workspace，活动仓库回到启动时的值，并被写回副本

- **位置**：`crates/gitwave/src/workbench.rs:237`（唯一设置 `session.workspaces` 的地方）、`:333-347`（`repos_arrived`）；`crates/gitwave/src/app_state.rs:211`（`remember_active_repo` 只写库）
- **描述**：`repos_arrived` 用 `self.session.workspaces()` 里的 `last_active_repo_id` 恢复活动仓库，那是启动时读到的快照，之后不再更新。复现路径：Workspace 有仓库 [a, b]，启动时没有活动仓库，激活 a；用户点 b，库里写成 b；切到另一个 Workspace 再切回，`restore_active(None)` 选了第一个 a，并把 a 写回库。用户选的 b 这一次运行里丢了，下次启动也恢复不到。
- **为什么是问题**：与 README:77 "the workspace and repository you were on stays in the copy" 的意图不符。两个切换 Workspace 的测试里每个 Workspace 只有一个仓库，测不出来。
- **建议**：`remember_active_repo` 之后同步更新快照（`WorkspaceSummary` 的字段是 pub 的），或者让 `Session` 按 Workspace 记住最后激活的仓库。补一条两个仓库的往返测试。
- **验证**：已验证（读代码；没有运行应用）。

### Y4. `show_history` 的 HEAD 回调用的是仓库列表的代数，点标签换仓库时它不加一

- **位置**：`workbench.rs:394-407`；`:302`（`generation` 唯一加一的地方）；`:364-378`（`repo_chosen` 不加一）；`:459-464`（`identity()` 的 `repo_id` 判断）
- **描述**：第 2 条问“代数在哪里加、哪些路径漏了”，漏的是这一处。点 a 的标签发出 HEAD 请求（代数 G），结果回来之前点 b，又发出一个，仍是 G。a 的结果晚到时 `generation == this.generation` 成立，`this.head` 被写成 a 的。
- **现状**：`identity()` 只在 `head.repo_id == repo.id` 时显示分支，串台的结果显示不出来。但同一个仓库 A → B → A 时，第一次的 HEAD（旧）可以盖掉第三次（新），因为 `repo_id` 相同。架构文档 `:59` 写的是“换了目标就加一”。
- **其余几处的结论**：壳层的 `repos_arrived` 和历史的 `page_arrived` 是对的，都有测试；`History::head_arrived`（`history.rs:282-295`）也是对的，但没有测试。
- **建议**：HEAD 请求用自己的代数（每次 `show_history` 加一），或者在回调里核对当前活动仓库的 id 与发起时的 `scope.repo_id` 相同。补一条测试：点 a、点 b，让 a 的 HEAD 晚到。
- **验证**：已验证（读代码）。

### Y5. 切换仓库后，列表的选中行和滚动位置没有重置

- **位置**：`crates/gitwave/src/history.rs:176-184`（`show`）、`:298-314`（`sync_list`）、`:412`；gpui-component 0.11.0 `src/list/list.rs:576-597`
- **描述**：`ListState` 自己管选中（`selected_index`），`render_list_item` 用它覆盖条目上的 `.selected(..)`。`sync_list` 只换 `commits`，不动它。用户在 a 里点过第 5 行，切到 b，第 5 行是高亮的。`show()` 里也没有回到顶部的调用；在 a 里滚到很深的位置再切到另一个大仓库，新仓库可能显示在历史中间，并且一上来就触发 `load_more`。
- **建议**：`show()` 时清空选中并回到顶部（`scroll_to_item` 需要 `&mut Window`，`History::show` 现在没有这个参数）。`CommitList.selected` 字段和 `:412` 的 `.selected(..)` 没有效果，可以删掉。
- **验证**：选中行不重置是读 gpui-component 源码确认的。滚动位置保留是推测，依据是没有任何重置调用，建议实机确认。

### Y6. 仓库列表读不出来时，主区停在 "Loading…"，没有办法重试

- **位置**：`workbench.rs:350-359`、`:288-290`、`:623-627`
- **描述**：`repos_arrived` 的失败分支只设置 `notice`，`selected.repos` 仍是 `None`，`main_pane` 把 `None` 画成 "Loading…"。再选同一个 Workspace 会被 `:288` 的提前返回挡住。
- **建议**：失败时把 `repos` 标成“已读、失败”，主区给出错误和重试（`History` 已经是这么做的）；或者让重选同一个 Workspace 也重新读。
- **验证**：已验证（读代码）。

### Y7. 游标过期时列表突然清空，界面没有说明；自动重试没有上限

- **位置**：`history.rs:268-276`；`crates/gitwave-core/src/infrastructure/git/history.rs:486`
- **描述**：过期分支 `pages.clear()` 后从头读，用户已滚到的位置没了，只有一条 `info` 日志。第一页也会过期（core 在建完快照后再比一次 fingerprint），所以这个分支是“立刻再发一个请求”，没有次数上限也没有退避。仓库的 ref 一直在变时，它会在后台循环做全量 revwalk，界面一直显示“正在加载历史”。
- **建议**：保留已显示的提交，后台从头重读，回来后续接新的游标；至少在页眉说明“历史有更新，已重新加载”。给自动重试加上限（比如 2 次），超过后走 `Failed`，交给重试按钮。
- **验证**：已验证（读代码；没有构造过期场景运行）。

### Y8. 复制失败对用户不可见；含数据的 staging 文件权限是 0644，rename 失败时不清理

- **位置**：`app_state.rs:106-117`；`snapshot.rs:122-142`；`locales/app.*.json` 的 `app.workbench.state.copy`
- **描述**：
  - `open_copy` 里 `refresh` 失败只写日志，照样打开目录里现有的库。从没复制成功过时，用户看到的是 "No workspaces / Create a workspace to get started."，状态栏写着“正式版状态的副本”。分不清是复制失败、没有正式版，还是真的没有 Workspace。
  - `VACUUM INTO` 生成的 staging 文件权限跟随 umask（实测 0644），rename 之后才被 `lock_down_db_files` 改成 0600。rename 失败（Windows 上另一个实例开着副本时会这样，见 O8），或者进程在这两步之间被 kill，一个含 token 的完整副本就以 0644 留在 `GitWave-dev/` 里；下次启动 pid 不同，也不会被清掉。
- **建议**：`Origin` 加一个“复制失败”的变体，状态栏用 `danger` 色说明，或把错误放进 `notice`；`vacuum_into` 之后立刻对 staging 收紧权限；rename 失败的分支也删除 staging。
- **验证**：0644 是实测；其余是读代码。rename 失败的场景是推演。

### Y9. 能直接打开正式库的 pub 入口没有守卫，测试守卫只盖住了一个

- **位置**：`crates/gitwave-core/src/bootstrap.rs:30-38`（`open_app_context`）；`sqlite.rs:21-25`（`state_dir`）、`:49-51`（`open`）；`app_settings_repo.rs:31-36`（`SqliteAppSettingsRepo::open`）；`persistence/mod.rs:14`（再导出）；`app_state.rs:62-68`（现有守卫）
- **描述**：`sqlite::state_dir()` 现在的调用者是 `sqlite::open()` 和 `bootstrap::open_app_context()`；`open()` 另外被 `SqliteAppSettingsRepo::open()` 调用。这四个都没有生产调用者了，但都是 pub 且被再导出。任何一个被调用，就会创建正式版的目录、打开正式库并跑迁移。
- **关于第 4 条的守卫**：`app_state::init` 在测试构建里 panic 这个做法是合适的，运行时没有更简单的办法拦住误调用。问题是它只盖住这一个入口：`open_development(Some(原库路径))`、`open_copy`、core 的上面三个、`tracing::init()` 在测试里调用都不会被拦。
- **一个事实**：现在两边的迁移相同（都是 0001–0004，`8466d55` 的 `src-tauri/migrations` 与 `crates/gitwave-core/migrations` 一致）。今天调用这些入口不会把正式库升到正式版不认识的架构。风险从开发版新增第一个迁移起才成立，而那正是最容易出事的时候。plan `:196` 把它写成了已经成立的事实。
- **建议**：给这四个入口的文档写明“开发版不得调用，接管真实库的路径在 M7 定”，加 `#[deprecated]` 并在 note 里写明原因；或者把 panic 守卫下沉到 `state_dir()`，让所有真实目录入口共享。plan `:196` 改成条件句。
- **验证**：已验证（grep 调用者；比对了迁移列表）。

### Y10. 新应用没有关闭 libgit2 的所有权校验，与 v0.11.0 不同

- **位置**：`crates/gitwave/src/main.rs:29-36`；`crates/gitwave-core/src/bootstrap.rs:22-25`；`8466d55:src-tauri/src/lib.rs:1670`
- **描述**：v0.11.0 启动时调用 `disable_git_owner_validation()`，新应用的启动路径没有。捆绑的 libgit2 1.9.7 的默认值是 `git_repository__validate_ownership = true`（`repository.c:42`）。接入真实状态之前用的是测试仓库，所以现在才显现。
- **影响**：所有者不是当前用户的仓库（v0.11.0 的注释举的例子是 Windows 上提权 clone 出来的目录）在新应用里 `ctx.open_repo` 会被拒绝，历史显示加载失败。实机上的 5 个仓库都是用户自己的，所以没遇到。
- **建议**：启动时调用一次，与 v0.11.0 一致。Windows 实机验证时专门找一个所有者不同的仓库。
- **验证**：默认值与调用差异已验证（读源码与 git 历史）。现象是推测，没有造一个所有者不同的仓库来跑。

### Y11. 四条既有文案在新界面里说的是做不到的事

- **位置**：`workbench.rs:572`（`workspace.tabs.empty`）、`:558-560`（`workspace.tabs.missingTitle`）、`:618-622`（`workspace.empty.description`）、`:633-637`（`branches.graph.selectRepoDesc`）
- **描述**：只读切片一不能 init / clone / add，没有「仓库」菜单，没有右键菜单和重新关联，不能创建 Workspace；仓库在标签行，不在侧边栏。这几条是从旧实现原样带来的，这次是第一次显示给用户。`missingTitle` 还被用作缺失仓库标签的 `aria_label`，读屏器会告诉用户“右键可重新关联”。
- **新增的 6 条没有问题**：键集合、占位符、数量都核对过（见 O9）；`app.workbench.history` 在 zh-CN 里本来就是 "History"，照用没问题。
- **建议**：换成只读版的说法，或者等对应功能进来之前先不用（比如缺失仓库的标签只说“路径缺失”）。
- **验证**：已验证（读了两种语言的文案与调用点）。

### Y12. 测试缺口与清理

- **位置**：`crates/gitwave/src/workbench_tests.rs:82-93`（`wait_for`）、`:96-107`（`temp_dir`）、`:128-174`（`git_repo`）、`:483-534`；`history.rs:194-208`
- **描述**：
  - “滚到末尾附近取下一页”这条真实的链路没有测试。测试窗口只显示前二十几行，300 条时 `visible_end >= 300 - 20` 不成立，所以测试直接调 `ask_for_more`，绕过了 `CommitList::has_more`、`load_more` 和 `cx.defer_in`。这三个里任何一个写错，测试照样通过；测试名里的 "reads on near its end" 名不副实。把列表的滚动 offset 设到底再 `render_frame`，就能走真实的触发。
  - 没有测试的路径：游标过期分支；行上的 HEAD 标记（`history-head` 没有被任何测试找过）和 `head_arrived`；视图层的失败与重试（`history-failure`、`history-retry`）；`repos_arrived` 的失败分支；多仓库 Workspace 的往返（Y3）；原库没有 `-wal` 时连续 `refresh`（Y1）。
  - `wait_for` 本身没有问题：`allow_parking` 加轮询 `run_until_parked`，20 秒上限；18 个窗口测试单次 0.93 秒。要注意 `allow_parking` 是执行器全局的，打开之后测试里任何意外的主线程阻塞都不再 panic。
  - `git_repo` 夹具没有发现依赖机器 Git 配置的地方：签名是显式的，libgit2 不跑 hooks，提交时间固定，`set_head` 不受 `init.defaultBranch` 影响。全局 `core.autocrlf` 只会改变 blob 的内容，断言用的是实际返回的 SHA。
  - 临时目录不删。`temp_dir()` 每次调用建一个新目录，从不删除；本机 `$TMPDIR` 里现在有 2999 个 `gitwave-shell-test-*`，每个约 52 KB，合计约 150 MB。core 里的 `temp_dir` 用 `let _ = remove_dir_all`，断言失败时也不执行。
- **建议**：补上面几条测试；用一个 RAII 的临时目录（`tempfile` 已经在 `Cargo.lock` 里，core 里五处手写的 `temp_dir` 可以合并）。
- **验证**：已验证（读了测试与 gpui-component 的触发条件；目录数量是本机实测）。60 遍连跑和六个变异没有重复。

### Y13. History 顶部多了一条 32px 的 "Commit history" 标题，与基线不符

- **位置**：`history.rs:337`、`:345-368`；`docs/design/09-workbench-redesign.md:7`
- **描述**：基线写的是“History 不再有重复标题栏和紧凑图切换”，M3 设计输入里主区是“上「分支图 + 提交表」”。现在 navigator 里已有高亮的 "History"，主区顶上又有一条标题。它存在的理由是放失败信息和重试按钮。
- **建议**：失败信息放进列表的空状态区域，或者主区顶部只在失败时出现的一条细提示，去掉常驻的标题。外观还没有实机看过，建议一并核对。
- **验证**：已验证（对照文档与代码；没有看实际界面）。

## 🟢 优化建议（可选）

- **O1. HEAD 读取跑了两次完整的工作副本 status**（`workbench.rs:395`、`history.rs:247`；`crates/gitwave-core/src/application/use_cases.rs:2656-2660`；`crates/gitwave-core/src/infrastructure/git/working_copy.rs:24-52`）
  - 第 6 条问的性能里，这一项比 `sync_list` 的克隆和每行的 `time_text` 重得多。`get_working_copy` 带着未跟踪文件、递归未跟踪目录、两处 rename 检测和 `ahead_behind`，只为取 `sha` 和 `branch`。每次切换仓库对同一个仓库跑两遍。都在后台线程上，不卡界面，但大仓库里 HEAD 标记和状态栏的分支名要等它跑完。
  - `sync_list`（`history.rs:299`）每页克隆全部提交两遍。10k 提交约 34 页，累计约 36 万次 `CommitSummary` 克隆，摊在主线程上估计每页 2–5 ms，低于一帧，随提交总数线性增长。
  - `row()`（`history.rs:483-560`）每行每帧一次 `time_text::ago` 和一次 `SystemTime::now()`。`List` 只渲染可见的行，按 28 行算是每帧几十微秒。对 10k 的列表不构成问题。
  - 另有一处不在本次改动里、但直接决定分页成本：`infrastructure/git/history.rs:554` 每取一页都把已解码的全部提交克隆一遍并重新分配 lane。10k 提交累计约 18 万行，后台线程上可接受；100k 提交（`SNAPSHOT_LIMIT`）累计约 1600 万行。
  - **建议**：给 HEAD 元数据单独一个轻量用例（`repo.head()` 加 peel），Workbench 读一次，把 sha 交给 History；`sync_list` 改成共享 `Arc<Vec<CommitSummary>>`。测性能预算时把 core 的分页成本单独量。耗时都是按代码估算的，没有测量。

- **O2. 相对时间不会随时间刷新**（`history.rs:417`、`workbench.rs:537`）
  - 行里的 "5m ago" 和状态栏的 "taken 5m ago" 只在重绘时重算，列表不动就一直不变。**建议**每分钟 `cx.notify()` 一次。

- **O3. 提交行（第 7 条）**
  - 主 ref 的选择（tag、本地分支、远端分支）与基线一致，有测试。HEAD 标记按 `working_copy.sha` 在已加载的提交里找，HEAD 不在已加载的页里时没有标记，合理。
  - 建议实机核对三处：ref 标签（`history.rs:508-518`）没有宽度上限也不截断，长分支名会挤压标题列；sha 列宽 46px（`:537`），7 个等宽字符在 12px 下约 50px，取决于主题的 mono 字体；日期列 64px（`:554`），`2026-11-20` 刚好放下。
  - 窗口最小 1024px、navigator 208–240px 时标题列约 400px，固定列不会出问题。更窄的情况要靠“窄面板下隐藏列”，计划里已经列为没做。
  - 无障碍标签（`row_label`，`:476-478`）只有标题和短 SHA，没有 HEAD、ref、作者和时间。
  - `CommitList` 为了 `Default` 让 `history` 可以是 `None`（`:382`、`:392`），这是一个表达不出来也不会发生的状态，显式构造即可。

- **O4. `Pages::push` 的“空的一页就算到底”现在对，加了搜索就不对**（`history.rs:66-73`）
  - 没有 filter 时每页都是 300 条，只有空仓库才会给出空页。带搜索之后，空页只是这一段扫描窗口里没有匹配，core 的游标会往前走，按现在的规则搜索结果会被截断。“无限取同一页”只在游标不前进时发生，更准确的判据是“空页且游标与上一页相同”。至少在注释和测试名里写明前提。

- **O5. 已翻译的字符串存进了状态**（`workbench.rs:245-248`、`:351-358`；`history.rs:277`）
  - `notice` 和 `Status::Failed { message }` 存的是翻译后的字符串，语言切换后重绘，这两处还是旧语言。**建议**存 `AppError`（实现了 `Clone`），渲染时再 `error_text::show`。

- **O6. 测试接口可以再收一收（第 4 条）**
  - `commits()` 是普通的只读访问器，合理。`generation()`、`ask_for_more`、`page_arrived_for_test` 是为了从 `workbench_tests.rs` 调到私有方法。把 History 的迟到测试挪进 `history.rs` 自己的 `mod tests`（用 `#[gpui_kit::test]` 开窗口），这三个 `pub(crate)` 都能去掉。给 `History` 注入数据源更彻底，但改动大，现在不值得。`app_state::init` 的守卫见 Y9。

- **O7. 小处**
  - `repo_chosen`（`workbench.rs:364-378`）点已经激活的标签也会重新加载历史并写一次库。
  - 没有 Workspace 时，"No workspaces" 在标签行（`:569`）和主区（`:618-622`）各出现一次。
  - `codes::usecases::BACKGROUND_JOB_FAILED` 放在语言设置的分节里（`error_codes/usecases.rs:65-66`），没有自己的标题。
  - `branches.graph.loading` 用 "..."，`loadingOlder` 和 `common.loading` 用 "…"。

- **O8. 两个开发版实例同时启动（第 1 条，推演）**（`snapshot.rs:119-158`）
  - staging 文件名带 pid，两个实例互不覆盖；但删除旧 sidecar 和 rename 是交错执行的。最坏的情况是 A 刚打开副本，B 删掉 A 正在用的 `-wal`、`-shm` 并替换 `state.db`。在 macOS / Linux 上 A 的文件描述符仍指向旧的 inode，自洽，不会损坏，只是 A 之后的写入落在已经 unlink 的文件上。record 与副本也可能各来自一个实例，下次启动会漏掉一次重新复制。
  - Windows 上删除或替换被占用的文件会失败，`copy` 返回错误，那个实例退回旧副本，staging 文件留下（Y8）。
  - 路径里的非 ASCII 和引号没问题。`to_string_lossy`（`:169`）遇到非 UTF-8 的目录名会把文件写到另一个路径，随后 rename 失败，属于降级。
  - **建议**在 plan 的“风险与未决项”里记一句“同时只运行一个开发版实例”，或者给副本目录加一个锁文件。

- **O9. 文档里会过期或说过头的地方（第 10 条）**
  - `docs/tech/architecture/00-overview.md:3`：“对照 `main` 当前的代码（2026-11-26）”，这批改动还没有提交，更不在 `main` 上。合并时改成提交号，或者现在写成分支名。
  - 同文件 `:59`：“换了目标就加一”对 HEAD 回调不成立（Y4），也没有提到 `head_arrived`。
  - `plan.md:196`：条件句写成了事实（Y9）。`plan.md:111`：三种状态之外还应该有“正常关闭、只读复制之后再启动一次”（Y1）。
  - 其余核对属实，见「验证记录」。

## 📝 总体评价

副本方案的核心是对的：只读 `VACUUM INTO` 不改正式版的 `state.db` 和已存在的 `-wal`，替换副本的顺序和失败处理都经得起推敲。需要处理的是三件事：Windows 上编译不过（R1，一行就能修，但不修 CI 必红）；“原库目录只被动了 `-shm`”这个说法与实测不符，连带 README 的承诺和 `Stamp` 的判定（Y1）；以及几处迟到结果和状态恢复的逻辑缺口（Y3、Y4、Y5、Y6）。日志写进正式版目录（Y2）和四个没有守卫的入口（Y9）是同一类问题里漏掉的部分，建议在开发版新增第一个迁移之前补上。

## 验证记录

### 验证过的

- **`cargo fmt --all -- --check`**：通过，无输出。
- **`cargo clippy --workspace --all-targets --locked -- -D warnings`**：`Finished`，无告警。
- **`cargo test --workspace --all-targets --locked`**：`gitwave` 53 passed；`gitwave-core` 409 passed、2 ignored。与实现方一致。
- **`cargo test -p gitwave-core -- --list`**：411 个测试。`domain` 29 + `application` 63 = 92；`infrastructure` 313；`host` 3；`lib.rs` 的守卫 3。与 engineering 文档的分类一致，92 + 313 + 3 + 3 = 409 + 2。
- **文案**（脚本比对 `crates/gitwave/locales/` 的 50 个文件）：每种语言 25 个文件、1133 条，中英键集合相同；占位符 349 处，没有不匹配；新增 6 条、新增占位符 4 处。与 tech-selection 文档一致。
- **plan 的接口事实**：`load_more_threshold` 默认 20，触发条件是 `visible_end >= entities_count.saturating_sub(threshold)` 且 `has_more`；`ListState::set_selected_index` / `scroll_to_item` 需要 `&mut Window`。均读自 gpui-component 0.11.0 源码。
- **只读复制实验**（临时 Rust 程序，rusqlite `=0.40.2` bundled，SQLite 3.53.2，与仓库锁定版本相同；照 `snapshot.rs:161-173` 的 `vacuum_into` 写了同样的三步，没有调用仓库里的代码）。原库目录每个场景前后比对文件名、字节数、mtime、摘要：
  - 场景 1 写入方开着、日志未回填：只有 `-shm` 变。
  - 场景 2 正常关闭后、无写入方：`state.db` 不变；**新增 `state.db-wal`（0 字节）和 `state.db-shm`（32768 字节）**。
  - 场景 3 崩溃后只剩 `-wal`：`state.db`、`-wal` 不变，新增 `-shm`。
  - 场景 4 崩溃后 `-wal` 与 `-shm` 都在：只有 `-shm` 变。
  - 场景 5 已完全回填的 `-wal` 残留：只有 `-shm` 变，`-wal` 没被删。
  - 场景 6 目标路径含 `'`、中文、`?`、`#`：成功。
  - 场景 8 连续两次只读复制：第一次之后 `-wal` 从不存在变为 0 字节；第二次之后它的字节数和 mtime 都不变。
  - 场景 9 只读复制留下文件后，一个读写连接打开、读到数据、写入、关闭都正常，关闭时删掉 `-wal` 和 `-shm`；之后再只读复制一次，两个文件又出现。
  - 场景 7 目标已存在且非空：`VACUUM INTO` 报 "file is not a database"。`snapshot.rs:123` 在复制前先删 staging，所以不受影响。
- **`VACUUM INTO` 输出文件的权限**：实测 0644（umask 022）。
- **用系统 Python 的 SQLite 3.54.0 重做了场景 2–5**：结论相同。
- **捆绑的 `sqlite3.c`（3.53.2）**：`pagerOpenWalIfPresent` 在主库 0 字节且 `-wal` 存在时会删除 `-wal`（`:62960-62961`，没有只读判断）。Python 实测 WAL 模式下第一次写入之后主库就是 4096 字节，所以有效的 WAL 不会遇到这个分支。
- **R1 的最小复现**：`rustc` 在“非 unix”分支报 `E0603`，在"unix"分支通过。
- **正式版的版本**：`8466d55` 的 `Cargo.lock` 里 `libsqlite3-sys 0.38.2` / `rusqlite 0.40.2`，与现在相同；迁移都是 0001–0004。
- **libgit2 1.9.7**（`libgit2-sys-0.18.8+1.9.7/libgit2/src/libgit2/repository.c:42`）：`git_repository__validate_ownership` 默认为 `true`。
- **调用者**（全仓 grep）：`state_dir()` 的调用者只有 `bootstrap::open_app_context()` 和 `sqlite::open()`；`open()` 只被 `SqliteAppSettingsRepo::open()` 调用；这三者没有生产调用者；`tracing::init()` 的调用者只有 `logging.rs:21`。
- **临时目录**：本机 `$TMPDIR` 里有 2999 个 `gitwave-shell-test-*`，抽样的一个 52 KB。
- **仓库状态**：结束时 `git status --short` 仍是 39 行（34 个已修改，5 个未跟踪），与开始时相同。`/tmp` 下原有的 `gw-*.txt` 和 `gw-review-tools` 没有动。

### 没有验证的

- 没有运行应用。界面外观、窄面板下各列的表现、选中样式、读屏器的朗读都没有看。
- Windows 与 Linux。本机只装了 `aarch64-apple-darwin` 和 `wasm32-unknown-unknown`，R1 是按代码加最小复现判断的，没有对仓库做交叉 `cargo check`。
- `refresh` 第二次返回 `Taken`：是按 `snapshot.rs:94-104` 和场景 8 的实测推演的，没有直接调用 `refresh`。
- 两个实例同时启动、Windows 上 rename 或删除被占用的文件、非 UTF-8 的目录名：都是读代码的推演。
- 跨 SQLite 版本的兼容。“留下文件之后正式版仍可读写”只用同版本验证过；正式版与开发版现在恰好是同一个版本。
- 性能预算（冷启动、10k commit）：O1 里的耗时都是估算。
- 切换仓库后滚动位置的实际表现（Y5 的一半）。
- 六个变异、测试程序连跑 60 遍：没有重复。
- 游标过期、仓库列表读取失败、所有者不同的仓库：没有构造这些场景运行。
- 实机上正式版目录的状态：按约束没有访问。


## 处理结果：M3 切片一

| 编号 | 发现 | 处理 | 核实 |
|---|---|---|---|
| 🔴 R1 Windows 上编译不过 | `lock_down_db_files` 的非 Unix 版本仍是私有的 | 两个版本都 `pub(super)` | 本机没有 Windows 的目标，没法交叉编译。PR #247 首轮 CI 的 Windows 任务通过 |
| Y1 留下空的 `-wal`，文档不符，每次多复制 | 空日志不算变化（`Stamp::of`）；四处文档改成与实测一致。补了两条测试：原库正常关闭后连续刷新两次，第二次是「没变」；原库被复制之后还能照常打开、读写、关闭 | 新测试第一次运行就证实了审查者的实测和推演。变异「空日志算变化」被抓住 |
| Y2 日志写进正式版目录 | 日志改到 `GitWave-dev/logs/`；应用启动时最先初始化日志 | 实机：修好之后的两次启动只写进开发版自己的目录。没有加 `build` 字段，文件分开了就够了 |
| Y3 回到同一个 Workspace 时活动仓库退回 | `Session::keep_active` 记下激活的仓库；只有变了才写库 | 模型和壳层各一条测试；两条变异都被抓住 |
| Y4 HEAD 的回调用错了代数 | HEAD 有自己的代数。HEAD 由壳层读一次，交给历史（`History::head_is`），历史不再自己读 | 测试 `a_head_that_arrives_for_the_repository_shown_before_is_dropped`；变异被抓住 |
| Y5 选中与滚动不重置 | `History::show` 在这次更新结束后重置列表的选中并回到顶部；删掉不起作用的 `CommitList.selected` | 测试 `choosing_another_repository_clears_the_selection`；变异被抓住。「回到顶部」没有单独的断言，实机截图里新仓库的列表从第一行开始 |
| Y6 读不出仓库列表时停在「加载中」 | 提示改成一个小枚举（工作区、仓库列表、副本），各带错误对象；仓库列表失败时主区显示原因和「重试」，不再显示「加载中」 | 测试 `repositories_that_cannot_be_read_are_said_of_and_can_be_read_again` |
| Y7 游标过期时突然清空、重试没有上限 | 保留已显示的提交，后台从头重读并给一条提示；最多两次，之后走失败和重试按钮 | `Pages` 的四条测试和壳层的一条；变异「无限重读」被抓住 |
| Y8 复制失败不可见；中间文件权限 0644、放不下时不清理 | 复制失败的原因进主区顶部的提示，状态栏用警示色写「没有更新到最新」；`VACUUM INTO` 之后立刻收紧权限；之后任何一步失败都删掉中间文件 | 应用和 core 各一条测试；变异「放不下时留文件」被抓住。权限本身没有测试 |
| Y9 四个能打开正式库的入口没有守卫 | 四个入口的文档写明开发版不得调用；计划里的说法改成条件句 | 核对过现在两边的迁移相同。没有加 `#[deprecated]`：它们是将来正式版的入口，不该标成弃用。守卫也只盖住应用这一层，core 的入口靠文档 |
| Y10 没有关闭 libgit2 的所有权校验 | 启动时调用 `disable_git_owner_validation`，放在日志初始化之后，它那一行日志不会丢 | 没有所有者不同的仓库可试。Windows 实机验证时专门找一个 |
| Y11 四条旧文案说的是做不到的事 | 另加三条只读版的说法；缺失仓库的无障碍标签只说「路径缺失」；没有工作区时标签行不再重复「暂无工作区」。四条旧文案原样保留，记在技术选型里 | 四条旧文案不再被这一步引用 |
| Y12 测试缺口与清理 | 取页改走真实的滚动路径，并因此发现列表的回调里不能更新历史，改成派发任务。补了游标过期、HEAD 标记与迟到的 HEAD、失败与重试（历史、仓库列表各一条）、两个仓库的往返、重选当前标签、清掉选中。应用测试改用 `tempfile`，删掉了此前留下的三千多个目录 | 15 个变异全部被抓住。core 里更早的三处手写临时目录没有改，这一步加的两处改用了 `tempfile`。没有覆盖的：所有者不同的仓库、Windows 上的文件占用、两个实例同时启动 |
| Y13 常驻的「提交历史」标题 | 去掉；失败和重读时才有一条提示 | 实机截图 |
| O1 HEAD 读两遍、每次克隆全部提交、每帧算相对时间 | HEAD 只读一次。其余没做，记在计划的风险里 | — |
| O2 相对时间不随时间刷新 | 没做，记在计划的风险里 | — |
| O3 提交行 | ref 标签限宽并截断，SHA 列 56px，日期列 96px，三列都禁止换行；无障碍标签带上 HEAD、ref、作者、时间；`CommitList` 改成显式构造 | 日期列是实机看出来的：80px 时最后一个字符掉到第二行，被行高切掉。审查者估计 64px「刚好放下」，实际不够 |
| O4 空页就算到底 | 改成「空页且游标不前进」才算到底 | 测试两条；变异被抓住 |
| O5 翻译后的字符串存进状态 | 改存错误对象，显示时再翻译 | 测试断言的是当前语言下的文字 |
| O6 测试接口 | 去掉了一个（`ask_for_more`），加了两个观察用的（滚到末尾、读选中行）。其余留着：把迟到的测试挪进历史模块要连窗口夹具一起搬，这一步不值得 | — |
| O7 小处 | 重选当前标签不再重读、不再写库；没有工作区时标签行不再重复；错误码挪到自己的分节。`branches.graph.loading` 的三个点是旧实现带来的文字，没改 | 测试 `choosing_the_tab_of_the_repository_already_shown_changes_nothing`；变异被抓住 |
| O8 两个实例同时启动 | 没做，记在计划的风险里 | — |
| O9 文档 | 架构总览的日期、迟到的结果（三处）、状态的说法；计划的四处；README 的承诺与实现一致，不再自相矛盾 | — |

### 处理过程中另外发现并更正的

- 启动顺序：关闭所有权校验原来放在日志初始化之前，它那一行日志会丢。调成日志最先。
- 走真实的滚动路径后，gpui 报「正在更新的实体不能再次更新」：列表的 `load_more` 在列表自己的更新里被调用，这次更新的延迟回调里列表还在绘制。改成用 `cx.spawn_in` 派发任务。
- 测试的临时目录名用「进程号加毫秒」，两条并行的测试拿到同一个目录、互相改坏对方的文件。改用 `tempfile`。
- 实机：满一周后显示成日期的那一列太窄（见 O3）。
- 「失败与重试」的壳层测试（Y12 里补的那一条）等错了条件：它在本机连跑 60 遍都通过，到 PR #247 的 CI 上，macOS 与 Ubuntu 才失败。改走真实路径，等到重试的那一页出现在屏幕上为止。顺着用「给读取加延迟」又查出 13 条同样在赌时序的测试，一并改了。经过见[计划](./plan.md) 2026-11-27 的记录，约定见[工程约定](../../tech/engineering/00-overview.md#测试策略测试金字塔)。

### 修复后的验证

`make check` 通过，`gitwave` 67 个测试、`gitwave-core` 412 个通过、2 个忽略；应用的测试程序连跑 60 遍全部通过，跑前跑后比对，副本和正式版的库都没有被改动（其中那条「失败与重试」当时是靠断言抢在答案之前通过的，见上一节末条，已重写）。十五个变异各自让对应的测试失败。实机（macOS）：启动后正式版的 `state.db` 和 `state.db-wal` 哈希与启动前一致；窗口截图按原始分辨率核对了布局和各列；开发版的日志只写进自己的目录。

### 仍然没有验证的

- Windows 与 Linux 的实机。R1 的修复、`snapshot` 的十条测试和 `git2` 的夹具在这两端的 CI 上跑过了（PR #247，提交 `24d85ca`），没有在真实的桌面上试过。
- 所有者不是当前用户的仓库（Y10）；两个开发版实例同时启动（O8）；非 UTF-8 的目录名。
- 读屏器的朗读：提交行在无障碍树里是没有名字的组，`ListItem::access_label` 给的名字读屏接口读不到，原因没有查。
- 性能预算（冷启动、10k commit）；窄窗口下各列的表现。
