# GitWave 桌面界面迁移到 GPUI Kit

状态：进行中。基线 `004c9f4`（v0.11.0 发布后的 `main`）。M0、M1 经 `feature/gpui-kit-desktop` 合并（PR #243）；移除旧应用在 `chore/remove-tauri-app`；M2 在 `feature/gpui-kit-app-shell`（PR #246）：切片一、切片二，加实机验证后的一处无障碍修复（`ef0ef3a`），三端 CI 通过。macOS 上做过实机验证（见 2026-11-26 的记录）：M2 退出标准里的菜单、快捷键、主题、语言切换和中文输入法都通过；没在实机上试的是 ⌥⌘H、剪贴板三项、「前置全部窗口」和读屏器的朗读。M3 在 `feature/gpui-kit-read-only-workbench`，进行中：切片一（接入真实状态、壳层按基线布局重排、提交历史列表）已实现，经过独立审查并在 macOS 上实机看过，开在 PR #247，三端 CI 通过。决策见 [ADR 0007](../../tech/decisions/00-overview.md#0007--桌面界面迁移到-gpui-kit)，选型依据见 [调研笔记](../../tech/tech-selection/gpui-kit-research.md)。

## 目标与验收基线

用 GPUI Kit 重写桌面界面，把 GitWave 改成不依赖 Tauri、WebView 与 Node.js 前端的纯 Rust 应用。Rust 后端的领域、应用、基础设施三层保留，抽成独立 crate，由界面在进程内直接调用。

以 [v0.11.0 实际交付的工作台](../../design/09-workbench-redesign.md)和 [PM 范围](../../pm/core/02-scope.md)为基线。每条用户流程覆盖正常、加载、空态、失败、确认、取消与恢复。保留 Workspace 无目录实体、多 Workspace 并行与单 active repo，repo 级草稿和错误，Git 写操作显式确认及 AI 本地优先。迁移完成前不发布可用的版本：Tauri 应用与 Node.js 前端已于 2026-11-25 先行从 `main` 移除，旧实现留在提交 `8466d55`，只作对照。

## 原则

上一次迁移（Go + NSGo）暴露的四个问题，这次逐条对应：

- **选型先留记录。** 决策与备选写入 ADR 0007，依据写入调研笔记。
- **最没把握的先做。** 构建与运行、后端抽离、发布链路排在界面铺开之前，不等做到中后期才发现问题。
- **三端 CI 从第一个里程碑开始。** 不再出现只在本地验证、合并时才第一次在托管环境运行的工作流。
- **每个里程碑单独成 PR。** 合并前跑完等价检查并经 code-reviewer 审查，不再把多个里程碑压进一个不可回退的大 PR。

## 工程结构

```
Cargo.toml                 # 唯一的 workspace
crates/
├── gitwave-core/          # 领域、应用、基础设施与服务容器，不含界面框架类型
└── gitwave/               # GPUI Kit 应用：窗口、菜单、各功能的 model 与 view；locales/ 放界面文案
assets/icons/              # 应用图标
scripts/bump-version.sh    # 改 workspace 版本并刷新 Cargo.lock
```

## 实施里程碑

| 阶段 | 实施内容 | 退出标准 |
|---|---|---|
| **M0 工程骨架** | 根 workspace 与 `crates/gitwave`；`src-tauri` 排除在 workspace 之外 | gpui-kit 应用在 macOS 构建并启动；`src-tauri` 的构建、锁文件与检查不受影响；根 workspace 的格式与 clippy 通过 |
| **M1 后端抽离** | `crates/gitwave-core`；服务容器与用例 API 去掉 Tauri 类型；Tauri 应用改为薄封装；tokio 运行时桥接 | Tauri 应用的全部现有检查通过；core crate 不含任何 Tauri 类型；三端 CI 构建通过 |
| **M1b 移除旧应用** | 删除 Tauri 应用、React 前端与 Node.js 工具链；删除前把界面文案转换到 `crates/gitwave/locales/`、把桌面图标挪到 `assets/icons/`；Makefile、CI、dependabot、pre-commit、版本脚本与文档改为只有 Rust | `main` 上只剩 Rust 工具链；`make check` 与三端 CI 通过；两种语言各 1108 条文案齐全，错误码与文案双向对应 |
| **M2 应用壳** | 窗口、自绘标题栏、应用菜单、快捷键、主题、中英切换、状态栏、tokio 运行时接入、启动时初始化日志、窗口壳的交互测试（无头窗口） | 三端 CI 构建通过；菜单、快捷键、主题与语言切换可用；中文输入法在对话框与多行输入中实测 |
| **M3 只读工作台** | Workspace 与仓库标签、History 列表与提交图、提交详情、文件列表、Diff、Blame | 对照 v0.11.0 逐项核对布局与交互；冷启动、10k commit History 与 1 MB diff 达到性能预算 |
| **M4 Changes 与日常 Git** | 状态列表、Stage / Unstage、Commit / Amend、Discard、分支 / 标签 / 检出、Fetch / Pull / Push、克隆与初始化 | commit → push、切换 Workspace、操作中切仓、失败重试与外部 Git 竞态通过端到端验证 |
| **M5 高级工具** | Merge 与冲突编辑、Rebase（含交互式）、Cherry-pick / Revert、Stash、Reflog 恢复、Worktree、Remote、Submodule、LFS、Hooks、`.gitignore`、健康摘要 | 对照旧版校验暂停后重启、继续与取消及失败恢复后的仓库状态 |
| **M6 设置与 AI** | 设置各页、AI provider 与 Prompt 模板、命令面板、各 AI 入口 | 三端桌面交互与各 AI 用例通过；离线仅连接 loopback |
| **M7 发布链路** | 三端打包、签名与公证、带签名的应用内更新与重启、三端 CI 的构建与发布任务 | 干净系统可安装使用；从 v0.11.0 升级及回退可重现；应用与安装包通过各平台签名验证 |
| **M8 收尾** | 官网与下载入口指向新版本；发布技能按新流水线改写；按三端验收结果收尾文档与 ADR | 官网、README 与发布技能都只描述新版本 |

## 测试门禁

