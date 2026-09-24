# F018 · 新手引导（应用内空状态 + 官网中英双语上手教程）

## 背景

新用户第一次打开 GitWave 会在最开始的三十秒里卡住：

- 全新安装、还没有任何 Workspace 时，侧边栏与主内容区都显示「请在侧边栏中选择或创建工作区」，但侧边栏里**没有任何创建工作区的入口**；「克隆 / 初始化 / 添加本地仓库」三个动作在没有活动 Workspace 时全部被禁用（菜单项 `enabled: !noWorkspace`，后端由外键兜底）。唯一能新建 Workspace 的入口在应用菜单里（macOS 是系统菜单栏），新手无从发现。
- 有 Workspace 但还没有仓库时，主内容区只显示一句「从侧边栏选择一个仓库」，而三个添加仓库的动作同样只存在于应用菜单里，界面上没有按钮、也没有快捷键。
- 空仓库（还没有任何提交）时，历史图只显示一句纯文本；`docs/design/03-layout.md` 承诺过「Create your first commit」按钮，但代码里从来没有实现。
- 标题栏的 Workspace 下拉在空列表时显示硬编码英文 `No workspaces yet`，绕过了「界面文案必须中英双语且键完全对齐」的强制校验（`src/i18n/locales/parity.test.ts`）。
- 对外只有一个英文官网首页，README 的 Quick start 讲的是开发者环境搭建；面向用户的上手材料为零——仓库里没有教程文档，官网上没有教程页面。

## 提议方案

### 一、应用内：把空状态变成能直接点出下一步的入口

1. **首次启动的 Workspace 空状态**（侧边栏 + 主内容区）：加「新建工作区」按钮，打开现有的新建工作区对话框。
2. **Workspace 下拉**：列表末尾加分隔线 + 「新建工作区」条目，并把该文件里所有硬编码英文（`Workspace` / `Workspaces` / `No workspaces yet`）改成中英双语文案键。
3. **没有仓库的空状态**：加三个按钮——克隆远程仓库（主）/ 初始化新仓库 / 添加本地仓库。
4. **空仓库的历史图空状态**：加「创建第一个提交」按钮，点击打开一个说明弹窗，讲清楚「提交需要有文件改动，空仓库里还没有文件」，并给出两条具体出路（先放文件再提交 / 改用克隆）；弹窗里提供「克隆远程仓库」次要按钮，避免它变成死胡同。
5. **接线方式**：全部复用 `src/stores/uiStore.ts` 的 `AppMenuAction` 动作总线（`requestMenuAction`，四个动作 `workspace:new` / `repo:init` / `repo:clone` / `repo:add` 已在类型定义里），不新增 context、不把 `ActionBar` 的内部回调上提。理由：该总线已经是应用内菜单栏与原生菜单共用的既有机制，`ActionBar` 是唯一的对话框宿主与动作实现方，一致性由构造保证。

### 二、官网：中英双语，首页 + 独立教程页

6. **双语结构**：首页两份（`site/index.html` 英文、`site/index.zh.html` 中文），新增教程页两份（`site/getting-started.html`、`site/getting-started.zh.html`）。切换方式是纯链接互跳，导航栏右侧「English / 中文」两个文字链接、当前语言加粗不可点；**官网保持零脚本**。
7. **样式**：把首页内联的 `<style>` 抽成 `site/style.css`，四个页面共用外链引用，避免两份样式各自漂移。
8. **导航与入口**：导航栏新增「Docs / 文档」链接；首页首屏在下载按钮旁新增次要按钮指向教程页。
9. **搜索引擎标注**：中英页面互相声明 `hreflang`，各自的 `canonical` 指向自己，避免被当成重复内容。
10. **教程内容**：五个区块——工作区与多仓库的概念、界面地图、主路径四步（添加本地仓库 → 在历史里选一个提交 → 提交改动 → 推送）、克隆远程仓库支线、常见卡点（HTTPS 凭据 / SSH 密钥 / 合并冲突 / 界面语言切换）。配 12 张截图（中英各一套，界面语言跟随页面语言）。**页面里不出现版本号和日期**，避免每次发版都要改教程。

### 三、文档同步

11. 更新 `docs/design/03-layout.md` 的空状态约定（含那个承诺了却从未实现的按钮）、`docs/pm/core/01-features.md` 的首次启动用户旅程、`AGENTS.md` 的文档同步清单，以及 `.agents/skills/gitwave-release/SKILL.md` 登记的官网同步点（官网从 1 个页面变成 4 个）。

## 影响

- 涉及模块：
  - 修改 `src/App.tsx`（三处空状态）、`src/components/WorkspaceDropdown.tsx`、`src/components/CommitGraph.tsx`
  - 新增 `src/components/FirstCommitHintModal.tsx`
  - 新增 `site/style.css`、`site/index.zh.html`、`site/getting-started.html`、`site/getting-started.zh.html`、`site/shots/README.md`
  - 修改 `site/index.html`（抽样式 + 导航 + 首屏按钮 + `hreflang`）
  - i18n：`src/i18n/locales/{en,zh-CN}/branches.json`、`workspace.json`
  - 文档：`docs/design/03-layout.md`、`docs/pm/core/01-features.md`、`AGENTS.md`、`.agents/skills/gitwave-release/SKILL.md`
- 影响版本：v0.9.x
- 是否破坏向后兼容：否。无数据、IPC、配置变更；新增 i18n 键只增不改。

## 不做（非目标）

- ❌ 不做首次启动的欢迎页或分步高亮漫游，不引入引导类依赖（driver.js / intro.js / shepherd 之类）
- ❌ 不接上那个未被任何地方引用的遗留组件 `src/components/WorkspaceSwitcher.tsx`
- ❌ 官网不做语言记忆、不按浏览器语言自动跳转（保持零脚本、两个独立页面）
- ❌ 教程页不写版本号与日期，因此不登记为发版同步点
- ❌ 教程页不做中文以外的第三种语言

## 待人工完成（交付时未闭合）

- 12 张截图（中英各 6 张）由用户提供，放入 `site/shots/en/` 与 `site/shots/zh/` 并替换页面里的占位块；文件名与要求见 `site/shots/README.md`
- 主路径的手动走查（清单见 `docs/tasks/feat-getting-started/plan.md`）

## 决策

- 状态：已接受（实现中）
- 决策人：用户（多轮问答逐项确认：目标读者、载体、判断标准、交付形态、双语实现、截图来源与数量、命名与分支）
- 决策日期：2026-09-23
- 关联：`docs/tasks/feat-getting-started/plan.md`、[F010](./F010-i18n.md)（界面双语文案约定）、`docs/design/03-layout.md` §5.3（空状态）
