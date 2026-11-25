# GPUI Kit 技术调研：以纯 Rust 界面替换 Tauri + React 前端

> **作者**：AI 调研代理（技术调研员子代理），未经人工复核
> **状态**：草案（调研）
> **最近更新**：2026-11-25
> **性质**：调研笔记，不是选型决策。事实以一手来源为准，判断处单独标注；没有构建、运行或实测任何 gpui-kit 代码。

## 结论摘要

**一句话结论**：gpui-kit 的组件目录和三端平台层基本覆盖 GitWave 的界面需求，背后有出货中的商业应用和三端 CI；但它仍是 0.x，改名以来 12 周内发了 6 个 minor 且每个都带破坏性变更（间隔已缩短到 1 到 2 周），而且**打包、代码签名、自动更新完全不由框架提供**。现有证据支持把它列为候选并进入原型验证，不支持「依赖已成熟、可以直接开工重写」的判断。

支撑这个结论的关键事实（均已由一手来源确认，来源见各节）：

| 事实 | 数据 | 出处 |
|---|---|---|
| 版本与节奏 | 最新 `0.11.0`（2026-11-23）。2026-09-03 至 2026-11-23 共 81 天发了 14 个 release，其中 6 个 minor 版本（0.6 到 0.11），minor 均带破坏性变更 | [crates.io](https://crates.io/api/v1/crates/gpui-kit)、[releases](https://github.com/longbridge/gpui-kit/releases) |
| GPUI 来源 | 默认后端是 `gpui-pre =0.3.14`，即 Zed GPUI 的未修改快照（`zed@f126950`，2026-11-22），精确钉版，每周跟随上游 | [Cargo.toml](https://github.com/longbridge/gpui-kit/blob/main/Cargo.toml)、[FAQ](https://gpui-kit.com/docs/faq) |
| 生产应用 | Longbridge Pro 官网提供 macOS（Apple Silicon，macOS 11+）、Windows（x64，Win10+）、Linux（x64）三端下载；另有下游 Windows 应用在 [#3863](https://github.com/longbridge/gpui-kit/issues/3863) 自述已出货 | [longbridge.com/desktop](https://longbridge.com/desktop) |
| 三端 CI | 测试矩阵为 `aarch64-apple-darwin` / `x86_64-linux-gnu` / `x86_64-windows-msvc` | [ci.yml](https://github.com/longbridge/gpui-kit/blob/main/.github/workflows/ci.yml) |
| 打包与更新 | 官方文档明写框架不负责安装包、签名、更新策略与安装 | [Packaging](https://gpui-kit.com/docs/packaging)、[Auto Update](https://gpui-kit.com/docs/auto-update) |
| Diff 组件 | unified / split、字符级强调、语法着色、行号、跨文件虚拟化齐备，但 0.8.0（2026-10-19）才加入，且是只读组件 | [Diff](https://gpui-kit.com/component/diff) |
| 无障碍 | 官方对比页自评为 Partial；对话框标题未暴露给读屏器等问题仍未关闭 | [Comparison](https://gpui-kit.com/docs/comparison)、[#3595](https://github.com/longbridge/gpui-kit/issues/3595) |
| 性能数据 | 没有官方的启动时间、内存占用或大列表基准；只有 Hello World 约 12 MB 的粗估 | [Comparison](https://gpui-kit.com/docs/comparison) |

进入决策前必须用原型回答的四个问题（本次调研无法回答，见 H 节「未核实」）：数万 commit 的提交图与 diff 在 gpui-kit 上的实际帧时间；Apple Silicon 冷启动能否低于 1 秒；中文输入法在对话框与多行输入中的表现；三端出包、签名、更新链路的实际工作量。

## A. 身份与治理

**是什么**。`gpui-kit` 是 Rust GUI 框架 crate，由 Longbridge 维护，仓库 [longbridge/gpui-kit](https://github.com/longbridge/gpui-kit)，官网 <https://gpui-kit.com>。它是一个聚合入口：依赖它即同时得到 GPUI、`gpui-base`（无样式行为与基础设施层）、`gpui-component`（完整样式组件库）和默认图标资源。同一仓库还发布 `gpui-component`、`gpui-base`、`gpui-kit-assets`、`gpui-component-macros`、`gpui-shell`（JavaScript 扩展运行时）、`gpui-webview`、`gpui-fps`（[根 Cargo.toml](https://github.com/longbridge/gpui-kit/blob/main/Cargo.toml)、[README](https://github.com/longbridge/gpui-kit/blob/main/README.md)）。

**与 gpui-component 的关系**：同一仓库的改名与继任，两者并存。证据：

- GitHub API 请求 `longbridge/gpui-component` 会重定向到 `longbridge/gpui-kit`；该仓库创建于 2024-06-13。
- crates.io 上 `gpui-component` 创建于 2025-02-06，共 40 个版本，仍与 `gpui-kit` 同步发到 `0.11.0`；`gpui-kit` crate 创建于 2026-09-03，共 17 个版本。新版本号从 0.6.0 起算，即 2026-09-03 之前叫 gpui-component，之后多了一个聚合 crate（[crates.io gpui-kit](https://crates.io/api/v1/crates/gpui-kit)、[crates.io gpui-component](https://crates.io/api/v1/crates/gpui-component)）。

**许可证**。软件源码与文档中的代码示例为 Apache-2.0；文档正文与插图另以 CC BY 4.0 提供，采用该文档内容需署名。仓库含 `LICENSE-APACHE` 与 `LICENSE-DOCS.md` 两份许可文件；GitHub API 的 license 字段返回的是 `NOASSERTION`（[README · License](https://github.com/longbridge/gpui-kit/blob/main/README.md)、[LICENSE-DOCS.md](https://github.com/longbridge/gpui-kit/blob/main/LICENSE-DOCS.md)）。

**发布历史与节奏**（[GitHub releases](https://github.com/longbridge/gpui-kit/releases)、[crates.io](https://crates.io/api/v1/crates/gpui-component/versions)）：

| 时期 | 状态 |
|---|---|
| 2025-02-06 → 2025-10-09 | 只有 0.1.0 一个版本，空档 8 个月 |
| 2025-10 → 2025-12 | 0.2 → 0.5，约每三周一个 minor |
| 2026-02-05 → 2026-09-03 | 停在 0.5.1，空档 7 个月 |
| 2026-09-03 至今 | 改名后每周一发版：0.6.0、0.6.1、0.6.2、0.6.4、0.6.6、0.7.0、0.7.1、0.7.3、0.8.0、0.8.1、0.9.0、0.9.1、0.10.0、0.11.0。minor 之间的间隔依次是 4、3、2、2、1 周 |

GitHub release 共 22 个，最早为 v0.2.0（2025-10-09）。下载量（crates.io，2026-11-25 查询）：`gpui-kit` 526,785；`gpui-component` 累计 776,673、近 90 天 680,823；`gpui-pre` 744,801。作为量级参照，`tauri` 累计 47,753,439、近 90 天 20,061,578。

**是否 pre-1.0 与破坏性变更策略**。是，全部 crate 都是 0.x。官方约定（[CONTRIBUTING.md](https://github.com/longbridge/gpui-kit/blob/main/CONTRIBUTING.md)）：

- 补丁版本「可以改变行为，但必须保持与上一版的源码兼容」；不兼容的改动推迟到下一个非补丁版本。
- 例外是「明确授权的上游 GPUI API 迁移」，即补丁版也可能因上游变化而破坏。
- 团队「让 GPUI Kit 适配上游，而不是给 GPUI 打补丁」，所以上游 GPUI 的 API 变化会直接变成 gpui-kit 的破坏性变更。

实际破坏性变更量（按 release 正文机械统计）：

| 版本 | 日期 | 破坏性变更条目 |
|---|---|---|
| [0.7.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.7.0) | 2026-09-28 | 16 |
| [0.8.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.8.0) | 2026-10-19 | 16 个小节，其中多条是「签名不变但默认行为改变」（例如 macOS 上滚动回弹默认开启、嵌套滚动区不再联动） |
| [0.9.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.9.0) | 2026-11-02 | 主题相关结构体全部变为 `#[non_exhaustive]`，结构体字面量写法不再编译 |
| [0.10.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.10.0) | 2026-11-16 | 3 |
| [0.11.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.11.0) | 2026-11-23 | 3（Linux X11 变为 feature、`chrono` 换成 `jiff`、上游 GPUI API 变化） |

**贡献者与集中度**。GitHub contributors API 列出 268 位贡献者。前两位是 `huacnlee`（1548 次提交）和 `madcodelife`（502 次）；默认分支提交总数按 commits API 分页估算约 2995，两人合计约 68%。这是按 GitHub 统计口径的估算，不是精确值。

**issue 健康**（2026-11-25 查询）：open / closed issue 为 85 / 750，open / closed PR 为 7 / 3144；近 30 天新建 issue 104 个、关闭 129 个。

**生产应用**：

- **Longbridge Pro**。gpui-kit README 称其「从第一天起就用 GPUI Kit 构建并在公开出货的商业桌面应用中持续打磨」；[官网下载页](https://longbridge.com/desktop)列出 macOS（Apple Silicon，macOS 11 or later）、Windows（X64，Win10 or later）、Linux（X64）。两点保留：该说法出自 gpui-kit 自述，下载页没有提到所用 UI 框架；Longbridge Pro 当前使用的 gpui-kit 版本未核实。
- **下游 Windows 应用**。[#3863](https://github.com/longbridge/gpui-kit/issues/3863) 的作者自述「我们在 gpui + gpui-kit 上出货一个 Windows 桌面应用（邮件 / 日历客户端）」，并提交了一批 Windows 平台补丁建议。同帖另一位用户报告在 Linux 上使用 gpui-component。

## B. 与 GPUI 的关系

**gpui-kit 不是 GPUI 的 fork**，而是叠在 GPUI 之上的组件与应用层（[FAQ](https://gpui-kit.com/docs/faq)）。

**它依赖哪一份 GPUI**：Zed 官方仓库里的 GPUI，以 `gpui-pre-*` 为包名发布到 crates.io 的**未修改快照**，不是 GPUI-CE。

- 根 `Cargo.toml` 钉的是 `gpui = { package = "gpui-pre", version = "=0.3.14" }`，平台层 `gpui-pre-platform =0.3.14`，宏、sum-tree 等同为精确版本。注释说明用精确版本是因为 caret 需求曾让周更快照把应用带到未编译过的版本上（issue #3156）；CI 用 `script/check-gpui-pin.ts` 拒绝非精确的版本要求（[Cargo.toml](https://github.com/longbridge/gpui-kit/blob/main/Cargo.toml)）。
- docs.rs 显示 `gpui-pre 0.3.14` 对应 `zed@f126950`，该提交日期为 2026-11-22；gpui-kit 0.11.0 次日发布，跟随上游约一天（[docs.rs gpui-pre-platform](https://docs.rs/crate/gpui-pre-platform/0.3.14/source/src/)、[zed@f126950](https://github.com/zed-industries/zed/commit/f126950f9d3b62666f97437ae5d4ccac2272c4db)）。
- 为什么不用 Zed 自己发的 `gpui` crate：crates.io 上的 [`gpui`](https://crates.io/api/v1/crates/gpui) 最新是 0.2.2，发布于 2025-10-22，此后没有新版本。FAQ 的说法是「Zed 不定期在 crates.io 发布 GPUI」（[FAQ](https://gpui-kit.com/docs/faq)）。

**跟随上游的节奏**。仓库的 [release-gpui.yml](https://github.com/longbridge/gpui-kit/blob/main/.github/workflows/release-gpui.yml) 每周一（cron `0 0 * * 1`）检查 Zed 的 `crates/gpui*` 并发布新快照；gpui-kit 随后发版把钉版抬到新快照。`gpui-pre` 的 0.3.9 到 0.3.14 均为周一发布。后果是 gpui-kit 每周都在追一个会变 API 的上游，0.11.0 的破坏性变更里就有三条直接来自上游（[v0.11.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.11.0)）。

**GPUI-CE 的位置**。GPUI-CE 是独立社区项目 [gpui-ce/gpui-ce](https://github.com/gpui-ce/gpui-ce)（创建于 2025-12-12，Apache-2.0）。crates.io 的 [`gpui-ce`](https://crates.io/api/v1/crates/gpui-ce) 最新 0.3.0（2026-10-20），共 4 个版本，累计下载 30,679。gpui-kit 不使用它；抽查 crates.io 的依赖记录：`gpui-component` 的 0.1.0 依赖 `gpui ^0`，0.3.0 与 0.5.1 依赖 `gpui ^0.2.2`，0.6.0 依赖 `gpui-pre ^0.3.1`。这几个抽查点都没有出现 GPUI-CE；其余版本未逐一核对（[gpui-component 0.6.0 依赖](https://crates.io/api/v1/crates/gpui-component/0.6.0/dependencies)）。

**GPUI Fast**。可选的另一个后端，是 Longbridge 维护的 [longbridge/gpui-fast](https://github.com/longbridge/gpui-fast)，描述为「把 GPUI 从 Zed monorepo 抽出为独立 workspace，并重做布局引擎以保留帧间未变化的部分」。它很新：仓库创建于 2026-09-24，crate 创建于 2026-10-06，最新 `0.1.19`，7 周内发了 19 个版本。默认后端仍是 gpui-pre；FAQ 建议在测出性能问题后再启用。[#3863](https://github.com/longbridge/gpui-kit/issues/3863) 报告即使启用 `gpui-fast`，`gpui-base`、`gpui-component`、`gpui-kit-assets` 仍会把 `gpui-pre` 拉进依赖图（0.10.0 上仍如此）。GitWave 不应把 GPUI Fast 计入方案。

**GPUI 自身在三端的渲染后端**（来源为 gpui-pre 0.3.14 的各平台 crate 源码与依赖声明，见来源列表）：

| 平台 | 渲染 / 文本 | 无障碍适配 | 出处 |
|---|---|---|---|
| macOS | Metal；CoreText（需 `font-kit` feature，`gpui-kit` 已启用） | gpui-pre 依赖 `accesskit`；`gpui-base` 另外给 `NSWindow` 装了一个 `accessibilityHitTest:` 转发器 | [gpui-pre-macos](https://docs.rs/crate/gpui-pre-macos/0.3.14/source/src/platform.rs)、[macos_accessibility.rs](https://github.com/longbridge/gpui-kit/blob/main/crates/base/src/macos_accessibility.rs) |
| Windows | DirectX（`directx_renderer.rs` 等）、DirectWrite | `accesskit` + `accesskit_windows` | [gpui-pre-windows](https://docs.rs/crate/gpui-pre-windows/0.3.14/source/src/platform.rs) |
| Linux | wgpu，启用 `vulkan`、`gles`、`metal` 特性；fontconfig | `accesskit_unix` | [gpui-pre-wgpu Cargo.toml](https://docs.rs/crate/gpui-pre-wgpu/0.3.14/source/Cargo.toml)、[gpui-pre-linux Cargo.toml](https://docs.rs/crate/gpui-pre-linux/0.3.14/source/Cargo.toml) |

GPUI 的成熟度可由 Zed 本身佐证：Zed README 写明在 macOS、Linux、Windows 三端提供下载（[zed README](https://github.com/zed-industries/zed/blob/f126950f9d3b62666f97437ae5d4ccac2272c4db/README.md)）。但 Zed 出货的是自己的应用代码，不等于 gpui-kit 组件层在三端同样成熟。

## C. 三端平台支持

**声明与测试矩阵**。仓库 [AGENTS.md](https://github.com/longbridge/gpui-kit/blob/main/AGENTS.md) 的 Platform Support 为 macOS（aarch64、x86_64）、Linux（x86_64）、Windows（x86_64），并说明 CI 在每个平台跑完整测试。Linux aarch64 与 Windows on ARM 不在列表内。[ci.yml](https://github.com/longbridge/gpui-kit/blob/main/.github/workflows/ci.yml) 的测试矩阵确为 macOS / Ubuntu / Windows 三个 runner。

**macOS**

- 版本要求存在三个互相不一致的数字，原因未核实：gpui-kit [Installation](https://gpui-kit.com/docs/installation) 写「macOS 15 or later」；Zed 快照的 `.cargo/config.toml` 设 `MACOSX_DEPLOYMENT_TARGET = "12.0"`（[zed .cargo/config.toml](https://github.com/zed-industries/zed/blob/f126950f9d3b62666f97437ae5d4ccac2272c4db/.cargo/config.toml)）；Longbridge Pro 官网写 macOS 11 or later。GitWave 当前 `tauri.conf.json` 没有 `minimumSystemVersion`，改造后最低系统版本大概率会抬高，抬高到多少未核实。
- 需要能跑 Metal 的环境。
- 标题栏：`TitleBar` 在 macOS 上使用原生红绿灯按钮，位置自动设为 (9px, 9px)，左侧留 80px 内边距，默认透明；`app_owns_titlebar_drag` 让标题栏接管拖动与双击（[TitleBar](https://gpui-kit.com/component/title-bar)）。

**Windows**

- Windows 10 或更高，需要 MSVC 工具链、Windows SDK 与 CMake（[Installation](https://gpui-kit.com/docs/installation)）。
- [#3863](https://github.com/longbridge/gpui-kit/issues/3863) 报告了一批下游在 `gpui-pre-windows 0.3.9` 上遇到的问题：启动时黑窗 80–200 ms；实时缩放时内容滞后于窗口边缘并出现黑边；路径中间纹理每窗口常驻约 33 MB（1600×1000，集显上计入进程内存）；`layout_line` 每行 40–100 µs。该 issue 仍为 open，当前 0.3.14 是否已修复未核实。
- 标题栏由 `TitleBar` 自绘窗口控制按钮。
- 从 Linux 交叉编译到 Windows 会失败（[#3902](https://github.com/longbridge/gpui-kit/issues/3902)，open）。

**Linux**

- 官方只在 Ubuntu 24.04 上验证。依赖脚本安装 `libfontconfig-dev libwayland-dev libxkbcommon-x11-dev libx11-xcb-dev libssl-dev libzstd-dev libvulkan1 vulkan-validationlayers` 等（[install-linux.sh](https://github.com/longbridge/gpui-kit/blob/main/script/install-linux.sh)）。
- 文档要求「在有可用 Vulkan 驱动的 Wayland 或 X11 图形会话中运行，仅安装 `libvulkan1` 不够」。CI 的做法是装 `mesa-vulkan-drivers`（lavapipe 软件光栅化）来跑渲染测试，说明软件 Vulkan 路径可用。wgpu 后端声明了 `gles` 特性，但在没有 Vulkan 驱动的机器上是否会自动回退到 GLES 未核实。
- Wayland 与 X11 默认都启用；0.11.0 起 X11 是默认 feature `x11`，Wayland-only 构建可以不再链接 `libxkbcommon-x11`。
- **GTK**：`gpui-pre-linux 0.3.14` 的依赖清单里没有 GTK，也不需要 WebKitGTK。安装脚本里的 `libwebkit2gtk-4.1-dev` 只服务于可选的 `gpui-webview` crate；`gpui-kit` 自身的依赖里没有它（[crates/kit/Cargo.toml](https://github.com/longbridge/gpui-kit/blob/main/crates/kit/Cargo.toml)）。
- **glibc 与发行版兼容性**：gpui-kit 是库，不声明 glibc 下限。它自己发布的 `gpui-shell` 二进制在 `ubuntu-latest` 上构建，只可作量级参考（[release-shell.yml](https://github.com/longbridge/gpui-kit/blob/main/.github/workflows/release-shell.yml)）。GitWave 需要自己在目标最老发行版上构建并验证。
- Linux 上的 `NativeMenu` 不是原生菜单，而是回退为 GPUI 绘制的 `PopupMenu`，并被裁剪在窗口内；应用菜单栏在 Linux 与 Windows 上由 gpui-kit 的 `AppMenuBar` 在标题栏内绘制（[Native Extensions](https://gpui-kit.com/docs/native-extension)、[app_menu_bar.rs](https://github.com/longbridge/gpui-kit/blob/main/crates/component/src/menu/app_menu_bar.rs)）。

**IME（中日韩输入法）**。平台层实现存在，来源为 gpui-pre 0.3.14 源码：

- X11：有 `xim_handler` 模块，走 XIM。
- Wayland：实现了 `zwp_text_input_v3`（text-input-v3 协议），合成器不支持时该能力为空（[wayland/client.rs](https://docs.rs/crate/gpui-pre-linux/0.3.14/source/src/linux/wayland/client.rs)）。
- macOS 与 Windows：由各自平台 crate 处理，本次未逐行核对；佐证是 issue 列表里有 Windows IMM32 与 macOS 拼音的专门修复。

维护者在用中文输入法做测试，IME 是近期活跃修复的区域。已关闭的例子：组合输入时按下 Enter 被误当成提交（#3255）、Windows 上输入框失焦后组合状态不结束（#4024）、候选框位置跳变（#3286、#3939）。仍然 open 的两个与 GitWave 场景直接相关：

- [#3605](https://github.com/longbridge/gpui-kit/issues/3605)（2026-10-19）：macOS 拼音候选窗在焦点转移到对话框后仍然残留。
- [#3456](https://github.com/longbridge/gpui-kit/issues/3456)（2026-10-12）：Textarea 中 Command-Return 会提交原生的 dead-key 组合文本。

桌面端没有找到 CJK 显示乱码的未决 issue；标题含「Chinese characters display garbled」的两条（#2242、#2123）都是 WebAssembly 网页端的问题，已关闭。

**CJK 字体回退与自定义字体**

- 桌面端按系统字体集合解析：macOS CoreText、Windows DirectWrite、Linux fontconfig；任意已安装字体可按名称直接使用。
- 字体族加载成功但缺少某个字形时，由 `Font::fallbacks` 控制回退。文档明确提醒「字体族名称不保证所有字形都存在」，并要求在每个目标平台用真实的拉丁、CJK、emoji 混排内容检查（[Fonts](https://gpui-kit.com/docs/fonts)）。
- 可通过 `cx.text_system().add_fonts(...)` 在首帧前嵌入字体，通过 `Theme::update` 设置 `font_family`、`mono_font_family`、`font_size`。
- 没有找到针对桌面端 CJK 回退的专项 issue 或测试声明，三端实际回退效果未核实。

**无障碍**

- GPUI 通过 AccessKit 向平台辅助技术提供无障碍树；组件在交互层内置 role、name、state、action（[Accessibility](https://gpui-kit.com/docs/accessibility)）。
- 官方对比页把 GPUI Kit 的 Accessibility 标为 **Partial**，并说明无障碍需要应用自己在目标平台验证。
- 基础能力是 2026 年 10 月之后才系统补齐的：输入框文本对读屏器不可见（#3818、#3870、#3894）、Label 文本不可见（#3698）、DataTable 缺少表格结构与行名称（#3647）都是在 2026-10 到 2026-11 之间关闭的。
- 仍为 open：对话框标题未暴露给读屏器且未标记为模态（[#3595](https://github.com/longbridge/gpui-kit/issues/3595)）；没有 live region（[#3659](https://github.com/longbridge/gpui-kit/issues/3659)、[#4094](https://github.com/longbridge/gpui-kit/issues/4094)）；禁用控件被读成启用（[#3660](https://github.com/longbridge/gpui-kit/issues/3660)）；Sidebar 菜单项没有 AccessKit role（[#2968](https://github.com/longbridge/gpui-kit/issues/2968)）；滚出视口的节点点击会落到错误位置（[#3658](https://github.com/longbridge/gpui-kit/issues/3658)）。
- 没有找到 VoiceOver、Narrator 或 Orca 实测通过的一手记录。

**高 DPI**。`window.scale_factor()` 给出逻辑像素到物理像素的换算，窗口跨屏移动时会变化；原生菜单与对话框在平台边界处自行换算（[Window](https://gpui-kit.com/docs/window)）。Windows 上 per-monitor DPI 切换的表现未核实。

**原生菜单与应用菜单**。macOS 上 GPUI 提供 `cx.set_menus(...)` 设置系统菜单栏，示例应用同时调用它并把同一份菜单定义交给 `AppMenuBar`（[app_menus.rs](https://github.com/longbridge/gpui-kit/blob/main/crates/story/src/app_menus.rs)）。文档提醒键位绑定要先于 `set_menus` 注册，绑定变化后需重建菜单（[Keybinding](https://gpui-kit.com/docs/keybinding)）。这与 GitWave 当前「macOS 用原生菜单、Windows / Linux 用标题栏内 AppMenuBar」的结构一致。

**文件与文件夹对话框**。GPUI 的 `cx.prompt_for_paths(PathPromptOptions { files, directories, multiple, prompt })` 在三个平台都委托给平台层实现：macOS 用 `NSOpenPanel`，Windows 用 `IFileOpenDialog`，Linux 调用 xdg-desktop-portal（`ashpd`）。Linux 这条路径依赖用户环境里有 portal 服务；除 ashpd 之外是否还有回退，本次只核对了函数开头，未核实（源码见来源列表）。

**打开文件与链接**。`cx.open_url`、`cx.open_with_system(path)`、`cx.reveal_path(path)`（在 Finder 等文件管理器中显示）。

**剪贴板**。`cx.write_to_clipboard` / `cx.read_from_clipboard`，Linux 另有 primary selection。

**外部拖放**（把文件从别的应用拖进窗口）。未找到支持证据：gpui-kit 仓库里搜不到 `external_paths`，`gpui-pre 0.3.14` 的 `DragMoveEvent` 公开方法只有 `drag` 与 `dragged_item`（[DragMoveEvent](https://docs.rs/gpui-pre/0.3.14/gpui/struct.DragMoveEvent.html)）。GitWave 当前也没有这个功能，不构成缺口；应用内部的拖动排序由 GPUI 的 `on_drag_move` / `on_drop` 承担。

**通知**。`cx.show_system_notification` 与组件的 `Notification::system()` 可投递到系统通知中心，文档标记为 platform-dependent：macOS 需要真实的 `.app` 包且在可信位置，Windows 需要先设置应用身份，Linux 需要 session D-Bus 与通知守护进程，且 Linux 实现既不替换也不撤回旧通知（[SystemNotification](https://gpui-kit.com/docs/system-notification)）。

**多窗口**。支持，每个窗口有自己的 `Root`、焦点与输入派发；`window.on_window_should_close` 返回 `false` 可以取消关闭（[Multi Window](https://gpui-kit.com/docs/multi-window)）。

**暗色模式跟随**。GPUI 的 `App` 提供 `window_appearance` 与 `set_window_appearance`；gpui-kit 的示例用 `ThemeMode::Light` / `Dark` 两个 action 切换主题（[app_menus.rs](https://github.com/longbridge/gpui-kit/blob/main/crates/story/src/app_menus.rs)）。「跟随系统」把两者接起来的现成机制，本次没有找到对应文档，未核实。

**菜单栏常驻模式**。未找到证据：gpui-kit 仓库里搜不到 `LSUIElement`、`NSStatusItem` 或 tray 相关实现。另外，GitWave 仓库源码里也没有找到状态栏常驻或托盘实现，README 的「menu bar app mode」在代码中具体指什么，需要向需求方确认；若指 macOS 系统菜单栏里的应用菜单，则已覆盖（见上一条），若指托盘常驻，则是缺口。

## D. 组件覆盖（对照界面需求 1–12）

覆盖情况取值：**覆盖** / **部分覆盖**（原语齐备，需要应用自己组合或补一块）/ **不覆盖** / **未找到证据**。

| 需求 | 覆盖情况 | 证据链接 |
|---|---|---|
| 1a 三栏工作台，分栏可拖动调整 | 覆盖。`h_resizable` / `v_resizable` 支持水平、垂直、嵌套、尺寸范围与固定面板；另有独立的 `resize_handle`。键盘调宽需应用自己绑定 Action | [Resizable](https://gpui-kit.com/component/resizable) |
| 1b 可横向滚动的仓库标签栏，支持拖动排序 | 部分覆盖。标签溢出时自动启用滚动，`track_scroll` 可显式把某个标签滚入视野；文档描述提到 reordering，但具体的排序 API 未核实，拖动排序可由 GPUI 的 `on_drag_move` / `on_drop` 自行实现 | [Tabs](https://gpui-kit.com/component/tabs)、[InteractiveElement](https://docs.rs/gpui-pre/0.3.14/gpui/trait.InteractiveElement.html) |
| 2a History 提交图：自绘节点、分叉与合并曲线、横向平移 | 部分覆盖。没有现成的提交图组件，需自建；绘制原语齐备：`canvas()` 元素配合 `PathBuilder`，支持 `line_to`、`curve_to`、`cubic_bezier_to`、`arc_to`、`stroke`、`dash_array` 与 `paint_path` | [Paint](https://gpui-kit.com/docs/paint)、[PathBuilder](https://docs.rs/gpui-pre/0.3.14/gpui/struct.PathBuilder.html) |
| 2b 虚拟化的提交列表（数万 commit） | 覆盖。`List` 基于 delegate 模式，支持虚拟化、分区、搜索、无限滚动；另有 `VirtualList` 支持可变高度与横向虚拟列表。数万行下的实际流畅度未核实 | [List](https://gpui-kit.com/component/list)、[VirtualList](https://gpui-kit.com/component/virtual-list) |
| 2c 虚拟化的 diff 内容（大文件不卡界面） | 覆盖。`Diff` 的行跨所有文件虚拟化，语法与行内强调在后台线程逐文件准备 | [Diff](https://gpui-kit.com/component/diff) |
| 3 Diff 视图：分栏与统一、字符级高亮、语法着色、行号、等宽对齐、超宽行横向滚动 | 覆盖。`DiffMode::Unified` / `Split`，`split_ratio`，`DiffInlineUnit::Character`，tree-sitter / syntect / 自定义 highlighter，行号，主题等宽字体，代码两栏一起横向滚动而行号留在原位。限定：只读；不计算 diff，由应用提供 patch 文本；超过 1000 字节的行不着色 | [Diff](https://gpui-kit.com/component/diff)、[v0.10.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.10.0) |
| 4 图片 diff：png / jpg / gif / webp / bmp / ico / svg | 部分覆盖。`img()` 经 `image` crate 解码 PNG、JPEG、WebP、GIF、BMP、TIFF、ICO 等，SVG 会被光栅化，动态 GIF / WebP 可播放；可传入已持有的编码字节（`Arc<Image>`）。但 `Diff` 对二进制文件只显示摘要，左右并排的图片对比界面需自建 | [Images](https://gpui-kit.com/docs/image)、[Diff](https://gpui-kit.com/component/diff) |
| 5 自定义绘制能力（画布类元素） | 覆盖。见 2a | [Paint](https://gpui-kit.com/docs/paint) |
| 6 多行文本编辑器（带首行长度提示）、单行输入、下拉、复选、开关 | 覆盖，首行长度提示需自绘。`Textarea` 支持固定行数、自动增长、软换行、`submit_on_enter`，不带 prefix / suffix 装饰；`Input` 支持校验、掩码、清理按钮；另有 `Select`、`Checkbox`、`Switch` | [Textarea](https://gpui-kit.com/component/textarea)、[Input](https://gpui-kit.com/component/input)、[Select](https://gpui-kit.com/component/select) |
| 7a 树形侧栏，支持分组折叠 | 覆盖。`Tree` 支持展开折叠、键盘导航、懒加载、自定义渲染；`Sidebar` 支持 Group、Header、Footer、折叠 | [Tree](https://gpui-kit.com/component/tree)、[Sidebar](https://gpui-kit.com/component/sidebar) |
| 7b 交互式 rebase 的拖动排序 | 部分覆盖。`Tree` 文档未提及拖放，需用 GPUI 的 `on_drag_move` / `on_drop` 自行实现 | [Tree](https://gpui-kit.com/component/tree) |
| 8 对话框与模态（受控关闭）、命令面板、右键菜单、tooltip、toast、进度与取消 | 覆盖，取消按钮需自加。`window.open_dialog` / `close_dialog` 由 `Root` 管理层；`Command` 是带分组与键位提示的命令面板，触发键由应用绑定；`PopupMenu` 作右键菜单；`Notification` 默认 5 秒自动消失；`Progress` 有确定与不确定两种。注意对话框标题对读屏器未暴露（#3595） | [Dialog](https://gpui-kit.com/component/dialog)、[Command](https://gpui-kit.com/component/command)、[Notification](https://gpui-kit.com/component/notification)、[Progress](https://gpui-kit.com/component/progress) |
| 9 自绘标题栏、原生应用菜单、菜单栏常驻模式、全局与局部快捷键 | 大部分覆盖。`TitleBar` + `AppMenuBar` + `cx.set_menus` 见 C 节；键位用 `cx.bind_keys`，`None` 上下文即全应用，带上下文谓词的即局部快捷键，`secondary-s` 自动映射为 ⌘ / Ctrl。菜单栏常驻模式未找到证据，需求本身也待确认 | [TitleBar](https://gpui-kit.com/component/title-bar)、[Keybinding](https://gpui-kit.com/docs/keybinding) |
| 10 主题（浅色 / 深色 / 跟随系统）、界面字体与等宽字体、中英文即时切换 | 覆盖，跟随系统的接线方式未核实。主题系统带 JSON 主题文件与 38 个预设；字体见 C 节；i18n 走 `rust-i18n`，组件内置 `en`、`zh-CN`、`zh-HK`、`zh-TW`、`ja` 等文案，示例通过 `SelectLocale` action 在运行时切换 | [Theme](https://gpui-kit.com/component/theme)、[Fonts](https://gpui-kit.com/docs/fonts)、[I18N](https://gpui-kit.com/docs/i18n) |
| 11 中文 / 日文 / 韩文输入法与 CJK 字体回退 | 部分覆盖。平台层实现齐备，CJK 回退依赖系统字体；两个与输入法相关的 issue 仍 open，三端实测未核实 | 见 C 节 IME 与字体 |
| 12 无障碍（VoiceOver / Narrator / Orca） | 部分覆盖。官方自评 Partial，多处缺口仍 open，三端读屏器实测无一手记录 | [Accessibility](https://gpui-kit.com/docs/accessibility)、[#3595](https://github.com/longbridge/gpui-kit/issues/3595) |

补充确认几项被特别问到的能力：

- **表格**。`DataTable` 支持虚拟滚动、可调整 / 可移动 / 固定列、排序、行 / 列 / 单元格选择、键盘导航、右键菜单、无限加载（[DataTable](https://gpui-kit.com/component/data-table)）。
- **富文本样式**。`TextView` 渲染 Markdown 与 HTML；`Label` 支持高亮样式；`SelectableText` 支持可选中文本与高亮。
- **主题**。所有组件经 `ActiveTheme` 取色；0.9.0 起主题结构体为 `#[non_exhaustive]`，后续加字段不再破坏调用方。
- **测试能力**。官网所说的 UI integration testing 是：在无头窗口里渲染真实组件，模拟指针与键盘输入，断言状态、焦点、布局和 AccessKit 属性（[Testing](https://gpui-kit.com/docs/test)）。可以在三端 CI 上跑，仓库自己的 CI 就是这样做的；像素级的离屏渲染测试只在 macOS（Metal）与 Linux（wgpu，可用 lavapipe）上运行，Windows 平台 crate 没有无头渲染器。局限是测试平台的文本输入不模拟完整的 OS 输入法组合，也不验证原生对话框、菜单定位与读屏器播报。
- **现成的 Git 示例**。仓库有一个 [tig 示例](https://github.com/longbridge/gpui-kit/tree/main/examples/tig)：用 `List` + `Resizable` + `Diff` 浏览最近 200 个 commit，含键盘导航和状态栏，通过系统的 `git` 命令取数据。它没有提交图，也不做任何写操作，可作为「列表 + diff」部分的起步参考。

## E. 应用基础设施

**gpui-kit 不提供打包、签名与自动更新**。原文：

- Packaging：「GPUI Kit supplies the application and UI layers; your application owns its package identity, icons, external files, installer, signing, and release tests.」文档给出的做法是手工 `mkdir` + `cp` 组装 `.app` 并手写 `Info.plist`、用 Inno Setup 做 Windows 安装器、用 `dpkg-deb` 打包（[Packaging](https://gpui-kit.com/docs/packaging)）。
- Auto Update：「GPUI Kit does not select a release or replace an installed app.」推荐的 `self_update` crate 只适用于「可写的单文件便携安装」；对 `.app`、Inno Setup、deb / rpm，文档把更新交给对应的安装所有者（[Auto Update](https://gpui-kit.com/docs/auto-update)）。
- gpui-kit 仓库里搜不到 `cargo-packager`。其 release workflow 只发布 crate 与 `gpui-shell` 二进制，未见签名或公证步骤。

**Tauri 现有能力与替代方案对照**（需求 14）：

| Tauri 现有能力 | gpui-kit / GPUI 侧对应 | 状态 |
|---|---|---|
| 三端安装包（DMG、NSIS、deb、rpm、AppImage） | 无。需自建或采用第三方打包工具 | 缺口 |
| macOS 签名与公证 | 无。文档给出 `codesign` → `hdiutil` → `notarytool` → `stapler` 的命令序列，但需自己接入 CI | 需自建 |
| 带签名的应用内自动更新与重启 | 无更新器。GPUI 有 `App::restart` 与 `set_restart_path`。`self_update` 只校验 checksum，不等于发布者签名，且不适用于打包安装 | 缺口 |
| 文件 / 文件夹选择对话框 | `cx.prompt_for_paths`，三端平台层实现 | 覆盖 |
| 用系统工具打开文件与链接 | `cx.open_url`、`cx.open_with_system`、`cx.reveal_path` | 覆盖 |
| 读取系统信息 | `cx.platform_info()`；返回字段未逐项核对 | 覆盖 |
| 重启应用 | `App::restart` | 覆盖 |
| 前后端事件（`clone-progress`、`sync-progress`、`credential-storage`） | 没有 IPC 层，改为进程内的 GPUI channel 或 Entity 事件 | 需改造 |
| 原生应用菜单 | `cx.set_menus` + `AppMenuBar` | 覆盖 |

**第三方打包工具**。[cargo-packager](https://github.com/crabnebula-dev/cargo-packager) 由 CrabNebula 维护，自述为「Rust executable packager, bundler and updater」。crates.io 最新 [`0.11.8`](https://crates.io/api/v1/crates/cargo-packager) 发布于 2025-11-27，近一年没有发版；仓库 2026-11-24 仍有提交，510 stars，38 个 open issue。它与 gpui-kit 应用的兼容性、是否覆盖签名 / 公证与带签名的更新，本次未核实。

**对 GitWave 的含义**。现在的发布链路是 tag 触发 → tauri-action 出三端包 → macOS 签名公证 → 生成带 minisign 签名的 `latest.json`。改造后这条链路要整体重建，工作量不在界面重写的估算之内。

## F. 与现有 Rust 后端集成

**gpui-kit 应用的工程结构**。最小形态是单 crate：`Cargo.toml` 只依赖 `gpui-kit`，`main.rs` 里 `gpui_kit::application().with_assets(...).run(...)`，先 `gpui_kit::init(cx)` 再 `gpui_kit::open_window`（[hello_world](https://github.com/longbridge/gpui-kit/blob/main/examples/hello_world/src/main.rs)）。入门文档建议应用变大后按 feature 拆 crate，每个 feature 自己持有 model 与 view（[Getting Started](https://gpui-kit.com/docs/getting-started)）。`gpui-kit` 要求 Rust 1.92 或更高，edition 2024（[Installation](https://gpui-kit.com/docs/installation)）。

**异步模型**（[Task](https://gpui-kit.com/docs/task)）：

- 前台 executor 在主线程上轮询 `cx.spawn` / `cx.spawn_in` 的 future；前台任务里的同步函数会阻塞 UI。
- 后台 executor 承接 `cx.background_spawn` 的 `Send` 工作，不能直接改 Entity。
- `Task<T>` 被 drop 即取消。
- GPUI 核心不依赖 tokio：`gpui-pre 0.3.14` 的依赖是 `async-task`、`scheduler`、`postage`、`async-channel` 等。

**与 tokio 共存**。上游 Zed 仓库有一个 100 行的桥接 crate [`gpui_tokio`](https://github.com/zed-industries/zed/blob/f126950f9d3b62666f97437ae5d4ccac2272c4db/crates/gpui_tokio/src/gpui_tokio.rs)：`gpui_tokio::init(cx)` 创建一个 2 个 worker 线程的 tokio runtime，`Tokio::spawn(cx, fut)` 在 tokio 上跑 future 并返回 GPUI `Task`，GPUI task 被取消时 tokio 任务也取消。它没有发布到 crates.io（`gpui-pre-tokio` 不存在），使用的话需要自己 vendoring。

**GitWave 后端的依赖面**（本仓库源码统计）：

- `src-tauri/src` 共 69 个 `.rs` 文件、约 2.6 万行；只有 `lib.rs`（2225 行，268 处 `tauri::`）和 `macos_window.rs`（72 行，4 处）引用 Tauri。`domain`、`infrastructure` 等目录下的 67 个文件不引用 Tauri。
- `lib.rs` 中有 130 个 `#[tauri::command]`、118 个 `async fn`、111 处 `tauri::State`、12 处 `AppHandle`、4 处 `tauri::async_runtime::spawn_blocking`、2 处 `tokio::spawn`。
- 事件只有三种（`clone-progress`、`sync-progress`、`credential-storage`），共 6 处 emit。
- 后端依赖 `reqwest`（0.12 与 0.13），而 reqwest 需要 tokio 运行时。

**判断**（基于以上事实的工程判断，未经验证）：后端可以保留。把 `lib.rs` 从 Tauri 命令层改写成普通库 API 是主要工作，领域与应用层基本不动。两个需要设计的地方：一是 `State<AppContext>` 托管的服务要换成由 GPUI `Global` 或应用根 Entity 持有；二是所有 reqwest 调用和 `async fn` 要跑在一个 tokio runtime 上，结果经 channel 回到 GPUI 前台。没有发现原则性的集成障碍。

## G. 性能与体积

**没有可引用的官方基准**。gpui-kit 没有发布启动时间、内存占用或大列表帧率数据。对比页自己说明：表格里的标记「描述可用性，不是性能」，没有跨框架的 FPS 基准；Hello World 的「约 12 MB」是「不同构建条件下的近似参考，不是同方法的基准」（[Comparison](https://gpui-kit.com/docs/comparison)）。

| 指标 | 已有的一手数据 | 出处 |
|---|---|---|
| 最小二进制 | Hello World 约 12 MB（官方粗估） | [Comparison](https://gpui-kit.com/docs/comparison) |
| 较大应用的体积 | `gpui-shell` 发布包压缩后 22.1 MB（macOS arm64）到 31.9 MB（Linux x86_64）；Cargo.toml 注释称 Linux 上未 strip 为 106 MB，strip 后 78 MB | [v0.11.0 assets](https://github.com/longbridge/gpui-kit/releases/tag/v0.11.0)、[Cargo.toml](https://github.com/longbridge/gpui-kit/blob/main/Cargo.toml) |
| Diff 解析 | 6.5 MB、200 个文件的 patch 在 release 构建下解析约 15 ms（文档自述） | [Diff](https://gpui-kit.com/component/diff) |
| Windows 内存 | 每窗口路径中间纹理约 33 MB（1600×1000，集显），来自下游在 0.3.9 上的测量 | [#3863](https://github.com/longbridge/gpui-kit/issues/3863) |
| 空闲 CPU | 下游测得空闲 CPU 3.0%、vsync 线程每秒唤醒 158 次；对方打补丁后降到 0.0% | [#3863](https://github.com/longbridge/gpui-kit/issues/3863) |

**冷启动**。没有任何 gpui-kit 应用的冷启动数据，GitWave 的「Apple Silicon 低于 1 秒」无法从现有来源判断。

**无 GPU 或远程桌面环境**

- Linux：文档要求可用的 Vulkan 驱动；CI 用 lavapipe 证明软件 Vulkan 可行。普通用户的远程桌面或虚拟机里 wgpu 能否自动回退到 GLES 未核实。
- Windows：[#3863](https://github.com/longbridge/gpui-kit/issues/3863) 的下游专门做了 `GPUI_FORCE_WARP=1` 补丁以便在虚拟机和 RDP 中使用，说明上游在这类环境下可能有问题；上游当前的行为未核实。
- macOS：需要 Metal。

## H. 风险清单

按可能导致项目失败的程度排序。「已确认」表示有一手来源；「未核实」表示找不到一手来源或本次没有验证。

| # | 风险 | 状态 | 证据 |
|---|---|---|---|
| 1 | **API 不稳定**。0.x 版本，改名以来 12 周内 6 个 minor 全部带破坏性变更，间隔已缩短到 1 到 2 周；每周跟随会变 API 的上游 GPUI；补丁版允许改行为，上游迁移还可能破例。重写期间每次升级都有返工成本 | 已确认 | A 节发布表、[CONTRIBUTING.md](https://github.com/longbridge/gpui-kit/blob/main/CONTRIBUTING.md)、[B 节](#b-与-gpui-的关系) |
| 2 | **发布链路要整体重建**。安装包、macOS 签名公证、带签名的自动更新与重启都没有框架级方案，`self_update` 不适用于打包安装 | 已确认 | [Packaging](https://gpui-kit.com/docs/packaging)、[Auto Update](https://gpui-kit.com/docs/auto-update) |
| 3 | **性能目标没有证据**。冷启动低于 1 秒、数万 commit 的提交图、大文件 diff 都没有任何 gpui-kit 上的测量数据 | 已确认无数据 | G 节 |
| 4 | **无障碍不完整**。官方自评 Partial，对话框标题、live region、Sidebar 菜单项等缺口仍 open，三端读屏器无实测记录 | 已确认 | C 节无障碍 |
| 5 | **维护集中且有停更先例**。前两位贡献者约占 68% 提交；发版史上两次 5 到 8 个月的空档；`gpui-kit` 这个 crate 只有 11 周历史 | 已确认 | A 节 |
| 6 | **关键组件很新**。`Diff` 在 0.8.0（2026-10-19）才加入，只读，不负责计算 diff；没有提交图组件；图片 diff 需自建 | 已确认 | D 节、[v0.8.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.8.0) |
| 7 | **Windows 平台缺口**。启动黑窗、实时缩放滞后、每窗口约 33 MB 额外纹理等下游报告的问题仍 open | 已确认；当前版本是否修复未核实 | [#3863](https://github.com/longbridge/gpui-kit/issues/3863) |
| 8 | **输入法边缘问题**。macOS 拼音候选窗在焦点移入对话框后残留；多行输入的 Command-Return 问题。界面是双语的，这两处正好落在 GitWave 的对话框与提交信息输入场景 | 已确认 | [#3605](https://github.com/longbridge/gpui-kit/issues/3605)、[#3456](https://github.com/longbridge/gpui-kit/issues/3456) |
| 9 | **测试资产与测试能力**。前端约 2.45 万行非测试代码需要用 Rust 重写，现有 Vitest / Testing Library 测试无法迁移；新的 UI 测试在 Windows 上没有像素校验，无头测试不覆盖输入法 | 已确认 | [Testing](https://gpui-kit.com/docs/test) |
| 10 | **reqwest 等依赖需要 tokio 桥接**。GPUI 核心不带 tokio 运行时，而 GitWave 后端的 `reqwest` 与 118 个 `async fn` 需要；上游的 `gpui_tokio` 桥接 crate 未发布到 crates.io | 已确认 | F 节 |
| 11 | **macOS 最低系统版本不确定**。文档 15+、上游 12.0、Longbridge Pro 11+ 三个数字不一致 | 未核实 | C 节 macOS |
| 12 | **无 GPU 环境行为不明**。Linux 文档要求 Vulkan 驱动；Windows 的 WARP 路径靠下游补丁 | 未核实 | G 节 |
| 13 | **Linux 发行版与架构覆盖**。只在 Ubuntu 24.04 验证，没有 glibc 基线声明，不支持 Linux aarch64 | 基线未核实；架构列表已确认 | C 节 Linux、[AGENTS.md](https://github.com/longbridge/gpui-kit/blob/main/AGENTS.md) |
| 14 | **菜单栏常驻模式**。若需求确指托盘常驻，gpui-kit 没有现成实现；GitWave 当前代码里也没有找到对应实现，需求本身先要确认 | 未找到证据 | C 节 |

## 来源列表

查询日期均为 2026-11-25。crates.io 与 GitHub 的数字是查询时的快照。

**crates.io API**

- [gpui-kit](https://crates.io/api/v1/crates/gpui-kit)
- [gpui-component](https://crates.io/api/v1/crates/gpui-component) 与 [versions](https://crates.io/api/v1/crates/gpui-component/versions)
- [gpui-pre](https://crates.io/api/v1/crates/gpui-pre)、[gpui-fast](https://crates.io/api/v1/crates/gpui-fast)
- [gpui（Zed 官方发布）](https://crates.io/api/v1/crates/gpui)、[gpui-ce](https://crates.io/api/v1/crates/gpui-ce)
- [cargo-packager](https://crates.io/api/v1/crates/cargo-packager)
- [tauri](https://crates.io/api/v1/crates/tauri)

**gpui-kit 仓库**

- [README](https://github.com/longbridge/gpui-kit/blob/main/README.md)、[根 Cargo.toml](https://github.com/longbridge/gpui-kit/blob/main/Cargo.toml)、[crates/kit/Cargo.toml](https://github.com/longbridge/gpui-kit/blob/main/crates/kit/Cargo.toml)
- [CONTRIBUTING.md](https://github.com/longbridge/gpui-kit/blob/main/CONTRIBUTING.md)、[AGENTS.md](https://github.com/longbridge/gpui-kit/blob/main/AGENTS.md)、[LICENSE-DOCS.md](https://github.com/longbridge/gpui-kit/blob/main/LICENSE-DOCS.md)
- [ci.yml](https://github.com/longbridge/gpui-kit/blob/main/.github/workflows/ci.yml)、[release-gpui.yml](https://github.com/longbridge/gpui-kit/blob/main/.github/workflows/release-gpui.yml)、[release-shell.yml](https://github.com/longbridge/gpui-kit/blob/main/.github/workflows/release-shell.yml)、[release.yml](https://github.com/longbridge/gpui-kit/blob/main/.github/workflows/release.yml)
- [GitHub releases](https://github.com/longbridge/gpui-kit/releases)：[v0.7.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.7.0) · [v0.8.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.8.0) · [v0.9.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.9.0) · [v0.10.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.10.0) · [v0.11.0](https://github.com/longbridge/gpui-kit/releases/tag/v0.11.0)
- 源码：[hello_world](https://github.com/longbridge/gpui-kit/blob/main/examples/hello_world/src/main.rs) · [tig 示例](https://github.com/longbridge/gpui-kit/tree/main/examples/tig) · [app_menus.rs](https://github.com/longbridge/gpui-kit/blob/main/crates/story/src/app_menus.rs) · [app_menu_bar.rs](https://github.com/longbridge/gpui-kit/blob/main/crates/component/src/menu/app_menu_bar.rs) · [macos_accessibility.rs](https://github.com/longbridge/gpui-kit/blob/main/crates/base/src/macos_accessibility.rs) · [install-linux.sh](https://github.com/longbridge/gpui-kit/blob/main/script/install-linux.sh)
- issue：[#3863](https://github.com/longbridge/gpui-kit/issues/3863) · [#3605](https://github.com/longbridge/gpui-kit/issues/3605) · [#3456](https://github.com/longbridge/gpui-kit/issues/3456) · [#3595](https://github.com/longbridge/gpui-kit/issues/3595) · [#3659](https://github.com/longbridge/gpui-kit/issues/3659) · [#4094](https://github.com/longbridge/gpui-kit/issues/4094) · [#3660](https://github.com/longbridge/gpui-kit/issues/3660) · [#3658](https://github.com/longbridge/gpui-kit/issues/3658) · [#2968](https://github.com/longbridge/gpui-kit/issues/2968) · [#3902](https://github.com/longbridge/gpui-kit/issues/3902)

**gpui-kit.com 文档**

- [FAQ](https://gpui-kit.com/docs/faq) · [Installation](https://gpui-kit.com/docs/installation) · [Getting Started](https://gpui-kit.com/docs/getting-started) · [Packaging](https://gpui-kit.com/docs/packaging) · [Auto Update](https://gpui-kit.com/docs/auto-update)
- [Native Extensions](https://gpui-kit.com/docs/native-extension) · [SystemNotification](https://gpui-kit.com/docs/system-notification) · [Window](https://gpui-kit.com/docs/window) · [Multi Window](https://gpui-kit.com/docs/multi-window) · [Context](https://gpui-kit.com/docs/context) · [Task](https://gpui-kit.com/docs/task)
- [Testing](https://gpui-kit.com/docs/test) · [Accessibility](https://gpui-kit.com/docs/accessibility) · [Fonts](https://gpui-kit.com/docs/fonts) · [TextSystem](https://gpui-kit.com/docs/text-system) · [I18N](https://gpui-kit.com/docs/i18n) · [Keybinding](https://gpui-kit.com/docs/keybinding)
- [Paint](https://gpui-kit.com/docs/paint) · [Images](https://gpui-kit.com/docs/image) · [Comparison](https://gpui-kit.com/docs/comparison) · [FPS Monitor](https://gpui-kit.com/docs/fps)
- 组件：[Diff](https://gpui-kit.com/component/diff) · [TitleBar](https://gpui-kit.com/component/title-bar) · [Resizable](https://gpui-kit.com/component/resizable) · [Tabs](https://gpui-kit.com/component/tabs) · [List](https://gpui-kit.com/component/list) · [VirtualList](https://gpui-kit.com/component/virtual-list) · [DataTable](https://gpui-kit.com/component/data-table) · [Tree](https://gpui-kit.com/component/tree) · [Sidebar](https://gpui-kit.com/component/sidebar) · [Textarea](https://gpui-kit.com/component/textarea) · [Input](https://gpui-kit.com/component/input) · [Select](https://gpui-kit.com/component/select) · [Dialog](https://gpui-kit.com/component/dialog) · [Command](https://gpui-kit.com/component/command) · [Notification](https://gpui-kit.com/component/notification) · [Progress](https://gpui-kit.com/component/progress) · [Theme](https://gpui-kit.com/component/theme)

**GPUI（gpui-pre 0.3.14，docs.rs）**

- [App](https://docs.rs/gpui-pre/0.3.14/gpui/struct.App.html) · [PathBuilder](https://docs.rs/gpui-pre/0.3.14/gpui/struct.PathBuilder.html) · [DragMoveEvent](https://docs.rs/gpui-pre/0.3.14/gpui/struct.DragMoveEvent.html) · [InteractiveElement](https://docs.rs/gpui-pre/0.3.14/gpui/trait.InteractiveElement.html)
- [gpui-pre Cargo.toml](https://docs.rs/crate/gpui-pre/0.3.14/source/Cargo.toml) · [gpui-pre-platform 源码](https://docs.rs/crate/gpui-pre-platform/0.3.14/source/src/)（快照对应的 Zed 提交）
- [gpui-pre-macos platform.rs](https://docs.rs/crate/gpui-pre-macos/0.3.14/source/src/platform.rs)（`prompt_for_paths`、`restart`、`reveal_path`、`open_with_system`）
- [gpui-pre-windows platform.rs](https://docs.rs/crate/gpui-pre-windows/0.3.14/source/src/platform.rs)（`prompt_for_paths`）与 [Cargo.toml](https://docs.rs/crate/gpui-pre-windows/0.3.14/source/Cargo.toml)
- [gpui-pre-linux platform.rs](https://docs.rs/crate/gpui-pre-linux/0.3.14/source/src/linux/platform.rs)（`prompt_for_paths` 走 ashpd）、[wayland/client.rs](https://docs.rs/crate/gpui-pre-linux/0.3.14/source/src/linux/wayland/client.rs)（text-input-v3）、[x11.rs](https://docs.rs/crate/gpui-pre-linux/0.3.14/source/src/linux/x11.rs)（`xim_handler`）、[Cargo.toml](https://docs.rs/crate/gpui-pre-linux/0.3.14/source/Cargo.toml)
- [gpui-pre-wgpu Cargo.toml](https://docs.rs/crate/gpui-pre-wgpu/0.3.14/source/Cargo.toml)

**Zed 仓库（快照 `f126950`）**

- [提交 f126950](https://github.com/zed-industries/zed/commit/f126950f9d3b62666f97437ae5d4ccac2272c4db)
- [crates/gpui_tokio/src/gpui_tokio.rs](https://github.com/zed-industries/zed/blob/f126950f9d3b62666f97437ae5d4ccac2272c4db/crates/gpui_tokio/src/gpui_tokio.rs)
- [.cargo/config.toml](https://github.com/zed-industries/zed/blob/f126950f9d3b62666f97437ae5d4ccac2272c4db/.cargo/config.toml) · [README](https://github.com/zed-industries/zed/blob/f126950f9d3b62666f97437ae5d4ccac2272c4db/README.md)

**其他**

- [GPUI-CE 仓库](https://github.com/gpui-ce/gpui-ce) · [GPUI Fast 仓库](https://github.com/longbridge/gpui-fast) · [cargo-packager 仓库](https://github.com/crabnebula-dev/cargo-packager)
- [Longbridge Pro 下载页](https://longbridge.com/desktop)
- GitWave 仓库：`README.md`、`docs/design/00-overview.md`、`docs/design/08-history-first.md`、`docs/design/09-workbench-redesign.md`、`docs/pm/core/01-features.md`、`src-tauri/tauri.conf.json`、`src-tauri/capabilities/default.json`、`package.json`、`src-tauri/Cargo.toml`、`src-tauri/src/lib.rs`、`src-tauri/src/macos_window.rs`
