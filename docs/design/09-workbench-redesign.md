# GitWave 工作台 · 全界面与交互重设计草案

状态：**第一阶段 accepted，已随 v0.11.0 发布**。日期：2026-10-15。

已实施范围：双工作区导航、主布局/分隔条、History 文件列表与 compact diff、Changes 主区、compact commit type/dialog、repo-scoped 会话和写操作、跨切换失败反馈。保留 Native Blue/Tide 和现有字体偏好，未强制迁移配色。用户明确批准第一阶段后，此范围替代旧 History-only 三纵栏规范。

本轮实测修订：History 不再有重复标题栏和紧凑图切换，固定完整分支图（18px lane）；refs/SHA/作者/时间按固定列对齐；同一 commit 的 tag 优先展示，其次本地分支、远端分支，展开列表同序。单文件文本 diff 填满剩余高度且继续虚拟滚动。Changes 分组标题、计数和操作保持同一行，极窄面板横向滚动。Blame 的归属、行号和代码分列，长作者省略、完整归属保留，代码横向滚动，文件路径只显示一次。受控弹窗关闭立即卸载，避免退出动画被取消后残留遮罩。列表高亮统一为分支样式（直角、整行浅色底、左侧3px竖线）；文件多选底色与仅预览标线可区分。工具栏分支名使用剩余空间，空间足够时不截断。Blame 小于512px时缩减归属列并取消固定列，日期保留在完整信息提示。详见 [修复验证](../tasks/fix-workbench-interactions/plan.md)。

后文是完整目标设计，**不代表全部已实现**：命令 registry/本地独立搜索、操作记录、布局备选、composer 内部可调高、commit 成功跳转、stash/recovery/tools/AI/conflict 容器迁移均待后续。第一阶段 History 搜索打开现有命令面板，stage 后追随同 path，文件消失清空选择；面板比例按设备保存，会话选择按 Workspace+repo 内存保存。

关联：[F018](../pm/features/F018-workbench-redesign.md) · [工程计划](../tasks/feat-workbench-redesign/plan.md)。

## 1. 结论

推荐把 GitWave 设计为一套专业 Git 工作台：稳定的 Workspace/repo 上下文、History/Changes 双主模式、宽幅代码阅读、持续可见的 Git 操作状态和按上下文出现的 AI。

这次重构应同时处理信息架构、阅读空间、操作语义和视觉组件。只改主题、间距或圆角无法解决任务被多个弹窗分割的问题。旧规范「History 永远是主角，不是 Tab」是本提案明确建议重新决策的内容；第一阶段已由用户接受；未迁移模块沿用旧交互。

## 2. 调查范围与证据边界

### 2.1 基线

- 源码：`fix/history-graph-geometry`，HEAD `85db011`；package.json 与 tauri.conf.json 为 **0.10.3**，调查开始时工作区干净。
- 实际运行应用：设置界面显示 **v0.10.4**。它不是已核实与当前 checkout 相同的构建。
- 实界面：macOS 中文、Native Blue/跟随系统浅色；读取到用户 UI 字号 15、mono 字号 13。截图为 Retina 像素，不能直接当 CSS 布局尺寸。
- 实际查看：History 主窗口、选中 commit、展开代码 diff、Changes clean 空态、Settings 通用/外观、Cmd+K 默认态、commit 右键菜单。仅导航和阅读，没有执行 Git 写操作或 AI 请求，没有更改主题、字体或网络设置。
- 源码覆盖：应用入口、所有功能组件的入口/状态/交互结构、UI primitives、关键 hooks/stores、IPC 能力边界、当前设计与产品文档、现有测试分布。核心布局与 Changes/diff/conflict/palette 读到实现逻辑；低频管理模块按入口和操作路径审查。
- **未完成的验证**：未在真实仓库制造冲突或执行 commit/push/rebase；未实机遍历所有错误状态、深色和三平台；未做性能测量或用户测试。下文对应部分是代码推导与设计建议。此次不是后端全部实现的逐行正确性审计。

