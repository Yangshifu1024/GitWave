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
