# feat-i18n · Code Review

- 分支：`feature/i18n`（未提交，对照 `main` diff）
- 审查方式：code-reviewer 代理，对照 `git diff main` 全量改动 + 定向源码核验；组件迁移抽查 10 个文件；另以脚本对全部 locale JSON 与 200+ 处 Rust 错误构造点做了自动化交叉校验（占位符 ↔ params、t() key 存在性、en/zh 占位符漂移）
- 结论：**可合入**（🔴 0 / 🟡 5 / 🟢 7；建议合入前顺手修掉 🟡 #1–#3，均为单行级修复）

## 审查认可项

- **菜单链路**：`appMenuSpec.ts` 单一事实源被自绘菜单与原生菜单共用；两个消费面均以 `i18n.language` 为依赖重建（`AppMenuBar.tsx:137-140` 显式 `void i18n.language` 过 lint，`useNativeAppMenu.ts:154` 注释说明模块级 `i18next.t` 对 exhaustive-deps 不可见）。模块级 `installQueue` 串行化启动安装与 hook 重建，`.then(install, install)` 的 rejected-handler 形式保证单次失败不毒化队列；effect cleanup 置 `run.stale` + 安装后复检 + `previous.close()` 释放旧菜单，竞态分析正确。`useAppMenuGating` 以原始值 memoize，2s 工作副本轮询不会触发原生菜单反复重装
- **错误 key 化架构**（ADR-0006）：6 个 tuple 变体改 struct 变体后 `Serialize` 的借用型 DTO（`AppErrorDto<'a>`）生命周期正确——DTO 在 `serialize` 调用栈内构造与消费，`code` 为 `&'static str` 拷贝；`BTreeMap` 保证 params 序列化顺序确定；空 params 正确省略；`From<git2::Error>` 把不可翻译的 libgit2 原文降为 `detail` 参数、只本地化框架句，方向正确
- **parity 测试质量高**：域文件集合 / 每域 leaf 集合 / `error_codes` 常量 ↔ errors 子树双向覆盖 / 空串断言四层防护，且经 CI `npm test` 强制；`nativeMenuBuild.test.ts` / `syncStore.test.ts` 用 top-level await 钉住英文 locale，测试不随宿主系统语言漂移
- **language.rs 边界完备**：`None` / 未知值 / `zh-CN`（UI locale ≠ AI tag）/ 空串均有用例；未知值静默降级为无指令（advisory 语义）合理；en 也显式追加指令防 mid-reply 漂移，有测试
- **自动化交叉校验结果**（本次审查新增，脚本未入库）：en/zh-CN 约 570 对 key 的插值占位符漂移 **0**；代码中全部静态 `t()` key 均存在（plural 基键经 `_one/_other` 解析正常）；202 处内联 Rust 错误构造点的 params 键与 locale 占位符逐一核对，除下述 🟡 #1 外全部一致；`ai.provider_http` 的 params 经预构建变量传入，手工核对一致
- **迁移质量抽查**（ChangesPanel / BranchList / ActionBar / CommandPalette / SettingsModal / CommitInfoHeader / UpdateModal / AppMenuBar / BlameView / useAutoRefresh / useRemoteSync）：均为机械替换 + 插值化，未发现条件分支、参数名或门控逻辑改变；`count` 复数正确落到 `_one/_other`（zh 同值符合 CLDR 规则）；CommitInfoHeader 顺手把遮蔽 `t` 的局部变量改名 `tag`，主动消除了一个真实 bug 类
- **i18n bootstrap**：`main.tsx` 中 `initI18n()` 先于 `installNativeAppMenuEarly()`，首帧菜单即为所选语言；`escapeValue: false` 有注释依据（React 已转义）；`readStoredLanguage` 校验合法值集合，localStorage 全部 try/catch 降级；`applyDocumentLang` 带 node 测试守卫

## 发现与处置