### 2.2 当前模块地图

| 模块 | 主要源码 | 当前容器/行为 | 调查方式 |
|---|---|---|---|
| 应用壳与布局 | App、ActionBar、TitleBar、ThreePaneLayout、Split | 40px 顶栏、repo tabs、三纵栏、32px 状态栏 | 源码 + 实界面 |
| Workspace/repo | WorkspaceDropdown、WorkspaceRepoTabs、WorkspaceSwitcher、workspaceStore | dropdown 切换；tabs/排序/移除/重链接；管理分布于菜单与弹窗 | 源码 + 主界面 |
| History | CommitGraph、CommitInfoHeader、CommitContextMenu、RefBadgeContextMenu、useCommitPages | 虚拟列表、拓扑、refs、详情/上下文操作 | 源码 + 实界面/菜单 |
| diff/blame | DiffViewer、DiffText、BlameView、diffRows、syntaxHighlighter | 右侧逐文件展开；unified/split；图片；放大；预览限制 | 源码 + 文本 diff 实界面 |
| Changes | WorkingCopyModal、ChangesPanel、CommitMessageBox、useWorkingCopy、commitDraftStore | modal 内列表+composer+diff；草稿按 repo 隔离 | 源码 + clean 实界面 |
| branch/tag | BranchList、TagsPanel、TagManagerModal、useBranchCheckout、checkoutGate | 单击定位、双击 checkout、右键动作、确认 | 源码 |
| 整合/冲突 | MergeBanner、MergeConfirmDialog、ConflictPanel、InteractiveRebaseDialog、useMergeConflicts | 确认/编辑弹层；文件缓冲；edit pause/continue/abort | 源码 |
| stash | StashPanel、StashDetailModal、useStashes | 侧栏按钮 + 详情 modal + dirty/drop 确认 | 源码 |
| 管理/恢复 | RemotesPanel、WorktreePanel、SubmodulesPanel、ReflogPanel、LfsPanel、HooksPanel、GitignoreEditor | 侧栏详情与 modal 混用 | 源码 |
| AI | AiProviderSettings、CommitExplainModal、PrDescriptionModal、HealthPanel、CommandPalette | 生成弹窗、inline 结果、设置与命令面板 | 源码 + palette 初态 |
| 设置/生命周期 | SettingsModal、SshKeyManager、AboutModal、UpdateModal、AuthPromptDialog、QuitConfirmModal | 设置分类、凭证/更新/退出确认 | 源码 + 设置实界面 |
| 后端边界 | api.ts、src-tauri/src/lib.rs、架构/工程文档 | 既有 Git/AI 用例；部分 mutation 隐式使用 active repo | 接口与架构审查 |

历史 mockups 和官网截图不作为当前界面证据。源码中存在的旧组件/布局分支也不自动视为当前可见入口，需沿 App 与菜单调用链判断。

## 3. 主要问题与重构目标

