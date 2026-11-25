# GitWave 全量迁移到 Go + NSGo

M0 平台验收待完成。M1 已接通原生 Workspace、History、diff 和 blame。M2 已接通 Changes、Stage/Unstage、Commit/Amend、Discard、本地分支与标签管理、干净和普通脏工作树检出、初始化、克隆、Fetch、Pull、Push 和 Workspace 自动刷新。M2 三端端到端验收待完成。M3 已接通 Merge、冲突编辑、中止合并、普通与交互式 Rebase、Cherry-pick、Revert、Stash、Reflog 恢复、Worktree、Remote、Submodule、Hooks、`.gitignore`、Git LFS 面板、本地 LFS 对象操作与传输，以及仓库健康本地指标。分支 `feature/nsgo-native-ui`，对照基线 `004c9f4`（v0.11.0）。

M4 已接通常规、外观和网络设置、中英界面切换、主题、配色、字体和「关于 GitWave」。网络设置沿用旧 SQLite 代理记录，并接入 AI、Git、Git LFS 和凭证 helper。Workspace AI 设置已接通 provider、模型、地址、离线模式、备用顺序、Prompt 模板及密钥保存。Go 调用链已接入 Changes 的提交建议、Reflog 解释、仓库健康总结、History 提交解释、PR 草稿和冲突解释。命令面板已接通静态命令、提交搜索定位和白名单 AI 建议。Go 已实现 Windows 和 Linux 旧版 AI key 的读取与清除，并接入 Git HTTPS 凭证回退、旧账号选择与独立 LFS 认证。WebView 偏好过渡桥、SQLite 首启复制和 SSH 密钥设置已接入。SSH 多行备注已通过隔离 agent 回归，Windows Git LFS 的 agent 选择、Git Bash socket 和三端交互继续实机验收。

M5 的 Go module 已迁到仓库根目录，三端预览 CI 已切换到新目录结构。macOS DMG 与 Linux deb、Arch、AppImage、RPM 已实际生成并验证随包 Git。Fedora 43 已通过 RPM 安装和桌面启动，Ubuntu 24.04 已通过 deb 安装和桌面启动，Ubuntu 22.04 已验证随包 GTK 的 deb 和 AppImage。Windows MinGit、LFS 与 NSIS 安装器已通过构建和解包校验，实际 Windows 安装与运行待验证。Go 更新器已接入旧偏好、签名清单、代理和下载校验，macOS 与 Linux AppImage 的原位替换和重启通过本地夹具。Go CI 已加入 Windows 旧包升级和失败回滚检查。正式签名与公证、正式发布切换、旧版升级与回退演练、旧工具链退役仍待完成。

## 目标与验收基线