| 级别 | 发现 | 依据与修复方向 |
|---|---|---|
| 🟡 | **`set_active_repo` 错误 params 键名不一致，UI 会渲染裸 `{{id}}`**：`workspace_repo.rs:152-155` 传 `("workspace_id", …)`，而 `persist.workspace.not_found` 两个 locale 均为 `…{{id}}`（`workspace_repo.rs:119/134/171` 另三处同 code 均传 `"id"`） | 已实测 i18next v26 对缺失插值**保留原占位符**（输出 `工作区不存在：{{id}}`）。修复：该处参数键改为 `"id"` 与其余三处对齐 |
| 🟡 | **zh `allFilesTooltip` 与 `stageAll` 组合产生重复语义**：`changes.json` zh `"{{action}}全部文件"` × action = 「暂存全部 / 取消暂存全部 / 丢弃全部」→ tooltip 显示「暂存全部全部文件」 | 修复：zh 改 `"{{action}}文件"`（→「暂存全部文件」），或 tooltip 改用独立键不复用 action 全量标签 |
| 🟡 | **`formatAppError` 的 params 展开顺序存在 i18next 保留字污染面**：`api.ts:132` `{ defaultValue: fallback, ...e.params }` 把 params 展开到 t() options。已实测：params 含 `ns`/`lng` 时 key 查找被劫持、静默走英文 `defaultValue` 路径；含 `defaultValue` 会顶掉 fallback；`count` 会触发复数分支。当前 Rust 全部 params 键（error/name/id/path/oid/repo_id/provider/url/workspace_id/exit/status/stderr/detail 等）已逐一核对**无碰撞**，属前瞻性风险 | 修复（一行）：调换顺序 `{ ...e.params, defaultValue: fallback }`，并可顺手 `delete` 白名单外保留字（`ns`/`lng`/`count`/`ordinal`/`keySeparator`/`nsSeparator`），加注释声明 params 键命名禁区 |
| 🟡 | **parity 测试不覆盖插值一致性，恰是本次唯一漏网缺陷的类别**：① en/zh 占位符集合漂移无断言（本次脚本实测为 0，但无防护）；② Rust 构造点 params ↔ locale 占位符一致性无防护（🟡 #1 即由此漏过） | 建议：parity 测试补 en/zh 占位符集合对比；Rust 侧把本次审查用的「构造点扫描 vs locale 占位符」脚本固化为 CI lint 或 Rust 单测（code 常量与 params 键均有稳定约定，可自动化） |
| 🟡 | **site 跳转脚本丢失 hash / query**：`site/index.html` 新增脚本是 `location.replace("/zh-CN/")`，站内有 `#download` 锚点与 `href="#download"` 链接；中文访客点开 `https://gitwave.work/#download` 外链会被丢到中文页顶部。无循环跳转（脚本仅在 en 页）、大小写已 `toLowerCase`、静态站无 SSR 问题均已核验 | 修复：`location.replace("/zh-CN/" + location.search + location.hash)` |
| 🟢 | `error.rs:231-232` DTO 注释已过时：「code / params are omitted when the UI is expected to fall back」——struct 变体化后 `code` 恒为 `Some`（永不省略），实际只有空 params 省略 | 更新注释为实际语义 |
| 🟢 | **部分错误诊断细节在翻译层丢失**：若干构造点传了 params 但 locale 文案未使用（如 `git.bare_repo` 丢弃底层 error、`ai.offline_mode` 未指明 provider、`persist.data_dir.resolve` 未含 dir/error）。旧英文 fallback 含这些细节，现在有 code 时只显示 locale 文案 | trace_id + 日志仍可溯源，非阻塞；建议对诊断价值高的（bare_repo / data_dir.resolve / cli_failed 类）在 en/zh 文案中补 `{{error}}` / `{{provider}}` |
| 🟢 | **`ai_palette_intent` 的用户可见 `explanation` 字段不受 AI 语言控制**（命令无 language 参数，plan.md 已声明意图解析路径不加，但 `explanation` 是展示给用户的散文） | 属范围决策边界；建议在 plan.md 明确「explanation 暂不本地化」，后续可让前端对该字段补一条回复语言指令 |
| 🟢 | **「贮藏并切换」的 stash message 随 UI 语言定稿**：`branches.checkout.stashMessage` 使 `git stash list` 出现「切换到 main」等中文。stash 为本地 ref 不外发，且已核实代码按 `stash@{0}` 应用、不解析 message 文本，无功能依赖 | 可接受；如希望仓库内数据语言中立，可保留英文模板（产品决策） |
| 🟢 | **文档与实现漂移**：ADR-0006 与 plan.md 写 `domain/error_codes.rs` 单文件（实际为 `error_codes/` 目录 4 子模块）；plan.md 的 errors locale 示例为 `{"message": "..."}` 对象叶子，实现为纯字符串叶子（实现更优） | 合入前顺手更新两处文档描述 |
| 🟢 | **resources.ts 深合并在非 errors 域重名时静默后者胜出**（按 glob 字母序 errors-cmds < errors-git < errors-infra < errors-usecases）；注释已声明「其他域不得重名」但无断言 | 建议 parity 测试补一条「每个非 errors 顶层域只在一个文件中声明」 |
| 🟢 | **parity.test.ts 复制了一份 deepMerge**，与 resources.ts 实现存在语义差（数组：测试版按对象合并、resources 版整体替换）；当前 locale 文件无数组，无实际影响 | 可改为 parity 测试直接 import resources.ts 合并结果，消除双实现漂移 |
| 🟢 | `languageFromNavigator` 把一切 `zh*`（含 zh-TW/zh-HK）映射到 zh-CN 简体 | 符合 F010「中英双语」范围；建议加一行注释声明该取舍 |