- `make check` 就是 CI 的等价物：`cargo fmt --all -- --check`、`cargo clippy --workspace --all-targets -- -D warnings`、`cargo test --workspace --all-targets`。CI（`.github/workflows/ci.yml`）在三端跑 clippy 与测试，rustfmt 只在 Ubuntu 上检查。旧实现的 367 个前端测试与 Tauri 集成测试随旧应用一起删除。
- 界面文案：`crates/gitwave/src/locales.rs` 的测试校验两种语言的键集合与占位符一致、每个错误码都有文案，并且文件能被 rust-i18n 加载。
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
- 旧版 README 所写的「menu bar app mode」指窗口内的菜单栏，还是常驻系统菜单栏的托盘模式。旧实现只有前者，gpui-kit 没有后者的现成实现；新应用是否需要托盘模式待定。
- `gitwave-core` 根上的 `#![allow(dead_code)]` 与 `#![allow(unused_imports)]`：旧实现删除后，core 的大量用例暂时没有调用方，这两条豁免要等新界面接上各功能后再逐模块收回。
- `cmds.*` 的 10 个错误码（等待后台任务结束失败）目前没有任何代码引用。提出它们的 Tauri 命令层已删除，新应用的处理器随功能迁移时再决定是否沿用；它们与语言文件里的文案互相绑定，要删就一起删。
- 外观是否提供「跟随系统」（旧实现有这一项）：gpui-kit 0.11.0 里没有找到它的接线方式，M2 只做了浅色 / 深色两项。
- 旧实现的语言选择不迁移，已按此处理：它存在 WebView 的 localStorage 里（`gitwave.language`），新应用读不到，v0.11.0 的用户要重新选一次语言。2026-11-26 汇报时列为待决，用户回复「按推荐继续」，没有单独对这一条表态。
- 「短小的本地写入在主线程同步做」换成磁盘数据库之后要实测一次写入耗时（`app_settings` 的连接是 WAL、5 秒 `busy_timeout`），再决定这条约定要不要加时长上限。
- `keyring` 的 4.x 改了 feature 名，清单里的 `apple-native` 等写法在 4.x 下解析不了。dependabot 配置忽略它的大版本升级；当前清单要求的是 `3`，所以这条忽略只是声明意图。升到 4.x 时要一起改 feature 名。
- dependabot 是否忽略 `gpui-kit` 的升级 PR。当前配置为忽略，理由与 ADR 0007 的「升级是独立任务」一致，代价是它的安全更新也不会自动提 PR。
- 一次只运行一个开发版实例。两个同时启动时，后启动的会替换先启动的正开着的副本：在 macOS 和 Linux 上先启动的那份自洽、不会损坏，但之后的写入落在已经被 unlink 的文件上；在 Windows 上后启动的替换失败，退回旧副本。这是推演，没有实测。要么接受这个约定，要么给副本目录加一把锁。
- 读 HEAD 用的是 `get_working_copy`，它带着整个工作副本的 status（未跟踪文件、重命名检测、ahead / behind），只为取分支名和 SHA。现在每次切换仓库读一次，在后台线程上，不卡界面；大仓库里 HEAD 标记和状态栏的分支名要等它跑完。M3 第三步量性能预算时，考虑给 HEAD 元数据单独一个轻量的用例。
- 相对时间（「5 分钟前」）只在重绘时重算，列表不动就一直不变。可以每分钟通知一次，这一步没做。

## M2 设计输入

实现 M2 之前已经对照 gpui-kit 0.11.0 源码核实的接口与约束，避免下次重新查：

- **菜单**：同一份 `Vec<Menu>` 定义同时交给 `cx.set_menus`（macOS 原生菜单栏）和 `GlobalState::global_mut(cx).set_app_menus(..)`（标题栏内的 `AppMenuBar`，Windows 与 Linux 使用），随后调用 `app_menu_bar.update(cx, |bar, cx| bar.reload(cx))`。`MenuItem::action` 可以链式 `.checked()`、`.disabled()`；编辑菜单直接复用 `gpui_kit::component::input` 的 `Undo`、`Redo`、`Cut`、`Copy`、`Paste`、`Delete`、`SelectAll` 等内置动作。切换语言或主题后重建菜单以刷新勾选项。参考上游 `crates/story/src/app_menus.rs`。
- **主题**：`Theme::change(ThemeMode::Light | Dark, Some(window), cx)`，`cx.theme().mode.is_dark()`，`cx.observe_global::<Theme>` 监听变化。「跟随系统」的接线方式没有找到文档。
- **国际化**：文案已转换到 `crates/gitwave/locales/<域名>.<语言>.json`，只在 Rust 侧维护。rust-i18n 取文件名最后一段作为语言代码，把每棵树展平成点号键，所以 `t!("app.sidebar.health")` 就是原来的 `t("app.sidebar.health")`；占位符是 `%{name}`；9 组 `_one` / `_other` 键（每种语言 18 个）由代码按数量选择。注册语句 `rust_i18n::i18n!("locales", fallback = "en")` 必须在 `main.rs`，因为 `t!` 转发到 crate 根的 `_rust_i18n_t!`。语言切换在切片二实现：偏好存在 core 应用设置的 `language` 键（`LanguageSettings`），系统语言用 `sys-locale` 读（GPUI 已经依赖它）。策略沿用旧实现：已保存的选择，否则系统语言，中文变体归 `zh-CN`，英语归 `en`，其余落英文。
  - `branches.irebase.*` 这个键名带着旧实现的拼写错误，转换时原样保留；接线时不要只改一侧。
  - 文案在编译期内嵌，cargo 自己不追踪这些文件；`crates/gitwave/build.rs` 让改动语言文件触发重新编译。