把桌面 UI、应用用例、Git 编排、数据访问、系统服务、AI 和发布入口迁入 Go + NSGo。Git 由 Go 调用官方 Git CLI，安装包采用经验证的 Git 交付方式，基础功能保持免手动安装 Git。最终默认开发、测试、构建及发布均不依赖 `gitwave-native-core`、Cargo、Tauri、React、桌面端 pnpm 或 WebView。迁移期间以旧版作为行为对照。技术取舍见 [ADR 0007](../../tech/decisions/00-overview.md#0007--全量迁移到-go--nsgo)。

以 [v0.11.0 实际交付的工作台](../../design/09-workbench-redesign.md)和 [PM 范围](../../pm/core/02-scope.md)为基线。每条用户流程覆盖正常、加载、空态、失败、确认、取消与恢复。保留 Workspace 无目录实体、多 Workspace 并行与单 active repo，repo 级草稿和错误，Git 写操作显式确认及 AI 本地优先。三端功能、数据、凭证、安装、更新与回退验收通过后切换默认入口。

Go module 位于仓库根目录。`cmd/gitwave` 包含桌面入口、NSGo 配置和图标，`cmd/` 下其余命令负责打包，`internal/` 包含应用库。切换后，默认构建、测试和发布使用根目录 Go 工具链，并清除旧桌面源码、包管理与锁文件、Tauri/Rust 配置、旧 CI/CD 和已失效的脚本及文档入口，历史实现通过 Git tag 追溯。逐项对照旧应用的实际入口、布局、文案、交互与 Git 结果，确认旧版缺陷时先记录可复现条件和预期行为，再用回归测试约束修复结果。最终发布链需在三端从干净环境产出安装包，验证签名、安装、升级与回退。

全量迁移及验收完成后，按约定提交并推送重构分支。

当前先完成 macOS 交互、安装与更新验收。Windows 的最终实机验证由用户执行。

macOS 正式候选沿用旧 CI 的 Developer ID 证书和 App Store Connect 公证凭证。过渡包沿用旧 Tauri 更新签名密钥。Go 更新清单的签名配置及正式安装回退继续验收。 [发布预检记录](./review.md#2026-11-25--正式发布更新密钥预检)

macOS 候选工件上传前执行安装升级回退门禁。联合发布时，校验旧 updater 下载的偏好导出版归档的 Minisign 签名、Developer ID 和公证票据，启动解包后的应用，让真实 WebView 的偏好经 SQLite 迁入 Go。本机已用官方 v0.11.0 和临时签名的导出版及 Go 候选完成该过程。正式 Go 候选的签名与公证，以及旧 updater 经发布清单自动下载和安装的完整流程仍待验收。 [macOS 安装升级记录](./review.md#2026-11-25--macos-候选-dmg-安装偏好迁移与回退门禁)

macOS 续发门禁在上传前从上一正式 Go 版本取已认证的更新 ZIP，启动前版并确认打开隔离工作区，再由当前版本的更新器测试进程替换并启动候选 ZIP 中的应用。本机两份实际 Go 应用已完成这条链路。已发布客户端从 GitHub 检查、确认和下载更新的流程仍待正式发行后验收。 [Go 应用包升级记录](./review.md#2026-11-25--macos-go-应用包升级与工作区恢复)

macOS 已用同一个仓库和工作区数据对照旧版与 Go 工作台，恢复 History 引用徽标、全部引用弹窗、HEAD 标记、元信息列和历史末尾提示。侧栏恢复空区块静态标题、Tags 切仓折叠、Remotes 与 Worktrees 的默认展开规则，保留新增首个 remote、worktree 和 submodule 的入口。本轮对齐工具栏计数和主要按钮位置、侧栏选中色及分支右侧留白，并检查 1024 点窗口。后续核对其他设置与页面，完成正式签名后的安装、更新和回退。 [macOS 对照记录](./review.md#2026-11-25--macos-工具栏与侧栏选中样式)

macOS 已恢复 v0.11.0 的「窗口」菜单、最小化与全屏快捷键，以及应用菜单的命令顺序。菜单结构纳入 AppKit CI，快捷键状态由模型回归验证。 [菜单对照记录](./review.md#2026-11-25--恢复-macos-窗口菜单)

History 的宽分支图现在可以横向滚动，切到 Changes 再切回时保留原视口。定位提交时让图中的节点可见。 [宽图对照记录](./review.md#2026-11-25--history-宽分支图的滚动与定位)

History 提交详情恢复标题展开、完整正文和指向所选提交的本地分支、远端分支及标签。正文在有高度上限的详情区内滚动，文件列表和 diff 继续可用。切换提交或在 History 与 Changes 间切换时保留展开状态，换仓后复位。提交行菜单恢复「复制提交信息」。 [提交详情对照记录](./review.md#2026-11-25--history-提交详情与复制)

文本 diff 的工具栏和截断提示可在窄栏内换行，达到上限后显示原因。统一和分栏视图使用原生字体宽度和虚拟列表中已显示行的尺寸校正横向范围。折叠或切换工作台视图时保留同一预览已测出的宽度及滚动位置。 [diff 对照记录](./review.md#2026-11-25--文本-diff-窄栏与长行)

Changes 已用一条已暂存加一条未暂存及 495 条路径变化的两份夹具对照。文件区块、文件状态、批量动作、提交编辑器和最窄分栏均复核。对 Git 未识别的唯一同名文本比较保留的内容后，495 条路径的未暂存列表已与原版一样识别出 12 对移动。补配项在未暂存区和暂存区均显示为一行，可预览两端差异，并在同一笔索引事务内暂存或取消暂存两个路径。暂存之后的工作树变化继续单独显示。 [Changes 对照记录](./review.md#2026-11-25--暂存区的同名文本移动)

未跟踪文本的预览直接生成可见行，保留总新增行数和既有预览上限。已在 Apple Silicon 上比较预览分配量，并为 495 条路径的原生用例加上长文件截断检查。混合重命名周期刷新会核对候选内容、模式和 Git 属性后复用配对。macOS 和 Linux 可并行执行独立的 Git 检查，最终列表一次更新。包含同名补配后，Apple Silicon 上 495 条混合变更的完整重命名扫描约为 171 毫秒，缓存命中的重复扫描约为 42 毫秒。 [大列表性能记录](../fix-nsgo-large-changes/review.md#首次打开混合变更)

Changes、仓库健康和退出提醒已接入子模块内部改动。父仓库支持单独暂存 gitlink，保留子模块的本地文件。子模块状态读取失败时，父仓库已确认的变更仍可查看和暂存，退出时会提示检查未完成。 [子模块对照记录](./review.md#2026-11-25--子模块工作树状态和退出确认)

Changes 状态刷新只读取当前分支和上游信息。495 条混合变更加 6,000 个引用时，Apple Silicon 实测状态读取从约 261 毫秒降至 84 毫秒。同一夹具通过窄窗口和滚动定位复测。 [引用查询验收](../fix-nsgo-large-changes/review.md#changes-状态的引用查询)

设置已恢复旧版的弹窗几何、右上角关闭入口、导航选中态和主题、配色、代理的等宽选项。系统切换外观时，设置控件保留焦点和输入。设置各页的三端对照继续验收。 [设置对照记录](./review.md#2026-11-25--macos-设置弹窗布局)

默认 `make dev`、`make build` 和 `make check` 已使用根目录 Go module 与 NSGo。原 Rust 行为对照程序已退役，核验过的 Git fixture、diff 边界和 History 结果由默认 Go 门禁覆盖。偏好导出版保留 `make legacy-dev`、`make legacy-build` 和 `make legacy-check`，用于完成第一个升级跳转。

## 实施里程碑

| 阶段 | 实施内容 | 退出标准 |
|---|---|---|
| **M0 可行性验证** | 三端 Git CLI 行为与分发、NSGo 原生控件与包装、SQLite 和旧凭证兼容、旧 updater 过渡。先交付可重复的 Go Git 探针。 | 已锁定每个平台的 Git 来源和最低版本。真实 fixture 通过 worktree、冲突、interactive rebase、HTTPS/SSH 验证。旧库、旧凭证、安装签名和跨引擎升级均有可复现证据。 |
| **M1 只读工作台和 Workspace** | 用 Go 替换预览中的 Rust 进程，完成 History、diff、blame、Workspace CRUD、导入导出、本地仓库批量添加、重排、缺失与重链接。接入 Go CI。 | 原生预览和测试在没有 Cargo 的机器可运行。旧 SQLite fixture 恢复、History/diff 行为对照和大仓预算通过。 |
| **M2 Changes 与日常 Git** | 工作区状态、stage/unstage、commit/amend、discard、branch/tag、checkout、init/clone、fetch/pull/push，及其确认、进度、凭证、取消和错误。 | commit → push、切换 Workspace、操作中切仓、失败重试、脏仓库检查与外部 Git 竞态通过端到端验证。 |
| **M3 冲突和高级工具** | merge、普通与 interactive rebase、cherry-pick/revert、冲突编辑、stash、reflog 恢复、worktree、remote、submodule、LFS、hooks、`.gitignore` 和健康摘要。 | 对照旧版校验暂停后重启、继续与取消、stash pop 冲突、孤立 index 冲突、worktree 占用和失败恢复后的仓库状态。 |
| **M4 桌面体验与 AI** | 设置、原生菜单、快捷键、命令面板、关于、外部工具入口、自动刷新、退出保护、中英双语、主题、字体、输入法与无障碍。迁移 BYOK、Ollama、prompt 模板、仓库规则、failover 和原有 AI 生成入口。 | 三端桌面交互与各 AI 用例通过。离线仅连接 loopback，验证 scrubber、取消、数据出站控制和显式写操作确认。 |
| **M5 发布切换与退役** | 三端签名、旧版过渡包和新更新链、数据兼容和回退演练、默认构建切换。退役旧桌面代码与 CI，更新技术文档、README、官网、版本脚本与发布技能。 | 干净系统可安装使用，旧版升级及回退可重现，应用与安装包通过各平台签名验证，默认构建和发布不再需要 Rust/Tauri 桌面工具链。 |

## Go 应用与 Git 边界

- NSGo 工作台位于 `cmd/gitwave`。View/Signal 管界面，Go 用例管操作和确认，Git、SQLite、系统服务与 AI 分开接入。后台任务通过 `context` 取消，并把结果投递回 NSGo。对 repo、提交及文件切换保留请求身份校验。
- Git 使用确定的可执行文件、独立参数和工作目录。优先解析 Git 稳定格式，例如 porcelain v2 的 NUL 分隔状态、index stages 和批量对象读取。History 按 refs 快照分页，后台生成图、diff 及语法数据，UI 虚拟化行并对超限、二进制和未知统计给出明确状态。
- 写操作绑定发起时的 Workspace、repo ID、worktree 和 common dir。同 common dir 的应用内写操作串行，确认后重新检查 HEAD、index 与仓库操作状态，并处理外部 Git 的并发改动。取消和超时覆盖 Unix 进程组及 Windows Job Object，结束后重新读取仓库状态。针对 hooks、外部 diff、filter、fsmonitor、credential helper 和 SSH 明确执行边界。Hooks 工具仍只提供编辑；LFS 等受支持的外部工具按实际能力启用。
- 保留 message-only amend、dirty checkout 保护、未解决 index 计数、冲突缓冲、merge 的显式 commit、rebase 暂停与回退、stash 的 untracked 内容及失败后保留、worktree 占用保护。回退失败展示真实状态，不能显示虚假的成功或可继续状态。
- Windows 验证 MinGit 与 Git Credential Manager / OpenSSH 的组合，macOS 验证随包 Git 嵌套签名与公证，Linux 分别验证 deb/rpm 依赖声明与 AppImage 运行方式。随附内容需纳入许可证、对应源码、SBOM 和独立更新流程。另见 [MinGit](https://gitforwindows.org/mingit.html)、[Git 命令参考](https://git-scm.com/docs/git)与 [Git 许可证](https://github.com/git/git/blob/master/COPYING)。
- Windows NSIS 打包已集成固定 SHA-256 的 MinGit 2.56.1 与 Git LFS 3.8.1。Git、LFS 和 Workspace 导入从应用安装路径发现工具，Git 与 SSH 面板共享 Windows SSH 客户端选择。NSIS 安装事务将 `gitwave-git` 与应用和卸载器一起发布或恢复。Go CI 包含清除系统 Git 路径、工具启动、LFS 初始化、失败回滚、安装与卸载并发，以及修改过的随包配置保全。Windows 运行结果、对应源码交付和 Unix 发行物仍须验收。 [MinGit 2.56.1](https://github.com/git-for-windows/git/releases/tag/v2.56.1.windows.1) [Git LFS 3.8.1](https://github.com/git-lfs/git-lfs/releases/tag/v3.8.1)
- macOS 和 Linux 的源码构建使用 Git 2.56.1 的 `RUNTIME_PREFIX=YesPlease` 与 `NO_RUST=YesPlease`，支持工具随安装目录迁移，并避免 Git 上游的默认构建步骤调用 Cargo。macOS 包装保留 Git 的符号链接，签名顺序为内嵌 Mach-O 文件、外层 `.app`、最终分发包。 [Git 2.56.1 构建选项](https://github.com/git/git/blob/v2.56.1/Makefile) [Git 官方发布目录](https://www.kernel.org/pub/software/scm/git/)
- Unix 预览从仓库根目录运行 `go run ./cmd/prepare-unix-setup -out "$output"`。命令在 macOS arm64 或 Ubuntu 22.04 x64 构建环境中使用独立的 NSGo CLI 副本保留 Git 的链接和权限，并生成 Git 对应源码附件。macOS 的 Go 主程序和随包工具以 13.0 为最低部署版本。Linux Git ELF 以 glibc 2.35 为基线，私有动态库仅由 Git ELF 加载。 [Go 1.27 系统要求](https://go.dev/doc/go1.27#ports) [NSGo Linux 后端](https://github.com/nsgo-dev/nsgo/blob/v0.2.0/internal/platform/linux/linux.go)
- Linux RPM 从同次构建的 deb 载荷生成，沿用应用程序、相对 Git 工具链接、桌面入口与图标。预览使用 `gitwave-preview` 包名、`/opt/gitwave-preview` 与 `/usr/bin/gitwave-preview`，正式包使用 `gitwave`。RPM 仅声明应用的宿主依赖，包内 Git 私有库不提供给其他系统包。系统证书位于 Fedora 路径时，随包 Git 在系统配置层设置本机服务和 HTTPS 代理的 CA，保留 `/etc/gitconfig`、用户和仓库配置的覆盖顺序。 [RPM spec 格式](https://rpm.org/docs/4.20.x/manual/spec.html) [libcurl HTTPS 代理 CA 配置](https://curl.se/libcurl/c/CURLOPT_PROXY_CAINFO.html)
- Linux 安装包在 Ubuntu 22.04 的 glibc 2.35 环境编译维护版 GTK 4.14、GLib 2.80 和 Wayland，源码及补丁逐个校验，原始归档、构建脚本和校验清单随 CI 工件交付。Linux 打包支持 `-gtk-assets` 读取已缓存的源码附件。应用通过子进程探测宿主 GTK，旧 GTK 环境按绝对路径加载包内运行库，私有 ELF 通过 `$ORIGIN` 查找随包依赖。AppImage 在校验运行库与 schemas 后才向旧进程确认交接。deb 依赖要求宿主满足新版 GTK 或随包 GTK 所需的 JPEG、TIFF 和 Cairo ABI。GTK 的输入法、辅助技术及其他旧版发行包环境仍需实测。 [GTK 运行时路径](https://docs.gtk.org/gtk4/running.html)

## 数据、凭证与隐私

- 正式构建沿用 `GitWave/state.db`、schema 1–4、Workspace/repo ID、排序及现有 JSON 字段，首次接管前通过在线备份保留包含已提交 WAL 的原版数据库。新增备份放在数据目录的 `backup-before-nsgo-*/state.db`。Go 正式版和 Tauri 过渡版持有同一进程级文件锁，首次接管的 SQLite 写事务把备份与迁移顺序固定。预览和开发继续使用独立数据库。已验证迁移失败、空库并发初始化、重复启动和 Rust 读取回退。macOS DMG 门禁已覆盖首次备份空间不足时保全原数据库及释放空间后的原位重试。按正式清单自动升级和正式签名回退仍需验收。
- Tauri 过渡版在启动、设置变更、退出和更新时，把主题、语言、字体、分栏、最近 Workspace、自动刷新与更新设置导出为 `app_settings` 内的白名单快照。Go 在同一数据库中幂等导入，并保留已有 Go 偏好。完整偏好迁移需要过渡版写入一次快照。直接跳过过渡版时，Go 仍会打开原工作区并提示偏好快照缺失。缺少可用快照且尚未保存 Go 自动检查偏好时，启动后台检查暂停，手动检查与之后的显式偏好仍可用。
- 延续 `gitwave.ai` / `workspaceID:provider` 与 `gitwave.remote` 的历史账号映射。Windows 旧 keyring 使用 `account.service` 作为目标，NSGo 使用 `service/account`；Linux 旧库查 `username`，NSGo 查 `account`。通过平台适配层读取旧条目，用旧版写、新版读的实机 fixture 验证，并保留系统 helper、应用凭证弹窗和 vault 回退。SSH 验证配置 key、agent、已知主机、端口和口令提示。凭证不写到日志、迁移快照、命令参数或环境变量。
- 保留系统／手动代理与 bypass。按操作构造代理配置，离线 AI 仅通过直连 loopback 并禁止重定向。所有 AI 出站经统一脱敏与授权流程；AI 只能提出建议，实际写操作由用户确认。

依据：[SQLite 路径和初始化](../../../src-tauri/src/infrastructure/persistence/sqlite.rs)、[旧凭证命名](../../../src-tauri/src/infrastructure/ai/secrets.rs)、[Git credential 协议](https://git-scm.com/docs/git-credential)、[NSGo Windows 凭证实现](https://github.com/nsgo-dev/nsgo/blob/v0.2.0/internal/platform/windows/secrets.go)、[NSGo Linux 凭证实现](https://github.com/nsgo-dev/nsgo/blob/v0.2.0/internal/platform/linux/secrets.go)。

## 三端包装、更新与回退

沿用 macOS arm64、Windows x64、Linux x64 三端现有承诺。NSGo 锁定 v0.3.0，目标放在 `build.targets`。Go CI 逐端构建禁用更新通道的预览安装器。正式包装启用独立发布流程，验证 Linux GTK 4.14+、Pango、Cairo，输入法、DPI、窗口行为、键盘及 VoiceOver/Narrator/Orca 每端分别实测。

| 平台 | 产物与更新 | 首次切换验证 |
|---|---|---|
| macOS arm64 | 签名公证的 `.app` / `.dmg`，应用内更新下载保留代码签名和符号链接的 `.app` ZIP。 | 正式 bundle ID `desktop.gitwave.work` 和签名团队延续。旧 updater 安装旧格式签名的 NSGo `.app.tar.gz`，再从 NSGo 升级并重启。 |
| Windows x64 | 签名 exe / NSIS，应用内更新复用同一安装器。 | 测试旧安装目录、卸载注册项、自定义路径和旧 updater 参数，原位升级仅保留一条应用记录，卸载保留数据。 |
| Linux x64 | deb、rpm、AppImage。NSGo 有 deb 和 AppImage，rpm 单独打包。deb/rpm 保留包管理器路径。 | 验证 AppImage 的签名清单、原位替换和重启。 |

macOS 正式候选包通过 [Go macOS release candidate](../../../.github/workflows/go-macos-candidate.yml) 在 `main` 上手动构建。Developer ID 签名沿用原 CI 的 `APPLE_CERTIFICATE`、`APPLE_CERTIFICATE_PASSWORD` 和 `APPLE_SIGNING_IDENTITY`。App Store Connect 公证沿用 `APPLE_API_KEY_P8`、`APPLE_API_KEY` 和 `APPLE_API_ISSUER`。候选构建和聚合发布均直接引用同名 GitHub Actions Secrets。Runner 在签名步骤中使用临时钥匙串和私有目录，步骤结束时清理。候选构建还需要 `GITWAVE_UPDATE_PUBLIC_KEY` 仓库变量和 `X.Y.Z` 版本号。

打包命令为 `go run ./cmd/prepare-unix-setup -out <新目录> -release-version <X.Y.Z> -sign <Developer ID Application 身份>`。本机执行可以用 `-notarize <keychain profile>` 提供公证凭证。核验签名、公证票据、更新器解包和应用签名哈希后，构建输出 DMG、Go 更新 ZIP、两份 SHA-256 和 Git 源码归档。上传工件前通过隔离的 DMG 安装、升级及回退门禁。三端正式发布、旧版过渡包和签名升级端到端验收仍在 M5。 [Apple 分发打包规范](https://developer.apple.com/documentation/xcode/packaging-mac-software-for-distribution)

三端聚合发布工作流在构建前校验 `GITWAVE_UPDATE_PUBLIC_KEY` 仓库变量与 `GITWAVE_UPDATE_SIGNING_SEED` Secret 是否配对。当前 GitHub 仓库缺少这两项配置。macOS 正式候选包只需公钥变量以及现有 Apple 签名和公证凭证，聚合发布另需 Go 更新签名种子。

2026-11-25 再次核对官方仓库，最新公开版本仍为 v0.11.0。原有六项 Apple secrets 已配置，Go 更新签名配置和 Windows Artifact Signing 仓库变量仍待补齐。

已有 Go 正式版本后，聚合发布还用当前公钥验签首个 Go 过渡版及最近 Go 正式版的 `gitwave-update.json`，并确认清单版本与 release tag 一致。旧版本的 Go 更新清单缺失时，即使 Go 过渡清单仍在，流程也会把它识别为已有 Go 发布并中止续发。维持已安装客户端所信任的更新公钥，除非先发布支持迁移密钥的客户端。

首次 Go 版本的 macOS 候选构建可启用 `legacy_bridge`。它复用候选中通过签名、公证和 arm64 校验的 `.app`，生成 `GitWave_<版本>_aarch64.app.tar.gz`、`.sig` 和 `.sha256`。Go 打包工具核对 `TAURI_SIGNING_PRIVATE_KEY` 与已安装 Tauri 客户端内置的公钥，签名的受信任注释绑定版本。候选门禁按旧更新器剥掉归档最外层目录的方式解包，核对归档和 DMG 的应用签名哈希，以归档中的应用完成首次迁移，再用 DMG 中的应用重开工作区并回退到 v0.11.0。两种产物在门禁通过后上传。 [Tauri CLI 签名协议](https://raw.githubusercontent.com/tauri-apps/tauri/tauri-cli-v2.12.2/crates/tauri-cli/src/helpers/updater_signature.rs) [macOS 旧更新器解包](https://github.com/tauri-apps/plugins-workspace/blob/updater-v2.13.3/plugins/updater/src/updater.rs#L1316-L1340)

Windows 优先验收实际发布过的 NSIS 安装。v0.11.0 的 Windows 资产是 `GitWave_0.11.0_x64-setup.exe`，已发布资产清单中未见 MSI。旧安装的卸载键按产品名 `GitWave` 注册，NSGo 正式包按 `desktop.gitwave.work` 注册。过渡安装器应复用旧键中的安装目录，兼容 `/P /UPDATE /R /ARGS`，在新安装验证成功后更新唯一的卸载记录。失败路径需要恢复应用自有文件与注册项，并保留用户数据和安装目录中的其他文件。 [v0.11.0 发布资产](https://github.com/Yangshifu1024/GitWave/releases/tag/v0.11.0) [Tauri NSIS 模板](https://github.com/tauri-apps/tauri/blob/tauri-v2.12.2/crates/tauri-bundler/src/bundle/windows/nsis/installer.nsi#L63) [Tauri updater 参数](https://github.com/tauri-apps/plugins-workspace/blob/updater-v2.14.0/plugins/updater/src/config.rs#L36)

Windows 正式候选由 [Go Windows release candidate](../../../.github/workflows/go-windows-candidate.yml) 构建。CI 使用 OIDC 登录 Microsoft Artifact Signing 公共信任证书配置，使用 `GITWAVE_WINDOWS_AZURE_CLIENT_ID`、`GITWAVE_WINDOWS_AZURE_TENANT_ID`、`GITWAVE_WINDOWS_AZURE_SUBSCRIPTION_ID`、`GITWAVE_WINDOWS_SIGNING_ENDPOINT`、`GITWAVE_WINDOWS_SIGNING_ACCOUNT`、`GITWAVE_WINDOWS_SIGNING_PROFILE` 和 `GITWAVE_UPDATE_PUBLIC_KEY` 仓库变量。打包命令 `go run ./cmd/prepare-windows-setup -script <NSGo 脚本> -out <新目录> -release-version <X.Y.Z> -signer <签名程序>` 也接受能在原路径签名一个文件的现有签名工具。按主 EXE、嵌入卸载器、最终安装器的顺序签名。候选上传前验证三者的签名、时间戳、版本和 v0.11.0 原位迁移，提供签名后的 `GitWave.exe` 给 Go 更新清单计算摘要。 [Artifact Signing 集成](https://learn.microsoft.com/en-us/azure/artifact-signing/how-to-signing-integrations) [SignTool](https://learn.microsoft.com/windows/win32/seccrypto/signtool)

先在隔离 feed 演练 Tauri v0.11.0 → Tauri 偏好导出版 → NSGo 首版 → NSGo 下一版 → 回退。保留已安装客户端使用的 `latest.json`，指向三端同版本的 Tauri 偏好导出包。偏好导出版从独立的 `gitwave-legacy-transition.json` 查找 Go 首版兼容包。两份旧格式清单各自引用固定发布 URL，平台包的签名版本与所在清单的全局版本一致。后续稳定 release 继续附带两份固定过渡清单，以覆盖稍后才升级的用户。

偏好导出版在过渡清单上线前回退到 `latest.json`，下载时要求签名绑定公告版本。发布侧用 `go run ./cmd/prepare-legacy-feeds -export-version <X.Y.Z> -export-assets <目录> -go-version <X.Y.Z> -go-assets <目录> -out <新目录>` 同时验收两份清单。每份输入包含 macOS 应用归档、Windows NSIS、Linux AppImage、deb、rpm 及各自的原始 Minisign `.sig`。工具验证已安装客户端公钥、受信任的版本及文件名，再输出包含原有八个目标的清单。两套安装包均通过安装升级验收后才附带清单公开发布。 [旧版更新器锁定版本](https://crates.io/crates/tauri-plugin-updater/2.13.3)

旧发布流程在三端工件齐全后，以 `-export-only` 从同一草稿下载五个包和签名并生成 `latest.json`。验签 job 按 Release ID 上传和回读清单。重复运行时核对已有清单的内容，差异会使 job 失败。Go 联合发布重新验签，并和已公开导出版的清单比对版本及八条路由，确认两条升级路径使用相同的包和签名。 [导出版发布流程](../../../.github/workflows/build.yml) [Go 联合发布](../../../.github/workflows/go-release.yml)

Linux 正式包通过 [Go Linux release candidate](../../../.github/workflows/go-linux-candidate.yml) 在 Ubuntu 22.04 构建。正式 deb 和 rpm 沿用 v0.11.0 的 `git-wave` 包名与 `/usr/bin/gitwave` 入口。包内保留 `GitWave.desktop` 作为兼容入口，并使用 `NoDisplay=true` 避免在应用菜单出现两项。候选流程检查 AppImage、deb、rpm 的 SHA-256，执行 v0.11.0 包升级、启动与回退，再上传产物。 [v0.11.0 发布资产](https://github.com/Yangshifu1024/GitWave/releases/tag/v0.11.0) [桌面入口规范](https://specifications.freedesktop.org/desktop-entry/latest/recognized-keys.html)

偏好导出版的 Linux deb/rpm 更新按钮打开清单所告知版本的固定发布页。打开失败时重试更新检查，保留包管理器安装流程。后续 Go 正式发布的说明需提供偏好导出版的固定链接，帮助仍在使用 v0.11.0 手动下载入口的用户完成两段升级。

Go 客户端读取独立的 `gitwave-update.json`，其 Ed25519 签名覆盖版本、发布说明、三端包的长度与 SHA-256，以及 Windows 安装器内已签名的 `GitWave.exe` 的 SHA-256。发布步骤通过 `go run ./cmd/sign-update` 生成清单，要求私钥种子匹配打包时注入的公钥，并通过 `-windows-exe` 传入打包使用的已签名可执行文件。初次升级保留数据备份，回退默认保留已经产生的用户数据。CI 收齐三端产物和校验文件后继续创建人工审核的 draft release。

联合发布由 [Go desktop release](../../../.github/workflows/go-release.yml) 从 `main` 手动执行。输入当前版本、已公开的偏好导出版和 Go 首版。首次 Go 发布时，三个候选连同旧格式 macOS 归档一并验收，Windows 与 Linux 兼容包使用旧 Minisign 密钥签名。后续发行版从固定历史标签重新核验导出版及 Go 首版资产，并携带相同的两份旧格式清单。三端门禁及 SHA-256 全部通过后再签署 `gitwave-update.json`。正式版本通过 `make bump VERSION=X.Y.Z` 写入 `cmd/gitwave/nsgo.yaml`。如启用 draft，输入版本必须与该配置一致，现有版本标签必须指向该次 `main` 的构建提交。Tauri 过渡版通过原工作流手动选择其标签生成草稿。 [GitHub Actions 可复用工作流](https://docs.github.com/en/actions/how-tos/sharing-automations/reusing-workflows) [GitHub CLI 草稿发布](https://cli.github.com/manual/gh_release_create)

依据：[现有 updater 配置](../../../src-tauri/tauri.conf.json)、[现有 release 工作流](../../../.github/workflows/build.yml)、[Tauri updater](https://v2.tauri.app/plugin/updater/)、[NSGo 安装包与签名](https://github.com/nsgo-dev/nsgo/blob/v0.3.0/docs/public/en/building.md)、[NSGo 更新机制](https://github.com/nsgo-dev/nsgo/blob/v0.3.0/docs/architecture/updates.md)。

Go 发布的组装 job 将全部发布文件的 SHA-256 随 Actions 工件一同交接。草稿 job 在上传前校验交接内容，再按 Release ID 从 GitHub 核对完整的资产名称、上传状态和 SHA-256。对同一标签的现有草稿重跑时，只读取并复验草稿。资产缺失、额外资产或摘要差异会使门禁失败。 [GitHub Release Assets API](https://docs.github.com/en/rest/releases/assets#list-release-assets)

## 测试门禁

- 旧版与 Go 版使用相同 fixture 的独立副本，比较 refs、index、工作区文件、stash、reflog 和操作状态。覆盖 unborn/detached HEAD、双侧修改、换行／Unicode 路径、冲突、断网、权限、外部进程竞争、取消与恢复。要求写操作退出后没有意外文件或状态变更。
- PR 门禁包含 `gofmt`、`go vet`、`go test`、支持平台的 `-race`、NSGo 模型测试、三端实机和安装冒烟。发布门禁加入旧数据库及凭证、更新、损坏产物拒绝与回退。保留旧版门禁直至切换。
- Go 测试的外部 Git 夹具使用 Git 2.46 或更新版本和 `PATH` 中的 `git-lfs`。LFS 回归包含在已启用 LFS 的仓库中执行原生 Git 命令及通过 Go 用例恢复工作树。安装包冒烟另行在系统 Git 缺失时验证随包工具。
- 使用原有 [性能预算](../../tech/engineering/00-overview.md#性能预算)：Apple Silicon 冷启动 < 1 秒、10k commit History < 500 毫秒、1 MB diff < 200 毫秒。另测分页增长、快速切 repo、跨分栏、长路径及后台 Git 是否阻塞主界面。

## M0 执行记录

- 迁移前的只读 History/diff 预览通过 `make native-test` 及 macOS 原生窗口交互测试。Windows 与 Linux 已验证 Go 前端交叉编译。预览仍可用 `make native-dev` 启动。
- 第一批 Go Git 验证覆盖仓库工作目录、linked worktree 共享目录、porcelain v2 状态、merge 与 interactive rebase 冲突以及 abort 后的仓库状态。`make native-git-test` 使用临时 Git fixture，当前适配层独立于正式界面。
- NSGo v0.2.0 按 `build.targets` 产出 macOS arm64 `.app`、Windows x64 EXE 和 Linux x64 预览包。此项验证使用未启用 installers 的构建。Git 发行物、网络凭证、旧 SQLite、签名安装、Windows/Linux 实机和升级桥接待验证。
- macOS arm64 使用 Git 2.54.0 运行 `make native-git-test`（7 项）、`make native-test`、原生 Go `-race` 测试、`go vet` 和 `make check` 均通过。Go Git 测试二进制已交叉编译到 Windows 和 Linux x64。

## M1 执行记录

- 原生预览的仓库分支、History 页、提交详情、提交 diff、图片和 blame 均切换到 Go Git CLI。M1 曾通过独立 `make native-parity-test` 对照旧版。M5 已将该入口的回归覆盖迁入默认 Go 门禁。
- 增加独立 [Go 桌面 CI](../../../.github/workflows/go-desktop.yml)，在三端检查格式、静态分析、测试和编译。旧桌面门禁仍作为迁移对照，签名安装及发布工作流待切换。
- History 用真实 Git fixture 对照旧 Rust，覆盖 refs、annotated tag、merge、detached HEAD、bare repo、过期游标续页和稀疏搜索。Go 从 Git 读取父子关系与提交时间，按旧版的规则处理同秒提交。跨两页的多分支 fixture 对照了每行提交顺序、父提交和泳道，单独压实 refs 不会使已有游标失效。
- 已修复 diff 汇总加载失败时提交标题、作者和正文一起消失的问题，并加入 NSGo 回归。只读 Git 操作同时禁止 partial clone 按需网络取对象和 Git transport，后续网络操作使用单独的用例入口。
- 提交 diff 以首父提交为基线，沿用文本行数、字节数和 Unicode 字符限额。对比初始提交、合并、删除、二进制、CRLF、无末尾换行、超限及扩展预览的旧 Rust 结果，并对比图片 blob 字节。修复文件被目录取代时无关子项影响预览限额、文件类型变化只显示旧侧或出现同名项、空图片卡在加载中的问题。
- 设置 Windows Git 子进程的无控制台标志，并阻断外部 diff/textconv、仓库 diff 排序文件和按需网络读取。Windows/Linux 交叉编译用于检查平台实现，实际窗口和平台安装仍待实机验收。
- 原生预览接入 Workspace SQLite，支持创建、重命名、删除、切换、批量添加仓库和跨重启恢复所选仓库。在加载 Workspace 时核对每个已保存的仓库根目录，缺失时回退到可用仓库，原路径恢复后清除缺失标记。仓库去重比较规范化路径，并保留旧数据库内的原始路径。
- Go 和 Rust 共用原有四个 SQL 迁移文件。Go fixture 验证从 schema 2 升到 schema 4，保留 Workspace 和 repo ID、排序、昵称及两种设置 JSON 的未知字段。Go 预览使用 `GitWave Preview/state.db`。首次创建时，通过 SQLite 在线备份复制 `GitWave/state.db`，涵盖旧库 WAL 中已提交的数据。复制、迁移检查和发布在独立目录内完成。已有 Go 数据、旧库原件及原有 WAL 文件都保留。正式安装器和回退仍待验收。
- 仓库标签支持拖动实时预览、右键左右移动、方向键及 Home/End、滚轮横向滚动、缺失仓库重链接和带确认的移除。SQLite 会拒绝覆盖拖动期间其他窗口已经保存的顺序。移除和更新选中仓库在同一事务内完成。重链接保留原 repo ID、排序、昵称和设置，遇到同 Workspace 的重复路径时保留已打开的输入。
- Workspace 菜单接入原有 v1 JSON 导入导出。导入用同一 SQLite 事务保存新 Workspace、仓库顺序和首个可用仓库，显示跳过的无效或重复路径，支持正常工作树和裸仓库。导出沿用旧格式的三个字段，已有目标需确认覆盖。共用数据库的预览进程串行复核覆盖凭证，保护打开中的 SQLite 文件及辅助文件。
- 返回前台和手动刷新可重新检查 Workspace 路径、分支、refs 和浅克隆边界。普通刷新保留原搜索、已加载深度、滚动锚点和选中 diff。路径恢复或缺失时更新标签状态，底层 worktree 换成其他仓库时重新绑定。刷新与交互写入按窗口当前 Workspace 和明确目标加载，重试保留上次的加载目标。
- 提交 diff 提供原生 blame 面板、返回 diff 和提交跳转。历史文件使用当前选中的提交，已删除文件使用首父提交。Blame 保留原始作者与提交者时间，关闭面板或切换文件与仓库时取消后台读取。大文件和异常大的 Git 输出有独立预览上限。
- Ubuntu 24.04 X11 已用同一 Git 仓库对照 v0.11.0 和 Go 工作台。已补齐选择 Workspace、仓库和提交的空状态，修复旧提示指向侧边栏的问题。搜索按钮回到命令面板，切换仓库或 Workspace 后恢复该仓库的 History 提交与 diff 文件。侧栏分组、History 表头与行样式继续对照，测试结果见 [审查记录](./review.md)。手动刷新后的远端 Fetch 和可配置自动刷新见 M2 执行记录。

## M2 执行记录

- 原生工作台接通 Changes 导航、标题栏入口和变更计数。按冲突、未暂存、已暂存分组显示，同一路径的两侧分别可预览。按仓库恢复模式和选中文件，外部刷新后优先保留同一侧，若该侧消失则跟随仍存在的另一侧。
- 选中文件可查看文本 diff、图片及 HEAD blame。已暂存比较 HEAD 与 index，未暂存比较 index 与工作树，未跟踪文件作为新增处理。常规未暂存文件通过有界副本生成 patch，状态与工作树 diff 使用私有 Git 配置读取。预览运行时无需旧 Rust 后端。
- 原生 Stage/Unstage 支持状态图标、选中项和分组全部操作，接通范围选择、切换选择、键盘导航和全选。每次写入复核仓库、HEAD 和所选项，按仓库绑定结果及错误。临时索引写完后经 Git 索引锁原子发布，重命名两个路径一并处理。预览版 Stage 通过 Git 2.42 起的属性路径查询冻结属性文件。外部过滤器支持留在 M3。
- 手动 Commit 和 Amend 已接通 Changes。支持 Conventional Commits 类型、首行长度提示、键盘提交、按 Workspace 和 repo 恢复会话草稿，以及成功后跳转 History。Amend 需要确认具体分支和 HEAD。取消确认保留当前草稿，进入修正后可恢复修正前的草稿。
- Commit 复核 HEAD、索引和操作状态，提交私有索引生成的树。支持首次提交、detached HEAD 上的普通提交、仅修改提交信息，以及工作树无额外改动时完成 merge。浅克隆的边界提交缺少父对象时，Amend 会保留原提交并报告错误。rebase、cherry-pick、revert 及带 autostash 的 merge 继续操作留在 M3。
- Discard 已接通未暂存文件右键、选中项和整组操作。确认区分恢复跟踪文件和删除未跟踪文件。操作从索引恢复已暂存版本，保留索引及未选中文件。确认后的外部编辑会使本次请求失效。参考旧版继续禁用直接丢弃未暂存的重命名，子模块及 intent-to-add 条目需要单独验收。
- 本地分支支持从 HEAD、分支或选中提交创建、重命名、设置或清除 tracking，以及确认后删除。重命名当前分支保留检出状态及 upstream。删除复核用户确认的 tip、现有 worktree 占用和暂停操作。
- 标签可在提交详情、History 右键及分支菜单创建，支持轻量与附注。替换和删除需要确认。侧栏 Tags 支持导航和删除。只被标签引用的提交仍可直接查看详情。
- 干净工作树可检出本地分支、远程跟踪分支、任一提交或标签。远程跟踪分支优先复用同名本地分支，无本地分支时创建并设置 upstream。分支检出需要 Git 2.46+ 的符号引用事务，Go CI 已加入版本门禁。 [Git 2.46 发布说明](https://raw.githubusercontent.com/git/git/v2.46.0/Documentation/RelNotes/2.46.0.txt)。
- 普通脏工作树恢复取消、丢弃并切换、贮藏并切换三个选项。执行前复核确认时的 HEAD、索引和文件内容，保护忽略文件。贮藏并切换分别保存暂存、工作树和未跟踪内容。成功重放保留原有 stash 列表，重放失败时保留可恢复的 stash，已有冲突索引留在目标分支。 [Git stash](https://git-scm.com/docs/git-stash)。
- 仓库菜单接入初始化。确认后在输入的目录创建 unborn `main` 并加入当前 Workspace，仓库登记和激活在同一个 SQLite 事务。支持尚不存在的路径与普通嵌套目录。Git 初始化成功而登记失败时，原表单可以用同一路径重试。 [Git init](https://git-scm.com/docs/git-init)。
- 原生克隆接入源地址、目标路径、目录选择器、进度、取消、重试及 HTTPS 认证。目标同卷的私有副本经 Git 检出验证后才会发布。替换非空目标需要确认，并保留原目录作为备份。对各 Workspace 的登记路径、目标变化和本地来源目录执行保护检查。登记失败时使用已经发布的仓库重试。 [Git clone](https://git-scm.com/docs/git-clone)。
- HTTPS 继续使用已配置的 Git credential helper。认证失败支持一次性输入用户名和令牌。用户选择记住时，远端验证成功后使用 Git helper 保存并读回验证。旧应用凭证库的跨版本迁移和系统 helper 缺失时的应用内持久化仍待接通。 [Git credential](https://git-scm.com/docs/git-credential)。
- 工具栏和仓库菜单接通 Fetch，状态栏展示进度并支持取消。依次获取所有远端，单个普通失败后继续尝试，认证挑战或取消中止批次。HTTPS 重试只访问发出挑战的远端，重新核对地址后才使用凭证。远端跟踪引用在隔离 Git 目录中计算后事务发布，保留本地分支、标签、索引及工作树。 [Git fetch](https://git-scm.com/docs/git-fetch)，[Git update-ref](https://git-scm.com/docs/git-update-ref)。
- 工具栏和分支菜单接通 Pull，对话框保留远端、源分支、目标分支、Rebase 和贮藏选项，工具栏显示落后计数。自定义 fetch refspec 按远端源分支显示。选中源分支的 OID 会与获取的跟踪引用核对，再经 Git 引用事务快进当前本地分支。分叉时，选择 Rebase 会先在私有工作树重放提交。重放冲突保留当前本地分支、索引和工作树。 [Git rebase](https://git-scm.com/docs/git-rebase)。
- Pull 的普通贮藏在推进分支前保存索引、工作树与未跟踪文件，推进后恢复。认证重试继续使用原操作确认。确认后的外部改动会使贮藏请求失效，重放冲突则保留可恢复 stash 并显示错误。 [Git stash](https://git-scm.com/docs/git-stash)。
- 工具栏和分支菜单接通 Push。工具栏支持选择远端、随分支 tip 推送标签、预览旧与新 OID 后二次确认强推。强推为每个目标引用指定 `--force-with-lease`。多推送地址逐一处理，HTTPS 认证重试从发出挑战的地址继续。批量引用逐行确认并按命令长度分批，已验证 700 个标签的普通推送、强推预览和强推。 [Git push](https://git-scm.com/docs/git-push)。
- `⌘R` / `Ctrl+R` 先刷新当前仓库的本地数据，再获取该仓库的远端更新。启用设置中的自动刷新后，定时从当前 Workspace 的各仓库获取远端更新。间隔可设为 1–1440 分钟，默认关闭，启用时默认 5 分钟。旧 WebView 的自动刷新偏好仍待导入。
- Fetch、Pull 和 Push 的仓库专属代理、凭证配置和 SSH 实际远端仍待兼容性验证。常规浅克隆可获取后续提交，改变浅克隆边界的操作尚待迁移。脏 gitlink、目录与文件相互替换、Changes 的外部过滤器、子模块内部脏状态及三端实机对照仍需覆盖。未执行 `git add -N` 的普通移动目前显示为删除和新增。

## M3 执行记录

- 侧栏本地分支与 History 引用菜单接通「合并到当前分支」。确认对话框展示源、目标、快进选项和冲突预检。执行绑定双方引用与提交，记录 `ORIG_HEAD`，保护脏工作树与忽略文件。支持快进、强制生成合并提交、三方合并及已合并时无改动返回。 [Git merge](https://git-scm.com/docs/git-merge)，[Git merge-tree](https://git-scm.com/docs/git-merge-tree)。
- 有冲突时保存工作文件标记、未合并 index stages 和 `MERGE_HEAD`，原 HEAD 保持不变。稀疏检出的冲突文件单独落到工作树，继续排除其他文件。Changes 可以暂存外部编辑后的冲突结果或明确暂存删除，并提交完成合并。
- Merge 横幅与 Changes 接通原生冲突编辑器。文件列表支持切换并保留会话草稿，展示我方、基准和对方内容。编辑区标记冲突块并支持导航、整文件选边及暂存。选择缺失一方会暂存删除。二进制支持按 Git stage 选边，保留原字节。外部已暂存的文件仍可打开未保存草稿并复制。
- Merge 横幅和冲突编辑器接通「中止合并」。取消保留合并和草稿，确认后恢复本次合并修改的索引条目和文件，并保留独立的暂存、未暂存、未跟踪及忽略文件。原生 Merge 保存 `AUTO_MERGE` 作为结果基线，常规外部 Git Merge 可使用同一入口。无法区分合并内容和额外编辑时中止请求失效，重叠文件保留。带 autostash 的外部 Merge 和缺少结果树且暂存归属不明确的外部 Merge 仍待迁移。 [Git merge](https://git-scm.com/docs/git-merge)，[Git ls-files](https://git-scm.com/docs/git-ls-files)。
- 本地分支菜单接通「将当前分支变基到此分支」。当前分支和上游均绑定菜单所见的提交。已包含上游时直接返回，落后时快进，分叉时复用 Pull 的私有重放。普通 Rebase 沿用旧版冲突后退出的语义，报告冲突文件并保留当前检出。成功时记录 `ORIG_HEAD`，保持原分支和 tracking。上游已 revert 的本地补丁会重新应用，确实已生效的等价补丁按空应用跳过。检测到线性 Rebase 会丢失 merge commit 独有内容时保留当前分支，多亲 merge 暂需单独处理。 [Git rebase](https://git-scm.com/docs/git-rebase)，[Git merge-tree](https://git-scm.com/docs/git-merge-tree)。
- 本地分支菜单接通交互式 Rebase。提交可拖动或用方向键排序，支持 pick、reword、edit、squash、fixup、drop 和提交信息输入。提交在私有工作树中重放，edit 暂停与当前分支通过同一引用事务发布。重启后可继续编辑、显式 Amend、暂存后 Continue，或在工作树干净时放弃暂停。Continue 遇到冲突且有待并入的暂存修改时保留暂停和修改。 [Git cherry-pick](https://git-scm.com/docs/git-cherry-pick)，[Git worktree](https://git-scm.com/docs/git-worktree#_refs)。
- 旧版 `gitwave-interactive-rebase.json` 通过 HEAD 和当前分支的 reflog 确认归属后迁移，切回原分支后可继续。缺少 `original_head` 的记录可只放弃暂停记录，保留当前改动。无法核对归属的旧记录保留原状。
- History 右键和提交详情「更多操作」共用 Cherry-pick 与 Revert 确认，绑定所选提交与目标 HEAD。Cherry-pick 保留作者、原提交日期、消息和来源 SHA，重复且结果为空时报告错误。Revert 生成反向提交，支持实际根提交与分离的 HEAD。两者在私有工作树重放，冲突时报告路径并保留原仓库状态。浅历史边界须有可用父提交，merge commit 仍需独立的主线选择流程。 [Git cherry-pick](https://git-scm.com/docs/git-cherry-pick)，[Git revert](https://git-scm.com/docs/git-revert)。
- 共用 checkout 在正向写入和失败回退前复核被移除的 gitlink。发现子模块目录含有内容时保留该目录，包括目标文件替换其父目录的情况。
- 工具栏接通保存 Stash，沿用可选消息和默认包含未跟踪文件。侧栏读取当前仓库的 stash reflog，双栏预览第一父提交的 tracked diff 及第三父提交的 untracked diff。切文件保留文件列表的滚动位置。私有索引分别记录暂存内容、工作树和选中的未跟踪文件。复核 HEAD 和索引，并在清理工作树前建立恢复引用。清理中途失败时保留快照。交互式 Rebase 暂停编辑时可以先保存再继续。 [Git stash](https://git-scm.com/docs/git-stash)。
- 侧栏和预览接通 Stash Apply。工作树有未提交改动时先显示确认，沿用旧版对干净索引的要求。Git 在私有工作树按确认的 stash OID 计算应用结果，发布前重新核对 reflog 条目与 HEAD。独立的本地文件保留在原位。三方合并有冲突时把未合并索引及冲突标记交给 Changes，并保留 stash。 [Git stash](https://git-scm.com/docs/git-stash)。
- 侧栏和预览接通 Stash Pop 和 Drop。Pop 只在工作树及索引成功更新且没有冲突后删除原条目。删除受阻时报告已经应用并保留可重新确认的条目。Drop 确认显示文件摘要，并在 Git 的引用锁内核对 reflog 快照及所选条目。普通 refs、packed refs 与 reftable 均覆盖最后一条的清理。旧 libgit2 1.9.7 在 Pop 有冲突时仍返回成功并删除 stash，Go 版回归验证了冲突时保留条目。 [Git stash](https://git-scm.com/docs/git-stash)，[Git reftable](https://git-scm.com/docs/reftable)。
- 侧栏恢复区接通 HEAD 和当前分支的 Reflog、创建恢复分支及硬重置。Reset 和 Amend 记录取真实的操作前 OID，即使其他日志记录已被清理。硬重置也可从 History 发起，确认绑定当前分支与 HEAD，记录 ORIG_HEAD，清除已确认的跟踪文件改动并保留独立的未跟踪文件。目标路径与本地未跟踪文件冲突时，原工作树与分支都保留。 [Git reflog](https://git-scm.com/docs/git-reflog)，[Git reftable](https://git-scm.com/docs/reftable)。
- Repository 菜单接通新建 Worktree。起点取对话框所见的本地分支提交，创建失败时保留原目录与分支状态。侧栏展示主 Worktree 和 linked Worktree，支持切换现有 Workspace 标签或添加标签后激活。默认折叠仅有主 Worktree 的列表。 [Git worktree](https://git-scm.com/docs/git-worktree)
- 移除前显示准确路径并确认。保留脏工作树、独立嵌套仓库、未初始化子模块目录中的文件，以及缺乏其他引用的 detached 提交。移除已失效记录时保留该路径上的文件。删除后刷新 Workspace 标签的可用状态。 [Git worktree](https://git-scm.com/docs/git-worktree)
- Remote 侧栏接通新增、编辑 Fetch 和 Push URL、改名、按远端 Fetch 和确认移除。Fetch 绑定所选远端的有效 URL 及 refspec，包括 Git URL 重写规则。改名和移除核对仓库配置与引用，保护共享的跟踪引用、符号引用和工作树 HEAD。 [Git remote](https://git-scm.com/docs/git-remote)，[Git update-ref](https://git-scm.com/docs/git-update-ref)
- History 引用菜单和分支侧栏接通远端分支删除。本地分支删除可以显式勾选对应远端分支。确认显示分支、实际推送目标和提交短 OID。每次删除使用确认时的完整 OID 作为 Git lease，并保留服务器已确认的分步结果。 [Git push](https://git-scm.com/docs/git-push)
- reftable 的远端改名会在持有 manifest 锁时准备私有副本，复制原始 reflog 记录，并由 Git 更新引用。发布前核对 checkout、manifest、原表、工作树 HEAD 及包括外部 include 在内的有效配置。发布失败时尝试恢复原配置，并清理未发布的新表。Git 2.46 和 2.54 的隔离仓库已覆盖 SHA-1、SHA-256、linked worktree 和多层索引。 [Git reftable](https://git-scm.com/docs/reftable#_log_block_format)，[Git update-ref](https://git-scm.com/docs/git-update-ref)
- Remote 侧栏识别旧式 `.git/remotes/` 和 `.git/branches/`，Fetch 使用其配置的 URL 与 refspec。编辑或改名将有效配置写入仓库配置，保留 Fetch/Push refspec，并在引用事务失败后恢复原文件。旧式 Remote 可以移除。对直接映射到本地分支的 refspec，Fetch 只允许明确的快进写入，拒绝改动已检出或被暂停操作占用的分支。当前分支的 Pull 继续通过工作树事务集成。 [Git fetch 的旧式远端格式](https://git-scm.com/docs/git-fetch#_remotes)
- Submodule 侧栏已接通列表、初始化、添加、递归更新和取消注册。同步状态以父仓库索引中的 gitlink 为准。添加成功后暂存 gitlink 和 `.gitmodules`。取消注册保留子模块工作树和未跟踪文件。递归更新从已验证的远端和 checkout 获取目标提交，认证重试从发生认证失败的子模块继续。 [旧 Submodule 后端](../../../src-tauri/src/infrastructure/git/submodule.rs)，[Git submodule](https://git-scm.com/docs/git-submodule)
- Repository 菜单接通 Git LFS 面板、仓库级安装和 `.gitattributes` 规则管理。linked worktree 从实际 Git 共享配置读取安装状态。安装会先由官方 CLI 在私有仓库准备结果，再按已验证的本地配置和 hooks 路径发布。仓库外共享 hooks 目录保持原样。规则编辑支持 C 风格引号，保留其他属性和换行格式，也能读取 index 中的备用规则。 [旧 LFS 实现](../../../src-tauri/src/infrastructure/git/lfs.rs) [Git LFS install](https://github.com/git-lfs/git-lfs/blob/v3.8.1/docs/man/git-lfs-install.adoc) [Git attributes](https://git-scm.com/docs/gitattributes)
- 已接通标准 LFS 对象的暂存、提交、状态、离线检出和 Stash 保存与应用。Go 通过私有 Git 过滤器生成 SHA-256 指针，写操作把对象存入仓库共享缓存或配置的 `lfs.storage`。检出校验本地缓存，缺失时保留指针。切换或应用 Stash 将覆盖工作树中仅存的完整内容时，先校验并补回缓存。 [Git LFS 指针规范](https://github.com/git-lfs/git-lfs/blob/v3.8.1/docs/spec.md)
- Push 按 Git 实际发出的 `pre-push` 引用更新计算 LFS 对象。文件远端直接写入校验后的对象，HTTPS 使用官方 Git LFS 基本传输。对象上传失败时保留原远端引用。确认框显示专用 LFS 目的地，确认后目的地配置变更会使请求失效。已通过本地裸仓库和 HTTPS 测试端点验证对象字节与远端引用。 [Git pre-push](https://git-scm.com/docs/githooks#_pre_push) [Git LFS pre-push](https://github.com/git-lfs/git-lfs/blob/v3.8.1/commands/command_pre_push.go)
- Pull 按所选远端分支 tip 的 Git 属性和 `lfs.fetchinclude`／`lfs.fetchexclude` 下载缺少的对象，校验对象和缓存后再发布引用与工作树。命中排除规则的文件即使已有缓存也保留指针。独立 HTTPS 媒体地址单独提示凭证，仅将实际发放的凭证路径交给 Git helper 保存。普通 Fetch 保持只更新 Git 对象与引用的语义。 [Git LFS fetch](https://github.com/git-lfs/git-lfs/blob/v3.8.1/docs/man/git-lfs-fetch.adoc)
- Clone 先在同文件系统的私有目录获取 Git 对象，再对初始 HEAD 复用标准 LFS 下载和路径筛选。检出时读取已验证的缓存，LFS 的备用地址指向私有文件端点。通过文件、索引和路径校验后发布目录并登记 Workspace。两次认证复用原目录确认，Git 与显式 LFS 地址分别使用各自凭证。具体目的地的 Git `includeIf.gitdir` 和实际 SSH 服务仍待验证或补齐。 [Git Clone](https://git-scm.com/docs/git-clone) [Git read-tree](https://git-scm.com/docs/git-read-tree)
- Changes 接通 `.gitignore` 编辑和未跟踪文件右键添加规则，可按当前文件、父目录或扩展名忽略。保存保留原文件及现有规则的换行格式。Git 预检子目录规则是否会覆盖新规则。选中文件被外部暂存、索引锁被替换或参与预检的规则发生并发编辑时，保留已有规则和文件。 [Git ignore patterns](https://git-scm.com/docs/gitignore)
- Repository 菜单接通八种客户端 Hooks 的状态、编辑和模板。路径沿用 Git 实际的共享目录或 `core.hooksPath`。保存不会执行脚本。有效的符号链接编辑其显示的目标，单条失效链接仍允许查看同目录的其他 Hooks。发布前会复核脚本、链接和 linked worktree 的共享 Git 目录。 [Git core.hooksPath](https://git-scm.com/docs/git-config#Documentation/git-config.txt-corehooksPath)
- 原生侧栏接通仓库健康的六项本地指标。按当前 worktree 获取操作状态，显示相对上游的未推送提交、脏文件、超过 30 天的本地分支、最多五个大文件和分支／标签计数。linked worktree 和 reftable 的操作状态按实际检出隔离。无效的非当前分支提交会被跳过，History 失败时仍可读取报告。大文件扫描到达预算会显示提示。
- 健康 AI 总结以及冲突与 Reflog 的 AI 解释随 M4 的 Workspace AI 配置与调用链迁移。高级 Git 的边界和三端实机验收继续按上述记录推进。
- 普通离线 checkout 按 `lfs.fetchinclude`、`lfs.fetchexclude` 及 `core.ignorecase` 选择可物化的路径。同一缓存对象的排除路径继续保留指针。仓库配置覆盖项目规则，包括显式空值。目标会写入项目配置时使用目标规则，目标删除或稀疏检出省略该文件时复用源检出的可用配置。项目配置文件名按实际卷判断大小写别名。脏切换和 Discard 使用即将恢复的项目规则，发布前复核配置，回退时保留源工作树已有的指针和物化文件。 [Git LFS 配置](https://github.com/git-lfs/git-lfs/blob/v3.8.1/docs/man/git-lfs-config.adoc) [Git LFS 路径匹配](https://github.com/git-lfs/git-lfs/blob/v3.8.1/filepathfilter/filepathfilter.go)