## 复验记录

- `npx vitest run parity / nativeMenuBuild / syncStore`：3 文件 18 用例全过
- `cargo test --lib -- error::tests language::tests`：10 用例全过（整库编译通过，即 172 处构造点类型校验通过）
- `npx tsc --noEmit`：零错误
- 自动化交叉校验（审查脚本）：en/zh 占位符漂移 0；静态 t() key 缺失 0（plural 基键除外，属预期）；Rust 构造点 params ↔ locale 占位符 202 处内联核对一致 + 1 处 🟡 #1 + `provider_http` 预构建变量手工核对一致
- 未跑全量 `cargo test` / Playwright（按任务约束抽样执行）

## 总体评价

改造规模大（约 3000 行新增、200+ 构造点、570 对 key）但工程纪律出色：单一事实源、双向 parity 防漂移、竞态与降级路径都有明确注释且经得起推敲，抽查未发现任何行为逻辑回归。已识别问题集中在「占位符 ↔ params 一致性无自动防护」这一测试盲区与三处单行级文案/参数缺陷，均不阻塞合入；最优先的后续投资是把本次审查使用的构造点交叉校验固化进 CI，使该缺陷类别从此不可回归。

## 修复记录（2026-08-30，审查后主会话执行）

| 项 | 修复 | 验证 |
|---|---|---|
| 🟡 #1 | `workspace_repo.rs` `set_active_repo` 的 params 键 `workspace_id` → `id`，与 locale `{{id}}` 对齐 | cargo check + fmt 通过 |
| 🟡 #2 | zh `changes.json` `allFilesTooltip` 改为 `{{action}}文件`（原「暂存全部全部文件」） | vitest 占位符 parity 通过 |
| 🟡 #3 | `formatAppError` 改为 `{ ...params, defaultValue: fallback }` 顺序，并剥离 `ns` / `lng` / `defaultValue` 保留键，防御 i18next 选项劫持 | tsc + vitest 通过 |
| 🟡 #4 | parity 测试新增「en/zh-CN 逐叶 `{{占位符}}` 集合一致」断言（固化审查脚本的占位符漂移检查；Rust params ↔ locale 全量核对已由本次审查完成且修复后 0 漂移） | vitest 116 用例通过 |
| 🟡 #5 | site 跳转保留 `location.search + location.hash`，`#download` 锚点不再丢失 | 人工核验 |
| 🟢（error.rs DTO 注释过时） | 注释更新为实际语义（code 恒在、仅空 params 省略） | — |
| 🟢（文档描述漂移） | ADR-0006 / plan.md 的 `error_codes.rs` 单文件描述改为 `error_codes/` 模块；errors JSON 叶子示例改为纯字符串 | — |
| 🟢（zh\* 映射取舍） | `languageFromNavigator` 补注释声明 zh-TW/zh-HK → zh-CN 属范围决策 | — |

🟢 其余各项（翻译层诊断细节、explanation 不本地化、stash message 语言、深合并重名断言、deepMerge 双实现）记录在案，不阻塞合入，作为后续投资跟踪。
