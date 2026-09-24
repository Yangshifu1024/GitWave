# feat-getting-started · 方案

> 需求来源：`docs/pm/features/F018-getting-started.md`（提案）
> 分支：`feature/getting-started`
> 交付形态：一个任务、一个分支、一次交付（应用内改动 + 官网双语页面 + 文档同步）

## 1. 目标与判断标准

让「会用命令行 Git、第一次打开 GitWave」的人，不看任何外部资料就能走通主路径：

**添加本地仓库 → 在历史里选一个提交 → 提交改动 → 推送**

路上不再出现「界面上没有入口、只能去应用菜单里翻」的死胡同。

判断标准由用户确认，验证方式分两步：先由实现方按代码逐项核对链路（按钮 → 动作 → 对话框、文案中英齐全、链接与锚点可达），再由用户在本机应用里手动走查（清单见 §5.2），结果记回本文件。

## 2. 现状与根因（均已核实）

| # | 现象 | 证据 |
|---|---|---|
| 1 | 全新安装没有任何 Workspace 时，侧边栏与主内容区都提示「请在侧边栏中选择或创建工作区」，但侧边栏里没有创建入口；三个添加仓库动作在无 Workspace 时被菜单门控禁用 | `src/App.tsx` 空状态；`src/lib/appMenuSpec.ts`（`enabled: !noWorkspace`）；`src/hooks/useAppMenuGating.ts` |
| 2 | 「克隆 / 初始化 / 添加本地仓库」只存在于应用菜单，界面无按钮、无快捷键，命令面板也没有 | `src/lib/appMenuSpec.ts` Repository 菜单；`src/components/CommandPalette.tsx` 只有 settings / fetch 两条静态命令 |
| 3 | 空仓库的历史图只有一句纯文本，设计文档承诺的「Create your first commit」按钮从未实现 | `src/components/CommitGraph.tsx` 空仓库空状态（键 `branches.graph.empty`）对比 `docs/design/03-layout.md` §5.3 |
| 4 | Workspace 下拉在空列表时显示硬编码英文，绕过双语校验 | `src/components/WorkspaceDropdown.tsx`（`Workspace` / `Workspaces` / `No workspaces yet`）对比 `src/i18n/locales/parity.test.ts` |
| 5 | 对外没有任何面向用户的上手材料：官网只有一个英文首页，README 的 Quick start 是开发者环境搭建 | `site/index.html`、`README.md` |

## 3. 改动清单

### 3.1 应用内

| 文件 | 改动 |
|---|---|
| `src/App.tsx` | 侧边栏「未选择工作区」、主内容区「选择工作区」各加「新建工作区」按钮；主内容区「未选择仓库」加三个按钮（克隆远程仓库 / 初始化新仓库 / 添加本地仓库） |
| `src/components/WorkspaceDropdown.tsx` | 列表末尾加分隔线 + 「新建工作区」条目；三处硬编码英文改为 i18n 键 |
| `src/components/CommitGraph.tsx` | 空仓库空状态加「创建第一个提交」按钮（保留原 DOM 结构，避免动 `CommitGraph.renderGuards.test.ts` 的源码断言）；组件内持有弹窗开关状态 |
| `src/components/FirstCommitHintModal.tsx`（新增） | 空仓库说明弹窗：讲清楚为什么现在提交不了、给出两条出路、提供「克隆远程仓库」次要按钮 |
| `src/i18n/locales/en/branches.json`、`src/i18n/locales/zh-CN/branches.json` | 新增 `graph.createFirstCommit`、`graph.firstCommitHint.{title,description,step1,step2}` |
| `src/i18n/locales/en/workspace.json`、`src/i18n/locales/zh-CN/workspace.json` | 新增 `selectorFallback` |

### 3.2 官网

| 文件 | 改动 |
|---|---|
| `site/style.css`（新增） | 从首页内联 `<style>` 抽出全部样式，并新增语言切换（`.lang`）、教程版式（`.doc-head` / `.prose` / `.doc-section` / `.toc` / `.doc-foot`）、步骤（`.steps` / `.step` / `.step-num`）、截图与占位（`.shot` / `.shot-ph`）、提示块（`.note`） |
| `site/index.html` | 内联样式换成 `<link rel="stylesheet" href="/style.css" />`；导航加 Docs 链接与语言切换；首屏加「How to get started」次要按钮；补 `hreflang`（en / zh-CN / x-default）与 `og:locale` |
| `site/index.zh.html`（新增） | 中文首页，结构与英文页逐项对应，全部文案中文 |
| `site/getting-started.html`（新增） | 英文教程：五个区块（概念 / 界面地图 / 主路径四步 / 克隆支线 / 常见卡点）+ 目录 + 下载区块，6 个截图占位块 |
| `site/getting-started.zh.html`（新增） | 中文教程，与英文版结构、锚点、图片路径逐项对应 |
| `site/shots/README.md`（新增） | 12 张截图的文件名、存放位置、拍摄要求，以及如何把占位块换成真图 |

### 3.3 文档

| 文件 | 改动 |
|---|---|
| `docs/pm/features/F018-getting-started.md`（新增） | 需求提案 |
| `docs/design/03-layout.md` | §5.3 空状态改成表格，登记四处空状态与各自的动作，并记下首次启动死胡同的修复 |
| `docs/pm/core/01-features.md` | §1.4 功能列表补「空状态引导」；用户旅程的「首次启动」一行改成与实现一致 |
| `AGENTS.md` | 文档同步清单：`site/index.html` 一条扩成四页说明，并登记教程页与空状态文案的联动 |
| `.agents/skills/gitwave-release/SKILL.md` | 第 4 节官网同步点：明确中英首页两处版本徽标、教程页刻意不含版本号、共享样式与截图目录 |
| `docs/tasks/README.md` | 任务目录数量统计随新增目录更新 |