| 问题 | 依据 | 影响与建议 |
|---|---|---|
| History 与实际工作地位失衡 | App 固定 graph/inspector；WorkingCopyModal 使用 `h-[62vh]` | Changes 升为同级工作区，审阅与提交无需离开应用主空间 |
| 默认代码阅读空间不足 | ThreePaneLayout 默认 240/360px；实界面展开 diff 后长代码行被横向截住，需要滚动 | 默认上方历史、下方宽详情；文件列表与代码区分离，提供专注阅读 |
| 高频任务被覆盖式界面分割 | Changes、stash detail、AI、interactive rebase 使用独立 modal；ConflictPanel 使用自定义 overlay | 长时间工作的编辑/审阅进入画布，短表单与确认继续用 dialog |
| 同一入口改变含义 | ActionBar 的 Changes 在 conflictCount > 0 时直接跳 ConflictPanel | Changes 永远进入 Changes；冲突有明确分组及 Resolve 入口 |
| 主次动作竞争 | Changes clean 实界面仍陈列 stage/discard 多按钮；10 个 commit type 常驻 | 选中才显示批量工具条；discard 入 More；commit type 改可选辅助选择器 |
| 壳层与弹窗组件风格不一致 | source list 较紧凑；Modal 明确沿用 HeroUI 默认大圆角/内边距，未映射项目 12px modal token | 定义一套桌面组件尺度，在 wrapper 层统一，不逐页覆写 |
| 搜索与命令入口弱 | CommitGraph 顶部只有标题/拓扑模式；搜索主要经 Cmd+K；palette 静态命令仅 Settings/Fetch | History 显式本地搜索；全产品命令注册；本地命令默认优先 |
| AI 与本地命令行为混合 | palette form submit 在无选中结果时触发 askAi；静态 Settings hint 硬编码 Ctrl+, | Ask AI 单独动作；平台键位统一生成，避免 macOS 显示 Ctrl+, |
| 反馈挤掉身份且难回查 | SyncStatusArea 在进度/结果/branch 三者之间互斥；结果 TTL 15s | repo/branch 常驻；增加本机会话操作记录，失败保留处理入口 |
| 导航与管理表单混杂 | App sidebar 放 8 类 section；Reflog 的详情和恢复操作也在窄侧栏 | sidebar 导航化；stash/recovery/tools 内容在画布 |
| 文档目标不等于代码能力 | 旧文档说 rebase 支持键盘排序；当前列表主要是 draggable/onDrop | 重设计补可见上移/下移；逐项验证，不能把历史验收文字当已实现 |

保留的优势：明确危险确认、HEAD 与 selection 分开、dirty checkout gate、repo 级 commit 草稿、冲突缓冲保护、虚拟滚动、cursor 分页、按文件懒加载、预览上限提示、键盘调宽、stash 明示动作。重构不应损失这些能力。

## 4. 设计方向选择

| 方向 | 核心形态 | 优点 | 主要代价 | 建议 |
|---|---|---|---|---|
| 原生专业 Git | 紧凑 source list、标准工具栏、列表与详情 | 熟悉，迁移成本低 | 品牌差异较弱，仍需重排任务 | 作为交互基底 |
| **GitWave 工作台** | 稳定壳层 + 双主模式 + 任务画布 + 上下文助手 | 同时改善日常提交与历史追查，可覆盖全功能 | 要重建导航状态与容器边界 | **推荐主方案** |
| 空间化分支画布 | branch 轨道、时间/范围可视化、操作预览 | 复杂拓扑解释直观，视觉识别强 | 数万 commit 密度、键盘与可访问性成本高 | 后续实验视图，不作为默认 |

同类参考只取经过核实的模式：

