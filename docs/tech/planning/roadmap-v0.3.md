# GitWave · v0.3 开发计划

> 工程视角的 v0.3 版本开发计划:里程碑拆解、依赖关系、验收标准与风险。
> 范围来源:`docs/pm/core/02-scope.md` §2.3、`docs/pm/core/03-roadmap.md` §2.3;
> 技术现状:2026-08-28 代码审计(结论已内联到各里程碑的"现状"小节)。
> 状态:**规划**(未开工)。

## 1. 范围决策

以下两项为本计划的边界性决策,实施中不得 silently 扩大范围:

1. **v0.2 依赖项前置(M0)**:v0.3 直系依赖的 v0.2 未完成项(reflog 浏览器、provider
   failover)作为 M0 前置阶段一并排期。与 v0.3 无依赖关系的 v0.2 尾巴——Git LFS、
   hooks 编辑器、AI command palette、per-repo AI rules(`.gitwave/`)、AI history 解释、
   AI PR 描述——**不入本计划**,维持独立排期;其中 AI PR 描述因协作范围缩减为
   仅 remote 管理(见决策 2)而暂失消费方,随 PR/MR 功能回归时再做。
2. **协作首批 = 仅 remote 管理**:remote 增删改 + URL / 状态可视化。PR/MR 创建、
   code review、Issue 链接留后续版本(届时再引入平台 API 与 OAuth)。

## 2. 里程碑

依赖关系:`M0 → M2`;`M3` 弱依赖 `M0` 的 failover(可选);`M1`、`M4` 独立,
可并行。建议顺序:M0 → M1 → M2 → M3 → M4(并行收尾)。

### M0 · v0.2 前置依赖(工作量:M)

现状:reflog 全仓 0 实现;`provider.rs::generate_text` 为单 provider 硬编码 match,
每次请求 `reqwest::Client::new()` 无超时无重试,失败直接抛给 UI。

| 项 | 范围 | 验收标准 |
|---|---|---|
| reflog 读取基础设施 | `infrastructure/git/reflog.rs`:按引用名读取 reflog 条目(时间/旧新 oid/消息/操作人);use case + Tauri 命令(暂不出 UI,M2 消费) | 对任意分支/HEAD 可列出完整 reflog;单测覆盖 \\
| provider failover | `WorkspaceSettings` 增加 `ai_providers: Vec<String>`(有序 fallback 链,主字段 `ai_provider` 保持为链首以兼容旧数据);dispatch 按链顺序尝试;每次请求固定超时(如 60s)+ 一次性重试;共享 reqwest::Client | 主 provider 失败自动落到链内下一个;全部失败时报最后一次错误;AI 设置 UI 可编辑链条 |

依赖:无。前置给:M2(reflog)、M3(可靠性,可选)。

### M1 · 协作 · Remote 管理(工作量:M)

现状:`list_remotes` 只返回名字;无 add/edit/delete remote 命令;侧栏 Remotes
区块是 "Remote list coming later" 占位。

| 项 | 范围 | 验收标准 |
|---|---|---|
| remote CRUD 命令 | add / set-url / rename / remove(libgit2 remote API);`list_remotes` 升级为带 URL 与 fetch/push 双 URL | 命令层单测;命令行 `git remote -v` 交叉一致 |
| Remotes 侧栏面板 | 替换占位:列表(name + fetch URL)、添加弹框、编辑/删除(删除需确认);每项 Fetch 按钮 | 面板可完成全部 CRUD;操作结果走顶部 toast |
| keyring 命名空间参数化 | `secrets.rs` 的 `SERVICE = "gitwave.ai"` 参数化为命名空间参数(AI 保持 `gitwave.ai` 不迁移),为 remote token 预留 `gitwave.remote:{host}`;本期 remote 认证仍走系统 `git credential fill`,不实现 token 流 | 现有 AI key 读写行为不变(回归测试);新命名空间可写入/读取 |

依赖:无。明确不做:PR/MR、Issue、OAuth/token 登录流程。

### M2 · AI 误操作恢复(工作量:L)

