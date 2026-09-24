# feat-getting-started · 审查报告

> 审查基准：`docs/tasks/feat-getting-started/plan.md`
> 需求来源：`docs/pm/features/F018-getting-started.md`
> 分支：`feature/getting-started`
> 审查方式：先由 code-reviewer 角色按基准逐条对齐（只读），再由 tester 角色实际执行全部门禁与浏览器验证；返工后复审一轮。

## 1. 结论

**可以合并。**

首轮审查判定「需要返工」（2 个 🔴 + 5 个 🟡 + 4 个 🟢），首轮测试判定「门禁未通过」（2 个 🔴）。返工一轮后逐条复验：**11 条全部闭合**，两条残余 🟡 已在合并前收掉，复审未发现新的 🔴。

## 2. 对齐表（计划 → 实现）

| 计划条目 | 实现 | 对齐 |
|---|---|---|
| 侧边栏 / 主内容区「未选择工作区」加「新建工作区」按钮 | `src/App.tsx`，均走 `requestMenuAction("workspace:new")` | ✅ |
| 主内容区「未选择仓库」加三个按钮 | `src/App.tsx`：克隆远程仓库（主）/ 初始化新仓库 / 添加本地仓库 | ✅ |
| 动作值与 ActionBar 路由一一对应 | `workspace:new` → `openCreateWorkspace()`；`repo:init` / `repo:clone` / `repo:add` → `startAdd("init" / "clone" / "local")`，四个值都在 `AppMenuAction` 类型里 | ✅ |
| Workspace 下拉加「新建工作区」条目 + 三处硬编码英文改双语 | `src/components/WorkspaceDropdown.tsx` | ✅ |
| 空仓库历史图加「创建第一个提交」按钮 + 说明弹窗 | `src/components/CommitGraph.tsx` + 新增 `src/components/FirstCommitHintModal.tsx` | ✅ |
| 保留空仓库空状态的 DOM 结构（不动 `renderGuards` 源码断言） | 只把外层 div 的 class 由 `flex items-center` 改为 `flex flex-col items-center gap-3` | ✅ |
| i18n 键中英成对、非空、占位符一致 | `branches.json` 新增 5 键、`workspace.json` 新增 1 键，`parity.test.ts` 通过 | ✅ |
| 不动 `src/stores/**`、`ActionBar.tsx`、`appMenuSpec.ts` | `git status` 无这三处改动 | ✅ |
| 官网四页：抽 `site/style.css`、双语、无脚本、无内联样式、教程页无版本号与日期 | 四页 + `site/style.css` + `site/shots/README.md`；`<script>` / `<style>` / 内联 `style` 属性均为 0；教程页零版本号零日期 | ✅ |
| 教程页五个区块 + 12 个截图占位 | 两页各 5 区块（概念 / 界面地图 / 主路径四步 / 克隆支线 / 常见卡点）+ 6 个占位块，文件名与 `site/shots/README.md` 一一对应 | ✅ |
| 文档同步（design / 产品 / AGENTS / 发版技能 / 任务计数） | 5 处均已更新，另补 `docs/pm/features/README.md`（见下） | ✅ |

## 3. 首轮问题与处理