- [Tower 的界面说明](https://www.git-tower.com/help/guides/first-steps/tower-overview/mac)：sidebar 选择任务，Working Copy 与 History 分工。借鉴任务导航与上下文明确性。
- [Sublime Merge 入门](https://www.sublimemerge.com/docs/getting_started)：Overview/Details 分工、本地命令面板。借鉴概览与细读的分离。
- [Fork 官方展示](https://git-fork.com/)：commit list、working directory、side-by-side diff 与 interactive rebase。借鉴专业 Git 信息密度与动作命名。

这些是官方资料参考，不是本次实际操作了上述竞品；不复制其商用资产，也不把它们的额外能力直接列入 GitWave 重构范围。

## 5. 信息架构与主布局

```text
应用
  Workspace 切换/创建/导入/管理
    Repo tabs + 搜索全部仓库 + 添加 local/init/clone
      History                 浏览 refs、commit、文件、diff、blame
      Changes [数量]           unstaged / staged / conflicts、composer
      Stashes                 列表 → 文件 → diff
      Recovery                reflog → 目标/影响 → 恢复方式
      Repository tools        remotes / worktrees / submodules / LFS / hooks / ignore / health
  Settings
    应用                      语言 / 外观 / 网络 / SSH / 更新
    当前 Workspace            AI provider / 离线 / fallback / templates / rules 来源
```

不增加 Activity Bar + sidebar + repo tree 三套常驻导航。Workspace 一直是第一入口，repo tabs 保留横滚、拖拽和选中揭示。全部仓库搜索和 `+` 入口补充横向 tabs 的溢出与可发现性。

### 5.1 壳层

- 单一 40–44px 标题工具栏：Workspace、当前 branch/upstream、外部工具、Fetch/Pull/Push；为 macOS 窗口控件和拖拽区域留独立区域。Windows/Linux 系统按钮与应用菜单按平台适配。
- 下方 32px repo tabs；不再把 Changes 放在一排同步动作中。
- 左侧 208–240px navigator：History/Changes；refs；Stashes；Recovery/Tools。branch section 的展开位置与当前 HEAD 有明确标记。
- 底部 26–28px：左侧 repo/branch 身份，中间操作进度，右侧近期操作入口。进度不伪造百分比；只有后端支持取消的操作显示取消。

### 5.2 History

默认：navigator |（上：graph+commit table；下：commit metadata + 文件列表 | diff）。

- 上部 history 初始约占可用高度 42%，可拖动及键盘调整；下部初始约 58%。数据密度与代码可读性同时保留。
- 选 commit 更新下方详情，选 file 更新 diff；无需先展开每个文件才能读代码。保留「连续审阅所有文件」作为可选模式，仍按文件懒加载。
- metadata 默认单行摘要，长 message 可展开；SHA、作者、日期可复制。所读 commit 与当前 HEAD 分开呈现。
- refs 单击定位 tip，双击/显式 Checkout 才切 branch。若提供「只看此 branch」必须是另一个明确动作，避免偷偷改变原单击语义。
- 图 lane 拓扑完整，收窄只改变可见区域而非连接关系；保留完整拓扑入口。完整图允许局部横滚，标题/详情不跟着横移。
- 搜索放在 History 顶部，显示范围、加载/继续扫描和无结果；首版复用当前可查询字段。branch/author/path 高级筛选依次核对后端支持，不能只画无效控件。
- 可专注 diff；返回恢复 commit、文件、滚动。宽屏用户可显式选「详情在右侧」，默认不自动跳布局。

### 5.3 Changes

默认：navigator |（文件列表+底部 composer）| diff。

- 文件列表按 Conflicts（仅有时）/Unstaged/Staged 分组；同一文件在两个区出现时，以 path + stage side 区分。
- 行单击是预览，checkbox/明确 stage 动作是修改 index，两者不可混用。多选后显示选中数量与批量 stage/unstage；discard 在 More 中明确可达。
- 列表和 composer 内部可调高，不让 message 挤掉全部文件。commit type 从 10 个常驻按钮改为可选 compact picker，保留自由输入。
- subject/body 可分区输入，但映射为原有 message 字符串；标题长度为提示而非新强制规范。Amend 进入有目标 SHA 的明确模式。
- AI 生成前标明 staged 范围及 provider；生成后可编辑、恢复原稿。生成不是 commit。
- Commit 按钮注明 staged 文件数量；未 stage 时说明原因。clean 状态提供查看 History、打开编辑器、message-only amend（符合条件时），不靠十余个禁用按钮表达空态。
- commit 成功后留在 Changes，显示新 SHA 与「在 History 中查看」；余下 unstaged 继续保留。Push 始终是另一次明确操作。
- stage 后尽量跟随同一文件到 staged 侧；若文件消失，选原位置邻项。外部更改导致内容失效时重新读取并说明，不显示过期 diff。

### 5.4 尺寸策略

真实桌面基线为 1024×768（当前 Tauri 最小尺寸）、1280×800、1440×900、1920×1080；另测 125%/150% 字体。

| 场景 | 布局策略 |
|---|---|
| 1440 宽 History | navigator 224；详情文件列表 240；diff 理论约 976px（扣除分隔和 padding 前） |
| 1280 宽 History | navigator 208；文件列表 220；diff 约 852px；优先 unified |
| 1024 宽 History | navigator 可主动收起；文件列表压至 180；不把三列都强塞到最小值 |
| Changes | 文件/composer 列 288–360；主 diff 吸收余量；低宽度不同时开 AI 侧栏 |
| AI 打开 | 宽屏可停靠约 300–340；空间不足时用任务内可切换面板，保留回到 diff 入口 |
| 高字号/低高度 | 工具栏允许 secondary actions 入 More；列表和代码独立滚动；固定主动作可见 |

以上为草案尺寸，不是已经测出的收益。分栏 diff 每侧不足约 360px 时提示用 unified，保留用户显式选择；不缩小代码字体来强行容纳。

## 6. 一套统一的视觉语言：Tide Workbench

气质：安静、精确、紧凑，有清晰的对象与操作边界。品牌主要出现在选中态与拓扑，不给每个区块加彩色卡片。

| 维度 | 规则草案 |
|---|---|
| 表面 | canvas / chrome / raised 三层；细分隔线；只有真正浮层有阴影 |
| 浅色候选 | canvas #FAFBFC、chrome #F0F2F4、raised #FFFFFF；正文 #20262D、次级 #59636F、分隔 #DCE1E7 |
| 深色候选 | canvas #171B20、chrome #1E232A、raised #262D36；正文 #E3E8EF、次级 #A0AAB6、分隔 #343D49 |
| 强调色 | Tide 深青 #147D87 / 深色 #65C9D0；保留 Native Blue 用户偏好，不强制迁移到 Tide |
| 语义色 | add/删除/conflict 用独立 tokens，配 +/−/状态文字；拓扑色不等同于错误/成功 |
| 字体 | UI 默认建议 13px，路径/代码 13px；关键元数据至少 11px。沿用系统 UI/CJK fallback 与现有 Roboto Mono、自定义字体设置 |
| 密度 | 普通列表 28px；工具栏控件 28–30px；菜单 28–32px。选中有整行浅底，HEAD 另有文字/图形标记 |
| 圆角 | 控件 4–6px，menu 8px，dialog 10–12px；不将大圆角库默认值当产品规范 |
| 间距 | 4/8/12/16/24；代码与列表高密度，设置表单允许更多呼吸空间 |
| 图标 | 保留 Lucide，16px 主体；核心动作图标+文字；禁止仅 hover 才出现必需操作 |
| 动效 | hover/focus 100–120ms，局部展开 160ms；无装饰循环；reduced motion 可关 |
| 状态 | hover、selection、keyboard focus、HEAD、busy、disabled 分别建模；不用一个 accent 背景承载全部含义 |

色值仅为起点：正文/辅助文字以 4.5:1 为验收，关键边界/focus 以 3:1 为目标；对实际透明度叠加和两种 palette 均测量。不能仅凭色值列表宣称已达标。现有用户字体不应在升级时重置为草案默认值。

组件规范必须覆盖：ToolbarAction、NavigationRow、RefRow、DataRow、FileRow、PaneHeader、DiffToolbar、CommitComposer、InlineNotice、OperationStatus、Dialog、ContextMenu、EmptyState、CommandPalette。先统一 anatomy/状态/键盘，再替换页面样式。

## 7. 全模块交互重设计

| 模块 | 新行为与落点 | 必须保留/异常状态 |
|---|---|---|
| 首次启动 | 单一欢迎区：创建/导入 Workspace；随后 local/init/clone | 不出现三个重复空态；init 不自动初始 commit；路径缺失可 relink |
| Workspace | switcher 内含创建、管理、导入导出、当前 Workspace 设置 | 明确是仓库集合，没有 root folder；删除集合不暗示删仓库磁盘目录 |
| 添加/clone | 短表单；clone 进度留任务区并可返回 | 多选去重/部分失败继续；凭证失败原位重试；新 repo 激活 |
| branch/tag | sidebar 导航+统一上下文菜单；表单显示目标 repo/base SHA | checkout dirty gate、remote DWIM、worktree 占用、detached HEAD；删除后果明示 |
| commit 操作 | 选中提交的 More/右键/命令入口统一提供从此建 branch/tag、checkout、reset、cherry-pick、revert、复制 SHA/message；详情保留 AI explain 与 PR 文案入口 | 显示所选 SHA 与当前目标 branch；reset 模式及丢弃影响、detached checkout、dirty gate 保留；cherry-pick/revert 冲突进入实际操作状态流程 |
| Fetch/Pull/Push | 常驻独立动作；dialog 显示 remote、branch、upstream 与策略 | 未完成 Git 操作先阻断；现有 force push 无 lease 保护，必须明示覆盖风险；force-with-lease 如新增需独立后端实现；不合成含糊 Sync 按钮 |
| merge/rebase | 来源→目标及操作类型预览；执行后展示真实 backend 状态 | 不把失败/auto_aborted 当可继续；持续展示 paused/冲突入口 |
| interactive rebase | 主画布 todo 列表+选中提交详情；pick/reword/edit/squash/fixup/drop | 拖拽和上/下移等价；首项 squash 等无效序列由前后端校验；执行前重读 dirty 状态 |
| 冲突 | 专用 Resolve 画布：文件进度、ours/theirs/base、result；显示真实 branch/SHA 角色 | 区分修改未保存、已标记解决、Git 操作未完成；不把 ours/theirs 解释固定为用户/远端 |
| stash | 主画布列表→文件→diff；View/Apply/Apply & Delete/Delete 同义一致 | untracked 文件可见；dirty gate；失败不误删 stash；OID 身份与执行时 index 重校验 |
| Recovery | reflog 列表+事件详情与 old/new OID；优先提供创建恢复分支 | hard reset 显示具体后果；没有通用 Undo；AI 只解释 |
| worktree | 工具区列表显示 branch/path/状态；可固定入口 | 切换后 repo 保持正确；删除确认与占用检查 |
| remotes/submodules/LFS | 工具区完整表格与详情，短表单仍 modal | URL/credential 来源明确；init/update/deinit、track/untrack 保留 |
| hooks/.gitignore | 工具区编辑器，保存状态与离开保护 | hooks 仅编辑不执行；明确 hooksPath/common dir；ignore 模式范围确认 |
| Health | Tools 下的检查结果；仅待处理项可进入状态提示 | 不用常驻大仪表盘；AI 报告与确定性扫描分开 |
| AI 历史/冲突/PR/恢复 | 同一任务助手容器、对象作用域、provider、生成/失败/采用/复制状态 | PR 仍只生成 title/body 文本；offline/local、scrubber、fallback 语义保留 |
| Command palette | 本地命令/branch/repo/commit 分组；Enter 执行当前本地选择；Ask AI 独立按钮 | 所有 Git 写操作走同一 gate；macOS/Windows 键位正确；无匹配不自动发送网络 |
| Settings | 应用与 Workspace 两个明确 scope；分组搜索可后续补 | 当前语言/字体/主题/代理/SSH/更新全部保留；key 不显示在操作记录 |
| 凭证/更新/About/退出 | 继续短 dialog；标准间距/按钮/焦点；更新进度与重启重试明确 | 不破坏 quit guard；关闭焦点回触发项；凭证不持久到前端草稿 |

### 冲突过程的能力边界

现有 resolveConflict 将写结果与 stage 组合为「标记解决」；本轮首先保留明确的原子动作。若后续增加「仅保存文件」需独立后端用例，不可只增加按钮。Continue/Abort 依据实际操作类型提供：merge 最终需显式 commit；interactive rebase 有 edit pause；auto_aborted 没有继续入口；孤立 index 冲突不能显示不存在的 merge Continue。完整统一的操作状态 API 是工程核查项。

### 状态、恢复与反馈

- 导航、数据选择、正在执行的 Git 操作分开；每个异步结果带 workspace/repo/request 身份，不更新另一仓库的画面。
- 同一会话内按 repo 记忆 mode、commit/file、筛选、滚动锚点、Changes 草稿和 AI 结果所属对象；切走再回来恢复。
- 重启恢复 repo、mode、布局等轻量偏好；**本次不承诺把代码/AI 输出/冲突内容写入磁盘**。commit 草稿目前仅会话保留，跨重启持久化需单独设计隐私及失效规则。
- 需要用户处理的错误留到用户处理/关闭；可打开本机会话操作记录，显示对象、时间、结果、重试条件。重试前重读当前 Git 状态，不盲目重放。
- 普通成功短反馈；操作记录不是 reflog，不冒充 Git 可恢复历史。取消请求与已取消分开显示。
- 导航离开冲突/文本编辑器保留缓冲或提示；刷新与删除对象导致 selection 失效时选择邻项并说明。

## 8. 范围分级

**现有能力重新组织**：History、文本/图片 diff、blame、stage 文件、commit/amend、branch/tag、sync、stash、merge/rebase、恢复、tools、AI、设置。

**本提案新增交互基础**：mode 导航与按 repo 状态恢复、History 显式搜索入口、全功能命令注册、会话操作记录、统一 AI 任务容器、键盘 rebase 排序。

**单列后续功能，不夹带进第一期**：全仓 file tree、path history 高级筛选、任意两 commit compare、hunk/line staging、PR/MR 平台集成、vim/自定义快捷键、跨 repo 并排、空间化图编辑、AI 自动修改 Git。后两项也不得突破既有单 active/显式执行原则。

## 9. 验收与交付顺序

1. 完成本草案与可点击 History/Changes 原型评审；确定默认布局和界面语气。
2. 壳层/primitive/state 基础 + History/Changes，先验证日常高频路径。
3. Resolve/interactive rebase、stash/recovery/tools 迁移，完成低频和高风险旅程。
4. AI/命令/设置统一；三平台、双语、两色板/深浅主题回归；补完整文档后发布。

验收场景：初次启动；长路径与大量 refs；空仓/clean/dirty；同文件同时 staged/unstaged；message-only amend；切 workspace/repo；外部 Git 改动；detached HEAD；dirty checkout；孤立 index 冲突；merge/rebase pause/auto-abort；stash untracked/pop 冲突；凭证失败；离线 AI/fallback；超大/二进制/图片 diff；退出未保存；更新重启失败。

关键标准：所有关键任务可用键盘完成、focus 可见且返回；核心操作不只靠颜色/hover；图不丢拓扑；读代码无需为一般行频繁横滚；无跨 repo 草稿/结果串用；失败可定位可恢复；完整 Git 能力按迁移表核销。

性能继承工程文档预算：Apple Silicon 冷启动 <1s、10k history 首次交互 <500ms、1MB 文件 diff <200ms。实施前记录硬件、仓库、冷/热缓存和测量口径，区分预览占位与正文就绪；此次没有测量。已知超出预览限制的文件须如实说明，不能用占位快充当渲染达标。

## 10. 本轮交付性质

本轮完成方案，不改应用代码、不 bump、不 commit/push。可交互示意使用虚构的 GitWave 示例数据，展示布局与任务切换；不连接真实仓库、provider 或 Git 操作。图线和 diff 是布局样例，未模拟真实提交拓扑或每个 commit 的内容，不能用于拓扑/正确性验收；Stash/Tools 等在本原型仅示意入口，详见全模块规格。完整实现仍须完成上述矩阵、回归与用户评审。
