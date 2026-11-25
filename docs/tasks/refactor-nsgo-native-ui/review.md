# GitWave 原生工作台预览审查

## 2026-11-25 · 工作台分支状态与旧工具链退役

按 v0.11.0 恢复 History 和 Changes 工具栏里的上游与分歧计数，分离 HEAD 显示短 SHA。外部切换分支后，工具栏、底部状态栏与 Pull 使用工作副本刷新所得的检出状态。重选当前工作区会恢复 diff 专注布局隐藏的分栏。AppKit 夹具覆盖 1024 点窗口、隐藏导航、再次选择 Changes、分离 HEAD、刷新后的 Pull 目标和取消。

外部 `git merge --autostash` 中止前先把原始快照保存至贮藏列表，再清理合并引用并恢复合并所改动的路径。合并期间新增的独立编辑留在索引和工作树。原始快照支持手动恢复，保留合并前的暂存与未暂存版本。files 和 reftable 的独立复核覆盖已存在的贮藏、竞态导致历史项丢失、被替换的引用，以及保存后中止失败的重试。 [Git 回归](../../../internal/gitcli/merge_abort_test.go) [原生窗口回归](../../../cmd/gitwave/merge_test.go)

在移除旧工具链的干净源码上，`make build` 通过 Go 格式检查、静态分析、全量测试和 NSGo 构建。macOS CI 所列的 26 项 AppKit 原生用例全部通过，包含 495 条变更、窄栏及长行 diff、设置、菜单、子模块、退出和恢复。最终代码另外通过 7 项冲突、Rebase、Pull、Push、Stash 和 Worktree 原生用例，新增的 3 项原生用例也已复跑。macOS arm64 应用通过本地 ad hoc 签名严格校验，Windows 与 Linux x64 Go 交叉构建通过。 [原生门禁](../../../.github/workflows/go-desktop.yml)