| 级别 | 问题 | 处理 |
|---|---|---|
| 🔴 | 教程页按钮名与应用实际文案不符：英文页写 `Clone remote repository` / `Init new repository`，实际是 `Clone remote repo` / `Initialize new repo`；中文页写「添加本地仓库」，实际是「添加已有本地仓库」；图注对话框名少了复数 | 已修，并把两页**所有**被当作按钮名 / 对话框名引用的文案逐项回查 i18n 实际值（含 `Changes` / `Commit` / `Generate` / `Tracking…` / `Settings → SSH Keys` 等），确认一字不差 |
| 🔴 | 教程页称推送对话框会一并设置上游，与实现不符（推送只提交远端 / 标签 / 是否强制） | 已改写：推送对话框只负责推送，设置上游走左侧分支列表右键的「跟踪分支…」 |
| 🟡 | 教程页「设置 → SSH」「测试连接」称呼不精确 | 改为「设置 → SSH 密钥」「测试 SSH 连接」，并顺带修正 SSH 面板能力的描述（该面板只能把已有私钥加入 ssh-agent，不生成也不导入密钥） |
| 🟡 | 教程页「常见卡点」缺空仓库场景（本次功能的核心场景） | 两页各补一条，说明按钮名与弹窗标题的实际文案；**未新增截图位**，截图数量仍为每语言 6 张 |
| 🟡 | `docs/pm/features/README.md` 未登记 F018，且仍写「F001–F017 全部已合并」「不再为每个功能单独开提案文件」 | 已登记 F018 表行、更新状态句，并把 F017 / F018 记为两处例外 |
| 🟡 | 两个首页各有一处内联 `style` 属性绕过共用样式表 | 新增 `.dl-note a` 规则，删掉两处内联样式 |
| 🟡 | `site/style.css` 窄屏注释与真实行为不符 | 注释改为描述真实行为（首页保留 Docs 链接，教程页自己的 Docs 链接会被 `.hide-sm` 隐藏） |
| 🟡 | 英文首页在约 318 像素以下溢出（320 窗口叠加 Windows 经典滚动条的实际内容宽约 305px） | 新增 `@media (max-width: 380px)`：进一步缩小导航间距、品牌字号与下载按钮内边距；实测 300–1280 全部无溢出 |
| 🟡 | `site/shots/README.md` 里对话框名漏改 | 已改为 `Add existing local repos` |
| 🟢 | 中文首页 `og:url` 与 `canonical` 不一致 | 已统一 |
| 🟢 | `docs/design/03-layout.md` 首句与表格最后一行自相矛盾 | 已改为「除 Inspector 的选择提示外」 |
| 🟢 | `AGENTS.md` 文档同步清单两条 `site/` 条目内容重叠 | 已合并为一条 |
| 🟢 | 英文首页 `og:url` 与 `canonical` 尾斜杠不一致 | 未处理（既有问题，两者指向同一资源，本次不扩大范围） |

## 4. 测试结论

| 检查项 | 结果 |
|---|---|
| `make check`（prettier + `cargo fmt --check` + eslint + clippy + typecheck + 前端与 Rust 测试） | **通过**（退出码 0）。eslint 40 条 warning、0 error，均为既有文件的历史告警 |
| 前端测试 | 32 个文件 / 247 个用例全部通过（含 `parity.test.ts` 与 `CommitGraph.renderGuards.test.ts`） |
| Rust 测试 | 364 passed / 0 failed / 2 ignored |
| 官网四页资源与链接 | 全部 200，无请求失败、无控制台错误、无断图 |
| 官网四页横向溢出 | 300 / 310 / 315 / 320 / 360 / 390 / 640 / 1280 全部无溢出 |
| 官网四页无脚本 / 无内联样式 | `<script>` 0、`<style>` 块 0、内联 `style` 属性 0 |
| 教程页无版本号与日期 | 通过（首页两份的版本徽标仍在） |
| 语言切换与 `canonical` / `hreflang` | 四页互指正确，教程页切换指向教程页而非首页 |
| 中英结构对应 | 两个首页 111 vs 111 个节点、两个教程页 171 vs 171 个节点，差异仅语言切换控件内「当前语言 / 另一语言」的先后顺序（预期） |
| 12 个截图占位块 | 两页各 6 个，路径与 `site/shots/README.md` 登记名一一对应 |

浏览器验证环境：静态服务用 `python3 -m http.server` 指向 `site/`；浏览器用系统 Google Chrome（`channel: 'chrome'`）驱动，因为 Playwright 自带内核未下载。

## 5. 未闭合项（不阻塞合并）

- **12 张截图未提供**：教程页显示的是占位块（不是断图）。拍摄要求与替换步骤见 `site/shots/README.md`；补齐后需要把占位块换成 `<img>`。
- **主路径手动走查未执行**：清单在 `plan.md` §5.2，需要人在图形界面里点。其中第 10 步（说明弹窗里点「克隆远程仓库」）要特别留意两个弹窗一关一开是否有叠影或焦点异常——静态审查无法证伪。
- **应用侧一处既有文案不一致**（本次未改，建议后续统一）：空状态按钮是「添加已有本地仓库」，同一个对话框的标题是「添加现有本地仓库」（`menu.repository.add.text` vs `commits.repo.addLocalTitle`）。
- **英文首页 `og:url` 尾斜杠**与 `canonical` 写法不同（既有问题）。