## 4. 接口与数据变更

- **数据 / IPC / 配置**：无变更，无迁移。
- **动作总线**：复用 `src/stores/uiStore.ts` 的 `AppMenuAction`（类型含 `workspace:new` / `repo:init` / `repo:clone` / `repo:add`）与 `requestMenuAction`；`src/components/ActionBar.tsx` 是唯一消费者与对话框宿主。**不新增 action，不改 ActionBar 的路由。**
- **新增 i18n 键**（`en` 与 `zh-CN` 必须成对且非空，占位符集合一致）：`branches.graph.createFirstCommit`、`branches.graph.firstCommitHint.{title,description,step1,step2}`、`workspace.selectorFallback`。其余全部复用既有键（`workspace.new`、`common.close`、`menu.repository.*.text` 等）。
- **官网样式契约**：`site/style.css` 是四个页面共用的唯一样式来源；页面里不再有内联 `<style>`，也不引入任何脚本。

## 5. 验证

### 5.1 自动检查

- `make check`：prettier 格式（含 `site/**` 的 HTML 与 CSS）+ `cargo fmt` + eslint + clippy + typecheck + 前端与 Rust 全部测试。
- i18n 对齐：`src/i18n/locales/parity.test.ts` 必须通过（新增键中英成对）。
- 源码断言：`src/components/CommitGraph.renderGuards.test.ts` 必须通过（空仓库空状态结构未被破坏）。
- 官网四页：链接与锚点可达、资源（`/style.css`、`/icon.png`、`/favicon-32.png`）全部 200、窄屏（320 / 360 / 390 / 640）无横向溢出、无 `<script>`、无版本号与日期。

### 5.2 手动走查清单（需要用户执行）

实现方无法操作图形界面，以下每一步都要在应用里真实点一遍。

| # | 步骤 | 预期结果 | 结果 |
|---|---|---|---|
| 1 | 全新状态启动（没有任何 Workspace） | 侧边栏与主内容区都出现「新建工作区」按钮，不再是只提示去侧边栏找 | |
| 2 | 点其中任意一个「新建工作区」 | 新建工作区对话框打开 | |
| 3 | 建好 Workspace 后看主内容区 | 显示「未选择仓库」+ 三个按钮：克隆远程仓库 / 初始化新仓库 / 添加本地仓库 | |
| 4 | 点「添加本地仓库」并加入一个本地仓库 | 添加本地仓库对话框打开，加入后侧边栏出现仓库、主内容区出现历史图 | |
| 5 | 在历史图里点一个提交 | 右侧显示这次提交的说明与文件改动，点文件能看到 diff | |
| 6 | 改一下工作目录里的文件，点顶部「改动」按钮 | 工作副本窗口打开，能暂存文件、写提交信息并提交；提交后历史图出现新提交 | |
| 7 | 点顶部推送按钮 | 推送确认出现，确认后推送成功 | |
| 8 | 新建一个空仓库（初始化新仓库），看历史图 | 出现「创建第一个提交」按钮 | |
| 9 | 点「创建第一个提交」 | 说明弹窗打开，内容讲清楚空仓库为什么提交不了、给出两条出路 | |
| 10 | 点弹窗里的「克隆远程仓库」 | 弹窗关闭，克隆对话框打开（注意两个弹窗一关一开是否有叠影或焦点异常） | |
| 11 | 打开标题栏的 Workspace 下拉 | 列表底部有「新建工作区」条目，点击打开新建工作区对话框 | |
| 12 | 把界面语言切成英文，在「没有任何 Workspace」状态下再看下拉 | 空列表提示是英文文案（来自 i18n，而不是硬编码），切换语言不残留另一种语言 | |
| 13 | 浏览器打开官网四个页面（本地静态服务即可） | 导航栏 English / 中文 互跳正确；Docs / 文档 进入教程页；首屏「怎么开始」按钮可用；教程页目录锚点能跳转；页面里 6 个截图占位块位置合理 | |

### 5.3 未闭合项

- 12 张截图尚未提供，四个页面的教程页显示的是占位块（不是断图）。补齐步骤见 `site/shots/README.md`；补齐后需要把占位块换成 `<img>`，这一步还没做。

## 6. 风险与回滚

- **风险：低。** 应用侧只改空状态的渲染与文案，不动数据层、不动 IPC、不动动作实现；官网是静态页面，合并到 `main` 即自动部署（GitHub Pages，无构建步骤），没有预演环境——这是用户明确选择的交付方式。
- **回滚**：应用侧 `git revert` 该提交即可；官网同理，回滚后再次推送会重新部署上一版页面。
- **已知取舍**：空状态里的按钮复用 `menu.repository.*.text` 的既有措辞（「远程」而不是「远端」），以保持与菜单完全一致；如果以后希望空状态用独立措辞，需要新增 `app.emptyState.*` 键。
- **注意**：`src/components/WorkspaceSwitcher.tsx` 是一个未被任何地方引用的遗留组件，本次不接、不改（用户已确认）。

## 7. 走查结论

（待用户走查后填写）