- **焦点**：GPUI 的按键与动作从聚焦元素向上派发。窗口里没有任何聚焦元素时，根上的监听器收不到。视图要用 `.track_focus(..)` 并在打开时 `window.focus(&handle, cx)`。跟踪焦点的元素还要同时有 id 和 role，否则不进无障碍树，每次焦点回到它时 gpui 都记一行 INFO 日志；容器用 `Role::Group`。约定见[工程约定](../../tech/engineering/00-overview.md#界面的无障碍)。
- **tokio 桥接**：全局对象持有运行时，`spawn_blocking` 把任务交给 tokio，再用 `cx.background_spawn` 等待它的 JoinHandle。测试里等待真实线程上的任务前要先 `cx.executor().allow_parking()`。
- **可调分栏**：面板尺寸按位置保存。面板要始终作为子节点存在，用 `resizable_panel().visible(bool)` 隐藏，不能增减子节点。
- **窗口**：`WindowOptions { ..TitleBar::window_options() }` 给出自绘标题栏所需的窗口选项；标题用 `window.set_window_title(..)` 设置。
- **切片二里学到的**：
  - `t!` 是运行时查找，返回的 `Cow<str>` 可以直接当 `SharedString` 用；切换语言后重建菜单即可。
  - rust-i18n 的语言是进程全局的。渲染窗口的测试调用 `locales::take_language_turn(cx)`，锁由测试的应用持有到销毁，因为框架在测试函数返回后还会再画一帧；不渲染窗口的测试持有 `locales::lock_language()` 返回的 guard。
  - 应用级动作用 `App::on_action` 注册，在冒泡阶段运行。`Context` 自己有一个同名的窗口级方法，会把它遮住，要写成 `let app: &mut App = cx` 再调用。监听器里不能再次更新正在派发的窗口，要操作窗口就用 `cx.defer`。
  - `window.open_dialog` 的构造闭包每帧重新调用，第三个参数是 `&mut App`；在里面读全局状态，打开着的对话框就会跟随切换。
  - `RadioGroup` 渲染时把子项的 id 改成序号。测试用 `window.within("<组的 id>").find(序号)` 和 `.click(序号, cx)` 在组的作用域里查找和点击，不必为了可测而放弃 `RadioGroup` 给读屏软件的语义。
  - `sys_locale::get_locales()` 返回迭代器；`MenuItem` 没有实现 `Debug`；`AsyncApp::update` 直接返回结果，不包 `Result`。

## M3 设计输入

实现 M3 之前对照 gpui-kit 0.11.0 源码核实过的接口与约束：

- **基线布局**：v0.11.0 交付的是 [09-workbench-redesign.md](../../design/09-workbench-redesign.md) 的已实施范围，不是三纵栏。壳层自上而下：标题工具栏（Workspace）、32px 的仓库标签行、navigator｜主区、状态栏（左边仓库与分支，中间操作进度，右边近期操作）。History 主区上「分支图 + 提交表」、下「提交信息 + 文件列表 | diff」，初始 42/58。普通列表行高 28px，选中是整行浅底加左侧 3px 竖线，HEAD 另有标记。
- **仓库标签不是可开关的标签页**：它是当前 Workspace 的仓库列表。点一下激活；移除标签等于把仓库移出 Workspace，是写操作；路径缺失的仓库变暗、不激活。
- **启动时显示什么**：旧实现把上次所在的 Workspace 和仓库记在 WebView 的 localStorage 里，新应用读不到。现在上次所在的 Workspace 记在 `app_settings` 的 `shell` 键，每个仓库的活动仓库仍是 `last_active_repo_id`。恢复时目标仓库缺失或不存在，就退到第一个没缺失的。
- **列表**：提交列表用组件库的 `List` + `ListDelegate`（`gpui_kit::component::list`）。它只渲染可见的行，每一行的内容由委托的 `render_item` 自己排版，带选中、键盘与无障碍；滚到距末尾 `load_more_threshold` 行（默认 20）以内时调用委托的 `load_more`，`has_more` 为真才会。`DataTable` 不合适：它的列是固定像素宽，没有占满剩余宽度的一列。`Table` 没有虚拟化。上游对「上万行」没有基准，只测到 2000 行。
- **标签行**：`TabBar` + `Tab`，不需要实现 trait。标签没有自己的 id，标签栏在渲染时按序号给它们编号，测试用 `window.within("<标签栏的 id>").find(序号)` 和 `.click(序号, cx)`。`TabBar::selected_index` 要一个 `usize`，给了就一定有高亮，所以只在有活动仓库时才给它。拖拽排序要上 `DockArea`，这一步没做。
- **Workspace 切换**：`Select` + `SelectState` + `SearchableVec<T>`。列表项自己实现 `SearchableListItem`，`Value` 是 Workspace 的 id，两个 Workspace 可以同名。`SelectState::set_items` 和 `set_selected_value` 要 `&mut Window`，所以读完工作区列表后的更新要在窗口的更新里做（`cx.update_window`）。
- **在后台线程上完成的更新**：`Context::spawn_in` 要 `&Window`，存不下来。不碰窗口的用 `cx.spawn` 加 `this.update`；碰窗口的存 `AnyWindowHandle`，在任务里 `cx.update_window(handle, ..)`。视图派发的任务用 `.detach()`，不存句柄：任务是弱引用回到视图的，视图没了也无害，存起来反而只增不减。
- **`Context` 上的 `App` 方法**：`defer` 等是 `App` 的，`Context<V>` 没有解引用，要写 `let app: &mut App = cx;` 再调用。
- **观察元素**：`window.find(id)` 只找得到叶子上的登记。组件库内部元素的 id（比如 `Sidebar` 的 `navigator`）不是叶子，找不到；要观察就自己套一层 `div().id(..).test_support()`。被隐藏的可调面板不再布局，里面的元素不存在，用 `try_find(..).is_some_and(|x| x.visible())`。
- **时间的措辞**：沿用旧实现的阈值。不到一分钟「刚刚」，然后是分钟、小时、天，满一周显示本地日期（`chrono`）。
- **复制状态库**：见技术选型的「本地存储」。只读的 `VACUUM INTO` 不改原库的 `state.db` 和它已有的 `-wal`，只读或创建 `-shm` 索引；原库正常关闭、没有 `-wal` 时，还会留下一个空的 `-wal`。正常关闭、崩溃后只剩 `-wal`、写入方开着，三种状态都实测过，第四种（正常关闭之后复制、再启动一次）由 core 的测试盯着：空日志不算变化。
- **列表自己管选中**：`ListState` 有自己的 `selected_index`，渲染时覆盖条目上的 `.selected(..)`。换了仓库要清掉选中、回到顶部，得调用 `set_selected_index` 和 `scroll_to_item`，它们要 `&mut Window`：在没有窗口的更新里（后台答案回来的那条路径），用 `App::defer` 等这次更新结束，再 `window_handle.update`。
- **列表的 `load_more` 在列表自己的更新里被调用**：那时不能更新历史，连这次更新的延迟回调里也不行（列表还在绘制，gpui 会报「正在更新的实体不能再次更新」）。要用 `cx.spawn_in` 派发一个任务，等调度器下一轮。
- **固定宽度的列**：宽度要按界面的字体留够，并禁止换行（`truncate`）。日期十个字符在默认字体下占 80px 出头，列宽 80px 时最后一个字符掉到第二行，被 28px 的行高切掉。

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

- 2026-11-25 · M1b 完成（分支 `chore/remove-tauri-app`）：
  - 删除：Tauri 应用（`src-tauri/`）、React 前端（`src/`、`public/`、`index.html`）与 Node 工具链（`package.json`、`pnpm-lock.yaml`、Vite、TypeScript、ESLint、Prettier、commitlint），连同 `lint`、`test`、`build` 三个工作流，删除 281 个受跟踪文件；另有 17 个图标、44 个语言文件和 `workspace.yml` 以重命名或转换的形式保留在新的位置。合计移除约 4.7 万行。磁盘上的 `node_modules` 与 `src-tauri/target` 一并删除。`main` 上不再有构建工作流，推送 tag 不会产生任何产物。旧源码读提交 `8466d55`。
  - 带走的两样。文案：1108 条中英文案转成 `crates/gitwave/locales/` 下 50 个文件（25 个域名 × 2 种语言），343 处占位符由 `{{name}}` 改为 `%{name}`，其余逐字不变；独立审查用 rust-i18n 4.2.4 把 2216 个键逐条读回比对，没有差异。图标：17 个桌面图标挪到 `assets/icons/`，逐字节一致；没搬的 35 个是 `tauri icon` 生成的 Android / iOS 启动图。官网 `site/` 只引用自己目录里的文件，不受影响。
  - 文案的守护（`crates/gitwave/src/locales.rs`，5 个测试）：磁盘上的语种与清单一致；两种语言的域名与键集合一致；占位符一致且写法合法；每条文案经 `t!` 读回与文件相同；错误码与文案双向对应，跨文件的重复键会被拒绝。变异实验里，重复键、未列入的语种、非 errors 文件里的孤儿键、写坏的占位符都被抓住。`crates/gitwave/build.rs` 让改动语言文件触发重新编译，否则 cargo 不会追踪过程宏自己读的文件。
  - 工程配置只剩 Rust：`Makefile`、`ci.yml`（由 `workspace.yml` 改名）、`dependabot.yml`、`.pre-commit-config.yaml`、`.gitignore`、`.gitattributes`、`.vscode/extensions.json`；`scripts/bump-version.sh` 代替 `bump-version.mjs`；发布技能改为休眠说明。
  - 依赖：`gitwave` 新增 `rust-i18n` 和测试用的 `serde_json`。`gitwave-core` 在 Windows 上的 `windows-registry` 由 0.100 改到 0.6：调用处用到的接口在两个版本里相同，改后与 hyper-util、GPUI 的 Windows 层共用 0.6.1。0.100 及其两个同伴是依赖图里仅有的要求 Rust 1.95 的包，改后 `rust-version` 保持 1.92。锁文件少了 4 个包。Windows 上的编译由本分支的 CI 验证。
  - Linux 依赖清单：去掉 `libxcb-shape0-dev` 与 `libxcb-xfixes0-dev`。它们是 M1 审查建议补上的，当时注明是推测；锁文件里 860 个 crate 没有一个提到 xcb 的 shape 或 xfixes，Zed 的 `script/linux` 里也没有。注释改为与上游清单逐项对应并附 URL。是否真的不需要，以本分支的 Linux CI 为准。
  - `gitwave-core`：相对 `main` 只有注释和上面那一行依赖变化，注释共 25 处（15 个文件），改掉的是指向已删除的命令层、IPC 边界和 `src-tauri/` 路径的说法，以及一句对 `windows-registry` 版本选择的错误解释。
  - 文档：README、CONTRIBUTING、AGENTS.md、技术文档索引与四份总览、ADR 0007、设计总览说明同步到移除后的状态。同时更正了此前文档里没有依据或不准确的说法，逐条见[审查报告](./review.md#处理结果m1b)。
  - 验证：`make check` 通过（`gitwave` 5 个测试，`gitwave-core` 394 个通过、2 个忽略）；应用构建并启动 8 秒无崩溃；`bump-version.sh` 在 `/tmp` 的一次性仓库里跑了 26 次，含 3 条回滚路径。本机没有 `pre-commit`，钩子没有实际运行过。

- 2026-11-25 · 首次托管 CI（PR #245，提交 `fa57247`）：`Rust (ubuntu-latest)`、`Rust (macos-latest)`、`Rust (windows-latest)` 全部通过，用时 Ubuntu 1 分 8 秒、macOS 2 分 20 秒、Windows 3 分 16 秒。三端的 clippy 都没有输出，`gitwave` 5 个测试、`gitwave-core` 394 个通过、2 个忽略。Windows 任务重新编译了 `gitwave-core`，Windows 上探测系统代理的代码在 `windows-registry` 0.6.1 下编译、链接通过。Ubuntu 任务只装了清单上的 14 个包，日志里没有 `libxcb-shape0-dev` 与 `libxcb-xfixes0-dev`，依赖和两个测试二进制都完成了链接。上面两处改动都不需要回退。三端都快，是因为 rust-cache 按 restore-key 命中了上一次的缓存，依赖的编译产物被复用。
- 2026-11-25 · M2 切片一（分支 `feature/gpui-kit-app-shell`，当时基于合并了 M0 / M1 的 `main`；M1b 合并后已变基）：
  - tokio 运行时桥接（`crates/gitwave/src/runtime.rs`）。应用状态默认放在内存里，不读写磁盘：core 新增 `AppContext::in_memory()`，等应用具备接管真实数据的能力再切换。
  - 应用菜单（GitWave、编辑、查看、窗口）用同一份定义同时接入 macOS 的系统菜单栏和标题栏内的 `AppMenuBar`；浅色 / 深色切换；一个带多行输入框的设置对话框，用来实测输入法。
  - 验证：五个无头测试通过，覆盖运行时桥接（任务在 tokio 线程上执行并返回结果、任务 panic 转为错误）、状态栏端到端读出 core 的工作区数量、`⌘,` 打开设置对话框、切换外观后「显示」菜单的勾选项更新。`cargo fmt --check`、`cargo clippy --workspace --all-targets -- -D warnings` 通过；应用在 macOS 上启动运行无崩溃；Tauri 一侧对 core 的测试和 clippy 通过。
  - 测试查出一个真实缺陷：窗口打开后没有任何元素获得焦点，而 GPUI 的按键和动作是从聚焦元素向上派发的，工作台上的监听器收不到。也就是说 M0 骨架里的 `⌘Q` 实际是失效的，当时只验证了启动不崩溃，没有验证交互。现已让工作台在窗口打开时获得焦点，并由上面的测试覆盖。
  - 当时的未完成项：语言切换（等文案来源确定，见切片二）；「进入全屏」菜单项（这个版本的 gpui 没有对应方法）；系统菜单栏和对话框的实际显示、中文输入法需要人工验证，本机终端没有录屏权限，无法截图。
- 2026-11-25 · M2 切片二（同一分支，变基到合并了 M1b 的 `main` 之后）：
  - 语言切换。core 的应用设置新增 `language` 键：`LanguageSettings` 只存用户选的语言标记，空白表示跟随系统；标记要是一到三段、每段一到八个 ASCII 字母或数字、首段以 ASCII 字母开头，否则拒绝。`gitwave` 的 `locales.rs` 列出提供的语言，启动时按「已保存的选择，否则系统语言」决定。入口在「显示 › 语言」子菜单和设置对话框，切换立即生效；保存是在主线程上同步做的，界面等它。
  - 菜单和对话框改读语言文件，补了 19 条文案（每种语言 1127 条）：标准菜单项、语言选项的名称、语言设置的两条错误文案。三个窗格的标题和状态栏仍是开发用的占位文字，不在语言文件里。
  - 日志：应用启动时调用 core 的 `infrastructure::observability::tracing::init`，写线程的 guard 存成全局状态（`crates/gitwave/src/logging.rs`）。core 那个模块的注释把文件名写成 `app.YYYY-MM-DD.log`，实际是 `app.log.YYYY-MM-DD`，注释和测试里的例子已改正。
  - 依赖：`sys-locale` 与 `tracing` 成为 `gitwave` 的直接依赖。两者都已在依赖树里，锁文件只多两条依赖边。
  - 验证：`make check` 通过，`gitwave` 19 个测试、`gitwave-core` 400 个通过、2 个忽略。新增的测试覆盖系统语言到语种的映射、已保存的选择优先、启动与切换（观察者被通知、保存到达 core）、切换后菜单的名称与勾选项、在设置对话框里点击单选、对话框打开时动作仍生效、两套菜单在切换后确实重建且文字与预期一致、日志的写线程活到应用退出。应用在 macOS 上启动后，当天的日志文件增长了 1628 字节，内容是迁移记录的 JSON 行。
  - 实现中改正的一处：从旧实现照搬的映射把 `en` 与系统语言做完全相等比较，`en-GB` 这样的标签会漏掉，现在只看语言部分。
  - 一个先做复杂、再改简单的地方：保存最初放在 tokio 后台，连着切换两次语言时两次保存并行执行，完成顺序没有保证，先选的可能后写入（用测试制造了出来）。我先加了「每次保存等上一次」的排队。用户看后指出，这类问题让界面冻结等它做完即可，不必考虑并发。于是保存改回主线程同步（`locales::switch_and_store`），后台任务和排队机制全部删除。约定记在架构总览的「界面与用例之间」：短小的本地写入同步做，只有耗时或时长不可预估的操作才放后台。
  - 没有验证的：macOS 系统菜单栏在切换后是否真的换成中文（无头测试检查的是菜单的定义，不是原生菜单栏）、设置对话框里单选的实际外观、中文输入法。这些需要人工看。三端 CI 没有跑，这个切片还没有 PR。
  - 仍未完成：「进入全屏」菜单项（这个版本的 gpui 没有对应方法）。
- 2026-11-26 · M2 切片二的独立审查与处理（[报告](./review.md#代码审查m2-切片二语言切换文案日志)，[处理结果](./review.md#处理结果m2-切片二)）：code-reviewer 子代理只读审查，🔴 一条、🟡 十条、🟢 十三条；它做了 44 个变异实验和 100 次去掉语言锁的压测。除两条转为上面的待决事项、一条不处理外全部处理。要点：
  - 🔴 对话框打开时，应用级动作到不了工作台。根子在切片一：动作挂在 `div#workbench` 上，而对话框拿走焦点、绘制在工作台之外，动作从焦点向上派发时经过不了工作台。后果是设置或关于对话框打开时，「显示 › 语言」和外观切换不生效，⌘Q 很可能也一样。现在十个应用级动作改用 `App::on_action` 注册。复现测试先失败、修复后通过。
  - 文档里「组件库自带的文字只有英文」写反了，已更正：组件库的每条文案带 8 种语言，并且随 GitWave 的语言一起切换。
  - 补了四类测试：两套菜单在切换后确实重建、且每一项文字与预期一致（之前测试直接调用菜单构造函数，绕开了观察者）；对话框打开时动作仍生效；语言标记的 ASCII 约束；日志的写线程活到应用退出。每一条都做了反向验证，确认去掉对应的代码后测试会失败。
  - `cargo test` 不再写用户真实的日志目录：core 的 `init_is_idempotent` 原先会往里写，现在写到临时目录。应用在退出时主动停掉写线程，把缓冲里的日志写完。
  - 测试之间的语言锁改由测试的应用持有。框架在测试函数返回后还会再画一帧，原先那一帧已经在锁外。
  - 自己另外发现并更正的：日志文件名里的日期按 UTC 算；实现方一度把一次没真正执行的反向验证当成通过（变异脚本没匹配到被折行的代码），重做后才成立。
  - 修复后的验证：`make check` 通过，零警告，`gitwave` 19 个测试、`gitwave-core` 400 个通过、2 个忽略；应用的测试二进制连跑 60 次全部通过；应用在 macOS 上启动并运行 7 秒，无输出，日志正常写出。
  - 仍然没有验证、需要实机看的：macOS 系统菜单栏在切换后是否换成中文；⌘Q 在对话框打开时是否生效；退出时缓冲的日志是否落盘；设置对话框里单选的外观；中文输入法；Windows 与 Linux。审查者的一次探针里，标题栏的菜单在切换后读到过一次英文，之后 286 次没有再现，原因不明，实机验证时留意。
- 2026-11-26 · 切片二提交并开 PR（#246）：四个提交——`d24482c` core 的语言设置、`db44073` 语言切换的界面、`50c2142` 日志、`31e81f4` 文档。推送前在临时克隆里把四个提交逐个检出，各自通过格式检查、clippy 和测试（410、418、419、419 个）。
  - 首轮 CI：Ubuntu 5 分 35 秒、Windows 9 分 58 秒通过。macOS 的任务排队 15 分钟后被 GitHub 取消，注解是「The job was not acquired by Runner of type hosted even after multiple attempts」，并提示 macOS arm64 runner 容量紧张。它一步都没执行，与代码无关。只重跑这一个任务后通过，用时 5 分 12 秒。三端的测试数都与本地一致：`gitwave` 19 个，`gitwave-core` 400 个通过、2 个忽略。这次 `Cargo.lock` 变了，缓存对不上，三个平台都是从头编译。
  - 同时做的收尾：关闭 Dependabot 的 #238–#241 并删除它们的分支（它们改的是已删除的 `package.json` 与 `pnpm-lock.yaml`）；删除已合并的 `chore/remove-tauri-app` 与 `feature/gpui-kit-desktop`，本地和远端都删了。#244（rusqlite 0.40 → 0.43）仍然开着，留给用户。
  - 没有合并：M2 的退出标准里还有要实机验证的项。
- 2026-11-26 · macOS 实机验证（窗口 1200×801，系统语言是简体中文。除非写明，用的是 `6dc6ee3` 构建的调试二进制；「修复后」指 `6dc6ee3` 加上 `ef0ef3a` 的修复）：
  - 系统菜单栏：通过。读屏接口读的是真实的菜单栏，点的是真实的菜单项。启动时跟随系统，菜单栏是 `gitwave / 编辑 / 显示 / 窗口`，各项都是中文，「跟随系统」打勾。点「显示 › 语言 › English」后是 `gitwave / Edit / View / Window`，各项都是英文，English 打勾。点「View › Language › 简体中文」回到中文，「简体中文」打勾。
  - ⌘Q：通过。设置对话框打开时按 ⌘Q，三次（修复前的二进制两次、修复后的一次），进程都退出了，量过的那次在半秒以内。
  - 退出时的日志：通过，但不是靠退出时的那段刷新。真实二进制上，关闭对话框时打出的一行（随后立刻 ⌘Q，进程在 17 毫秒后结束）完整地写进了当天的文件。另外在仓库之外的一次性工程里，把真实的 `logging.rs` 编进去，连续写 5000 行或 10 万行后立刻退出：带退出时的刷新 18 次、去掉它的对照 21 次，每一次文件里的行都齐全，没有截断。对照也不丢，是因为 tracing-appender 0.2.5 的写线程每取到一行就立刻写文件，并不攒着；实测最后一行入队后进程还要约 20–60 毫秒才结束，队列早就排空了。刷新留着的理由是不把结果交给调度：丢弃 guard 时会等写线程把队列排空。
  - 设置对话框：通过。中、英文界面各截了一张只含应用窗口的图，按原始分辨率看：标题、小标题、三个单选项（选中的带勾）、说明、输入框、关闭按钮和右上角的 × 排列整齐，字体是同一套，标题只是加粗。鼠标按控件的实际位置点：两个语言单选项都能切换，× 和「关闭」都关掉对话框，点面板的空白处对话框留着，点遮罩关闭。
  - 中文输入法：用户本人在对话框的输入框里验证通过。
  - 发现并修了一条（`ef0ef3a`）：无障碍接口激活后（读屏器，或者任何读取无障碍树的程序），每次焦点回到工作台，gpui 都记一行 INFO 级别的日志，说获得焦点的元素没有无障碍节点。验证时它在当天的日志里占了 78 行（共 109 行）。原因在根元素 `div#workbench`：它有 id、跟踪焦点，但没有 role，所以不生成无障碍节点；按 gpui 的说法，这时读屏器念的是整个窗口，而不是聚焦的元素。现在给它 `Role::Group`（三端都是中性的容器，理由见[工程约定](../../tech/engineering/00-overview.md#界面的无障碍)），并加测试 `the_workbench_is_exposed_to_assistive_technology`：窗口打开时、设置对话框关闭之后，根元素都是有无障碍节点、有焦点的。实机上无障碍激活后开关对话框 4 次，日志零条，无障碍树里工作台是一个 group 节点。
  - 这处修复经过独立审查（[报告](./review.md#代码审查m2-实机验证后的无障碍修复工作台根元素的-role)，[处理结果](./review.md#处理结果无障碍修复)）：🔴 零条、🟡 四条、🟢 七条。除「给 group 加名称」一条按审查者的建议不做（先用 VoiceOver 听过再定），其余都处理了。审查指出实机验证漏了 M2 退出标准里的主题切换和 ⌘Q 以外的快捷键，于是补做了一轮（`ef0ef3a` 构建的二进制）：
    - 主题：点「显示 › 深色」，勾选移到「深色」，侧栏的颜色从 (255,255,255) 变成 (10,10,10)；点「显示 › 浅色」，勾选和颜色都恢复。
    - 快捷键：⌘B 隐藏侧栏，「History」的标题挪到最左边，再按一次恢复。⌘M 让窗口最小化，⌘H 让应用隐藏。
    - 菜单项：「关于 GitWave…」打开一个两行文字的对话框，「设置…」打开带输入框的那个，Esc 都能关掉。「缩放」把窗口从 1200×801 放大到 1446×949，再点一次恢复。
    - 输入框：打字生效；⌘Z 撤销，⇧⌘Z 和「编辑 › 重做」都能重做；⌘A 全选（全选后打一个字符，原来的内容被整体替换）。
    - 另外加了两个无头测试，把这轮里只有一次实机证据的两处钉住：`switching_the_appearance_repaints_the_window` 读一帧实际画出的背景，切换到深色后不得残留浅色的背景；`the_sidebar_shortcut_hides_the_sidebar_and_brings_it_back` 覆盖 ⌘B。
    - `make check` 通过，`gitwave` 22 个测试、`gitwave-core` 400 个通过、2 个忽略。四个变异各自让对应的测试失败：去掉 role、删掉打开窗口时的 `window.focus`、让工作台缓存背景色、改掉 ⌘B 的绑定。
    - `ef0ef3a` 推送后的 CI 一次通过：Ubuntu 1 分、Windows 3 分 10 秒、macOS 1 分 28 秒（排队约 4 分钟后才开始）。三端的测试数都与本地一致。
  - 记下三条观察，都没有处理：
    - macOS 只在菜单是英文标题时，往 Edit、View、Window 里追加自己的标准项（AutoFill、Start Dictation…、Emoji & Symbols、Enter Full Screen、Fill、Center 等）；中文标题时不追加。所以英文界面里有「进入全屏」，中文界面里没有。
    - 对话框的无障碍节点没有名称（读不到标题），也不标为模态。这是组件库对话框层的行为，[调研笔记](../../tech/tech-selection/gpui-kit-research.md)里记为上游 [longbridge/gpui-kit#3595](https://github.com/longbridge/gpui-kit/issues/3595)，在 0.11.0 上实测仍在。
    - 应用菜单的标题是 `gitwave`：调试二进制没有打包，用的是进程名。
  - 验证中自己弄错的两次，都没有写进文档，记下来免得再犯。一是在截图上量控件的位置，把位置读小了 5/6，以为对话框画的位置和能点的位置不一致。截图是 2400 像素宽，看图的工具把它缩到 2000 宽显示，我按显示坐标当真实像素换算了。改用像素分析重新量面板（x 377–823、y 81.5–401.5，与布局一致），再按真实坐标点击，才否定。二是据此以为标题用的是衬线字体，放大到原始分辨率后看，是同一套无衬线字体，只是加粗。以后量位置用像素分析，或者先裁到不超过显示尺寸再看。
  - 排除的一个怀疑：当天日志里有三组「applied migration」，时间点像是我跑测试的时候，一度以为测试写进了真实的日志目录。受控启动一次应用，恰好追加 8 行，是 `AppContext::in_memory()` 启动时迁移内存数据库写的。测试仍然不写真实目录。
  - 仍然没有验证的：
    - ⌥⌘H（隐藏其他）、剪切、复制、粘贴：「隐藏其他」会藏起用户正在用的其它应用，剪贴板三项会改动用户的剪贴板，都没有在实机上试。
    - 「前置全部窗口」：没有可以读出来的结果。
    - 读屏器的实际朗读（VoiceOver），以及工作台的 group 在 Windows 与 Linux 上映射成什么。
    - Windows 与 Linux，没有机器；审查者那次标题栏菜单读到英文的现象。macOS 用原生菜单栏，不显示标题栏里的菜单，在这里看不出来。
- 2026-11-26 · M3 切片一（分支 `feature/gpui-kit-read-only-workbench`）：接入真实状态、壳层按基线布局重排、提交历史列表。
  - 真实状态：用户选定「用副本」。core 新增 `sqlite::open_in(dir)`、`bootstrap::open_app_context_in(dir)` 和 `persistence::snapshot`：用只读的 `VACUUM INTO` 复制，旁边留一份记录，原库变了才重新复制。应用启动时把正式版的状态复制到 `GitWave-dev/`，只打开副本。现在两边的迁移相同，直接打开还不会出事；从开发版新增第一个迁移起，直接打开就会把正式版的库升级到它不认识的架构。
  - 壳层：标题工具栏里是 Workspace 切换，下面一行仓库标签，再往下是 navigator｜主区；状态栏左边是「仓库 · 分支」，右边写明状态来自副本。上次所在的 Workspace 记在 `app_settings` 的 `shell` 键，启动时回到那里。
  - 提交历史（`crates/gitwave/src/history.rs`）：分页的状态机是纯逻辑，每页 300 条，空的一页就算到底；列表滚到末尾附近取下一页；迟到的结果按代数丢弃；游标过期时从头重读；失败时留着已显示的，给出重试。每行是分支图的占位、主 ref（tag 优先，其次本地分支、远端分支）、标题、短 SHA、作者、相对时间，HEAD 有标记。
  - 没做的：分支图的线、提交详情、文件列表与 diff、Blame、搜索；标签的拖拽、移除、重新关联；标题工具栏里的分支名和 Fetch / Pull / Push；窄面板下隐藏列。
  - 独立审查（[报告](./review.md#代码审查m3-切片一只读工作台第一步)，[处理结果](./review.md#处理结果m3-切片一)）：code-reviewer 子代理只读审查，🔴 一条、🟡 十三条、🟢 九条。除两条有意不做、几条留到后面之外全部处理，要点见下。
  - 审查抓住的严重问题：`lock_down_db_files` 只给 Unix 版本加了可见性，快照模块在 Windows 上编译不过，而本机的 `make check` 发现不了。本机没有 Windows 的目标，没法交叉编译验证，以三端 CI 为准。
  - 审查纠正的一处说法：文档里写「只读连接只动 `-shm`」，实测原库正常关闭、没有 `-wal` 时，只读连接会留下一个空的 `-wal`。它不含数据，正式版下次关闭时会删掉，但「原库变了才重新复制」的判断会因此每次误判。现在空日志不算变化，文档四处都改成了与实测一致。
  - 审查指出的逻辑缺口，都已改并各补了测试：离开再回到同一个 Workspace，活动仓库退回启动时的值；读 HEAD 的回调用错了代数，迟到的结果可能盖掉新的；切换仓库后列表的选中行没有清掉；仓库列表读不出来时主区停在「加载中」，没有办法重试；游标过期时列表突然清空、自动重试没有上限（现在保留已显示的提交，后台从头重读，最多两次，并给出提示）；复制失败对用户不可见；新应用没有像 v0.11.0 那样关闭 libgit2 的所有权校验；开发版的日志写进了正式版的目录；四条旧文案在只读版里说的是做不到的事。
  - 审查的优化建议里做了的：HEAD 只读一次，由壳层交给历史（原来各读一次）；错误存对象而不是翻译后的字符串，语言切换后提示跟着变；空的一页只有在游标也不前进时才算到底（为搜索留的余地）；迟到的结果多补了一处测试；列宽与无障碍标签；去掉常驻的「提交历史」标题，失败和重读时才出现一条提示。没做的记在「风险与未决项」里。
  - 变异实验暴露的两处：去掉壳层的代数判断，测试照样通过，因为那个场景被另一道防线挡住了（拒绝别的 Workspace 的仓库列表）。补了「离开再回到同一个 Workspace」的场景，迟到的答案是给同一个 Workspace 的。允许激活缺失的仓库，壳层的测试也照样通过，因为没有真的去选它。补上之后两条都能抓住。
  - 自己出过的问题，共三个。一是一条语言测试调用 `app_state::init`，测试于是往副本里写了一次，没有碰到正式版的库：现在 `app_state::init` 在测试构建里直接 panic，那条测试改用内存状态，测试创建的副本已删除。二是测试的临时目录名用「进程号加毫秒」，两条并行的测试拿到同一个目录、互相改坏对方的文件，跑一次还留下三千个目录不删：应用测试和这一步在 core 里加的两处都改用 `tempfile`。三是测试没有走真实的滚动路径，直接调了内部的「取下一页」，改走真实路径后才暴露出列表的回调里不能更新历史（见「M3 设计输入」）。
  - 验证：`make check` 通过，`gitwave` 67 个测试、`gitwave-core` 412 个通过、2 个忽略；应用的测试程序连跑 60 遍全部通过。十五个变异各自让对应的测试失败：壳层、历史、HEAD 三处的代数判断，取下一页的锁、空页到底的两种情形，缺失的仓库不激活，副本不重复复制、空日志不算变化、放不下时不留中间文件，记住上次的 Workspace、记住激活的仓库（模型和壳层各一条），切换后清掉选中，重选当前标签不重读。
  - 实机（macOS）：启动后正式版的 `state.db` 和 `state.db-wal` 哈希与启动前一致，目录里没有多出文件；副本里是 3 个 Workspace、5 个仓库；修好日志目录之后，开发版的日志只写进 `GitWave-dev/logs/`。窗口截图按原始分辨率核对：标题工具栏里是 Workspace 切换器，下面一行仓库标签，左边 navigator，主区的提交列表第一行带 `HEAD`、`main` 和「+1」，各列对齐，状态栏左边是「仓库 · 分支」、右边是副本说明。第二次启动落在第一次离开时的 Workspace 和仓库上。
  - 实机看出来的一个缺陷：满一周后显示成日期的那一列太窄，最后一个字符掉到第二行，被行高切掉。加宽并禁止换行后重新截图确认。
  - 验证时自己弄错的两次，都不是应用的缺陷。一是截图按屏幕区域截，截到了别的前台窗口；之后改成按窗口编号只截 GitWave 自己的窗口，截到别的那张没有保留。二是裁图时偏移给了 (0, 0)，工具当作未指定而按居中裁，看起来像提交行盖住了标题栏；改用整窗的缩略图核对后否定。
  - 还没有做的：Windows 与 Linux（没有机器，以三端 CI 为准）；性能预算。
- 2026-11-27 · 切片一提交并开 PR（#247）：三个提交——`1e7d844` core 在指定的目录打开状态并只读地复制原库（连两个新错误码的文案一起）、`9de06e1` 工作台与提交历史、`35f9700` 文档。推送前在临时克隆里把三个提交逐个检出，各自通过格式检查、clippy 和测试（434、479、479 个）。
  - 逐个检出抓到一处拆分错误：第一次把四个错误文案文件放在第二个提交，第一个提交单独检出时目录测试失败（`errors.persist.snapshot.failed` 没有文案）。文案跟着错误码走，挪进第一个提交后重做；重做前后的内容逐字节一致。
  - 首轮 CI：Windows 通过，用时 3 分 51 秒，R1 的修复和 `snapshot` 的十条测试在这一端成立。macOS 与 Ubuntu 都在 `cargo test` 失败，失败的只有一条：`a_page_that_does_not_come_is_said_of_and_can_be_asked_for_again`，屏幕上成了 4 行，应为 2 行。
  - 原因在测试，不在应用，有两层。其一，它在历史已经读到末尾时注入「下一页没来」，再点重试；末尾之后没有下一页，重试从头读，把两个提交又追加了一遍。真实使用中到不了这个状态：只有路上的页会失败，重试读的是它自己的游标（现在写在 `Status::Failed` 的注释里）。其二，它等的是「失败提示消失」，而提示在点重试的那一刻就没了，所以最后的断言抢在后台答案之前。用一条临时探针实测（没有保留）：那个等待返回时屏幕上是 2 行，半秒后是 4 行。本机连跑 60 遍都是断言先执行，CI 的那两端是答案先回来。
  - 修法：改走真实路径。仓库有 301 个提交，用新加的测试钩子 `page_on_its_way_for_test` 把历史带到「下一页在路上」而不发请求，注入失败，点重试，等到第二页出现在屏幕上为止。两个变异都被它抓住：重试改成从头读、改成什么都不读，都会超时失败；旧测试在这两种情况下照样通过。
  - `run_until_parked` 不等后台的答案，其它测试也可能在赌时序。逐个核对等待之后，用「给读取加延迟」查了两轮（延迟是临时代码，没有保留）：
    - 给 HEAD 和仓库列表的读取各加 400 毫秒，四条失败，都是等了 A 就断言 B。三条等了提交页就断言状态栏的「仓库 · 分支」，而分支名来自对 HEAD 的读取，是另一个请求，HEAD 没到时状态栏只有仓名——改用 `settled_on`。`the_shell_starts_where_it_was_left` 等的是「选中了工作区」，断言的却是仓库列表读到后才出现的那行字——改等仓库列表读到。
    - 给四个读取都加延迟跑全部 67 条，又有九条失败，都是 M2 的测试（语言、外观、设置对话框、侧栏、无障碍）。启动现在要在后台读工作区列表，这是切片一带来的；它们打开窗口后不等启动就做自己的事，测试结束时读取还在路上，调度器报「不确定」。共用的 `open` 现在打开窗口后等到启动读完，九条不用各自改。
    - 两轮修完，加着延迟 67 条全部通过；去掉延迟 `make check` 通过，测试数不变；应用的测试程序连跑 60 遍全部通过。约定记在[工程约定](../../tech/engineering/00-overview.md#测试策略测试金字塔)。
  - 修复提交 `24d85ca` 推送后的 CI 一次通过：Ubuntu 1 分 21 秒、macOS 2 分 50 秒、Windows 3 分 20 秒。三端的测试数都与本地一致：`gitwave` 67 个，`gitwave-core` 412 个通过、2 个忽略。`24d85ca` 单独检出也通过格式检查、clippy 和测试（479 个）。到这里，R1 的修复、`snapshot` 的十条测试和 `git2` 的夹具在三端的 CI 上都跑过了。
  - 用户确认合并，方式选了 squash（AGENTS.md 写的是 merge commit、不 squash；这一个 PR 按用户的决定）。`main` 上是合并后的一个提交，上面引的逐个提交不在 `main` 的历史里：它们列在 PR #247 的提交页，`git fetch origin refs/pull/247/head` 可以取回。
  - 文档提交 `0965dc2` 推送后又跑了一轮 CI：`paths-ignore` 对 PR 看的是整个 PR 的差异，PR 里有代码改动时，只加文档的推送也跑（PR #246 的两次文档推送也是）。工程约定和 README 原先写成「纯文档变更不跑 CI」，已更正。