现状:依赖 M0 的 reflog 读取;AI 走既有 `generate_text` + `ensure_ai_online` 守卫。

| 项 | 范围 | 验收标准 |
|---|---|---|
| reflog explorer UI | 侧栏/面板入口:按时间倒序的语义化时间线(操作类型中文描述,如"重置 / 合并 / 提交 / 检出"),可按引用筛选 | 展示 HEAD 与当前分支 reflog;事件可读 |
| 恢复建议 | 对每条事件给出确定性建议(如"提交丢失 → 建议在此点建恢复分支""错误 reset → 建议 reset --hard 回 N");**所有恢复操作需用户在确认弹框中显式执行**(P1) | 每类 reflog 事件至少一条可执行建议;执行后 reflog 面板自动刷新 |
| AI 解释(增强) | 选中事件 → AI 解释发生了什么 + 推荐恢复路径;prompt 模板键 `reflog` 纳入 `PromptTemplates`(默认内置,可被 workspace 模板覆盖) | AI 输出仅建议不执行;离线模式被 `ensure_ai_online` 拦截 |

依赖:M0(reflog 基础设施)。验收对照完成定义"AI 智能能力端到端可用"。

### M3 · AI repo health dashboard(工作量:L)

现状:从零。数据采集用确定性本地指标,AI 只做总结——保证无 AI 时功能仍可用。

| 项 | 范围 | 验收标准 |
|---|---|---|
| 指标采集 | `infrastructure/git/health.rs`:未推送/未拉取提交数、陈旧分支(可配阈值)、merge 冲突残留(MERGE_HEAD 等)、工作区脏文件数、仓库体积 top 文件、tag/branch 数量 | 纯 libgit2 本地计算;单测覆盖各指标 |
| Dashboard UI | 侧栏或独立面板:指标卡 + 异常项高亮,一键跳转对应面板(如冲突残留 → ConflictPanel) | 打开 < 1s(本地计算);空仓库有友好空态 |
| AI 总结 | 指标序列化后喂 LLM 生成健康报告与改进建议(只读建议) | 报告可重新生成;离线时仅展示确定性指标 |

依赖:M0 failover(可选,提升可靠性)。

### M4 · Linux 稳定版(工作量:S)

现状:CI 已出 deb/rpm/appimage 三产物;但仅 tag 触发、artifact `if-no-files-found: warn`
(打包静默失败 CI 仍绿)、无 desktop 元数据定制、无 release 发布步骤。

| 项 | 范围 | 验收标准 |
|---|---|---|
| 打包元数据 | `tauri.linux.conf.json`:desktop 文件(name/icon/category/terminal=false)、mime-types | deb 安装后应用菜单出现、图标正确 |
| CI 加固 | Linux artifact `if-no-files-found: error`;PR 或分支可选构建;新增 release job(tag → GitHub Release 上传三产物) | 打包失败必红;tag 推送自动出 Release |
| 真机验收 | Ubuntu 22.04:deb/rpm 安装、appimage 直跑;核心场景(commit→push、history、diff)冒烟 | 验收清单勾完;问题回修 |

依赖:无(可提前并行)。完成定义"三平台可用"的落点。

## 3. 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| keyring 命名空间迁移 | 误伤既有 AI key | M1 只加参数不动 AI 路径 + 回归测试 |
| failover 改 settings 结构 | 旧数据兼容 | `ai_provider` 保留为链首;新字段 serde default |
| Linux 回归发现滞后 | M4 验收延期 | CI 加固提前到 M4 首项;可选分支构建 |
| 范围蔓延(协作想提前做 PR) | 版本失焦 | 决策 3 已锁;PR 随 v0.4 评估 |

## 4. 完成定义对照(02-scope §2.3)

| 完成定义 | 落点 |
|---|---|
| 三平台可用 | M4(macOS/Win 已可用,Linux 补验收) |
| AI 智能能力端到端可用 | M2 + M3(M0 保障可靠性) |
| 协作能力走通 | M1(本轮决策:协作验收 = remote 管理闭环;PR/MR 顺延) |
