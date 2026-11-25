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
| 7 | Linux 依赖清单有疑点 | 采纳低风险的补全：加上 `libxcb-shape0-dev`、`libxcb-xfixes0-dev`，保留上游安装脚本的全部条目，注释改为与来源一致 | 已修改；未经托管环境验证 |
| 8 | 侧栏显隐让三栏宽度串位 | 三个面板始终存在，侧栏用 `resizable_panel().visible(..)` 隐藏 | 已修复，clippy、构建、启动通过；未手工验证往返 |

处理过程中另外修正的两处：

- 通读差异时发现 `docs/tech/engineering/00-overview.md` 里 `frontend-lint` 一行被误改成 `tsc --no-ese`，已恢复为与 `main` 一致的 `tsc --noEmit`。
- 验证 bump 脚本时有两次操作失误，都把版本文件改到了 0.11.1：一次是备份步骤在 zsh 下没有生效，一次是误在后台启动了脚本。两次都已还原，并用逐包比对确认 Tauri 锁文件相对 `main` 仍只有预期的差异，两个 workspace 的 `--locked` 解析通过。

修复后的验证：`make fmt-check`、`make lint`、`make test` 全部通过；前端 367 个测试，Tauri 集成测试 1 个，core 在两套锁文件下各 396 个（394 通过，2 忽略）。