当前分支移除了 Tauri/Rust/React/pnpm 源码、构建入口与 CI。偏好导出版的完整源码、锁文件和标签工作流保存在 [`e175c65`](https://github.com/Yangshifu1024/GitWave/commit/e175c657d1516cd9b06f8ad7b973097613c0d001)。正式签名与公开升级按发布计划另行验收。 [导出版流程](./plan.md#偏好导出版检查点)

## 2026-11-25 · 文本 diff 窄栏与长行

在 307 点宽的提交 diff 中复现截断提示把「加载更多内容」推到面板外。工具栏和提示现在会换行，扩展预览仍超限时显示上限提示。可用的工作树文件同时提示在编辑器中打开。

原先按字符列数估算横向范围，连续 Tab 会留下大片空白，宽字形组成的 Unicode 长行可能被裁。现在先用 NSGo 测量较宽的含 Tab 和普通行，虚拟列表再以已挂载文本的实际宽度校正。继续沿用有界预览和虚拟列表，原始制表符和 Unicode 文本保留。同一 patch 已测出的范围在折叠及视图切换后保留，Changes 重新加载相同结果时复用原 patch。

AppKit 实测 50,100 行文件在窄栏内可扩展到预览上限，长 Tab 与旗帜 emoji 在统一及分栏视图均可滚到行尾，语言切换保留横纵位置。独立模型验证了折叠和 Changes 与 History 往返后保留横向位置。最终 `make check`、定向竞态检查和 Windows/Linux x64 交叉构建均通过。两项新增场景进入 macOS CI。 [文本 diff](../../../cmd/gitwave/diff.go) [回归测试](../../../cmd/gitwave/diff_test.go) [Go 桌面 CI](../../../.github/workflows/go-desktop.yml)

## 2026-11-25 · History 提交详情与复制

按 v0.11.0 恢复标题展开、完整正文及本地分支、远端分支和标签。标题折叠时显示一行，展开时可换行，详细信息在分栏高度的 45% 内滚动，文件列表和 diff 保留空间。展开状态跨提交及 History、Changes 切换保留，换仓后复位。详情分别显示同一提交上的本地和远端分支，只高亮当前本地分支。提交菜单的「复制提交信息」包含全 SHA、作者、本地时间和消息摘要。中文、美式和英式时间格式分别处理，其余地区目前采用美式格式。

920×640 的 AppKit 夹具验证了长正文可滚到最后、文件和 diff 仍可选择、无邮箱的长作者名不会撑宽详情，并用键盘打开提交菜单读取复制结果。独立复审用 320 点宽、长作者、长标题和长引用复核详情边界。10 项 AppKit 回归、`make check`、定向竞态测试及 Windows/Linux x64 交叉构建通过。新回归已加入 macOS CI，Windows 实机验收留待发布阶段。 [提交详情](../../../cmd/gitwave/view.go) [AppKit 回归](../../../cmd/gitwave/history_test.go) [Go 桌面 CI](../../../.github/workflows/go-desktop.yml)

## 2026-11-25 · Go 发布草稿产物核对

Go 组装阶段为安装包、签名、源码归档和更新清单生成 SHA-256 索引。草稿 job 先比对 Actions 下载结果，再按 Release ID 核对 GitHub 上完整的资产名称、上传状态和摘要。对同标签的现有草稿重跑时复验已有工件。部分上传的草稿会留给人工检查。上传前后重新检查发布历史，避免其他 Go 版本在组装期间公开后仍按首版条件通过。 [Go 联合发布](../../../.github/workflows/go-release.yml)

用 26 份已组装的首版夹具执行上传步骤的本地模拟，并覆盖续发、同标签重跑、分页、内容变化、缺少和额外资产、上传状态异常及发布历史并发变化。GitHub API 上公开 v0.11.0 macOS 归档的摘要与本地官方原件一致。独立审查确认首版历史检查会传播失败状态。正式 GitHub 草稿的创建和审核仍待发行时执行。 [GitHub Release Assets API](https://docs.github.com/en/rest/releases/assets#list-release-assets)

## 2026-11-25 · macOS 首版 Go 桥接包升级门禁

首版 macOS 候选先生成给旧更新器的 Go 归档。门禁剥掉归档最外层目录后验证应用的严格代码签名，并比对同批 DMG 的 CDHash。首次接管和磁盘空间不足后的重试由归档中的应用执行。迁移成功后从 DMG 安装并重新打开同一工作区，然后回退到 v0.11.0，确认旧版能读取升级后写入的数据。两种产物都在检查成功后上传。 [macOS 候选流程](../../../.github/workflows/go-macos-candidate.yml) [升级回退脚本](../../../scripts/verify-macos-upgrade.sh)

本机从官方 v0.11.0 DMG 启动，用预置 SQLite 偏好快照及同批 ad hoc 签名的 Go 归档和 DMG 完成实跑。另一份版本号相同但签名内容不同的归档在打开工作区前被拒绝。独立审查分别用锁定 Tauri updater 的 Rust 解包逻辑和系统 tar 解包同一夹具，验证严格签名及 CDHash 一致。旧应用经公开更新清单下载和安装的流程仍待正式发行验收。

## 2026-11-25 · 偏好导出版清单验签

三端构建分别上传安装包和 Minisign 签名。汇总 job 按固定 Release ID 下载工件，用 v0.11.0 客户端内置的公钥验证五个包、签名版本和文件名，再生成八条旧更新器路由。清单按同一个 Release ID 上传，按 Asset ID 下载核对。重复运行时已有清单必须与生成内容一致。Go 联合发布重新验签导出版及过渡版，并在组装前比对已公开导出版的版本、下载地址和签名。 [导出版发布流程](../../../.github/workflows/build.yml) [清单生成器](../../../cmd/prepare-legacy-feeds/main.go) [Go 联合发布](../../../.github/workflows/go-release.yml)

使用 v0.11.0 的五份官方签名资产复验，八条路由与公开清单一致。GitHub 公开 API 的 Release ID 枚举和 Asset ID 签名下载也通过。使用临时 Minisign 密钥和本地 GitHub 接口替身运行两个工作流的 shell 步骤，覆盖重复运行、草稿状态或标签变化、缺少签名、篡改安装包、草稿清单冲突和已发布路由不一致。独立审查发现的按可变标签覆盖清单问题已修复。Apple CI 引用继续使用原有六项 secrets。正式导出版的托管构建、Developer ID 签名、公证和公开更新升级仍待发行验收。

## 2026-11-25 · macOS Go 应用包升级与工作区恢复

联合发布将最近公开的 Go 版本交给 macOS 候选流程。候选上传前，门禁按已发布清单的公钥、版本和 SHA-256 校验旧 ZIP。真实的前版应用先启动并打开隔离数据库。随后测试进程让当前更新器读取隔离签名清单，校验并解包候选 ZIP，原位替换并通过正常的进程交接启动候选。正式 CI 对两个包使用项目 Developer ID 团队的代码签名要求。

门禁等待候选显示窗口，并确认签名哈希匹配的候选进程持有原数据库。它核对工作区、已选仓库、自动更新偏好与临时目录清理。测试应用和测试进程无法访问真实用户目录或建立出站 IP 连接。候选 ZIP 和测试进程在启动前复制到 `/private/tmp` 的隔离目录，以兼容托管 runner 在真实用户目录下的临时路径。 [候选门禁](../../../.github/workflows/go-macos-candidate.yml) [包升级测试](../../../internal/appupdate/packaged_update_darwin_test.go)

本机已用当前源码构建的 0.12.0 和 0.12.1 NSGo ZIP 实跑，两个应用使用 ad hoc 签名。将输入 ZIP 和测试编译临时目录放在真实用户目录内运行 `-race` 的复测通过。正式门禁的负例拒绝了 ad hoc 签名的前版，以及不匹配签名清单的 ZIP。`make check`、工作流静态检查与 v0.11.0 DMG 安装回退复测均通过。正式 Developer ID 产物，以及已发布前版从 GitHub 检查、确认和下载更新的完整流程仍待真实发行后验收。

## 2026-11-25 · macOS 磁盘空间不足后的升级恢复

macOS 候选门禁将隔离的 SQLite 资料放在固定 48 MiB 的 APFS 磁盘映像中，保留足够打开原数据库的空间，把可用容量限制到备份所需空间以下。新应用打开窗口并在首次在线备份中收到 `SQLITE_FULL`。释放空间前核对原数据库 SHA-256、完整性和迁移标记，确认没有残留的 WAL 内容或半成品备份。释放空间后在同一数据库上重新启动，完成偏好导入并保留一份可用的回退备份。

本机使用官方 v0.11.0 DMG、临时签名的 0.11.1 偏好导出版归档和 0.12.0 Go 候选 DMG，串行验证真实 WebView 导出、低空间失败、原位重试及旧版回退。单独运行 Go 候选的路径也通过。`make check`、Bash 3.2 语法和工作流静态检查通过。夹具退出时卸载三个测试卷并清理临时目录。 [升级回退脚本](../../../scripts/verify-macos-upgrade.sh)

## 2026-11-25 · macOS 候选 DMG 安装、偏好迁移与回退门禁

macOS 候选流程在签名、公证和产物校验后，先执行升级回退门禁，再上传工件。门禁校验公开 v0.11.0 DMG 的固定 SHA-256，只读挂载旧版和候选镜像，并把两版依次安装到隔离目录内的同一个 `GitWave.app` 路径。各阶段检查签名和 bundle 元数据，等待测试 PID 的可见窗口及其 SQLite 连接就绪。应用进程从空环境启动，使用独立的用户数据目录，沙盒阻止访问真实用户目录和出站网络。 [升级回退脚本](../../../scripts/verify-macos-upgrade.sh)

测试先用真实 v0.11.0 生成原版 schema，再加入两个仓库。联合发布时，CI 下载旧 updater 使用的 macOS `.app.tar.gz` 和 `.sig`，验证已安装客户端内置的 Minisign 公钥、签名版本和文件名，再解包并检查 Developer ID 和公证票据。组装阶段比对候选流程返回的 SHA-256，确保清单使用经过实测的归档。夹具在隔离的 `WKWebView` 中写入同一应用 ID 下 `tauri://localhost/` 的偏好。解包后的偏好导出版必须实际把六项设置写入 SQLite，并排除夹具故意添加的未知字段。Go 候选应恢复中文、深色主题、中文字体、选中的仓库和关闭自动更新的设置，并保留一份接管前的备份。向兼容 schema 写入一项升级后的修改后，回退安装 v0.11.0，验证工作区名称、保留字段、仓库昵称和排序。单独运行候选构建时保留预先填充 SQLite 偏好的测试入口。

本机用官方 v0.11.0 DMG、当前源码构建的 0.11.1 偏好导出版归档和 0.12.0 Go DMG，通过四次桌面启动与数据库核对。两个新构建使用 ad hoc 签名。用官方 v0.11.0 更新归档验证了 Minisign 签名、Developer ID 团队和公证票据。修改归档后验签失败，拿没有导出功能的 v0.11.0 充当导出版时，迁移门禁按预期失败。在 WebView 助手启动后但 PID 登记前取消，已验证进程结束、测试镜像卸载和隔离目录清理。版本不匹配的候选按预期被拒绝。Go 正式候选的 Developer ID 签名与公证仍待 CI 实跑，旧 updater 按正式过渡清单自动更新也待验收。

## 2026-11-25 · History 宽分支图的滚动与定位

v0.11.0 在同一仓库内切换 History 和 Changes 时保留 History 视口，定位提交时横向揭示对应的提交节点。用 64 条侧分支汇合的 Git 夹具复现 Go 版列表挤入视口的问题。列表现在会保留内容宽度，在 968 点视口内可横向滚动 1,588 点宽的 History 列表。

同一工作台内切换模式会保留 History 的横纵位置以及已选提交。刷新时，如果原来的首个可见提交仍在列表中，即使当前停留在 Changes，纵向位置也会沿原提交调整。搜索定位沿用已有的纵向定位，并把视口外的节点移入横向可见范围。从 Changes 直接定位时，等待分支图和列表取得实际尺寸后再滚动。

独立复审的模型连续五次通过隐藏 History 时的刷新、从 Changes 定位最左或最右泳道，以及普通三泳道分支图的窗口缩放。十项 AppKit 回归通过，覆盖宽图、搜索定位、495 条 Changes、设置切换及 diff 滚动。最终 `make check`、定向竞态测试、工作流静态检查和 Windows/Linux x64 交叉构建均通过。宽分支图用例加入 macOS AppKit CI。

## 2026-11-25 · 子模块工作树状态和退出确认

v0.11.0 的 Changes 和退出提醒共用工作副本状态。Go 版在保留父 Git 隔离选项的同时，递归读取已检出的子模块。子模块的已修改、已暂存和未跟踪文件会让父仓库显示一条未暂存记录，预览使用 Git 的 `-dirty` 标记。扫描尊重 Git index 的 skip-worktree 和 assume-unchanged 标记，也覆盖没有 `.gitmodules` 的 stage-0 gitlink。

修复子模块 HEAD 前移时暂存可能无效果的问题。父仓库在私有索引写入核对过的 gitlink，取消暂存只恢复父仓库的指针。只有内部文件改动时，未暂存行仍可预览，Stage All 会跳过该行。子模块索引损坏时可继续查看并暂存父仓库已读取的改动，Changes 和健康检查会显示部分结果。退出检查遇到读取错误或超时也会请求确认。

普通仓库的状态刷新增加一次流式索引枚举。含 100,000 个索引条目的本机夹具中，Git 枚举中位数为 28 毫秒。Go 扫描仅保留 gitlink，额外耗时与子模块数量及工作树内容有关。

六项 AppKit 回归通过，涵盖 495 条变化、狭窄窗口、移动文件、子模块 diff、Stage All 和非活动仓库退出告警。SHA-256 定向竞态测试、Windows/Linux x64 交叉构建和最终 `make check` 均通过。首次全量检查有一项 LFS 代理用例失败，连续六次定向复测及最终全量检查均通过。独立复审验证了损坏索引、检查超时、三阶段 gitlink 冲突和混合批量暂存失败时的索引原子性。现有隔离用例确认不会执行子模块的自定义 Git filter。

## 2026-11-25 · 恢复 macOS 窗口菜单

按 v0.11.0 恢复系统「窗口」菜单及其最小化、缩放、全屏和全部前置操作。系统菜单角色提供 `⌘M` 和 `⌃⌘F`。GitWave 应用菜单恢复关于、检查更新、设置、隐藏和退出的顺序，Workspace、Repository 和 Branch 使用旧版命令名称。菜单从当前工作区和分支状态计算可用性，现有命令处理器继续处理 Git 操作。

`make check` 和工作流静态检查通过。六项 AppKit 回归核对菜单结构以及设置、工作区导入导出、LFS、PR 草稿和带确认的退出。最小化与全屏的快捷键还通过模型测试。AppKit 测试进程缺少键窗口时无法验证全屏切换，原生回归检查实际安装的菜单角色。独立复审核对了中文空工作区、正常分支和 detached HEAD 下的可用菜单项。

## 2026-11-25 · 系统外观切换时保留设置输入

系统深浅色和应用配色变化时，设置弹窗通过响应式样式更新颜色。弹窗和正在编辑的原生控件继续挂载，保留焦点、文字选区和未保存的输入。主题和配色单选组继续接受连续方向键，退役了上一轮重建控件后手动恢复焦点的逻辑。

复审同时复现了 Settings 叠在 Amend 或独立更新窗口上方时，工作台重绘会改变弹窗顺序。Settings 入口现在等待已有工作台弹窗关闭。通用设置内打开更新详情仍是正常的嵌套流程。

`make check`、工作流静态检查和 14 项 AppKit 回归通过。字体用例在 AppKit 注入外观变化后，核对了原来的输入控件和焦点，再用按键替换选区，最后关闭并核对数据库里的完整字体列表。两个新增模型用例各连续通过八次。独立复审覆盖菜单与快捷键、Amend 确认和两种更新弹窗的层叠顺序。

## 2026-11-25 · macOS 设置弹窗布局

按 v0.11.0 设置组件的尺寸和结构恢复居中的弹窗。窗口宽度达到 889 点时，弹窗宽度为 800 点，设置内容区随窗口保持约 60% 高度。宽屏原生模型的设置内容区由固定的 390 点增至 480 点，1024×768 下为 460 点。关闭入口恢复至右上角，保留 Escape、点按背景、拖动标题栏和关闭前提交输入。可在默认字号下直接操作「检查更新」。

通用页恢复语言设置的分组和全宽下拉框。主题、配色和代理使用等宽卡片、选中描边和横向图标布局。应用主题或配色时，原生卡片可连续使用方向键切换。模糊遮罩在 macOS 启用，Windows 和 Linux 使用半透明遮罩。Windows 后端会在绘制模糊前复制渲染目标，实际帧耗待实机测量。此轮发现的系统外观变化时丢失输入焦点，已在后续复测中修复。

`make check`、工作流静态检查和 12 项 AppKit 设置及相邻交互回归通过。另用一项截图用例检查通用、外观、网络和 1024×768 窗口。原生回归覆盖连续方向键、语言切换保留 diff 和滚动、调整窗口尺寸时保留选中页，以及通过关闭按钮、Escape 或背景退出。旧版几何以 v0.11.0 源码为基线，本机旧 WebView 的设置弹窗截图受辅助功能输入权限限制。

## 2026-11-25 · macOS Changes 未暂存移动配对

Changes 将可识别的未暂存移动显示为一行，列出源路径和目标路径。diff 继续显示移动后实际改动的行，暂存从同一笔索引事务发布两个路径。取消暂存后，目标文件留在工作树。行及批量丢弃沿用旧版，对重命名禁用。

后台识别使用私有 Git 索引、对象库和工作树，以 Git 内置相似度查找配对。同一 Git blob 对应多个候选时保留独立路径。暂存复核目标快照、Git 转换后的 blob 和源仍已移除，遇到外部变化时整批拒绝更新原索引。独立复审复现并验证了 CRLF 与 LF 归一化歧义、转换规则改变、源重新出现和悬空符号链接四个边界。

复用 495 条变化的隔离仓库。Go 版识别 11 对，列表显示 484 条。原版识别 12 对，列表显示 483 条。额外一对是 `native/history_test.go` 至 `cmd/gitwave/history_test.go`，Git 给出 48% 相似度，低于本轮使用的 50% 阈值。五次后台配对扫描约 230–340 毫秒，未测帧率。

五项 AppKit 回归和随后完成的 `make check` 通过。495 条逻辑变化中，移动项位于列表末尾。新增外部文件后手动刷新，选择和 diff 继续保留，单行暂存只改动移动的两条路径。大列表、批量暂存和窄窗口提交区也通过验证。普通移动和大量变更下的刷新已纳入 macOS CI。

## 2026-11-25 · 补齐同名文本移动

旧版 libgit2 使用文本片段签名衡量相似度，Git CLI 按复制内容相对较大文件的字节占比评分。495 条变更夹具中，`native/history_test.go` 移到 `cmd/gitwave/history_test.go` 后仍有 126 条原文相同行，Git 给出的 48% 低于 50% 阈值。 [libgit2 状态读取](https://github.com/libgit2/libgit2/blob/v1.9.7/src/libgit2/status.c#L319-L339) [libgit2 相似度指标](https://github.com/libgit2/libgit2/blob/v1.9.7/src/libgit2/diff_tform.c#L308-L323) [Git 相似度计算](https://github.com/git/git/blob/v2.54.0/diffcore-rename.c#L121-L197)

Git 未配对时，Go 对两侧唯一同名的常规文本比较去除行首缩进后的不同文本行。存在竞争路径时保留独立条目，即使竞争文件超出内容扫描范围。竞争路径出现或消失会改变缓存键。补配预览直接使用已配对的源 blob 和目标快照，继续按目标路径的 Git 属性生成 diff。目标内容及 Git 清理后的对象 ID 仍需通过暂存校验。

同一隔离仓库现在识别 12 对移动，未暂存列表为 483 行。12 个源路径逐项与旧版状态快照一致。补配项的预览有 102 行新增和 19 行删除。Apple M4 上三组各 15 次的扫描中位数为首次约 171 毫秒、缓存命中约 42 毫秒。低于 Git 阈值的补配项可以一键暂存两个路径，已暂存列表仍使用 Git 的原生配对。Git 仍拆分时，取消暂存需选中两个路径或使用全部取消暂存。

用隔离仓库在 AppKit 窄窗口中打开该补配项，核对 diff、暂存的两个路径以及全部取消暂存后的 483 行。永久回归覆盖低于阈值的文本、重复行、同名竞争和大候选引起的缓存失效。独立夹具验证 CRLF 仍按 Git 属性写入索引，转换规则变更后整笔暂存被拒绝，补配读取失败时只保留 Git 已确认的配对。`make check`、SHA-1 和 SHA-256 定向竞态测试、Linux 和 Windows x64 交叉编译均通过。

## 2026-11-25 · 暂存区的同名文本移动

Git 将低相似度移动拆成已暂存的删除和新增时，Changes 依据源路径的 HEAD blob 和目标路径的 index blob 补配唯一同名文本。已有 Git 移动保持原配对。同名竞争或一侧出现重复 blob 时保留独立条目。合并只影响已暂存侧，目标后续编辑或删除、源路径重建仍各自出现在未暂存区。

已暂存预览把捕获的源 blob 放到同名目标路径的私有树，再和实际 index 比较，沿用目标路径的 Git 属性。展示前复核 HEAD、源仍缺席和目标的 stage-0 blob。单行取消暂存通过一个索引事务复位两端，工作树和无关暂存项保持原状。外部换掉一端的已暂存内容时，整次请求失效。

495 条混合变更中，暂存原先低于 Git 阈值的 `history_test.go` 移动后，Go 把 Git 的两条已暂存记录折成一个移动，预览仍显示 102 行新增和 19 行删除。AppKit 窄窗口显示 482 行未暂存、1 行已暂存，单行取消暂存后恢复为 483 行未暂存和 0 行已暂存。四项 AppKit 回归、SHA-1 和 SHA-256 定向竞态测试，以及 Linux、Windows x64 交叉编译通过。原生回归覆盖由未暂存移动进入已暂存区、后续目标改写和删除、重建的源及成对取消暂存。独立夹具还验证了目标已暂存的 `-diff` 属性、外部仅复位源、HEAD 已包含变更和 linked worktree 使用 split index。

已暂存补配按捕获的 HEAD、源与目标条目和候选唯一性复用完整扫描结果。495 条混合变更含一对已暂存补配时，Apple M4 上三组各 30 次的补配中位数为首次约 16 毫秒、缓存命中约 3 微秒。这个采样只覆盖已暂存补配。完整 `make check` 通过。

## 2026-11-25 · macOS Changes 布局与移动文件

用同一份 SQLite 工作区和两份隔离 Git 夹具在 macOS 比较 v0.11.0 与 NSGo。第一份含一条未暂存和一条已暂存，第二份含 495 条 Git 路径变化。对照后，文件区标题恢复全宽，批量操作靠右，文件行恢复旧版的状态字母及 32 点行高。提交区恢复弹性布局，AI 生成、Amend 和 Commit 位于编辑器下方，空 diff 恢复独立标题和说明。

More 固定在横向滚动区外，选择文件后即使 Stage 按钮使标题溢出也能打开 More。在最窄分栏、英文 20 号字体下，提交类型选择器和恢复草稿按钮可按宽度收缩。AI 建议可单独撤销，原 Amend 的恢复草稿操作仍可退出修正模式。

495 路径夹具中，旧版列出的 483 条包含 12 个未暂存重命名。旧版只交付源路径和重命名标记，没有目标路径。用 `old.txt` 移到 `new.txt` 复现后，从旧版暂存留下 `D  old.txt` 与 `?? new.txt`。Go 版列出源路径与目标路径，选中两条后暂存得到完整 `R100`，取消暂存保留工作树中的新文件。未暂存移动在 Go 版仍显示为两条路径。

1200×800 的 Go 截图中，未暂存列表仍受 188.5 点视口约束，提交编辑器高 96 点。1024×768 复测可以滚到第 495 条、打开 More、取消丢弃确认、查看 diff 并输入提交信息。八项 AppKit 交互和两份夹具的截图验证通过。`make check`、定向竞态测试及 Windows 和 Linux 交叉构建通过。Apple Silicon 预览 DMG 通过校验和、应用签名及内置 Git 验证。

## 2026-11-25 · 退役 Rust 行为对照程序

在删除 Rust 对照程序前，用现有二进制重新比较十项 Git fixture。History 三套快照保留旧版的顺序、父行位置、泳道、引用和元数据，按行归一化对象 ID 以兼容 Git 的 SHA-1 与 SHA-256 仓库。diff 用可读的 Go 期望保持首父提交、精确路径、二进制 blob、CRLF、无结尾换行、大文件和行数上限。暂存、commit/amend 和丢弃直接检查 Git index、提交元数据和工作树文件。Blame 核对重命名前后的每行作者和提交者时间。健康摘要核对脏文件数、分支、标签、残留操作和大文件。

移除 `gitwave-native-core`、Cargo 的 `native-preview` feature，以及 `make native-parity-test`。上述回归由默认 `make check` 执行。偏好导出版仍包含 `gitwave` 应用目标、库和更新器重启测试，继续承担旧安装的偏好迁移。

在 `PATH` 只包含 Go、Git 和 Git LFS 的环境中执行十项回归，默认 SHA-1 与 SHA-256 两次均通过。额外用外部 Git 作者与 reftable 默认配置复测，混合 packed/loose refs 的夹具显式使用 files 格式。`make check`、偏好导出版 `gitwave` 目标检查、Rust 偏好导出和前端 updater 定向测试通过，Go 测试二进制成功交叉编译到 Windows 和 Linux。最终一轮七项 macOS AppKit 测试通过，丢弃菜单早期出现过间歇性超时，独立复测也通过。

旧 Rust 和前端 CI 仅由偏好导出版源码、构建输入或共享 SQLite 迁移文件触发。Go 源码由 Go 桌面 CI 检查。变更前核对了 GitHub `main` 的规则，旧检查没有被配置为必需状态项。工作流静态检查和本地路径匹配用例通过。

## 2026-11-25 · macOS 工具栏与侧栏选中样式

沿用 1200×800 的原版和 Go 版工作区夹具。两份截图转换到相同的 sRGB 色彩空间后再比较像素。恢复工具栏的 36 点数字区域、等宽数字、按钮内边距和同步分隔线位置。Changes 图标起点与旧版的偏差从 33 点缩至 5 点，Push 图标起点一致。系统关闭按钮的截图纵向边界均位于 2× 图的第 14 至 41 像素。1024×768 下，主要操作保留图标和文字，Changes 保留计数。

侧栏按原版 Accent 的 10% 透明度绘制选中背景，悬停时使用 15%，分支右侧恢复约 13 点留白。选中导航背景与旧截图的采样色一致，主分支背景各通道最多还差 1 个色阶。分支行恢复旧版 32 点最小行高、远端标识、上游箭头和等宽提交号。导航通过继承样式同步更新文字和图标颜色。

使用真实分歧分支复现了最窄侧栏里的长名称问题。中文、20 号字体、分支行宽 140 点时，当前分支名称从 0 点恢复到 32 点的可绘制空间，HEAD 完整保留。窄栏收起行内领先与落后计数，Pull 和 Push 的悬停提示继续报告数值，拉宽后恢复分支行计数。长远端名称允许省略，分支总数仍显示并支持键盘展开。

108 组工具栏模型覆盖三端、1024 至 1440 点、长工作区名、中英文和放大字体。macOS AppKit 回归验证了最窄侧栏、分支检出与上游跟踪、495 条变更、同步与退出。`make check`、定向竞态测试及 Windows 和 Linux 交叉构建通过。Apple Silicon 预览 DMG 通过校验和、应用签名和独立 PATH 下的内置 Git、LFS 与 HTTPS 测试。

## 2026-11-25 · 正式发布更新密钥预检

Go macOS 候选链沿用旧 CI 配置的六项 Apple Secret。Developer ID 证书通过临时钥匙串加载，原 App Store Connect `.p8`、Key ID 和 Issuer 交给 NSGo 公证。逐项比对确认 macOS 候选和聚合发布引用的名称均与 v0.11.0 一致，工作流静态检查通过。GitHub Actions 元数据确认六项名称仍在，v0.11.0 的 macOS 构建、DMG 公证和附加票据均成功。Go 候选链在构建前验证 `.p8` 能解析，正式候选仍须确认凭证有效并验证产物。 [v0.11.0 macOS CI](https://github.com/Yangshifu1024/GitWave/actions/runs/38751856940/job/116480101803)

旧 `TAURI_SIGNING_PRIVATE_KEY` 继续用于现有安装读取的升级过渡包。Go 更新清单使用独立的 `GITWAVE_UPDATE_PUBLIC_KEY` 仓库变量和 `GITWAVE_UPDATE_SIGNING_SEED` Secret。两项 Go 更新配置及 Windows Artifact Signing 的 `GITWAVE_WINDOWS_*` 仓库变量当前尚未配置。核对只读取配置名称。正式偏好导出版与升级链待验收。

聚合发布工作流在三端构建前调用 `sign-update -check-key`。它与更新清单签名共用 Ed25519 密钥解析和配对校验，分别报告缺少公钥、缺少种子、种子格式有误或公私钥不匹配。用临时密钥验证了实际 CLI 的成功和错误退出，签名工具竞态测试、工作流静态检查和 `make check` 均通过。

续发预检再用桌面客户端的清单校验逻辑验证首个 Go 过渡版与最近 Go 正式版，校验签名、格式和 tag 版本。只保留 Go 过渡清单的 release 仍被识别为已发布 Go 版本。用临时密钥和真实发布步骤的本地替身验收了同密钥通过、两处记录相同时只下载一次、整体替换密钥对受阻、最近清单密钥不同受阻，以及缺失 Go 更新清单时保留原过渡版本。当前 GitHub 暂无带 Go 更新清单的公开版本，正式历史包验签仍待首版发布后实跑。 [发布工作流](../../../.github/workflows/go-release.yml)

## 2026-11-25 · macOS 侧栏区块对照

使用 History 对照时的同一个本地仓库和 SQLite 工作区，在 macOS 上比较 1200×800 窗口。侧栏区块使用 28 点标题和 8 点间距，分支对照本地组、文件夹缩进和当前行高度。空 Stash、Tags、Remotes 和 Submodules 使用静态标题。恢复 Tags 切仓折叠，以及 Remotes 和 Worktrees 在一条和多条之间切换时的默认折叠规则。

空 Remotes 可用右键或 Shift+F10 添加首个远端，首个额外 worktree 沿用 Repository 菜单。普通仓库的空子模块区在标题右侧提供小型新增按钮，修复旧版隐藏首个新增入口的问题。裸库没有工作树，子模块标题保持静态。远端、工作树和子模块的列表错误与操作错误独立记录，刷新恢复后自动清除旧的列表错误。

11 项 AppKit 交互回归通过，覆盖 495 条变更、侧栏滚动、分支检出、标签管理、Stash、远端条目折叠、工作树切换和子模块入口及错误刷新。2× 截图中 Stash 到 Recovery 的标题纵向位置一致，本地组和分支文件夹相差 1 像素。此轮发现的分支选中色、右侧留白与工具栏差距已在后续对照中处理。

`make check`、定向竞态测试、工作流静态检查和 Windows、Linux 交叉构建通过。Apple Silicon 预览 DMG 通过 SHA-256、应用签名和无系统 Git 环境下的随包 Git、LFS 和 HTTPS 测试。

## 2026-11-25 · 大量工作树变更的 Changes 布局

用当前工作树的 495 条变更复现并修复文件列表撑出分栏的问题。AppKit 未暂存视口从约 6,946 点回到 192.5 点，初始挂载 14 个文件行，暂存区和提交编辑器重新可达。macOS 原生 CI 增加大列表回归。 [Changes 性能验收](../fix-nsgo-large-changes/review.md)

## 2026-11-25 · macOS History 引用与提交行对照

在同一台 macOS 上用同一个 Git 仓库和同一份 SQLite 工作区夹具对照两个 1200×800 窗口。旧版从 v0.11.0 对照提交 `004c9f4` 构建，临时副本隔离应用 ID、数据和日志目录，通过旧版字段选择同一 Workspace、repo、语言和主题。Go 端用 NSGo 的 AppKit 测试进程打开该 SQLite 夹具的副本。两版均显示 `c057e25` 和 `687a004`，且初始未选提交。

History 恢复引用类型徽标、同提交的本地与远端分支合并、`+N` 列表和徽标上下文菜单。展开列表保留原选中提交，右键选择引用所属提交。弹窗正文虚拟滚动，关闭按钮留在正文外。标签删除、分支检出、合并和远端删除继续走已有的仓库与引用校验。远端名含斜杠时识别最长已配置前缀。History 同时恢复 HEAD 左侧标记、标题字重、列断点、时间文案和历史末尾提示。

审查复现大量引用使旧的弹窗实现超出窗口，已加入隔离回归，滚动到第 28 个标签后从徽标菜单删除该标签。macOS 截图中可对照两版的行高、徽标位置、提交列和结束提示。侧栏折叠、主题颜色细节和窗口布局仍有差异。macOS 正式签名后的安装升级与回退为独立验收项。

最终通过 `make check`、macOS AppKit 的 6 项交互回归及隔离仓库截图用例、工作流静态检查和 Windows/Linux x64 交叉编译。独立模型复审另外在 620×450 窗口用 55 个标签核对关闭按钮可达，并在滚动后删除最后一个标签。

## 2026-11-25 · NSGo v0.3.0 与默认 Go 工具链

Go module 已锁定 NSGo v0.3.0。目录选择 fixture 使用新版 `nsgotest.DirectoryPick`。macOS 13 最低版本由新版 NSGo 写入 `Info.plist`，移除 GitWave 的模板补丁。Unix 包装器保留上游资源处理，再复制和签名随包 Git。Go 格式检查跳过 `.nsgo` 与 `dist`，从而支持在构建后立即运行。 [NSGo v0.3.0 发布说明](https://github.com/nsgo-dev/nsgo/releases/tag/v0.3.0)

新版打包生成第三方许可证文件。确认 `modernc.org/memory` 的未识别文件为 logo 链接，`modernc.org/sqlite` 的未识别文件为 SQLite 的公有领域说明，仅列入这两个模块的例外，保留原始许可证文本。Windows 安装器将许可证写入受管的 Git 目录并登记在 bundle inventory 中。升级和卸载夹具额外检查安装目录根部的同名用户文件。Linux AppImage 显式清空外部更新地址。已检查实际产物的 ELF `.upd_info` 全为零，发布工作流上传 GitWave 签名清单对应的 AppImage。 [NSGo 打包文档](https://github.com/nsgo-dev/nsgo/blob/v0.3.0/docs/public/en/building.md#license-notices)

macOS 11 项 AppKit 测试通过，覆盖 Workspace 导入导出和仓库标签、命令面板、History 定位和刷新、blame、字体与语言切换、分支侧栏、关于页及退出保护。更新器的隔离签名应用替换和重启测试通过。新预览 DMG 已通过 SHA-256、只读挂载、严格代码签名、macOS 最低版本和预览 ID 检查。包内依赖许可证、Git 2.56.1、Git LFS 3.8.1 和 HTTPS helper 符号链接已核验。正式 Developer ID 签名、公证和从已发布旧版升级回退仍待验收。

`make dev`、`make build` 和 `make check` 使用根目录 Go 工具链。Tauri 偏好导出版仍有显式 `legacy-*` 入口。`make version` 读取 NSGo 配置，`make bump VERSION=X.Y.Z` 原位更新配置版本。候选构建可用临时版本，正式草稿检查配置版本与输入版本相同且标签指向构建提交。定向竞态测试、模块校验、actionlint 和草稿版本不一致拒绝夹具通过。开发服务监听应用和库中的 Go 源文件，修改根目录模块依赖后需重启开发服务。

最终 `make check` 和新版 macOS 预览构建通过。在 Linux 容器编译最终 Windows NSIS，校验安装包 SHA-256 后解包，核对许可证原文并用 Go 安装器调用的 `CheckWindowsBundle` 通过完整目录验证。PowerShell 升级夹具通过语法检查，Windows 实机由用户验收。

## 2026-11-25 · Go 三端联合发布

统一工作流复用 macOS、Linux、Windows 候选门禁。组装时核对每个安装包的 SHA-256，首次 Go 发布为 Windows 安装器与三种 Linux 安装包补旧 Minisign 签名，与 macOS 兼容归档一起生成两份过渡清单。新版 Ed25519 清单同时绑定三端更新包和签名 Windows EXE 的摘要。公开资产保留安装器、相应校验文件及可用的源码归档，签名 Windows EXE 留作清单输入。 [GitHub Actions 可复用工作流](https://docs.github.com/en/actions/how-tos/sharing-automations/reusing-workflows)

本地联合演练使用实际 0.12.0 macOS ad hoc 预览、Linux 候选与临时证书签名的 Windows 候选。原版 v0.11.0 资产用一次性密钥构造偏好导出版夹具，仅用于测试两段清单。两份旧清单各八条路径和签名核对通过，Go 客户端验签通过，Windows EXE 与三端产物摘要均匹配。还验证了重复签名输出保护、篡改包拒绝、历史路由冻结、远端轻量及附注 tag 的提交核对，并通过定向 Go 竞态、静态检查及 actionlint。

后续发布比较上一公开 Go 版的过渡清单版本与已验证的平台签名，并携带两份冻结清单的原始字节。草稿创建前后核对远端标签提交，旧 Tauri 流程改为手动创建新草稿，已存在的发布记录会被拒绝。尚未使用正式签名环境运行联合工作流或创建草稿。 [GitHub CLI 草稿发布](https://cli.github.com/manual/gh_release_create)

## 2026-11-25 · Windows 正式候选签名

Go 打包工具核对 NSGo 正式 ID 与公告版本，在私有目录签名 `GitWave.exe`，由 NSIS 对卸载器签名后嵌入，再对完整安装器签名。安装器和签名 EXE 的 SHA-256 均在签名后计算并一起发布到候选输出目录。候选工作流使用 OIDC、Artifact Signing 与 Windows SignTool，验证 Authenticode 和时间戳，通过 v0.11.0 实包升级夹具核对安装后签名 EXE 的字节、卸载器签名和用户文件保护。 [Artifact Signing 集成](https://learn.microsoft.com/en-us/azure/artifact-signing/how-to-signing-integrations) [SignTool](https://learn.microsoft.com/windows/win32/seccrypto/signtool)

Ubuntu x64 隔离容器使用一次性测试证书完成 Windows 正式配置打包。验签覆盖主 EXE、编译时嵌入的卸载器和最终安装器。解包得到的 `GitWave.exe` 与清单输入逐字节相同，随包 Git 和 Git LFS 完整，原始 NSGo 主程序保持原有字节。实际公开 v0.11.0 安装器 SHA-256 与夹具一致，其 PE 证书目录为空。公开信任证书签名和 Windows 最终实机验收仍待执行。

审查独立核对三个 PE 的版本资源、签名结构和摘要。打包及清单竞态测试、Windows x64 签名工具交叉编译、Go 静态检查、工作流检查和 PowerShell 语法解析均通过。CI 显式安装所固定签名组件要求的 .NET 10。 [Artifact Signing Client 1.0.151](https://www.nuget.org/packages/Microsoft.ArtifactSigning.Client/1.0.151)

## 2026-11-25 · Linux 正式安装包与手动升级

Linux 正式构建复用 Go + NSGo 打包入口，写入正式应用 ID、版本和 Ed25519 更新公钥。DEB 与 RPM 使用已公开的 `git-wave` 包名和 `/usr/bin/gitwave`，发布资产仍按 Go 更新器约定命名。包内保留原 `GitWave.desktop` ID，通过 `NoDisplay=true` 隐藏重复菜单入口。预览构建继续使用独立包名。 [v0.11.0 发布资产](https://github.com/Yangshifu1024/GitWave/releases/tag/v0.11.0) [桌面入口规范](https://specifications.freedesktop.org/desktop-entry/latest/recognized-keys.html)

新增候选工作流从 Ubuntu 22.04 构建 AppImage、DEB、RPM 和源代码附件，并用公开 v0.11.0 的 DEB/RPM 核对升级及回退。验证脚本检查包管理器元数据、桌面兼容入口、实际 GTK 窗口、沿用的 SQLite 数据和首次备份。正式候选使用临时公钥做本地构建，旧更新器使用的 Minisign 签名需要发布环境接入。

本机 Ubuntu 22.04 x64 容器完成正式配置候选构建。三种产物、额外的 Arch 产物和 GTK 源码附件均通过 SHA-256 校验。Ubuntu 实包回归完成 v0.11.0 DEB → Go DEB → v0.11.0 DEB，并确认替换期间同一包拥有启动命令、正式窗口可见、旧工作区记录和备份可读。Go AppImage 解包启动使用单独的旧数据夹具，通过相同的工作区和备份检查。

Fedora 43 实包回归完成 v0.11.0 RPM → Go RPM → v0.11.0 RPM，通过安装身份、旧桌面入口、GTK 启动和工作区数据库核对。启动验收同时等待可见窗口与首次备份完成，避免结束进程时数据库尚在初始化，并在新 Fedora 容器用同一 RPM 复跑通过。

审查发现偏好导出版的手动安装原来打开 GitHub 最新发布页，浏览器失败后重试还可能调用插件安装。现在等待偏好导出完成，再打开公告版本对应的固定页面。DEB/RPM 更新关闭插件下载句柄，重试只重新检查。13 项前端回归、TypeScript 及工作流静态检查通过，Go Unix 打包与 macOS 更新器竞态测试也通过。已发布的 v0.11.0 仍使用其原有手动入口，正式切换说明需提供导出版固定链接。

## 2026-11-25 · 两阶段旧更新清单

偏好导出版优先检查 `gitwave-legacy-transition.json`，过渡清单未上线时回退到已安装客户端使用的 `latest.json`。Go 生成工具使用同一内置公钥验证两套资产的 Minisign 签名、受信任版本与文件名，保留 App、NSIS、AppImage、deb、rpm 共八个目标，并生成指向相应固定版本的下载 URL。原版 macOS 签名在发布归档改名之前生成，允许该目标使用历史的 `GitWave.app.tar.gz` 签名文件名。 [旧版更新器锁定版本](https://crates.io/crates/tauri-plugin-updater/2.13.3)

下载了公开 v0.11.0 的五个实际安装包及对应签名，核对发布 SHA-256。Go 生成工具验证全部签名，结果的八个 URL 和签名与原发布清单一致。两阶段夹具覆盖不同版本、篡改包、同版本资产错放和 Minisign 算法标记失配。两个清单在私有目录写齐后一次排他发布。定向 Go 竞态测试、Go 静态检查、Windows 和 Linux 清单工具交叉编译，以及偏好导出版的 6 项更新与数据导出测试通过。正式偏好导出版与 Go 首版的清单仍需连接发布流水线并实测。 [v0.11.0 发布资产](https://github.com/Yangshifu1024/GitWave/releases/tag/v0.11.0)

## 2026-11-25 · macOS 旧更新器过渡包

候选工作流可选生成 Tauri 格式的 `.app.tar.gz`、`.sig` 和 SHA-256。Go 工具先核对候选应用的 Developer ID、bundle ID、版本和 arm64 架构，再核对归档解包后的代码签名哈希和公证票据。输出目录通过文件身份校验与应用隔离，可以拒绝大小写不同的 APFS 别名。旧签名密钥必须匹配 v0.11.0 内置公钥，签名受信任注释绑定归档名和版本。 [Tauri CLI 签名协议](https://raw.githubusercontent.com/tauri-apps/tauri/tauri-cli-v2.12.2/crates/tauri-cli/src/helpers/updater_signature.rs)

用 Tauri CLI 生成的临时加密密钥为真实 Go 预览应用签名。v0.11.0 锁定的 Minisign 验证和逐项解包夹具通过。解包后代码签名哈希一致，`git-remote-https` 链接保留，错误密码与任意测试密钥均被预期的检查拒绝。正式 Apple 签名以及实际旧密钥签名需要发布环境验证。三端旧 feed 必须分别固定偏好导出版和 Go 首版，清单版本应与每个平台的签名版本一致。 [旧更新器锁定版本](https://crates.io/crates/tauri-plugin-updater/2.13.3)

把公开发布的 v0.11.0 应用用同一 Go 工具重新归档，并经旧更新器逻辑解包。严格代码签名验证通过，公证票据文件逐字节相同，Apple `stapler validate` 通过。审查复测覆盖 APFS 大小写别名、arm64 和签名归档。打包及更新器定向竞态测试、`go vet ./...` 与工作流静态检查均通过。 [v0.11.0 发布资产](https://github.com/Yangshifu1024/GitWave/releases/tag/v0.11.0)

全量 Go 测试首跑有四项 LFS 夹具因 PATH 缺少 `git-lfs` 失败，其余测试通过。临时 PATH 加入验证过的随包 Git LFS 3.8.1 后，`internal/gitcli` 的所有 LFS 测试通过。

## 2026-11-25 · macOS 正式候选安装包和更新 ZIP

正式候选入口使用纯数字 `X.Y.Z` 版本、`desktop.gitwave.work` bundle ID、`ZE5SZ85EZQ` Developer ID 团队和外部传入的 Ed25519 更新公钥。已下载公开的 v0.11.0 arm64 DMG，核对发布页的 SHA-256，再以只读方式挂载并验证原版签名、bundle ID 和团队。候选工作流在临时钥匙串中导入签名证书，使用临时公证密钥，上传验收通过的构建产物。 [v0.11.0 发布资产](https://github.com/Yangshifu1024/GitWave/releases/tag/v0.11.0) [Apple 版本字段规范](https://developer.apple.com/documentation/bundleresources/information-property-list/cfbundleshortversionstring)

构建产物的验收比较独立 `.app`、DMG 内应用和更新 ZIP 解包后应用的代码签名哈希，并核对签名团队与公证票据。ZIP 使用新版更新器的实际解包路径复验，产物校验通过后用排他重命名发布。已用真实 Go 预览应用验证 ZIP 往返后的 ad hoc 签名、版本和随包 Git HTTPS 符号链接，正式签名检查按预期拒绝该夹具。 [Apple 公证工作流](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow)

公开 v0.11.0 更新包、签名和 `latest.json` 均通过发布资产的 SHA-256 核对。旧更新器锁定的 Minisign 库验证了该归档，清单内的签名与 `.sig` 一致。使用旧更新器锁定的 `tar` 和 `flate2` 复现其逐项解包逻辑后，公开原版应用和打成同格式的 Go 预览应用均通过严格签名检查，Go 预览的 `git-remote-https` 仍指向 `git-remote-http`。 [v0.11.0 发布资产](https://github.com/Yangshifu1024/GitWave/releases/tag/v0.11.0) [旧更新器锁定版本](https://crates.io/crates/tauri-plugin-updater/2.13.3)

定向竞态测试、macOS 更新重启夹具、`go vet ./...`、`actionlint` 及 Linux 打包命令交叉编译通过。重建的 macOS 预览 DMG 通过 SHA-256 和应用签名检查，使用独立预览 bundle ID。本机没有可用的 Developer ID，首次正式签名公证待在发布环境验证。旧版兼容包、签名 feed 和升级回退仍需联调。Tauri v2.12.3 的 macOS 重启实现会通过应用 bundle 解析新版入口，原版 `gitwave` 到新版 `GitWave` 的实际切换仍需用签名产物验证。 [Tauri v2.12.3 重启实现](https://raw.githubusercontent.com/tauri-apps/tauri/tauri-v2.12.3/crates/tauri/src/process.rs)

## 2026-11-25 · macOS 分支侧栏与退出保护

分支侧栏按本地、远端和分支名前缀分组。优先按已配置的最长远端名前缀匹配，避免将带斜杠的远端拆错。侧栏隐藏与本地分支同名的远端显示项，跟踪设置可读取完整引用。选中分支时立即读取 tip 详情，同时定位 History。虚拟列表复用视图时按分支或分组身份更新，保留键盘焦点。双击只有两次事件都命中同一分支才会检出，从 History 检出后展开并选中目标分支。

ahead/behind 在开仓后异步读取，避免跟踪分支数量阻塞 History。浅克隆补全后，即使引用 ID 未移动也更新侧栏计数。工具栏保留计数位置，非零值采用旧版括号格式，Changes 用颜色区分普通变更和冲突。定向回归覆盖分组、键盘焦点、真实检出、浅克隆和标题栏宽度。 [Git `for-each-ref`](https://git-scm.com/docs/git-for-each-ref)

退出确认检查当前 Workspace 内各仓库的工作树和暂存区状态，扫描有独立时限。取消操作默认获得焦点。macOS AppKit 测试验证 ⌘W、提示内容和取消，模型验证窗口关闭及确认退出。在独立 bundle ID 的实际 macOS 应用中按 ⌘Q，验证了提示、取消保持窗口、再次确认退出和未提交文件内容保全。Go CI 已加入 AppKit 回归。Windows 最终实机验收由用户执行。

最新 macOS arm64 DMG 经过 SHA-256、只读挂载、嵌套代码签名、最低系统版本和 Git HTTPS 符号链接核验。清空系统 Git 路径后，随包 Git 集成测试通过。macOS 更新器的签名检查、符号链接保全和重启夹具通过竞态检查。DMG 使用 ad hoc 签名，本机未找到可用的 Developer ID，正式公证和旧版签名包的升级回退仍待验收。

隔离构建夹具验证了 NSGo 可注入正式 bundle ID、测试版本和更新公钥，分别核对 `Info.plist` 与可执行文件。该夹具仅用于核验打包参数，应用和安装包的正式签名继续通过发布配置验收。

macOS 的 Go 全量回归通过。随后，退出扫描去掉分支统计读取，最终版本再次通过退出保护和分支的定向竞态测试、原生 ⌘W 测试、`go vet ./...` 及安装包校验。

## 2026-11-24 · 工作台空状态与 History 搜索

在 Ubuntu 24.04 X11 用相同的 SQLite Workspace 和 Git 仓库对照 v0.11.0 与 Go 版的 1200×800 工作台。没有选中仓库时，主区域显示整块空状态并禁用 History、Changes、搜索和专注按钮。选中仓库后恢复分栏。未选提交时显示原版的标题和说明。中英文提示指向实际可用的工作区菜单或仓库标签，修正旧版提示用户去侧边栏的错误。

History 的搜索按钮现在与快捷键共用命令面板。选中结果定位提交并加载 diff，保留未命中的历史记录。命令面板加入旧版的清除按钮，清除后焦点回到输入框。切换仓库或 Workspace 后回到原仓库，可恢复此前选中的提交和文件。用户在恢复完成前再次离开时保留记忆。已移除的旧提交恢复为空态，损坏对象保留错误；diff 暂时读取失败时，切换仓库或重试都保留先前选中的文件。回归覆盖这几种不同结果。

macOS AppKit 已通过清除搜索、定位、外部 commit 刷新及已选 diff 保全。Ubuntu 的实际窗口对两版执行同一个搜索，均定位到目标提交并显示相同补丁。Go 版还通过点击终端和编辑器，验证两种工具均收到当前仓库目录。Go 竞态回归通过 Workspace 切换、文件恢复、移除提交、导入中文偏好、分栏及相关工作台操作。Windows 与 Linux 测试通过交叉编译，Go CI 在 macOS AppKit 与 Linux GTK 门禁增加搜索回归。实际窗口仍显示侧栏分组、History 行和标题栏布局差异。

使用随 macOS 预览包交付的 Git LFS 3.8.1，`go test -count=1 ./...` 全量通过。`go vet ./...`、Go 格式检查及 `actionlint` 也通过。

## 2026-11-24 · 工作台外部工具与旧版截图对照

原生工具栏的终端与编辑器入口已接通。编辑器菜单按旧版顺序显示已安装的 Visual Studio Code、Zed 和 VSCodium。终端与编辑器使用触发操作时的仓库路径，切换仓库后不会把启动错误显示到新仓库。复审发现 Windows 命令解释器不能直接保留 UNC 工作目录。编辑器 shim 现在接收完整共享路径，命令提示符后备入口用 `pushd` 切到共享目录。shim 与 Windows Terminal 的启动路径均要求为绝对路径。 [Windows `pushd`](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/pushd)

Linux x64 验证了系统工具查找、终端参数、编辑器目录和从工作台点击两种入口。Windows 回归夹具通过交叉编译，UNC 共享目录的实际启动仍待 Windows 验收。应用进程已启动后才报告的错误仍需补充反馈。

用同一个 Git 仓库及同等的 Workspace 数据在 Ubuntu 24.04 X11 捕获了旧版 v0.11.0 与 Go 预览的 1200×800 History 窗口。Go 截图来自接通外部工具前的构建，用于后续逐项对照。 [GitWave v0.11.0](https://github.com/Yangshifu1024/GitWave/releases/tag/v0.11.0)

## 2026-11-24 · Go module 迁至仓库根目录

Go module 使用 `gitwave.work`，桌面入口、NSGo 配置和图标位于 `cmd/gitwave`，打包命令位于 `cmd/`，应用库位于 `internal/`。Makefile 的 Go 目标、三端预览 CI、Linux 隔离构建与 Windows 旧包升级夹具均已切换路径。根目录的全量 Go 测试、`go vet`、模块一致性检查及 `actionlint` 均通过。

独立复审确认四份 SQLite 迁移与旧 Rust 版本逐字节一致，旧 Rust 端的 26 项持久化测试通过。macOS AppKit 的关于页与 AI 命令面板回归通过。新路径生成的 macOS DMG 已只读挂载，校验图标、最低系统版本、应用签名以及随包 Git 和 Git LFS。Windows 和 Linux 的应用均已从根模块完成交叉编译，Windows 旧包升级夹具的 NSIS 脚本已验证正式应用 ID 与版本。

在 Linux 容器用新目录生成 Windows 预览 NSIS 安装包并通过 SHA-256 核验，解包清单包含 GitWave、MinGit 和 Git LFS。Windows 安装、签名升级及回退仍待实机验证。Go 对旧 Rust 的 8 项定向对照通过，覆盖 History 顺序、blame、暂存、commit/amend、discard、diff 和仓库健康。

从 CI 的根目录源码清单在 Ubuntu 22.04 容器完整构建 Linux 预览，生成 deb、RPM、Arch 和 AppImage。四种安装包及 GTK 对应源码的 SHA-256 均通过。在独立的 Ubuntu 22.04 容器中安装新 deb 并启动 X11 窗口，AppImage 在 X11 和 Wayland 启动，两类损坏 GTK 载荷仍在更新交接前被拒绝。

## 2026-11-24 · Linux GTK 运行库接入安装包

用 Ubuntu 22.04 从维护版源码构建 GTK、GLib 和 Wayland，把运行库、GSettings schemas、打印模块及许可证写入四种 Linux 安装包。Go 预览流水线核对并上传源码、发行版补丁和构建脚本。运行时通过独立进程验证宿主 GTK，避免旧 GTK 的依赖进入应用的主进程，包内 GTK 的路径也不进入 Go 启动的 Git 子进程环境。 [GTK 运行时路径](https://docs.gtk.org/gtk4/running.html)

复审发现了两个安装风险。AppImage 已把 GTK 和 schemas 的校验放到通知旧窗口退出之前，新增私有运行库整目录缺失和 GTK 库缺失的交接测试。deb 现在声明现代宿主 GTK 与旧宿主的必要 ABI 两种路径，避免 Debian 12 一类缺少旧 TIFF/JPEG SONAME 的宿主误判依赖满足。

复用完整构建得到的 GTK 和 Git 暂存目录打包最新应用后，deb、RPM、Arch 和 AppImage 的 SHA-256 均通过。Ubuntu 22.04 安装最新 deb 并打开 X11 窗口，AppImage 在含空格的路径解包后通过 X11 和 Wayland 启动。进程映射确认加载随包 GTK 和 GLib，启动环境没有 `LD_LIBRARY_PATH` 或 `LD_PRELOAD`。Ubuntu 24.04 安装同一 deb 后通过 X11 启动，进程映射确认使用系统 GTK。真实应用二进制验证两类损坏的 GTK 载荷均在交接确认前退出。默认在线构建的 11 个 Ubuntu 源文件也已实际下载并核对 SHA-256。

## 2026-11-24 · 命令面板贮藏建议失效处理

全量 Go 回归复现了一个刷新竞态。AI 建议生成后，外部 Git 可在同一提交上切换分支，Changes 快照还可能指向旧分支。旧流程先关闭命令面板，待实际 Git 操作拒绝后才在手动贮藏表单显示错误。

命令面板确认后读取实时仓库状态，比对建议中的分支与 HEAD。持索引锁后再次核对 HEAD 和 Git 操作状态，清理工作树前仍由 Git 引用事务验证。保存期间面板显示进度并暂停编辑与重复确认。建议失效时提示重新询问，同路径更换仓库后端时关闭旧面板。手动贮藏继续使用用户打开表单时看到的快照。NSGo 模型与 macOS 原生窗口均覆盖用外部 Git 切到同一提交上的新分支，验证贮藏列表、新分支和未跟踪文件字节，并重新输入命令。Go CI 已加入同一 AppKit 回归。

## 2026-11-24 · Ubuntu 22.04 的 GTK 兼容原型

在 Ubuntu 22.04 x64 用源码构建 GTK 4.14.5、GLib 2.80.5 和 Wayland 1.22 后，现有预览 AppImage 在虚拟 X11 中取得可见窗口，在 Weston 中发送 GitWave 窗口标题并确认 Wayland 配置。Wayland 会话同时启动了 AT-SPI 注册服务，辅助技术交互仍待验证。

从 Ubuntu 签名的软件源索引下载 GTK `4.14.5+ds-0ubuntu0.10`、GLib `2.80.0-6ubuntu3.10` 和 Wayland `1.22.0-2.1build1` 的源包并应用发行版补丁，在 Ubuntu 22.04 重新构建。三个维护版组件也通过了现有预览 AppImage 的 X11 和 Wayland 启动检查。 [Ubuntu 源码包目录](https://archive.ubuntu.com/ubuntu/pool/main/g/gtk4/)

第二个原型把运行库移到含空格的应用安装目录，给 ELF 设置 `$ORIGIN`，让 Go 在 NSGo 启动前按绝对路径加载 GTK。启动程序未设置 `LD_LIBRARY_PATH` 或 `LD_PRELOAD`，进程映射确认 GTK 和 GLib 来自该安装目录。实测范围为 X11 窗口启动。GTK 文件对话框、桌面服务、主题、输入法、完整许可和对应源码以及正式打包待验收。 [GTK 运行时路径](https://docs.gtk.org/gtk4/running.html)

## 2026-11-24 · M5 Fedora RPM 和 Linux 预览隔离

Linux Go CI 已从 NSGo 生成的 deb 载荷生成 RPM，并验证 deb、Arch、AppImage 和 RPM 的 SHA-256。RPM 保留应有的文件权限、启动器和 Git 私有 `RUNPATH`，只注册应用包自身的能力。预览的 RPM、deb 和 Arch 包使用 `gitwave-preview`，独立安装目录和命令未覆盖旧 `gitwave`。Fedora 43 用 DNF 安装预览 RPM 并通过 `rpm -V`，旧路径及命令夹具原样保留。 [RPM spec 格式](https://rpm.org/docs/4.20.x/manual/spec.html)

Fedora 43 在系统 Git 不可用时验证随包 Git、LFS、原生 CA 路径和 HTTPS CONNECT 代理。`.git/config`、用户配置与 `/etc/gitconfig` 可覆盖随包默认的 Fedora CA 路径。使用实际安装的 RPM 在虚拟 X11 显示中打开了 GitWave 工作台。旧版正式 RPM 的签名原位升级仍待专门演练。 [libcurl 代理 CA 配置](https://curl.se/libcurl/c/CURLOPT_PROXY_CAINFO.html)

Ubuntu 22.04 上从 AppImage 实际复现 GTK 4.6.9 启动失败。首个失败符号为 `gtk_css_provider_load_from_string`。符号对照还发现 GtkFileDialog、GtkAccessible 和 GSK 路径绘制等功能缺失，单独修复 CSS 无法恢复桌面功能。新版 GLib 源码和 Wayland 库已在 Ubuntu 22.04 的 glibc 2.35 环境构建，GTK 原型继续验证。 [GTK 4.14.5 构建要求](https://github.com/GNOME/gtk/blob/4.14.5/meson.build)

## 2026-11-24 · M5 macOS 和 Linux 安装包集成 Git

Unix 预览打包命令从固定 SHA-256 的 Git 2.56.1 与 Git LFS 3.8.1 发行物构建工具，在临时 NSGo CLI 源码副本中保留工具目录的相对符号链接和权限。Go 项目的 NSGo SDK 和 module 文件继续使用原有版本。Git 源码与摘要作为独立附件输出。 [Git 官方源码](https://www.kernel.org/pub/software/scm/git/) [Git LFS 发行物](https://github.com/git-lfs/git-lfs/releases/tag/v3.8.1)

macOS 实际 DMG 已通过摘要、挂载、应用深度与严格签名校验。Go 1.27 主程序、Git、HTTPS helper 和 LFS 的最低版本归一到 macOS 13.0，安装器内 `Info.plist` 的最低版本也为 13.0。挂载包保留 HTTPS helper 的相对链接，取出的应用在无系统 Git 的 `PATH` 下验证了仓库初始化、LFS 和回环 HTTPS。当前签名为 ad hoc。 [Go 1.27 Darwin 基线](https://go.dev/doc/go1.27#ports)

Linux 包在 Ubuntu 22.04 x64 构建，保留 glibc 2.35 基线。Git、HTTPS 等 ELF 使用私有 `RUNPATH` 加载附带库，宿主仍提供 glibc、CA 证书与 OpenSSH。安装包记录每个私有库的 Ubuntu 二进制包、源包、版本和许可证。独立 Ubuntu 22.04 与 Ubuntu 24.04 容器均在无外部网络、无系统 Git 的条件下验证了 `.deb`、Arch 包和 AppImage 中的工具，确认 30 个私有库的实际加载位置。每种格式通过仓库初始化、LFS、本地 CA 指定及默认系统 CA 的回环 HTTPS。Ubuntu 24.04 的独立容器也使用 apt 安装 `.deb`，在无系统 Git 的环境中打开了 GitWave 窗口。

Ubuntu 22.04 的验证范围是包内 Git 工具。NSGo 桌面后端仍需要 GTK 4.14，Ubuntu 22.04 自带 GTK 的完整桌面兼容仍待处理。还需验收原生 Arch 与 RPM、AppImage 的其他发行版 CA 路径、附带库完整源码和 SBOM、macOS Developer ID 签名与公证，以及正式升级和回退。 [NSGo Linux 后端](https://github.com/nsgo-dev/nsgo/blob/v0.2.0/internal/platform/linux/linux.go)

## 2026-11-24 · M5 Windows Git 随包交付

NSIS 预览包从固定 SHA-256 的官方发行物引入 MinGit 2.56.1 和 Git LFS 3.8.1，保留 Git、Git Credential Manager、OpenSSH、LFS 的许可证及来源记录。仓库操作、Workspace 导入和 LFS 子进程优先从 `gitwave-git` 发现工具，子进程使用自己的 `PATH`。Windows 命名管道 agent 优先使用系统 OpenSSH，Git Bash socket 可以继续使用其 Git for Windows runtime。仓库级 `core.sshCommand` 继续优先。 [MinGit 2.56.1](https://github.com/git-for-windows/git/releases/tag/v2.56.1.windows.1) [Git LFS 3.8.1](https://github.com/git-lfs/git-lfs/releases/tag/v3.8.1) [Git LFS SSH 客户端选择](https://raw.githubusercontent.com/git-lfs/git-lfs/v3.8.1/ssh/ssh.go)

NSIS 安装和卸载使用同一个 Windows 会话互斥对象，覆盖 Go helper 返回后的 NSIS 操作。安装事务保留原 Git 目录，和应用／卸载器一起发布或回滚。库存和配置摘要保护新增用户文件、修改过的随包配置及注册失败期间写入新目录的文件。主程序缺失时，卸载器从自身解出原应用中的清理入口。 [NSIS System 插件](https://nsis.sourceforge.io/Docs/System/System.html)

macOS Go 竞态验证了 Git 与 LFS 调用、安装文件事务、应用更新和 Workspace 导入。Windows x64 安装应用与测试已通过交叉编译。Ubuntu 容器编译并解包 Windows NSIS，核对安装器 SHA-256、应用 EXE、官方 Git 和 LFS 的实际字节。Windows CI 已配置清除系统 Git `PATH` 后的工具验证，以及安装回滚、互斥、配置保全和主程序缺失时的卸载。Windows 实际运行、正式签名、完整对应源码与 SBOM，以及 macOS／Linux 的随包 Git 继续验收。

macOS arm64 使用 kernel.org 对应 SHA-256 的 Git 2.56.1 源码构建了可重定位的 Git 和 Keychain helper。使用 `NO_RUST=YesPlease` 后构建无需调用 Cargo。Git、HTTPS helper、Keychain helper 只链接系统库。在含空格的目录中，仅使用随包 Git 和官方 Git LFS 3.8.1 完成仓库初始化、LFS 安装和跟踪。NSGo v0.2.0 的资源构建会把符号链接复制成普通文件。包装夹具在生成 `.app` 后保留符号链接复制 Git 目录，逐个签名 Mach-O，再对应用进行 ad hoc 签名。`codesign --verify --strict --deep` 通过，包内 `git --exec-path` 指向预期目录，`git lfs version` 正常。正式 CI 打包、Developer ID 签名与公证仍待接入。 [Git 官方源码和校验清单](https://www.kernel.org/pub/software/scm/git/) [NSGo 资源复制实现](https://github.com/nsgo-dev/nsgo/blob/v0.2.0/cmd/nsgo/resources.go)

Ubuntu 24.04 x64 在没有 Cargo 的容器中使用相同源码和 `NO_RUST` 构建。移动到含空格的目录后，`git --exec-path` 指向新目录，Git 初始化、Git LFS 3.8.1 本地安装和跟踪通过。`ldd` 确认 HTTPS helper 使用 Ubuntu 的 `libcurl.so.4`、`libz.so.1` 和它们的依赖。Linux 软件包依赖声明、AppImage 携带策略和真正的 HTTPS 传输尚待验收。 [Git 2.56.1 构建选项](https://github.com/git/git/blob/v2.56.1/Makefile)

## 2026-11-24 · M5 正式数据接管与旧版回退

正式包按 NSGo 应用 ID 打开原 `GitWave/state.db`，开发和预览包使用 `GitWave Preview/state.db`。工作台启动后才打开数据库，因此 Windows 安装器的通知注册和注销过程不会触发接管。Go 正式版与 Tauri 过渡版共用 `.gitwave-profile.lock`，通过进程句柄生命周期约束并行访问。已发布的旧包使用 SQLite 锁，其安装器升级和回退仍需验证当时应用进程已退出。 [Rust 文件锁](https://doc.rust-lang.org/std/fs/struct.File.html#method.try_lock)

首次接管在 SQLite 保留写事务期间，从独立只读连接备份原库。备份包含旧 WAL 已提交的修改，校验后写成独立 `backup-before-nsgo-*/state.db`。备份发布完成后才迁移 live 库并记录接管标记。失败留存原库的旧 schema，重试以当时的原库重新备份。新建空库无需回退副本。 [SQLite Online Backup](https://sqlite.org/backup.html) [SQLite 写事务](https://sqlite.org/lang_transaction.html)

回归覆盖并发打开空库时的 WAL 初始化、已存在库的并发初始化、迁移失败后再次接管、旧快照中的仓库已移除时保留现有选择，以及缺少旧偏好且没有显式 Go 偏好时暂停后台检查。macOS 跨进程夹具验证 Rust 与 Go 对同一个 profile 的互斥，并用 Rust 旧仓库接口分别读取 Go 修改后的 live 库和接管前的备份。过渡 Rust 端更新 AI 设置时现在合并 JSON，可保留未知字段。已经发布的旧端覆盖写入仍可能丢弃它不认识的字段。Go 全量竞态及旧 Rust Git 对照、六项 macOS 原生窗口测试、Ubuntu x64 正式库和锁回归均通过，Windows 测试通过交叉编译。正式签名包跨版本安装和数据回退仍待实机验收。

## 2026-11-24 · M5 Windows 安装事务和旧 NSIS 过渡

Windows 包继续使用 NSGo 生成的 NSIS 界面和卸载器基础。新安装段先将包解至临时目录，再调用包内 Go 程序在原安装卷暂存 `GitWave.exe` 和 `Uninstall.exe`。替换前收存旧文件，替换全数成功后写入注册项。普通写入失败会恢复已替换的文件和已修改的注册值。回滚时发现并发换件会保留外部文件与旧副本，并报告恢复目录。安装和卸载会等待现有 GitWave 退出。 [NSIS 文件移动行为](https://nsis.sourceforge.io/Reference/Rename)

正式安装识别旧版按产品名写入的卸载键，复用旧安装目录，写入新卸载记录后移除同目录的旧记录。兼容旧 updater 的 `/P /UPDATE /R /ARGS`，Go updater 也使用被动进度窗口。新 Go CI 包含 v0.11.0 发布 NSIS 的 SHA-256 固定夹具、含空格目录原位迁移、用户文件保留，以及锁定第二个安装文件后回滚并重试。 [v0.11.0 发布资产](https://github.com/Yangshifu1024/GitWave/releases/tag/v0.11.0) [Tauri updater 参数](https://github.com/tauri-apps/plugins-workspace/blob/updater-v2.14.0/plugins/updater/src/config.rs#L36)

本机通过文件事务回归、NSIS 生成测试、Go 竞态检查、Windows amd64 交叉编译及 actionlint。Ubuntu 使用 makensis 编译了预览和正式 ID 的两种安装包，校验包摘要，并逐字节对照解出的 `GitWave.exe`。Windows 运行、正式签名、程序被强制终止后的恢复，以及未来随包 Git 文件的更新和回退仍待验收。当前包仅包含 Go 可执行文件，包生成器会拒绝未知载荷以免遗漏安装或回滚步骤。

## 2026-11-24 · M5 Windows NSIS 更新交接

Windows Go 更新器识别正式 per-user NSIS 注册项，以及与注册项对应的运行文件。安装器放入具有当前用户与 SYSTEM 访问权限的私有目录。下载后及交接前均核对 SHA-256、Authenticode 和安装器版本。独立 Go 进程为当前安装获取互斥保护，再通过继承的进程句柄等待旧实例退出，按安装目录调用 NSIS，在重新启动前核对新 EXE 的签名、版本以及更新清单签署的可执行文件摘要。确认下载后在安全退出条件满足时自动继续安装，若发现仓库写入或未保存的冲突草稿则保留重试入口。 [WinVerifyTrust](https://learn.microsoft.com/en-us/windows/win32/api/wintrust/nf-wintrust-winverifytrust) [NSIS 命令行](https://nsis.sourceforge.io/Docs/Chapter3.html#installerusage) [Tauri 旧更新流程](https://github.com/tauri-apps/plugins-workspace/blob/updater-v2.14.0/plugins/updater/src/updater.rs#L941)

静态复审发现的同路径换件和安装器版本检查问题已处理。Windows amd64 应用、测试交叉编译和更新器 `go vet` 通过，签名清单、macOS 安装回归和更新弹窗的 Go 竞态测试通过。五项 NSGo 原生窗口用例在 macOS 通过，包括 Windows 路径的 UI 交接。Go CI 在 Windows job 增加预览安装器自定义路径安装、卸载和用户文件保留门禁。本机 Wine 在运行测试程序前因 Rosetta 的 GDT 限制退出。后续安装事务、旧 NSIS 过渡及验收状态见上节。

## 2026-11-24 · M5 macOS 应用内更新与重启

macOS arm64 使用临时 ad hoc 签名的应用包验证下载校验、交换新旧应用包、保留符号链接与扩展属性。归档夹具覆盖路径穿越、大小写和 HFS+ 路径别名、链式符号链接越界、符号链接下的归档条目及无效长度。交换后对两个应用包重新执行完整验签，目录同步失败时尝试交换回原路径。新旧应用包在发布失败时都保留，回滚后再次核对路径身份。 [Apple HFS Plus 卷格式](https://developer.apple.com/library/archive/technotes/tn/tn1150.html) [Darwin rename flags](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/sys/stdio.h)

重启从正式安装路径挂起创建子进程，读取内核给该 PID 载入的 CDHash，核对后才允许运行。子进程使用独立会话和标准输入输出，通过本地 socket 报告就绪，并等待旧实例退出后继续打开数据。真实应用包夹具通过错误代码身份在执行前被拒绝、新版启动失败保留旧进程与备份、就绪后安装路径变化时取消重启，以及旧进程退出后的启动。临时 AppKit 探针确认重启后 `NSBundle` 和 `NSRunningApplication` 均识别原安装路径。Go CI 已加入该重启夹具。NSGo 原生窗口复测通过菜单、自动检查偏好、进度、隐藏及重试。正式 Developer ID 签名与公证、Dock 固定项与登录项、其他文件系统及旧版过渡升级与回退仍待验收。 [Darwin spawn flags](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/sys/spawn.h) [Darwin code-signing operations](https://github.com/apple-oss-distributions/xnu/blob/main/bsd/sys/codesign.h)

## 2026-11-24 · M5 Linux AppImage 应用内安装与重启

下载在安装目录的私有暂存区完成，通过已打开的目录句柄操作文件，并在写入前再次核对散列、AppImage 类型、架构和当前安装文件身份。支持交换的卷原子交换新旧镜像，支持非覆盖式 rename 的卷单独保留旧镜像再发布新文件。检测到并发换件时保留现场与恢复文件，重启完成交接前保留旧镜像。

更新弹窗已接通下载进度、隐藏、安装重试、已安装和重启失败重试。重启核对所有仓库的在途写入和冲突草稿。Linux 从验证过的镜像句柄用私有临时目录启动解压模式；新版通过管道报告就绪，再等旧进程退出后才打开 SQLite。新版启动失败时旧窗口仍可操作且保留旧镜像。 [AppImage type 2 运行时](https://raw.githubusercontent.com/AppImage/type2-runtime/20251108/src/runtime/runtime.c) [AppImage 环境变量](https://docs.appimage.org/packaging-guide/environment-variables.html)

Ubuntu 24.04 x64 复测成功替换、错误架构拒绝、下载期间并发换件、实包重启和启动失败保留旧镜像。Go CI 已加入相同的 AppImage 实包门禁。macOS 原生窗口和 NSGo 模型验证更新交互及跨仓库重启保护。Windows 的应用内安装流程仍待实施。

## 2026-11-24 · M5 更新检查与签名清单

通用设置与原生应用菜单共用 Go 更新检查。启动检查读取 SQLite 中从旧版导入的开关和当前代理。偏好或代理读取失败时，手动检查仍可用。关闭自动检查会取消进行中的后台检查，已被代替的响应无法覆盖手动结果。发现更新时，GitWave 自己的弹窗显示发布说明，稍后可从设置重新打开，打开发布页时使用发现的精确版本。

发布工具要求三端资产齐全、Ed25519 种子与构建公钥匹配，再原子生成 `gitwave-update.json`。客户端只通过可信 HTTPS 位置读取签名清单，比较 SemVer，拒绝把旧版当作更新。下载以签名中的长度和 SHA-256 为准，失败或取消时清理临时文件。预览构建尚未注入生产公钥。 [架构决策](../../tech/decisions/00-overview.md#0007--全量迁移到-go--nsgo)

本地 TLS 和代理夹具覆盖手动代理、非可信重定向、签名篡改、降级、损坏包、成功下载和下载完成前取消。NSGo 模型在 macOS 与 Ubuntu 24.04 x64 通过，macOS 原生窗口通过应用菜单、发布说明、重新打开通知、旧开关迁移及取消后台检查。用临时密钥对本地 macOS `.app` ZIP、Windows NSIS 和 Linux AppImage 生成清单，并独立校验签名与三个产物的长度、摘要。Windows x64 通过交叉编译。三端安装、重启、生产密钥接入发布流水线及过渡升级待验收。

## 2026-11-24 · M5 预览安装包门禁

Go CI 在 macOS arm64、Windows x64、Linux x64 的各自 job 构建 NSGo 预览安装包，要求产物齐全并验证 SHA-256，再上传安装包及校验文件。预览保留 `desktop.gitwave.work.native-preview` 应用 ID，使用 `none` 更新通道，沿用现有 GitWave 图标和 MIT 许可证。 [Go CI](../../../.github/workflows/go-desktop.yml) [NSGo 打包说明](https://github.com/nsgo-dev/nsgo/blob/v0.2.0/docs/public/en/building.md)

本地验证 macOS DMG 镜像和 ad hoc 应用签名，逐字节比较了打包后的原版 ICNS。Ubuntu 24.04 x64 容器编译并列出了 Windows NSIS 安装器内的 `GitWave.exe`，验证了 Linux deb、AppImage 和 Arch 包的校验值、deb 元数据与 AppImage 解包结果。CI 配置通过 actionlint。托管三端运行、Windows 安装、正式签名及公证、RPM、更新及回退仍待验收。

## 2026-11-24 · M4 SSH 密钥设置

设置页通过 OpenSSH 加载密钥，通过 SSH agent 协议读取公钥和备注。独立字段保留多行备注。移除时在同一个连接内按指纹选中公钥，再把该公钥发送给 agent；无需写入临时公钥文件。测试连接禁用交互式认证和转发，并从客户端日志判断是否认证成功，兼容已认证但未提供 shell 的 Git 主机。 [SSH agent 协议](https://datatracker.ietf.org/doc/html/rfc9987#section-5.4)

macOS 原生窗口使用独立 agent 验证加载、移除和 Escape 关闭内联表单。macOS 和 Ubuntu 24.04 x64 的真实 agent 回归加载两把密钥，让第一把的备注包含另一把密钥的路径和完整公钥行，验证列表仍为两项且仅移除选中项。Ubuntu 回环测试验证服务端接受 agent 中的密钥、拒绝未加载密钥、Git 主机认证后以状态 1 退出，以及使用已加载密钥执行 Clone、Fetch 和 Push。Go 全量竞态及旧 Rust 行为对照通过。

Windows 连接系统 agent 的本地命名管道，显式配置的 Git Bash socket 则通过其 MSYS 回环协议连接。独立 LFS 下载和上传复用 Git 的 Windows OpenSSH 默认选择，遇到显式 `core.sshCommand` 时给该配置保留优先级。定向测试覆盖了子进程环境的优先级和已有 HTTPS LFS 流程。Windows x64 的命名管道和 Git Bash 隔离夹具通过交叉编译，仍待 Windows 实机运行，并验收启动 agent 的 UAC。 [Git for Windows 的 MSYS socket 实现](https://raw.githubusercontent.com/git-for-windows/msys2-runtime/main/winsup/cygwin/fhandler/socket_local.cc) [Git LFS 客户端选择](https://raw.githubusercontent.com/git-lfs/git-lfs/v3.8.1/ssh/ssh.go)

## 2026-11-24 · 首启 SQLite 复制与 WebView 偏好

Go 预览首次启动时，用 SQLite 在线备份从旧 `state.db` 读取一致快照。新副本通过 schema 迁移、完整性检查和 checkpoint 后，再发布到 `GitWave Preview`。Windows、Linux 和 macOS 使用各自已有的文件无覆盖发布机制。遇到孤立 WAL 等文件时保留原状并报错，以免其他数据库的日志被接入新副本。

过渡版把允许的 WebView 偏好写入旧库，覆盖主题、语言、字体、四处分栏、最近工作区和仓库、自动刷新及更新开关。macOS `Cmd+Q` 走同一个退出流程，普通退出保留有限等待。安装及重启会等待最新快照写入，写入失败通过原有更新错误提示反馈。快照只收集白名单字段。

Go 端从数据库复制时点读取偏好，一次事务导入，保留已有的 Go 设置和包括显式 `null` 在内的未知字段。已有 NSGo 分栏值优先。最近仓库会验证所属工作区和可用状态。更新开关值已保存，等待接入新版更新器。没有快照时，Go 仍保留已复制的工作区和仓库，并提示旧 WebView 偏好不可用。

macOS 与 Ubuntu 24.04 x64 的 SQLite 夹具通过 WAL 内提交复制、来源后续更新隔离、无效 schema 不发布、孤立 WAL 保留和幂等导入。NSGo 模型与 macOS 原生工作台验证语言、主题和分栏恢复。Go 全量竞态和旧 Rust 行为对照、旧前端 369 项测试及 Rust 全量测试通过。Windows x64 通过交叉编译。真实签名安装升级、旧版跳版升级及 Windows 实机仍逐项验收。

## 2026-11-24 · 网络设置与代理

Network 设置沿用 SQLite `app_settings` 的 `proxy` 条目。支持跟随系统、手动和关闭，保留关闭时的手动地址。进入新请求时读取最近一次保存成功的设置。手动地址可用 Enter 提交，也可通过点击其他模式、离开输入框或关闭设置提交。两次快速模式切换保留最后选择。

系统模式读取 macOS 顶层网络代理和排除项、Windows Internet Settings，并沿用 Linux 的启动环境。手动模式覆盖启动代理和绕过列表，保留回环豁免。关闭模式沿用启动环境。Git 隔离传输继承实际仓库所选地址的代理设置，包括远端配置的空值。代理仅传给本次操作的请求和对应子进程。离线 Ollama 直连回环地址。

SQLite 兼容用例覆盖旧记录、未知字段、地址清除和校验。带用户名和密码的旧代理 URL 沿用原数据库格式，macOS 与 Linux 打开旧数据库时将数据库及 WAL 等文件限制为本人可读写。Git 与 LFS 的错误输出清除已知代理密码、URL 凭证和 Basic 编码。代理地址只有用户名并由 Git helper 补充密码时，错误使用固定提示。回环代理夹具覆盖 AI、Clone、Fetch、实际仓库代理优先级，以及本地 Git 目的地使用独立 HTTP LFS 服务的上传。LFS 失败重试用例验证错误信息脱敏和分支发布结果。

NSGo 模型竞态、macOS 原生窗口和 Ubuntu 24.04 GTK/Xvfb 原生窗口通过模式切换、输入校验和保存。Go CI 的 Linux job 已加入设置和 Recovery 原生用例。复审通过 Git helper 密码和 LFS 用户名令牌被回显的夹具。Go 全量竞态及旧 Rust 行为对照通过，收口后的凭证脱敏、系统代理解析和快速切换均完成定向复测。Windows 实际代理注册表、各平台特殊绕过规则和更新器网络路径继续验收。 [F013](../../pm/features/F013-system-proxy.md)

## 2026-11-24 · 健康面板与 Recovery 侧栏

侧栏导航固定，仓库工具区恢复旧版的纵向滚动与分组顺序。分支列表按行数设置高度和最小高度，超过九行时单独滚动，矮窗口由外层工具区承担剩余的滚动。1024 × 608 的 macOS 和 Ubuntu 24.04 GTK 原生窗口，以及 NSGo 模型竞态回归，都通过滚动、打开一条 Reflog 记录并进入创建恢复分支对话框。Ubuntu 用例使用 Git 2.46.0。macOS 原生窗口也通过单条记录的中央点击、查看解释、创建恢复分支和确认重置。

健康面板保持状态按钮的身份，后台测量结果更新时只刷新读数和明细。定向竞态回归在按下和释放之间发布测量结果，释放后仍能展开健康面板。

## 2026-11-24 · HTTPS 远端与 Git LFS 凭证

Clone、Fetch、Pull、Push、远端分支删除与子模块操作优先使用配置好的系统 Git helper。隔离的传输仓库继承实际仓库针对所选 URL 解析出的用户名和 `useHttpPath`。手动操作收到 HTTPS 认证挑战后，可用系统密钥库中的凭证重试同一端点。定时刷新走非交互式 helper 路径。认证框显示脱敏后的仓库或 LFS 地址，同一主机的不同仓库可分别确认。

勾选「记住凭证」后，仅保存服务器接受过的凭证。新条目按 HTTPS 地址、仓库路径与账号区分，多个账号保持独立。如果该路径已选择账号，旧格式的回退也只接受同一账号。旧 `gitwave.remote` 的 `https/<host>` 和 `https/<host>\x1f<username>` 条目可通过指定用户名读取，在成功认证后保存为当前路径的新条目。对于旧格式中 IPv6 地址与端口可能重合的记录，仍需在认证框重新输入令牌。 [旧版远端凭证实现](../../../src-tauri/src/infrastructure/git/credentials.rs) [Git credential 协议](https://git-scm.com/docs/git-credential)

默认从 Git 地址推导的 LFS 地址可复用 Git 凭证。显式配置的 LFS 地址使用独立挑战与凭证。Clone 保留 Git 与 LFS 各自的确认绑定，媒体地址在确认后变更时中止。Push 和远端分支删除的认证重试沿用已确认计划和已经报告成功的目的地。

真实回环 HTTPS 用例覆盖 helper 优先、仓库级账号、定时刷新、失败凭证仅重试一次、同源路径重定向拒绝、两个 LFS 账号、部分 Push、嵌套子模块与用户选中旧账号。Ubuntu 24.04 x64 的 GNOME Keyring 用例覆盖旧 AI 与旧远端凭证的服务隔离。Windows 凭证库、Linux 桌面解锁提示与 KWallet 仍需实机验收。

## 2026-11-24 · M4 Linux 旧版 AI 凭证

Go 在 NSGo 密钥库缺少当前账号时，通过用户会话总线查找 `gitwave.ai` 的旧条目。读取优先使用 `username` 和 `target=default` 匹配的唯一条目，然后查询默认集合中无 `target` 的历史格式。清除同时预检两种格式，并在解锁后核对账号、服务、target 和锁定状态。已有多个匹配项时，设置页提示用户处理重复条目。其他 target 和 NSGo 的 `account` 条目保持独立。 [旧版 Rust keyring 源码](https://docs.rs/keyring/3.6.3/src/keyring/secret_service.rs.html) [Secret Service 解锁规范](https://specifications.freedesktop.org/secret-service/latest/unlocking.html)

Ubuntu 24.04 x64 容器的真实 GNOME Keyring 集成用例通过。测试复现 Rust 写入的两种属性格式，验证优先级、Unicode、一次清除两种旧格式、拒绝清除重复项，以及保留不同 target 和 NSGo 条目。Go CI 的 Linux job 已配置相同的独立 D-Bus 夹具。桌面解锁提示、KWallet、实际 Windows 凭证库和 `gitwave.remote` 的兼容继续验收。

## 2026-11-24 · M4 命令面板

`Cmd/Ctrl+K` 打开或关闭命令面板。方向键与 Enter 选择设置、Fetch 或搜索结果。提交搜索匹配 SHA、作者、邮箱和提交信息，逐页显示最近匹配，追加页保留滚动位置。命令面板使用独立游标，定位时按需刷新 History，以便选中终端新提交。

AI 使用 Workspace provider 和用户显式提交的请求。仓库上下文包括已脱敏的引用名称和近期提交主题，仓库采用昵称或末级目录名。响应严格检查已知参数类型并按旧版白名单解释为建议。可确认的写操作限于建分支、建标签、检出、贮藏和 Fetch。建分支和标签绑定建议中的提交。检出同时校验源与目标引用及 OID，脏工作树会被拒绝。贮藏建议也绑定生成时的检出，终端切换分支后需重新确认新的建议。

竞态定向回归和 macOS 原生窗口通过快捷键、分页、外部新提交定位以及分支、标签、检出和贮藏的确认流程。独立模型复审验证分页滚动位置、输入替换取消旧请求、字段脱敏和切换到同一 OID 的另一分支会拒绝旧检出或贮藏建议。命令面板实现通过 Go 全量竞态和旧 Rust 行为对照，追加的贮藏与凭证状态改动通过定向竞态。Windows/Linux 的物理键盘、输入法与窗口样式仍需实机对照。

## 2026-11-24 · M4 History 提交解释、PR 草稿与冲突解释

History 可解释所选提交，以该提交及其首父提交为上下文。Branch 菜单可生成当前分支的 PR 标题与描述，以默认基准分支的合并基线取已提交内容。草稿支持编辑、重新生成和复制，生成前后均复核源分支与基准分支。提交与 PR 补丁沿用暂存建议的对象检查、脱敏和大小限制。

冲突解释读取当前文件在 Git 索引中的 Base、Ours 和 Theirs。测试验证未保存的编辑草稿留在本地。索引的同一路径换成其他冲突阶段时，旧解释失效，未保存的解决草稿仍可复制。模型定向竞态测试和 macOS 原生窗口通过提交解释、PR 草稿、冲突解释和编辑保留。独立复审通过同分支外部提交后重新生成、优先级更高的默认基准出现以及同路径冲突阶段变更。PR 复制入口复核最后一次成功生成的源分支和基准分支快照。外部改动使草稿过期时，保留用户手改文字和系统剪贴板，并提示重新生成。

## 2026-11-24 · M4 AI 提交建议、健康总结与 Reflog 解释

Changes 的生成入口读取 Workspace 配置、用户选择的回复语言、提交 Prompt 与当前仓库规则。暂存消息生成前复制索引，核对界面已显示的文件路径、暂存 mode、OID 和分支 HEAD。补丁只读取可核对的完整对象，含私钥、不可读取和超限对象从上下文移除。脱敏后再按行截断补丁。索引在请求前改变时不发送数据，在请求后改变时保留现有草稿。手动取消、切仓或切出 Changes 会取消请求，建议可以编辑和恢复原草稿。

健康总结使用当前仓库的本地指标。Reflog 解释使用选中记录的前后提交与标题，切选或切仓后旧结果失效。短解释在恢复面板内显示，较长解释使用可滚动窗口。AI 建议只生成文本，Git 写操作仍需用户发起。模型回环测试覆盖三项生成、索引变动、取消、私钥片段排除、新分支首次提交、重命名和删除。独立夹具验证外部暂存覆盖会使 provider 收到零请求。

macOS 原生窗口通过生成、恢复原草稿、未配置时打开 AI 设置、健康总结、Reflog 解释长文滚动以及既有恢复分支与硬重置确认。Windows/Linux x64 交叉编译、`go vet` 和模块校验通过。

## 2026-11-24 · M4 Workspace AI 设置与 provider

Workspace 菜单接入 AI 设置。OpenAI、Anthropic 和 Ollama 可配置各自的模型、基础地址与有序备用服务商，保存五项 Prompt 模板。Ollama 检测只访问 loopback，展示安装模型。若工作仓库存在有效的 `.gitwave/AI.md`，设置页会显示规则摘要。SQLite 只更新旧版 AI 字段，保留未来字段和其他 Workspace 设置，API key 由操作系统密钥库保存。

统一 Go 客户端实现旧版请求协议、网络失败重试及备用服务商顺序。认证、输入或响应格式错误会停止。离线模式在读取密钥前筛选 loopback Ollama，直连且禁止重定向。在线模式仅允许同源同方法重定向，避免将 provider key 和已脱敏的 prompt 交给另一个目标。出站前脱敏覆盖 PEM、编码段、常见 token、Basic Authorization、X-API-Key 和配置中的有效 key。HTTP 失败提示只输出 provider 与状态，避免把服务端回显写进诊断。

本地 HTTP fixture、NSGo 模型和 macOS 原生窗口通过协议、脱敏、重试、取消、Ollama 探测、配置重启恢复、密钥遮盖与保存清除。密钥用例使用测试内存库，实际操作系统凭证迁移单独验收。Windows 已增加旧 Credential Manager 项的读取与清除适配及 Windows 专用用例，待实机执行。Linux 旧 Secret Service 属性兼容、系统代理设置迁移及各 AI 生成入口仍待完成。

## 2026-11-24 · M4 桌面设置与关于

常规设置包含中英界面、AI 回复语言和自动刷新。外观设置支持跟随系统、浅色、深色及 Native Blue、Tide Studio 配色。正常启动从 Workspace SQLite 读取设置。保存采用单字段更新，保留同时保存的其他设置及未知字段。界面语言、主题和配色即时生效。「关于 GitWave」从原生菜单打开，显示当前 `nsgo.yaml` 的版本，并提供应用数据目录与 GitHub 入口。

NSGo 模型和 macOS 原生窗口的定向用例通过设置切换、重启恢复和「关于」显示。模型测试验证了打开应用数据目录及 GitHub 时传给系统的路径与 URL。AI 回复语言与应用调用入口的接入、旧 WebView 偏好迁移及 Windows/Linux 实机验收仍待完成。

配色切换后的回归覆盖 Changes 文件选择、列表滚动位置与长文本 diff 的垂直滚动位置。工作台和 Changes 面板使用窗口分配的高度，长 diff 在预览区域滚动。回归在 NSGo 模型的竞态检查及 macOS 原生窗口均已通过。Windows/Linux x64 交叉编译、`go vet` 和模块校验通过。

外观页接入界面与等宽字体、字号、即时示例和单项重置。字体名单和字号范围沿用旧版输入语义。界面字体用于工作台自绘文案，等宽字体和字号应用到文本 diff、blame、冲突编辑和仓库文件编辑。字体写入只更新本次变更字段。独立复审在 SQLite 写入暂停时验证了切换输入框、关闭和重新打开设置时保留草稿，新输入及光标不会被首次保存打断。各平台系统菜单和 NSGo 控件的尺寸仍需逐项与旧版对照。

最大字号下的 Windows 模型测试覆盖多种窗口宽度，Push 等操作仍位于系统窗口按钮外。宽 CJK 字符和用户选择的比例字体用于长行预览时，横向可滚范围包含完整行内容。最新 macOS 原生窗口回归通过字体保存、重启恢复、重置及双轴滚动。

长文本 diff 恢复横向可滚动范围，横向位置在新视口布局后恢复。NSGo 模型和 macOS 原生窗口通过切换语言后保留 History 文本 diff 双轴位置。后台 Git 提示读取 UI 线程发布的语言快照，Clone、Remote、Submodule 和 Workspace 原先报竞态的用例已通过定向复测。修复语言竞态后的 Go 全量竞态与旧 Rust 行为对照、旧版 `make check` 均通过。

## 2026-11-24 · M3 离线 checkout 的 LFS 路径筛选

离线 checkout 使用与 Git LFS 相同的 wildmatch 规则按文件路径选择缓存媒体。以同一媒体对象在三条路径验证目录规则、排除优先级及大小写折叠。未入选的路径保留指针。Git LFS 3.8.1 的实际 smudge 对照通过，目标 `.lfsconfig` 和仓库配置中的空值覆盖也已验证。 [Git LFS smudge](https://github.com/git-lfs/git-lfs/blob/v3.8.1/commands/command_smudge.go) [Git LFS 路径匹配](https://github.com/git-lfs/git-lfs/blob/v3.8.1/filepathfilter/filepathfilter.go)

复审覆盖稀疏检出省略目标配置时从源索引读取，大小写不敏感卷读取目标 `.LFSCONFIG`，以及丢弃或暂存修改过的项目规则后再切换。Discard 恢复未被目标提交修改的文件时同样保留排除路径的指针。普通文件的 checkout 不会读取未用到的 LFS 配置。Pull 继续使用本次下载收据锁定的路径并校验缓存对象。

发布前重新读取筛选配置。预先物化期间修改配置会中止 checkout。故障注入在实际写入目标文件后使 Git 返回失败，回退保留原 HEAD，并分别恢复已有指针与已有媒体内容。

Go 全量竞态及旧 Rust 对照、Git 2.46 定向竞态、`go vet`、模块校验、旧版 `make check` 及 Windows/Linux x64 交叉编译通过。macOS 原生窗口连续两轮通过分支 checkout 和 LFS Pull，Discard、脏切换和 Stash 的原生用例也通过。Windows/Linux 原生窗口及实际 SSH 服务仍待验收。

## 2026-11-24 · M3 仓库健康本地指标

侧栏可展开查看未推送提交、操作残留、脏文件、过期分支、大文件和分支／标签计数。切换仓库取消旧测量并收起报告。手动刷新和已检测到的工作树变化会更新报告。真实 Git 夹具与旧 Rust 输出一致，覆盖了暂存重命名、未跟踪和被忽略路径、未推送提交、过期分支、操作残留以及普通与 annotated tag。

独立复审确认 reftable 根引用即使没有松散文件也能报告，linked worktree 的操作状态不会显示在另一个检出中。缺失 OID、坏提交和截断的压缩对象会被跳过。损坏引用导致 History 失败时，仍能显示可用的健康指标。子模块工作树改动计入父仓库的脏文件数，状态检查使用隔离配置。大文件检查跳过链接目录和构建产物目录，最多显示五项，并提示扫描到达目录项上限。

Go 全量竞态及旧 Rust 对照、Git 2.46 定向竞态、`go vet`、旧版 `make check` 和 Windows/Linux x64 交叉编译通过。macOS 原生窗口连续两轮通过展开、刷新、收起和切换到 History 无法读取的仓库。Reflog、Remote、Stash、Submodule 和 Worktree 的相邻原生用例也通过。健康 AI 总结和 Windows/Linux 原生窗口仍待完成。

## 2026-11-24 · M3 Hooks 与 `.gitignore`

Repository 菜单可查看和编辑八种客户端 Hooks。编辑器显示实际脚本路径，支持模板和保存。保存只改脚本内容和所需权限。默认路径使用 linked worktree 的 Git 共享目录，自定义路径由 Git 解析。有效的符号链接保留在原位置，脚本更新到显示的目标。失效链接只影响对应条目。 [Git hooks](https://git-scm.com/docs/githooks) [Git core.hooksPath](https://git-scm.com/docs/git-config#Documentation/git-config.txt-corehooksPath)

Changes 可打开根 `.gitignore` 编辑器，也可从未跟踪文件的右键菜单按当前文件、父目录或扩展名添加规则。添加规则保留文件，由 Git 对子目录规则预检。独立复审覆盖外部暂存替换原索引锁、预检中新增或修改子目录规则、linked worktree 的共享 Git 目录被替换和 Hook 链接目标变更。回归还覆盖 CRLF、路径通配字符转义和准备发布的文件锁被原位改写。 [Git ignore patterns](https://git-scm.com/docs/gitignore)

macOS 原生窗口连续两轮通过 `.gitignore` 编辑、取消、按范围忽略、Hook 模板和保存后切换脚本。Apple Git 和 Git 2.46 的定向竞态、`go vet`、旧版 `make check`、Windows/Linux x64 交叉编译通过。Windows/Linux 原生窗口仍待实机验收。

## 2026-11-24 · M3 Clone 的 Git LFS 下载

Clone 在私有目录执行无检出的 Git 传输。有效 Git 源地址只重写一次，最终仓库保留用户配置的远端名和原始地址。Go 从初始 HEAD 核对 LFS 属性与筛选规则，只下载所需对象。标准 Git LFS 检出使用已校验的缓存和私有文件端点。发布前核对索引、媒体内容、引用、源地址及缓存映射。 [Git ls-remote](https://git-scm.com/docs/git-ls-remote#Documentation/git-ls-remote.txt---get-url) [Git LFS fetch](https://github.com/git-lfs/git-lfs/blob/v3.8.1/docs/man/git-lfs-fetch.adoc)

本地及 HTTPS 回归覆盖媒体缺失、发布前删除、独立 Git 与 LFS 凭证、认证期间仓库更换媒体地址，以及同一对象分别出现在包含和排除路径时的检出。检出时移除已下载的对象会使克隆失败，媒体端点未收到新的网络请求。绝对、相对、大小写别名及符号链接缓存能按最终目录映射。缓存别名在发布之前发生变化时保留原目录，已有缓存目录随原目录备份保留。独立复审也通过了使用官方 LFS extension 的转换与验证。

Go 全量竞态与旧 Rust 对照、Git 2.46 和 2.54 定向竞态、`go vet`、旧版 `make check` 和 Windows/Linux x64 交叉编译通过。macOS 原生窗口连续两轮验证 Git 与 LFS 分别认证、媒体下载后登记 Workspace、原有 Clone 与 Pull 交互。Worktree 的 ACL 回归改用 UUID 输出比较，避免系统名称查找暂时失败引发误报。 [Apple ls ACL 实现](https://github.com/apple-oss-distributions/file_cmds/blob/main/ls/print.c#L261-L291)

只对最终目录命中的 `includeIf.gitdir` 在私有目录尚未激活。由此启用的 LFS 过滤器可能在克隆后仍留下指针。自定义过滤器继续使用用户配置。Windows/Linux 实机及实际 SSH 服务仍待验收。 [Git includeIf](https://git-scm.com/docs/git-config#_conditional_includes)

## 2026-11-24 · M3 Pull 的 Git LFS 下载

按所选远端分支 tip 列出受路径筛选允许的标准 LFS 指针，并核对目标树的 `filter=lfs` 属性。普通指针形状的文本继续按普通文件检出。文件远端读取其自己的对象缓存。HTTPS 使用官方 CLI 获取精确的缺失对象集合，下载和校验留在隔离目录。命中排除规则的路径继续显示指针，即使同一对象也被另一个包含路径使用。 [Git LFS fetch](https://github.com/git-lfs/git-lfs/blob/v3.8.1/docs/man/git-lfs-fetch.adoc) [Git check-attr](https://git-scm.com/docs/git-check-attr)

缓存校验跨越 Fetch 引用事务与工作树发布。独立回归复现并验证缓存目录替换、同目录内删除对象、保留 inode、大小和 mtime 后改写对象三种变化都会阻止发布。独立 HTTPS 端点提示实际媒体地址，可在 Git 和 LFS 分别认证后继续同一次 Pull。凭证助手按授权路径发放并保存凭证，Git LFS 同源重定向由官方 CLI 处理。 [Git LFS HTTP client](https://github.com/git-lfs/git-lfs/blob/v3.8.1/lfshttp/client.go#L548-L564)

Go 全量竞态与旧 Rust 对照、Git 2.46 与 2.54 定向回归、`go vet` 和旧版 `make check` 通过。覆盖 SHA-1、SHA-256 和开启 Rebase 的路径筛选、两个独立 HTTPS 端点认证及文件远端 Clone 的现有过滤器路径。macOS 原生窗口通过 Pull 实际下载和工作树物化、普通 Pull、LFS Push。Windows/Linux x64 通过交叉编译。

旧 Git LFS CLI 缺少 `ls-files --json` 时沿用离线检出行为。普通 Fetch 保持只获取 Git 对象。Clone 的媒体事务、普通离线检出的路径筛选、实际 SSH 服务和 Windows/Linux 原生窗口仍待验收。

## 2026-11-24 · M3 Git LFS 推送

Push 在隔离的 Git 传输仓库安装 GitWave 的 `pre-push`。按 Git 宣告的实际引用更新查找新增提交中的 LFS 指针，包括当前树中已经删除的文件。文件远端接收经过 SHA-256 和大小校验的对象。HTTPS 使用官方 CLI 从私有缓存上传经过相同校验的对象。接收端验证能在 Git 引用更新之前读到完整内容。对象缺失时，远端和本地 tracking 引用保持原状。 [Git pre-push](https://git-scm.com/docs/githooks#_pre_push) [Git LFS pre-push](https://github.com/git-lfs/git-lfs/blob/v3.8.1/commands/command_pre_push.go)

复审覆盖确认后 `.lfsconfig` 变更、本地 Git 目标被另一个仓库替换、目标对象目录被替换和网络读取期间本地对象缓存发生变更。HTTPS 试验覆盖默认和独立的同源媒体路径。工具栏 Push 在打开确认前重读活动分支的 tip 和标签，修复 Commit 后立即 Push 时因 History 还在刷新而引用过期。分支菜单继续校验菜单显示的 tip。

Git 2.46.0 和 Apple Git 2.54 通过本地文件远端与 HTTPS 的定向竞态测试。Go 全量竞态测试及旧 Rust 对照通过。macOS 原生窗口连续两轮通过 Changes 暂存、Commit、Push 和分支菜单推送。Windows/Linux x64 交叉编译、`go vet` 和旧版 `make check` 通过。实际原生窗口仍需在 Windows 和 Linux 验收。

LFS 对象下载和路径筛选仍待接入。独立认证的跨域媒体端点、只在 LFS 路径发起认证时的凭证保存、实际 SSH 服务及自定义传输适配器仍待验证或完善。

## 2026-11-24 · M3 本地 Git LFS 对象

Changes 暂存标准 LFS 指针，在索引发布前将通过 SHA-256 校验的对象写入本地缓存。提交后索引和 commit 保存指针，工作树保留完整内容。状态与 diff 计算使用只读过滤，读取编辑中的资产不会向缓存写入对象。真实 Git LFS 3.8.1 对照了空文件、普通二进制和旧版本指针的输出。 [Git LFS 指针规范](https://github.com/git-lfs/git-lfs/blob/v3.8.1/docs/spec.md)

离线检出和 Stash 使用已验证的本地对象。目标对象缺失时检出保留指针，遇尺寸相同而 SHA-256 不匹配的缓存则报告错误。切换和 Stash Apply 会在覆盖前保存工作树中唯一的已物化副本。复审覆盖已跟踪和仅本地使用的 `.gitattributes`、目标删除规则后存在形似指针的普通文件、无法识别的仓库格式、skip-smudge 以及发布前 LFS 配置变更。

Git 2.46 和 Apple Git 2.54 的定向竞态、Go 全量竞态及旧 Rust 对照、`go vet`、旧版 `make check` 和 Windows/Linux x64 交叉编译均通过。macOS 原生窗口连续两轮验证 LFS 面板及从 Changes 暂存真实资产。推送和下载对象仍待接入。本地检出仍需接入 `lfs.fetchinclude` 和 `lfs.fetchexclude`。暂存后同秒内的状态读取有时会按 Git 的索引规则重新计算文件哈希。Windows/Linux 原生交互仍待实机验证。

## 2026-11-24 · M3 Git LFS 面板

Repository 菜单打开 Git LFS 面板，支持检测 CLI、仓库级安装、跟踪和取消跟踪。CLI 缺失时依然可以编辑规则。规则编辑保留 BOM、CRLF、引号和同一行的其他属性，索引中的 `.gitattributes` 可作为工作文件缺失时的规则来源。写入持有文件和 index 锁，拒绝覆盖符号链接及未解决的 `.gitattributes` 冲突。 [Git attributes](https://git-scm.com/docs/gitattributes)

安装由官方 CLI 在私有目录生成配置与 hook，再发布到仓库实际的共享配置和本地 hooks 目录。现有自定义 hook 拒绝覆盖时，配置也保持原状。复审验证 linked worktree 的相对 hooks 路径、本地 include 的优先级、`required=false`、worktree 配置覆盖、仓库格式版本及发布失败后的回滚。仓库之外的 hooks 路径保持原样，面板会报告只接入本地过滤器。 [Git LFS install](https://github.com/git-lfs/git-lfs/blob/v3.8.1/docs/man/git-lfs-install.adoc) [Git LFS 3.8.1](https://github.com/git-lfs/git-lfs/releases/tag/v3.8.1)

官方 Git LFS 3.8.1 在 Git 2.46 和 Apple Git 2.54 上通过定向竞态测试。NSGo 模型覆盖有无 CLI，macOS 原生窗口连续两轮通过安装、无效模式、更正后回车跟踪和取消跟踪。Go 全量竞态、旧 Rust 对照、`go vet`、旧版 `make check` 和 Windows/Linux x64 交叉编译通过。Windows/Linux 原生交互以及 GitWave 内的 LFS 对象暂存、检出和传输仍待验收。

## 2026-11-24 · M3 Submodule

侧栏显示子模块名称、URL、当前提交与同步状态。初始化由 Git 解析相对 URL，并注册所选索引 gitlink。取消注册移除本地配置，保留工作树、未跟踪文件、索引和 `.gitmodules`。列表刷新不会重新注册已取消注册的子模块。 [Git submodule](https://git-scm.com/docs/git-submodule)

递归更新先绑定目标 checkout 与远端，再尝试获取索引记录的提交。现有 checkout 通过原生工作台共用的工作树事务切换，保留不冲突的本地改动。缺失 checkout 在私有目录完成后发布。独立复审覆盖认证前和传输期间替换 checkout、嵌套 TLS 认证重试、目标 URL 变更和工作树冲突。 [Git submodule update](https://git-scm.com/docs/git-submodule)

添加在私有目录克隆并准备索引。发布同时维护 gitlink、`.gitmodules` 与 Git 注册配置，保留目标空目录的权限。回归覆盖克隆失败、索引锁遗失后的回滚、原文件及其暂存模式、包含等号的本地路径和大小写别名。索引发布失败时恢复原目录和配置，已公开过的 checkout 移到恢复目录，以保留用户可能添加的本地文件和 Git 元数据。

Apple Git 2.54 和 Git 2.46.0 的定向竞态通过。Go 全量竞态及旧 Rust 对照、`go vet`、旧版 `make check` 和 Windows/Linux x64 交叉编译通过。macOS 的添加、初始化、更新和取消注册原生窗口场景连续通过四轮。Windows/Linux 原生交互和安装仍待实机验收。

## 2026-11-24 · M3 旧式 Remote 配置兼容

Remote 侧栏可以查看、Fetch、编辑、改名和移除 `.git/remotes/` 与 `.git/branches/` 中的远端。迁移保留原始 URL 和 Fetch/Push refspec，引用发布失败时恢复旧文件及配置。回归覆盖 linked worktree、files 与 reftable、排除 refspec、配置已发布后的故障回滚及外部替换旧文件。 [Git fetch 的旧式远端格式](https://git-scm.com/docs/git-fetch#_remotes)

独立复审复现了两种旧式文件共同组成一个远端的情况，URL 可来自 `branches/`，附加 refspec 来自 `remotes/`。迁移保留两份有效配置。复审也在真实 Git 事务中复现了切换分支与 Fetch 发布并发时造成的工作树不一致。Fetch 向本地分支发布时持有各 worktree 的索引锁，并复核检出和暂停操作。Pull 处理映射到当前分支的旧式远端时，先获取提交，再通过正常的工作树事务集成。 [Git update-ref](https://git-scm.com/docs/git-update-ref)

Git 2.46.0 和 Apple Git 2.54 的竞态回归覆盖 URL 空值重置、首次 Pull 的明确源分支和无效 refspec。Git 拒绝读取的旧配置保留在原文件，避免写入主配置后影响其他远端。macOS 原生侧栏覆盖旧远端的 Fetch、改名、Push URL 编辑、取消移除及确认移除。

Go 全量竞态及旧 Rust 对照、`go vet`、旧版 `make check` 和 Windows/Linux x64 交叉编译均通过。分支删除对话框用例等待跟踪引用加载完成后再点击，连续 8 次定向竞态及工作台全量复跑均通过。Windows/Linux 原生交互和安装仍待实机验收。

## 2026-11-24 · M3 reftable 远端改名

远端改名会保留跟踪引用的 reflog，原始提交 OID、记录人、时间和消息随记录写入新名称。共享跟踪引用及其日志保留在原名称，linked worktree 的 HEAD 和独立日志也得到保护。定向回归覆盖 SHA-1、SHA-256、悬空符号引用、多层引用索引及需要拆分的长日志。 [Git reftable](https://git-scm.com/docs/reftable#_log_block_format)

私有 Git 引用事务跳过自动压缩，防止 Git 将旧日志写入较小的默认块后失败。故障注入在配置已发布后撤销 manifest 锁，验证原配置、原 manifest 和源引用日志得到恢复，并清除未发布的表。 [Git reftable 后端](https://github.com/git/git/blob/v2.54.0/refs/reftable-backend.c#L375)

独立复审复现了外部 include 在改名准备期间新增共享 refspec 的情况。发布前会再次核对有效远端和 tracking 配置。检测到变化时中止，保留原引用与 reflog。Git 2.46.0 和 2.54 的独立竞态回归通过。改名期间需要私有副本所占的临时磁盘空间，并在内存中保留要转移的原始日志记录。

Go 全量竞态及旧 Rust 对照、`go vet`、旧版 `make check`、macOS 原生 Remote 回归和 Windows/Linux x64 交叉编译均通过。Git 2.56 的 Remote 配置竞态回归也已通过。Windows/Linux 原生交互和安装仍待实机验收。

## 2026-11-24 · M3 远端分支删除审查

History 引用菜单和分支侧栏可删除远端分支。本地分支确认框提供显式选项删除对应的远端分支。Go 按实际 fetch refspec 反查服务器分支，确认框显示推送目的地及所选提交。Git 以该 OID 作为显式 lease，拒绝删除确认后已经前进的分支。指向同一仓库和分支的别名合并为一次远端删除，别名所见 OID 冲突时保留原状态。 [Git push](https://git-scm.com/docs/git-push)

HTTPS 重试从当前待认证的目的地继续。回归验证前一处删除后即使同一提交被重新推送，后续认证也会保留该分支和新 Fetch 的跟踪引用。组合删除会分别报告已完成的本地和远端操作。指定的 file 推送路径被替换为另一个 Git 仓库时拒绝发送。传输中源仓库被替换，或 file Fetch 路径改指其他仓库时，保留替代仓库中的跟踪引用。

独立复审和真实 Git 回归覆盖 lease 冲突、重复目的地、自定义 refspec、源或目的仓库更换及部分完成。Go 全量竞态、旧 Rust 对照、`make check`、`go vet`、macOS 原生窗口及 Windows/Linux x64 交叉编译均通过。Windows/Linux 原生窗口仍待实机验收。

## 2026-11-24 · M3 Remote 侧栏审查

侧栏按配置顺序显示远端。新增保持标准 Fetch refspec。编辑支持 Fetch URL、可单独配置的 Push URL 和改名，保留附加 URL。单独 Fetch 沿用现有凭证流程，使用选择时的有效 URL 和 refspec，配置变化后需要重新选择。 [Git remote](https://git-scm.com/docs/git-remote)，[Git config](https://git-scm.com/docs/git-config)

复审发现，共享 refspec、负 refspec 和多级符号引用会让直接调用 Git 的改名或移除超出所选远端。改名与移除先在私有配置草稿计算变更，再由 Git 引用事务核对并锁定涉及的引用。共享引用、排除项、本地符号分支和工作树 HEAD 链得到保护，files 引用的跟踪 reflog 随改名保留。目标锁或配置失效时保留已有引用和配置。 [Git update-ref](https://git-scm.com/docs/git-update-ref)

外部配置文件中的远端或 tracking 项仍由其配置来源管理。旧式 `.git/remotes/` 文件已能阻止同名新增，列表和编辑兼容仍待完成。

Go 全量竞态、旧 Rust 对照、`make check`、`go vet`、macOS 原生窗口及 Windows/Linux x64 交叉编译通过。附加故障注入验证了在配置已发布、引用事务中止时恢复原配置和 reflog。Windows/Linux 原生窗口仍待实机验收。

## 2026-11-24 · M3 Worktree 审查

Repository 菜单可以选择本地分支起点，输入新分支名和工作树路径。侧栏区分主 Worktree、当前 Worktree 与其他 linked Worktree。切换在同一笔 Workspace 写入中复用已有标签或登记新标签。独立复审验证了标准引用、SHA-256 reftable、外部相对路径注册和 cone sparse-index。 [Git worktree](https://git-scm.com/docs/git-worktree)，[Git update-ref](https://git-scm.com/docs/git-update-ref)

创建先在同文件系统的私有目录检出，再发布目录和 Git 引用。真实 Git 故障注入覆盖引用提交失败后恢复既有空目录的身份与内容。macOS 回归覆盖原目录权限、访问 ACL，以及外部进程在创建期间修改 ACL。Windows 使用原目录的 DACL，待实机复核。

独立复审复现了 Git 状态漏报三种本地数据的情况，包括保留 mtime 的跟踪文件改动、未初始化子模块内的文件、以及父库跟踪部分文件的独立嵌套仓库。移除用无缓存 stat 的私有索引核验文件，并检查子模块路径与独立 Git 元数据。目录核验后先移动到同级临时路径，防止清理重用旧路径的新目录。锁定或当前 Worktree、没有保存到其他引用的 detached 提交均受保护。确认提示说明干净目录移除和失效记录清理的不同影响。

本轮 Go 全量竞态、旧 Rust 对照及 `make check` 通过。最终改动通过 Worktree 定向竞态、macOS 原生窗口、`go vet` 和 Windows/Linux x64 交叉编译。macOS 窗口覆盖起点选择、取消创建、切换既有标签、拒绝移除脏目录、取消移除及移除后的缺失标签。Windows/Linux 原生窗口仍待实机验收。

## 2026-11-24 · M3 Reflog 恢复与硬重置审查

恢复区按 HEAD 或当前分支读取原始操作前后 OID、操作类型、消息与时间。Reset 和 Amend 的恢复分支指向操作前提交。Git 仓库回归覆盖删除中间 Reflog 记录后仍可定位原提交、无消息的 files 记录、写入中的残行、SHA-1/SHA-256 reftable、多个索引层、仅含日志的表及 linked worktree 隔离。 [Git reflog](https://git-scm.com/docs/git-reflog)，[Git reftable](https://git-scm.com/docs/reftable)。

硬重置沿用需要检出本地分支的限制。提交前核对确认时的 HEAD、索引与跟踪文件，保护目标处的未跟踪和忽略文件。复审复现了 Reset 到当前 HEAD 时，仅比较 HEAD 和 ORIG_HEAD 可能将实际中止误判为成功。修正后不确定的回执会回退工作树，保留原暂存与未暂存改动。macOS 场景覆盖创建恢复分支、取消和确认 Reset，以及从 History 提交右键发起 Reset。Reflog AI 解释仍待 M4 接入。

Go 全量竞态、旧 Rust 对照、`go vet`、`make check`、macOS 原生回归及 Windows/Linux x64 交叉编译均通过。Windows/Linux 原生窗口仍待实机验收。

## 2026-11-24 · M3 Stash Pop 与 Drop 审查

侧栏与预览均可执行 Pop 和 Drop。Drop 有文件摘要与取消入口，确认绑定仓库、所选 reflog 条目和存储文件身份。外部 Git 重排后，旧确认失效。即使外部 Git 用相同 OID、消息、作者和时间重新写入条目，也需要重新确认。普通 refs 的删除持有 ref 和 reflog 锁，清理最后一条时同步处理 packed refs。reftable 在私有副本由 Git 重写，再持有 manifest 锁发布。两种格式均保留共享仓库权限。 [Git files 引用实现](https://github.com/git/git/blob/v2.54.0/refs/files-backend.c)，[Git reftable](https://git-scm.com/docs/reftable)。

真实 Git 仓库覆盖删除旧条目、最新一条、最后一条、packed 与 loose 重叠、SHA-256 reftable linked worktree、被外部进程锁定的引用、损坏 reflog 和同名分支。Pop 遇到未合并文件时保持条目，成功应用但无法删除时明确报告已应用。复审验证了使用严格 umask 时仍保留共享读写权限。

独立复审在发布 loose tip、packed refs 和 reftable manifest 三处注入失败，保留原 stash 与其他引用，`git fsck` 通过。Go 全量竞态、旧 Rust 对照、`go vet`、`make check`、macOS 原生 Stash 回归及 Windows/Linux x64 交叉编译均通过。Windows/Linux 原生窗口仍待实机验收。

## 2026-11-24 · M3 Stash Apply 审查

Apply 按旧版 libgit2 的干净索引要求执行，新增跟踪文件的工作树版本会暂存，已跟踪文件的改动会保留在未暂存侧。存在独立未暂存文件时先确认。正常和冲突结果执行同一套文件发布检查，冲突后的 stash 仍可预览。若同一次应用也遇到未跟踪文件重名，反馈会指出对应的 Git 错误。 [Git stash](https://git-scm.com/docs/git-stash)。

真实 Git 用例覆盖独立的未暂存改动、忽略文件和子模块目录保护、文件与目录互换、符号链接、稀疏检出、外部重排 reflog 及冲突索引。独立复审补充了应用前的 reflog 竞争用例，拒绝失效请求时真实工作树不产生新目录。直接调用旧版锁定的 libgit2 1.9.7 验证了干净索引约束及文件与目录互换后的暂存结果，并复现 Pop 有冲突仍删除 stash。Apply 当前会复制物化的跟踪文件及可见的未跟踪文件到临时目录，大工作树需要对应的临时磁盘空间。

Go 全量竞态、旧 Rust 对照、`go vet`、`make check`、macOS 原生窗口及 Windows/Linux x64 交叉编译通过。macOS 用例覆盖从侧栏打开预览、在预览内执行 Apply、脏工作树确认与取消、冲突提示及切到 Changes 后的冲突编辑入口。Windows/Linux 原生交互仍待实机验收。

## 2026-11-24 · M3 Stash 保存与预览审查

Git 仓库测试覆盖分别保存同一个文件的暂存和未暂存内容、选择是否包含未跟踪文件、保留已忽略文件、保护同名未选中的替换文件、拒绝过期 HEAD 和外部 stash 锁竞争，以及清理部分失败时保护可恢复快照。Git CLI 可以用 `stash apply --index` 恢复 Go 写出的快照。原生交互式 Rebase 的 edit 暂停可以保存改动，随后继续原序列。 [Git stash](https://git-scm.com/docs/git-stash)。

独立复审覆盖 reftable linked worktree、隐藏 index 改动，以及 Git 完成引用提交但确认回执丢失的情况。macOS 原生窗口验证空态、取消、保存、未跟踪文件预览和长列表滚动位置。Go 全量竞态、旧 Rust 对照、`go vet`、`make check` 和 Windows/Linux x64 交叉编译均通过。Windows/Linux 原生窗口仍待实机验收。Apply、Pop 和 Drop 留在下一批。

## 2026-11-24 · M3 Cherry-pick 与 Revert 审查

两个入口使用同一确认窗口。Go 按确认时的分支与 HEAD 校验目标，先在私有工作树运算，再通过共用 checkout 发布。Cherry-pick 保留原作者和来源 SHA，并拒绝空应用。Revert 沿用旧版对已撤销内容的空提交语义。新增根提交和分离 HEAD 的回归。浅克隆记录中已有父提交但缺少有效父历史时，保护原树。 [Git cherry-pick](https://git-scm.com/docs/git-cherry-pick)，[Git revert](https://git-scm.com/docs/git-revert)。

独立复审复现了 Git 在将已初始化 gitlink 或其祖先替换成普通文件时清除子模块目录的情况，包括 Git 先写文件后报错的路径。共用 checkout 在清理与切树前拒绝移除非空 gitlink 目录，失败回退也执行相同检查。独立用例确认目录内已跟踪修改、未跟踪文件与 HEAD 均得到保留，并覆盖 Gitlink 仅更新引用、配置了 `ignore=all` 的子模块及逆向回退期间出现的新文件。

Go 全量竞态与旧 Rust 对照、`go vet`、`make check`、macOS 原生回归和 Windows/Linux x64 交叉编译通过。原生用例覆盖 History、更多操作、取消、冲突反馈和提交后的工作树。两端原生交互仍待实机验收。

## 2026-11-24 · M3 交互式 Rebase 审查

原生分支菜单接通六种 todo 操作、拖动和键盘排序、提交信息编辑、edit 暂停、Amend、Continue 与放弃暂停。Git 在私有工作树重放提交，保留原作者。edit 暂停记录与分支进度一起发布到 Git 引用，暂停引用属于当前 worktree。独立验证覆盖 files 和 reftable 的 linked worktree、重启后继续、连续 edit 和 fixup、以及暂存修改并入后的晚期失败恢复。 [Git cherry-pick](https://git-scm.com/docs/git-cherry-pick)，[Git worktree](https://git-scm.com/docs/git-worktree#_refs)。

复审复现的三项数据保护问题均已修复。Continue 拒绝处理被 `assume-unchanged` 或 `skip-worktree` 隐藏的编辑，保留源文件。分支改名和删除会核对各工作树的暂停归属。旧记录用当前分支和 HEAD 的 reflog 核对旧 reset、原提交和当前打开的分支，切回原分支后可继续。独立复审使用实际旧版 libgit2 命令生成记录，覆盖新旧记录并存的重启恢复、切错分支、切回原分支和最早无 `original_head` 的脏工作树清理。可信 reflog 缺失或归属不明时保留旧记录和工作树。

Go 全量竞态与旧 Rust 对照、`go vet`、`make check`、macOS 原生回归及 Windows/Linux x64 交叉编译通过。macOS 用例覆盖拖动、连续方向键、提交信息输入、edit 暂停和 Continue。Windows/Linux 原生交互待实机验收。

## 2026-11-24 · M3 普通 Rebase 审查

原生本地分支菜单沿用旧版单次选择即开始的交互。普通 Rebase 有冲突时报告路径，保留当前分支、索引与工作树。变基成功后先更新分支快照，再刷新 History，避免紧接着从菜单发起操作时使用过期 HEAD。成功的分支推进在引用事务中校验两端提交并记录 `ORIG_HEAD`。 [Git rebase](https://git-scm.com/docs/git-rebase)，[Git update-ref](https://git-scm.com/docs/git-update-ref)。

复审复现了两种线性重放丢失内容的情况。上游曾应用并 revert 同一补丁时，现重新尝试应用本地提交，只丢弃实际应用后为空的补丁。检查本地独有的双亲 merge commit 是否含手工新增或冲突解决内容，存在这类内容时拒绝线性变基。多亲 merge 保留在原分支。Pull 和普通 Rebase 共用这两项保护。 [Git merge-tree](https://git-scm.com/docs/git-merge-tree)。

真实仓库用例覆盖无操作、快进、分叉、冲突、等价补丁、过期引用、受保护的忽略文件、被索引标记隐藏的改动、晚期发布失败和同名 `ORIG_HEAD` 分支。macOS 原生回归覆盖分支菜单、状态反馈及操作后的 History。

Go 全量竞态与旧 Rust 对照、`go vet`、`make check`、macOS 原生 Rebase/Merge/Pull 回归和 Windows/Linux x64 交叉编译均通过。两端原生窗口仍待实机验收。

## 2026-11-24 · M3 中止合并审查

Merge 横幅及冲突编辑器都有「中止合并」入口。两个入口共用确认，关闭确认会保留冲突编辑器中的草稿。原生合并保存 `AUTO_MERGE`，完成合并时在同一引用事务内清除，覆盖 loose refs 和 reftable。 [Git merge](https://git-scm.com/docs/git-merge)，[Git update-ref](https://git-scm.com/docs/git-update-ref)。

中止只恢复合并影响的路径。执行前重新检查 HEAD、暂存内容、冲突阶段、合并文件和稀疏规则。复审发现自动合入的文件上还可能包含用户额外暂存的内容，工作树改动也可能被 `assume-unchanged` 或 `skip-worktree` 隐藏。这些情况现会拒绝中止并保留文件。发布过程中失败会尝试恢复已移动的文件，遇到并发改动时保留可定位的恢复副本。带 autostash 的外部 Merge，以及缺少结果树且存在归属不明暂存内容的外部 Merge，仍待迁移。 [Git ls-files](https://git-scm.com/docs/git-ls-files)。

真实 Git fixture 覆盖独立暂存、未暂存、未跟踪和忽略文件、文件与目录互换、确认失效、引用类型错误和稀疏检出。独立复审覆盖 loose refs 和 reftable 的主工作树与 linked worktree，并复验两类隐藏改动和上次合并留下的 resolve-undo 信息。macOS 原生回归覆盖两个入口、取消后继续编辑和确认后的仓库状态。

Go 全量竞态、Merge 和 Commit 定向竞态测试、`go vet`、旧版 `make check` 与 `make native-parity-test` 已通过。Windows/Linux x64 交叉编译通过，两端原生交互仍待实机验收。

## 2026-11-24 · M3 冲突编辑器审查

Merge 横幅和 Changes 接入原生冲突面板。面板按文件保留草稿，展示我方、基准、对方与工作文件，支持跳转冲突块、选边、编辑和显式暂存。默认标记、自定义宽度和属性变更前留下的成组标记均能识别。含冲突标记的手工文本需要二次确认。选择缺失一方暂存删除，二进制选边保留对应 Git 对象。 [Git update-index](https://git-scm.com/docs/git-update-index)，[Git hash-object](https://git-scm.com/docs/git-hash-object)。

保存复核 HEAD、仓库操作、index 冲突阶段和工作文件指纹。发布途中的失败恢复原文件，外部进程经旧文件描述符写入的内容保留到可定位的恢复副本。外部 CLI 暂存某个冲突后，其未保存草稿仍可回选、复制，编辑器禁用该文件的重复暂存。独立复审覆盖稀疏检出相关回归、同大小同 mtime 的外部编辑、发布失败、历史标记宽度和非当前文件草稿。冲突 AI 解释和原生中止入口仍待迁移。

macOS 原生用例覆盖冲突块导航、选边、标记确认、关闭时的草稿确认、外部 Git 暂存后的跨文件草稿恢复以及后续合并提交。Go 全量竞态测试、`go vet`、旧版对照、`make check` 和 Windows/Linux x64 交叉编译均通过。Windows/Linux 原生交互仍待实机验收。

## 2026-11-24 · M3 Merge 审查

侧栏本地分支和 History 引用菜单使用同一确认窗口。Git 在私有配置下预检合并树。执行时核对源与目标，利用现有工作树检出和引用事务发布结果。快进保留源 commit，三方与不快进模式创建双父 commit。`ORIG_HEAD` 精确保存合并前的目标 commit，同名本地分支保持原位。 [Git merge](https://git-scm.com/docs/git-merge)，[Git merge-tree](https://git-scm.com/docs/git-merge-tree)。

冲突分支保留 HEAD，发布 `MERGE_HEAD` 和实际 index stages。复审发现稀疏检出会省略配置外的冲突文件，现仅物化这些冲突文件并保护已有路径，继续排除其他文件。合并状态发布失败时恢复此前的工作文件及索引。Changes 可暂存外部编辑后的结果或明确暂存删除，并显式提交合并。旧版独立冲突面板、AI 说明和原生中止入口仍待迁移。

独立复审验证了稀疏冲突暂存后立即执行 `git merge --abort`、多个冲突中只暂存选中项、状态发布失败恢复及 detached HEAD 与同名分支隔离。macOS 原生用例验证侧栏与 History 入口、取消确认、不快进和冲突解决后提交。Go 全量竞态测试、`go vet`、`make native-parity-test`、旧版 `make check` 及 Windows/Linux x64 交叉编译通过。Windows/Linux 原生交互待实机验收。

## 2026-11-24 · M2 Push 审查

工具栏推送当前分支，可选远端、分支 tip 上的标签和强推。分支菜单可推送选中的本地分支。强推二次确认展示每个目的地及待变更引用的旧、新 OID，执行时用对应租约拒绝确认后的远端更新。确认同时绑定本地分支、标签和有效推送地址。 [Git push](https://git-scm.com/docs/git-push)，[Git ls-remote](https://git-scm.com/docs/git-ls-remote)。

普通 Push 跳过已被不同对象占用的远端标签。多地址推送保留已确认结果，认证重试从发出挑战的地址继续。独立复审验证 HTTP 重定向会被拒绝、含换行的单个 `pushurl` 不会被拆成多个目的地、多个远端源共用的本地跟踪引用不会被覆盖。SSH 实际远端和 Windows/Linux 原生交互待实机验收。

已修复批量标签推送在远端成功后被误报为失败的问题。逐行读取 Git 状态，按命令参数长度分批，强推只对完整名称匹配的引用建立租约。700 个标签的普通推送、强推预览及强推通过。独立复审验证中间批次租约冲突后，重试保留已成功的标签并继续剩余批次。 [Push 传输实现](../../../internal/gitcli/push_transport.go)。

## 2026-11-24 · Workspace 远端刷新审查

快捷键刷新绑定发起时的 Workspace、仓库和路径。仓库失效并自动切到备用仓库时，原请求停止远端获取。定时刷新默认关闭，在设置中启用后把开关和分钟数写入 SQLite。修改间隔会废弃旧计时排队的请求。定时任务跳过缺失仓库，按成功和失败仓库数给出状态。同一 Workspace 内切换仓库时继续定时任务，切到其他 Workspace 时取消。HTTPS 认证失败计入部分失败，可用显式 Fetch 处理认证。旧 WebView 的自动刷新偏好仍待导入。 [旧版刷新流程](../../../src/hooks/useAutoRefresh.ts)。

独立复审使用临时仓库和回环 HTTP 验证备用仓库、过期定时请求与切换标签页。macOS 窗口回归覆盖快捷键、设置开关、间隔变更和部分失败提示，并检查获取远端更新后本地 HEAD 保持原位。Go 全量竞态测试、`go vet`、旧版对照与 `make check` 门禁均通过。Windows/Linux x64 已交叉编译，原生交互仍待实机验收。

## 2026-11-24 · M2 Pull 审查

工具栏和分支菜单接入远端、远端分支、当前分支、Rebase 与贮藏选项。默认上游读取 Git 的实际远端分支。自定义 refspec 的候选项按远端源分支显示。候选来自本地跟踪引用。当多条通配规则共用跟踪引用时，列表可能包含未观测过的源分支，实际 Pull 会精确获取所选源并拒绝缺失分支。发布同时锁住 HEAD、当前分支及所选跟踪引用，核对实际提交与获取结果。 [Git fetch](https://git-scm.com/docs/git-fetch)，[Git update-ref](https://git-scm.com/docs/git-update-ref)。

领先或已同步时保留本地状态。快进将选定提交写入当前分支，分叉且未选 Rebase 时停止。选中 Rebase 后在私有工作树重放，遇到冲突显示路径，实际索引、工作树与分支保持原状。私有重放成功后沿用检出事务发布。贮藏确认绑定实际工作树与暂存内容，同一确认随 HTTPS 认证重试保留。重放冲突时保留目标分支和可恢复的 stash。 [Git rebase](https://git-scm.com/docs/git-rebase)，[Git stash](https://git-scm.com/docs/git-stash)。

临时仓库测试覆盖 unborn 分支、快进、分叉拒绝、重放成功与冲突、未合并索引、贮藏恢复与贮藏冲突、认证重试后的新编辑以及自定义 refspec。macOS 原生用例覆盖取消、实际远端源分支选择、工作树改动恢复和回环 HTTPS 认证。独立复审、Go 全量竞态测试、`go vet`、`make native-parity-test`、旧版 `make check` 及 Windows/Linux x64 交叉编译均通过。

## 2026-11-24 · M2 Fetch 审查

工具栏和仓库菜单使用同一 Fetch 入口。同步状态随当前仓库显示，支持取消。完成一次或多次获取后刷新 History 和远端引用。普通远端错误会保留已完成的结果并继续批次，HTTPS 认证或取消会中止。认证重试绑定远端及其有效 URL，远端发生变化时需要重新认证。使用临时唯一的 URL 映射避免对已解析地址再次应用 `insteadOf`。 [Git fetch](https://git-scm.com/docs/git-fetch)，[Git credential](https://git-scm.com/docs/git-credential)。

先在隔离 Git 目录获取数据，内容寻址的对象写入仓库对象库。真实远端跟踪引用通过 Git 事务核对旧值并原子更新，标签只新增。发布前及事务准备阶段检查引用路径，避免符号引用和目录别名将修改导向本地分支。事务也检查各 worktree 的 HEAD。`FETCH_HEAD` 在确认内容未被外部进程修改后更新。Fetch 不写索引或工作树。常规浅克隆保留已知边界，获取结果要求改变边界时会拒绝发布。 [Git update-ref](https://git-scm.com/docs/git-update-ref)。

临时仓库回归覆盖正常更新与清理失效的跟踪引用、普通失败后继续、认证绑定、下载取消、本地标签保护、已附着的 linked worktree、悬空符号引用及 URL 改写。macOS 原生回归验证历史刷新、状态栏取消和回环 HTTPS 认证。Windows/Linux 交叉编译通过，SSH 远端与仓库专属网络配置仍需验证。

## 2026-11-24 · M2 克隆审查

仓库菜单接通 URL、目标目录、进度、取消和重试。下载和工作树验证都在目标同卷私有目录执行。旧版在失败或认证重试时可能递归删除目标。新流程在发布前复核目标目录身份、目录变化及各 Workspace 登记路径。如果替换会移动本地来源，或目标文件系统无法完整表示源文件名，则拒绝发布。原目录通过无覆盖重命名保留为备份。Git 层成功而 Workspace 写入失败时，重试可直接登记已克隆的仓库。 [Git clone](https://git-scm.com/docs/git-clone)。

HTTPS 认证失败时可以在表单输入用户名与访问令牌。凭证通过限定原始 HTTPS 主机和仓库路径的 Git helper 请求传递，令牌不写入 URL、参数、环境或仓库配置。勾选记住后由 Git helper 保存，并以读取结果验证。保留 Git 对用户已配置 clean/smudge filter 的行为，发布前用同一配置验证检出。Changes 层的外部 filter 与旧应用凭证库的兼容仍待补齐。 [Git credential](https://git-scm.com/docs/git-credential)，[Git attributes](https://git-scm.com/docs/gitattributes)。

临时仓库用例覆盖正常发布、非空目录确认、下载中目标变化、取消清理、大小写别名拒绝、本地来源冲突、可逆过滤器、回环 HTTPS 和 Git helper 保存后自动复用。macOS 原生用例覆盖替换确认、取消认证、令牌重试、原目录备份、激活仓库和拒绝重复登记。Go 全量竞态、定向克隆回归、`go vet`、`make native-parity-test`、旧版 `make check` 和 Windows/Linux x64 交叉编译通过。Windows Job Object 和 SSH 远端仍需实机验证。 [Windows Job Objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects)。

## 2026-11-24 · M2 仓库初始化审查

仓库菜单沿用初始化入口和带目录选择器的位置输入。确认后调用 Git 创建 unborn `main`，登记及激活在同一个 Workspace 事务。初始化复用有效工作树，保护普通库和裸库的 Git 元数据目录及大小写别名。普通子目录可独立初始化。保留路径中的空格，重复登记留在表单显示错误。新库使用 SHA-1 和文件型 refs，旧 Git 使用自身的文件型 refs 默认值。 [Git init](https://git-scm.com/docs/git-init)。

模型与 macOS 原生回归覆盖取消、目录名包含空格、Git 已成功而 SQLite 写入失败时的原路径重试和重复登记。Git fixture 验证保护已有文件和 HEAD、允许嵌套初始化，并阻止写入裸库及现有 Git 元数据。独立复审、Go 全量竞态测试、`go vet`、`make native-parity-test` 和 Windows/Linux x64 交叉编译均通过。Windows/Linux 原生交互待实机验收。

## 2026-11-24 · M2 脏工作树检出审查

检出确认绑定源 HEAD、索引项、工作目录身份和受影响文件的内容，确认后再次修改会使请求失效。取消保留工作树。丢弃和贮藏两条路径都会先保存可恢复的 Git 对象。删除未跟踪的 `.gitignore` 后，原先被忽略的文件仍留在磁盘。如果目标会覆盖忽略路径，检出会在清理改动前拒绝。

贮藏重放使用 `git stash apply --index`，保留同一路径不同的暂存与未暂存版本。重放失败时发布目标 HEAD 及 Git 生成的冲突索引，并将恢复条目加入 stash 列表。检出写入后、发布前报错时会尝试逆向切换并恢复源改动。无法安全恢复时保留条目及错误提示。快照和重放遵循 Git 属性的文本规范化，非可逆属性转换仍可能改变原工作树的字节。 [Git stash](https://git-scm.com/docs/git-stash)，[Git attributes](https://git-scm.com/docs/gitattributes)。

通过 macOS 原生检出回归、Go 全量竞态测试、`go vet`、`make native-parity-test` 和旧版 `make check`。临时仓库覆盖过期确认、忽略路径冲突、忽略规则移除后的未跟踪文件、带不同暂存版本的重放、冲突时的 stash，以及晚期报错后对两种模式的源状态恢复。Windows/Linux x64 已交叉编译，实际窗口与平台文件行为待验收。

## 2026-11-24 · M2 干净工作树检出审查

原生工作台接通分支双击与菜单、History 提交菜单及 Tags 菜单。Git 写入前核验源 HEAD、目标引用、工作树状态、关联 worktree 和暂停操作。当前本地分支重复检出为无操作。远程引用先按最长 remote 名确定本地分支名，复用现有分支或创建本地分支并设置 upstream。如果 Git 因重叠 fetch refspec 拒绝设置 upstream，界面显示分支已切换并报告配置错误。

隔离配置下的 Git 检出保留忽略文件。两侧受影响的文件先在临时私有工作树物化，拒绝缺失对象和目标路径别名，再进入真实工作树写入。实际检出检查 Git 诊断和受影响文件的状态，随后发布 HEAD 和索引。开始写入后完成发布或回滚，避免取消留下半次操作。异常恢复已验证能撤销检测到的晚期 Git 失败。底层 I/O 只写入部分文件、外部对象在预验后被删除或索引文件安装失败时，自动恢复仍可能受限，错误会报告现场状态或索引副本路径。 [Git checkout](https://git-scm.com/docs/git-checkout)，[Git update-ref](https://git-scm.com/docs/git-update-ref)。

测试覆盖本地与远程分支、附注标签与 detached HEAD、unborn HEAD、忽略文件冲突、外部 filter、过期目标、缺失 blob、大小写别名和取消。三端 Go CI 运行镜像当前均提供 Git 2.46 以上版本，流程会验证实际使用的 Git。 [Ubuntu 24.04 镜像](https://github.com/actions/runner-images/blob/main/images/ubuntu/Ubuntu2404-Readme.md)，[macOS 26 镜像](https://github.com/actions/runner-images/blob/main/images/macos/macos-26-arm64-Readme.md)，[Windows 2025 镜像](https://github.com/actions/runner-images/blob/main/images/windows/Windows2025-Readme.md)。

普通并发 Git 操作受现有 worktree 的索引锁约束。其他进程若已越过索引锁而即将写 HEAD，或直接更新符号引用、新建 worktree，仍需后续覆盖。

## 2026-11-24 · M2 本地分支与标签管理审查

本地分支支持创建、重命名、确认删除，以及设置和清除 tracking。重命名使用 Git 更新 HEAD 和分支配置，配置锁已存在时会提前拒绝。Tracking 保留完整 ref，当前本地 upstream 与同名远程分支可以区分。标签在提交详情、History、分支菜单和侧栏可管理，支持创建轻量或附注标签、确认替换和确认删除。

删除分支、替换及删除标签都在 Git 引用事务中复核原 OID。准备后再次检查符号引用。删除分支会检查工作树占用、磁盘路径别名、暂停的 rebase 或 bisect，并持有现有 worktree 的索引锁。临时仓库复现并验证了普通 `git switch` 在索引锁已持有时会被拦截。外部 checkout 若已释放索引锁且正在写 HEAD、直接修改 HEAD，或并发新建 worktree，仍存在需要在 checkout 阶段处理的窗口。 [Git branch](https://git-scm.com/docs/git-branch)，[Git update-ref](https://git-scm.com/docs/git-update-ref)，[Git checkout 实现](https://github.com/git/git/blob/v2.54.0/builtin/checkout.c)。

独立审查复核了引用竞态、tracking 歧义和已替换 checkout 的 macOS 菜单。macOS 原生回归覆盖分支创建、tracking、重命名、删除取消和确认，及标签注释、替换取消和确认、侧栏删除。Windows/Linux 桌面交互仍待实机验证。安全 checkout、标签 checkout、远程分支操作及 History 独立引用徽章菜单仍待迁移。

## 2026-11-24 · M2 Discard 审查

Changes 未暂存文件支持右键和键盘菜单、丢弃选中项及丢弃全部。确认对话框分别说明恢复索引版本和删除未跟踪文件。确认绑定发起时的 checkout、索引项、目录身份、文件身份和内容。检测到过期内容时刷新状态并保留用户修改。

跟踪文件经私有 Git 索引和冻结属性生成临时副本。Git 为每个索引条目单独导出内容，同名别名无法互相覆盖。检测到实际工作文件与索引路径为同一文件时，拒绝把它作为未跟踪文件删除。发布及恢复使用各平台的无覆盖重命名，出现并发命名冲突时保留原件或报告备份路径。Changes 错误栏可复制完整错误。 [Git checkout-index](https://git-scm.com/docs/git-checkout-index)，[Git 属性优先级](https://git-scm.com/docs/gitattributes)，[Windows FILE_RENAME_INFORMATION](https://learn.microsoft.com/windows-hardware/drivers/ddi/ntifs/ns-ntifs-_file_rename_information)。

独立审查中的硬链接依赖、路径别名和菜单作用域问题已修复。属性副本以独占方式写入，路径别名出现时返回错误。Go 和旧 Rust 对照验证索引、工作树与未选中文件，macOS 原生回归验证取消、过期确认和三种菜单入口。Windows/Linux amd64 已交叉编译。Windows 启用 `core.symlinks` 后，指向目录的相对符号链接仍需实机验收。涉及外部过滤器和文件与目录互换的情况暂不执行丢弃。

## 2026-11-24 · M2 Commit 与 Amend 审查

Commit 使用实际索引锁和私有索引。发布前对照用户看到的 HEAD、分支、已暂存快照和 Git 操作状态。引用事务通过 HEAD 解引用锁定本次 checkout，复核符号目标与提交，再发布新引用和 reflog。工作树及原索引内容保留。显式提交沿用旧版行为，使用 Git 配置的身份或 GitWave 回退身份，跳过仓库 hooks 和签名命令。 [Git commit-tree](https://git-scm.com/docs/git-commit-tree)，[Git update-ref](https://git-scm.com/docs/git-update-ref)。

Amend 保留原作者身份、时间和父提交。确认绑定分支、HEAD 及 upstream，外部切到同一提交上的另一分支会要求重新确认。进入 Amend 后仍可恢复原草稿。提交成功留在 Changes，可跳转 History 查看新提交。浅克隆边界缺失父对象时返回错误，原提交保留。

独立审查无本轮阻断。Go 和旧 Rust 对照提交的树、父提交、身份、消息、索引与工作树状态；补充用例直接检查 unborn HEAD、detached HEAD、无差异 merge、引用锁并发以及失效预览。macOS 原生回归验证类型切换、草稿恢复、History 跳转、失效确认与多行消息键盘提交。Windows/Linux amd64 已交叉编译，桌面交互仍待实机验证。

## 2026-11-24 · M2 Stage 与 Unstage 审查

独立审查覆盖单文件、批量选择、全部操作和原生键盘路径，复核索引锁、绑定的 checkout、过期索引、重命名及 file/directory 互换。发布索引前会检查 HEAD，所选项还须匹配文件模式及索引对象 ID。工作区外部改变已选索引项时，UI 显示错误并刷新当前状态。文件层级冲突若会覆盖未选择的索引项，整批操作返回错误。

Stage 从绑定的工作树打开文件，逐级确认父目录身份，在私有工作树运行 Git。属性快照包含系统、用户、仓库和祖先目录的规则。需要外部过滤器的文件明确拒绝暂存，识别 Git 属性状态同名的过滤器。预览版 Stage 需要 Git 2.42 或更高版本，这两项属性路径变量从该版本起提供。Unstage 使用 HEAD 树中的确切路径更新索引，支持 unborn HEAD 和重命名的两端。 [Git 2.42 属性路径变量](https://github.com/git/git/blob/v2.42.0/builtin/var.c)，[Git add](https://git-scm.com/docs/git-add)，[Git update-index](https://git-scm.com/docs/git-update-index)。

索引写入由同 common dir 的应用操作串行，并持有 Git 的 `index.lock`。写入暂存 blob 时 Git 可能先向本地对象库写入对象。取消或失败时不会发布部分索引，未引用对象由 Git 清理。临时工作树可能导致索引 stat 缓存暂时与原工作树不同，大文件的重复刷新和 split index 成本仍待实测。

Rust 索引对照、Go race、`go vet`、`make check`、macOS 原生 Stage/Unstage 以及 Windows/Linux amd64 交叉编译通过。macOS 用例覆盖状态图标、Shift 范围选择、方向键、全选、批量与全部操作。

## 2026-11-24 · M2 Changes 读取与预览审查

独立审查覆盖暂存两侧、未跟踪文件、重命名、轮询、切换仓库和文件访问边界。状态和两侧 diff 使用私有 Git 目录，配置只传入内置的换行、文件模式、属性和忽略规则相关设置。Git 仍读取原 index 和对象，禁用可选 index 写入。暂存侧冻结 HEAD，并使用空私有工作树。常规未暂存侧通过有界读取复制目标文件和祖先属性文件。生成补丁时核对同一份输出中的 raw delta 与预检结果。 [Git status 手册](https://git-scm.com/docs/git-status)，[Git diff 手册](https://git-scm.com/docs/git-diff)，[Git attributes 手册](https://git-scm.com/docs/gitattributes)。

Go fixture 验证部分暂存、按路径精确筛选、split index、创建副本后 Git 配置或文件被改变、普通暂存 rename、intent-to-add rename、图片版本变化及超限。旧 Rust 对照覆盖同一文件的两侧、删除和未跟踪文本，含 CRLF、空行和无末尾换行。切换仓库时取消旧读取，同路径重绑也取消旧状态进程。刷新保留列表节点，文件仍存在时跟随其所在的另一侧。

这一阶段普通未暂存移动仍由 Git porcelain 报告为删除加新增，子模块也只报告 gitlink 提交变化。后续加入了 [未暂存移动配对](#2026-11-25--macos-changes-未暂存移动配对)和 [子模块工作树状态](#2026-11-25--子模块工作树状态和退出确认)。split index 的共享索引仍需复制至私有目录，大型共享索引的轮询开销待测量。

Go race、`go vet`、`make native-parity-test`、`make check`、macOS 原生 Changes 交互和 Windows/Linux amd64 交叉编译通过。原生回归验证两侧预览、History 返回以及外部 unstage 后跟随到未暂存侧。

## 2026-11-24 · M1 History 排序与游标复审

旧 libgit2 的拓扑遍历按提交者时间取出当前可显示的提交，同秒时由其二叉堆和引用入队顺序决定先后。Go 通过 `git rev-list` 流式读取可达图，按同一规则排序。引用按 loose refs、packed refs、非本地分支 HEAD 的顺序加入。文件型 loose refs 的大小写比较仅折叠 ASCII。 [libgit2 1.9.7 revwalk](https://github.com/libgit2/libgit2/blob/v1.9.7/src/libgit2/revwalk.c)，[引用遍历](https://github.com/libgit2/libgit2/blob/v1.9.7/src/libgit2/refdb_fs.c)，[Git rev-list](https://git-scm.com/docs/git-rev-list)。

对照 fixture 包含跨两页的同秒提交、三父 merge、大小写和 Unicode 分支名、混合 loose/packed refs、仅由标签引用的提交和 detached HEAD。两页逐行比较 SHA、父提交、引用装饰和泳道。已有游标在 `git pack-refs --all` 后继续使用原顺序，逻辑引用变更仍会重建快照。修复 HEAD 符号引用指向标签或远端引用指向附注标签时的遗漏，各有真实仓库和 Rust 对照覆盖。

快照保留前 100,000 个提交。为计算正确的拓扑顺序，排序前会读取全部可达提交的边，内存随图大小增长。Apple Silicon 上，10k 打包提交样本的新开后端、首屏数据及泳道计算中位数约 57 ms。200k 打包样本同项约 220 ms，验证了首屏及 100k 截断标记。这些数字来自本地合成样本和后端计时，三端窗口首次可交互耗时仍待验收。

Go race、`go vet`、`make native-parity-test`、`make check`、macOS 原生 History 交互及 Windows/Linux amd64 交叉编译通过。Windows/Linux 原生窗口行为仍待实机验收。

## 2026-11-24 · M1 Blame 审查

独立审查覆盖 Git 参数、输出解析、内存上限、后续选择覆盖、取消和滚动。历史 diff 的 blame 读取所选提交，删除 diff 使用与 diff 相同的首父提交。浅克隆 fixture 验证首父对象仍在但 History 省略父 SHA 的情况。用户手动选择其他提交时，清除尚未完成的跳转。

Go 与旧 Rust 对照逐行文本、SHA、原始作者、邮箱与提交者时间，样本覆盖重命名、CRLF、空行和无末尾换行。Go fixture 另验证未保存内容、Unicode 路径、mailmap、外部 textconv 和缺失的 `blame.ignoreRevsFile`。增量输出按结果行重排，解析时验证区间覆盖。 [Git blame 手册](https://git-scm.com/docs/git-blame#_incremental_output)，[Git 参数实现](https://github.com/git/git/blob/v2.54.0/builtin/blame.c)。

文件上限为 8 MiB 和 25 万行，增量输出与提交元数据各有 32 MiB 总预算。超限显示独立提示。滚动列表按视口虚拟化，宽视图横向滚动时固定归属栏与行号。切换文件、提交或仓库时取消旧请求。Git 子进程尚在运行时的切换仍需专门的取消回归。

macOS 原生窗口通过历史 diff、返回、切文件、删除文件及点击归属跳转回归，中日韩长行通过横纵滚动测试。固定归属栏的位移由 NSGo 模型测试验证。Go race、`go vet`、`make native-parity-test`、`make check` 和 Windows/Linux amd64 交叉编译通过。Windows/Linux 实机和三端视觉对照仍待验收。

## 2026-11-24 · M1 前台与手动本地刷新审查

独立审查覆盖窗口生命周期、模态框、切换过程中丢弃过期结果、浅克隆、History 分页和不同窗口的启动选择。审查后按原始路径重新确认仓库身份，处理同路径 linked worktree 重建及旧路径符号链接指向变化。刷新只在 refs 或浅克隆边界变化时重建快照，手动刷新可强制重读。内容准备完成后一次替换，保留搜索、已加载深度、视口锚点及成功加载的 diff，重试仍选中的失败详情或 diff。

Workspace 不会因为另一窗口修改启动选择而随之切换。交互写入按明确目标重新加载，前台探测与旧结果按窗口操作编号检查。成功写入后若加载失败，记录其目标供下次重试。缺失和恢复用真实临时仓库验证，附带检查 SQL 中保存的回退选择及跨窗口重命名。

Go 模型覆盖窗口从失焦回到前台及模态框后的延迟刷新。macOS 原生回归通过手动入口检查同一刷新流程。在本机测试会话中，最小化成功，AppKit 未能在还原后把测试窗口设为键盘焦点窗口，真实前台切换仍待验收。Windows/Linux 的窗口生命周期与文件身份仍需实机对照。当前刷新流程调用本地 Git 读取，远端 fetch 和定时刷新偏好分别留在 M2/M4。

## 2026-11-24 · M1 Workspace 导入导出审查

独立审查覆盖旧 v1 JSON、导入事务、导出字段、已有目标的确认、跨进程串行及文件写入。旧格式样本覆盖 Unicode、Windows 路径和换行路径。导入 fixture 验证重复与缺失条目跳过、裸仓库恢复、首个可用仓库激活，以及模拟插入失败后的整体回滚。导出仅序列化版本、名称和按顺序排列的仓库路径。

审查后移除了可提前暴露部分 JSON 的写入路径。新文件通过操作系统的独占发布接口写入，已有文件先暂存完整 JSON，再在 SQLite writer lock 内复核覆盖确认。活动数据库和辅助文件禁止作为目标。Windows 发布处理扩展长度及 UNC 路径。取消对话框后清除旧提示，异步错误仅展示在其发起路径仍是当前输入时。

双连接并发 fixture 验证新文件和已确认文件均只有一个发布成功。macOS 原生窗口通过菜单导入、失败后重试、切到导入的裸仓库、取消覆盖与确认覆盖。Go race、`go vet` 和 Windows/Linux 预览交叉编译通过。外部进程不共享数据库锁，最终复核与重命名之间仍可能有外部修改。Windows/Linux 文件发布需实机验证。

## 2026-11-24 · M1 仓库标签审查

独立审查覆盖拖动预览和撤销、滚轮、键盘菜单、SQLite 状态与跨窗口并发。审查后恢复方向键及 Home/End，保留移动中标签的焦点，阻止忙时点击只移动焦点。删除其他标签导致当前标签移出视口时会重新滚动到当前标签。修复了 macOS 临时目录经符号链接访问时，测试将规范化后的重链接路径与原始路径比较的问题。空标签栏提示当前可用的添加操作。

仓库移除仅删除引用，并以事务保存可用仓库回退。重链接保留 repo ID、排序、昵称和设置，校验 Git 根目录，拒绝重复路径与过期引用。重排校验原始顺序和目标排列，避免覆盖已保存的并发修改。定向测试覆盖旧路径、重链接重复失败后重试，以及操作后重新打开 SQLite 的顺序和选择。

macOS 原生交互覆盖拖动、键盘移动、右键重链接、移除、溢出滚动和启动回退。Go race、`go vet`、`make native-parity-test`、`make check` 和 Windows/Linux amd64 预览交叉编译通过。Windows/Linux 实机和逐像素视觉对照仍待完成。

## 2026-11-24 · M1 Workspace 存储和切换审查

独立审查覆盖 SQLite 迁移、事务、旧路径兼容、工作区切换及 NSGo 交互。审查后改为检测当前 Workspace 中的全部仓库，恢复原路径时清除缺失标记，拒绝让已失效的嵌套路径打开父仓库。比较规范化路径以去重，并保留已存的路径和 ID。SQLite 读取使用 WAL 只读快照。写入成功后若刷新失败，关闭已提交的弹窗，重试仅重新加载。选中的仓库标签会滚动到可见范围。

旧 schema 2 fixture 升级至 schema 4，保留 repo 次序、昵称和两种设置 JSON 中的未知字段。模型测试覆盖批量添加时跳过无效路径和重复项、切换 Workspace、重命名、删除确认和取消。macOS 原生测试覆盖从 SQLite 启动、缺失路径回退及 History 交互。Go race、`go vet`、`make native-parity-test`、`make check` 和 Windows/Linux amd64 预览交叉编译通过。

三端实机视觉对照、Workspace 导入导出、仓库移除、重链接、拖动重排、前台探测和正式数据升级仍待验收。测试使用临时数据库，预览使用独立的数据目录。

## 2026-11-24 · M1 提交 diff 和图片审查

独立审查覆盖 Git 路径范围、首父提交、浅克隆边界、blob 大小与补丁限额、取消及原生运行入口。已修复仓库 `diff.orderFile` 指向失效文件时预览报错、Windows GUI 启动 Git 时的控制台标志缺失，以及连续无效 UTF-8 字节的替换行为。Windows 控制台行为已交叉编译，实机仍待验收。

旧 Rust 对照覆盖初始提交、合并、行号、CRLF、无末尾换行、二进制、展开限额和图片字节。旧版在文件变成目录时会返回同名前缀的子项，在文件类型变化时也会给出两个同名项。本次按精确路径输出，类型变化同时显示删旧和增新。浅克隆缺少父树时会报告读取失败，提交详情仍可查看。

## 2026-11-24 · M1 History 首批审查

独立审查覆盖仓库及 refs、分页、提交详情、Go CI 和 Rust diff 过渡。审查后拆开提交详情和 diff 的呈现，Rust diff 失败时保留已读到的 Go 提交详情，并加入 NSGo 回归。只读命令同时阻断 Git transport，过滤 symbolic ref 的走图入口，保留旧版的 direct ref 行为。

Go 和 Rust 在有时间区分的 merge、annotated tag 和 detached HEAD fixture 上对照通过。首次用同秒提交构造复杂多分支 DAG 时发现 Git CLI 与旧引擎的排序差异，后续复审记录了修复和回归。Go CI 通过 actionlint 检查，尚未在托管三端运行。发布工作流留待 M5 切换。

## 2026-11-24 · 全量迁移 M0 首批审查

独立审查覆盖 Go 仓库身份、porcelain v2 解析、Git 进程边界、测试隔离和 NSGo 配置。NUL 分隔支持保留原始路径和重命名源路径，真实 Git fixture 覆盖 linked worktree、暂存与未暂存、merge/rebase 冲突及 abort。

审查发现两处已修复的问题：

- 状态读取曾只依赖工作目录。嵌套仓库的 Git 目录移除后，Git 可能向上发现父仓库。现显式传递选中的 Git dir 与 worktree，回归测试验证目录被移走后读取失败。
- fixture 曾继承发行物的默认 Git 模板。现使用空模板初始化，避免默认模板影响测试文件及 hooks。

当前审查范围是受控 fixture 里的只读适配层。真实用户仓库的 Git 配置和外部程序执行策略、进程树取消、Windows/Linux 实机行为仍属 M0 验收。NSGo 三端预览包已构建，签名安装与更新尚需演练。

## 原生工作台预览审查记录

## 优点

- History、提交详情和有界的 diff 沿用 Rust 实现。NSGo 预览通过独立的构建入口运行。
- 交互测试使用真实的临时 Git 仓库，覆盖选择、切换、分栏和文本及图片 diff。

## 严重问题

两轮审查发现的阻断已修复。目录选择后的子进程归工作台生命周期，过滤后定位提交等列表挂载再滚动，失效游标重载后继续原来的分页请求。标题栏工具组共用扣除系统控件后的宽度预算，覆盖了多个窗口宽度的布局断点。

## 一般问题

- 为提交和文件选择加入独立取消状态，跳过排队的过时请求。已发的响应会读完，以维持进程间协议同步。
- 分栏比例按稳定标识保存，切换 diff 显示模式保留文件折叠状态。macOS 恢复标准编辑菜单角色，Files 按钮直接打开仓库目录。
- Linux 的窗口边缘缩放尚待实机检查。macOS 使用 NSGo/AppKit 的标题栏双击行为，旧 Tauri 的即时缩放仍待对照。

## 总体评价

首个原生预览涵盖自绘标题栏、工作台布局及本地 History/diff。完整 UI 等价和三端实机验收仍按[任务计划](./plan.md)推进。
