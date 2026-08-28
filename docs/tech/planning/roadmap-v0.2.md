# GitWave · v0.2 开发计划

> 工程视角的 v0.2 版本开发计划:里程碑拆解、依赖关系、验收标准与风险。
> 范围来源:`docs/pm/core/02-scope.md` §2.2、`docs/pm/core/03-roadmap.md` §2.2;
> 技术现状:2026-08-28 代码审计(基线 `main` = `556074c`,已含 v0.1 gap batch 全部成果)。
> 状态:**已批准,未开工**。

## 1. 范围决策

以下三项为本计划的边界性决策,实施中不得 silently 扩大范围:

1. **AI command palette = 确认门控操作集**:只读操作(定位 commit / 打开设置 / 切换
   repo)立即执行;变更类(建分支 / checkout / 打 tag / stash / fetch)展示 AI 解析结果、
   用户点确认才执行;commit / push / merge / rebase **不由 palette 执行**
   (P1:AI 永不自动变更仓库)。
2. **AI PR 描述 = 仅生成**:输出可编辑 title + markdown body(含 Copy),不做 GitHub API
   创建与鉴权——PR/MR 随 v0.3 协作范围回归时再做(见 [roadmap-v0.3.md](./roadmap-v0.3.md)
   决策 3,彼时其消费方恢复)。
3. **交付方式**:`git worktree add D:/Work/GitWave-v02 -b feature/v0.2 main` 在独立
   worktree 开发,不动主工作区;**每项功能一个 conventional commit**(用户显式授权,
   仅限本分支),每项单测绿才 commit;全部完成后 cargo / npm 全量验证,审查报告仍落
   `docs/tasks/feat-v02-batch/review.md`(执行产物归 docs/tasks,本文件属跨任务计划)。

## 2. 里程碑

依赖关系:`M1` 依赖 `M0`(failover 链、rules 注入);`M2`、`M3` 独立,可并行。
建议顺序:M0 → M1 → M2 → M3。

### M0 · AI 基础设施(工作量:M)

现状:AI 侧单 provider 硬编码 match(`provider.rs::generate_text`),无 fallback;
`prompt_templates` 的 `pr` 键已存在但无消费方(commit / conflict 已覆盖);
全仓无 per-repo AI rules 概念;`ai_offline` 守卫已随 v0.1 gap batch 落地。

| 项 | 范围 | 验收标准 |
|---|---|---|
| provider failover | `WorkspaceSettings` 增 `ai_failover: Vec<AiProviderConfig>`(`#[serde(default)]`,主字段 `ai_provider` 保持链首兼容旧数据;`AiProviderConfig{provider, model, base_url}`,key 仍按 provider 走 keychain);`resolve_ai_chain` + `generate_text` 有序尝试,Network / 5xx / 429 切下一个,返回实际使用 provider(顶部 toast 提示);AiProviderSettings 增有序 fallback 列表(增删 / 上移 / 下移) | 主 provider 失败自动落到链内下一个;全部失败报最后一次错误;UI 可编辑链条;旧配置 serde roundtrip 单测 |
| per-repo AI rules | 读 `<repo>/.gitwave/AI.md`(约定唯一路径,无则跳过),经 `scrub_secrets` 后以 "Repository AI rules" 追加进所有 AI 调用的 system prompt(commit / conflict / PR / history / palette 全覆盖);AiProviderSettings 检测到文件时显示提示 chip | 有 / 无规则文件的 prompt 组装单测;离线守卫不受影响 |
| prompt 模板收尾 | `pr` 消费方随 M1 接入;三 textarea 补可用变量说明文案;空串视为「用默认模板」 | 模板覆盖与默认回退各有单测 |

依赖:无。前置给:M1 全部。

### M1 · AI 进阶(工作量:L)

现状:AI 面板仅 commit message(staged diff + 风格参考)与 conflict explain 两处;
无 command palette(`lib/palette.ts` 是颜色主题,非命令面板);
CommitInfoHeader 操作区(revert / cherry-pick / tag)已就绪可挂新入口。

| 项 | 范围 | 验收标准 |
|---|---|---|
| AI PR 描述 | `use_cases::generate_pr_description(workspace_id, base)`:HEAD..base 提交列表 + 合并 diff(复用 12k `append_diff_patch` 预算)+ 最近消息风格参考;system 用 `prompt_templates.pr` 或默认;新命令 + api.ts + Modal(可编辑 title / body + Copy) | 端到端生成可编辑;diff 预算截断有界;模板覆盖生效 |
| AI history 解释 | `use_cases::explain_commit(workspace_id, sha)`:CommitDetails + `diff_commit_vs_parent`(同一 diff 预算)→ 自然语言;内置默认 prompt(三套模板之外);CommitInfoHeader 操作区加 "AI Explain" → 结果 Modal | 端到端可用;离线被 `ai_offline` 拦截 |
| AI command palette | 后端 `cmd_ai_palette_intent(query, context 快照)`:LLM 受限输出 JSON `{action, params, explanation}` + 容错解析(非法 JSON 拒绝),走 failover 链 + rules 注入;白名单三级:只读立即执行 / AI 生成器(commit message 预填、explain commit)/ 需确认(建分支 / checkout / tag / stash / fetch);commit / push / merge / rebase 服务端拒绝;前端 `CommandPalette`(Cmd+K / Ctrl+K 全局监听,对齐 Toolbar Ctrl+, 模式;静态命令 + "Ask AI" 区;动作卡 Confirm / Cancel) | 非法 JSON 拒绝;白名单过滤单测;确认门控实操可用 |

依赖:M0(failover 链 + rules 注入)。验收对照完成定义「AI 进阶能力端到端可用」。

### M2 · Git 扩展(工作量:L)

现状:LFS / hooks / reflog 全仓 0 实现(git/mod.rs 仅有「hooks 不自动执行」设计注记);
submodule 有 list / init / update(`submodule.rs` + `SubmodulesPanel` 已挂侧栏),
缺 add / update --recursive / deinit / status;侧栏 Reflog 区不存在,复用 TagsPanel 模式。

| 项 | 范围 | 验收标准 |
|---|---|---|
| Git LFS | `infrastructure/git/lfs.rs` CLI 封装(全部经 `hidden_command`;git2 不支持 LFS):`git lfs version` 探测、install、track / untrack(直接操作 `.gitattributes` 保幂等)、tracked patterns 列表;ActionBar Repository 组 "LFS" → Modal(安装状态 + pattern 增删) | 未装 git lfs 时 UI 明确引导而非裸报错;`.gitattributes` 读写幂等单测 |
| 完整 submodule | 扩展 `submodule.rs` + `SubmodulesPanel`:add(URL / path)、update --recursive、deinit、status 同步标记 | fixture add / update roundtrip 单测;面板可完成全部操作 |
| reflog 浏览器 | `infrastructure/git/reflog.rs`(git2 reflog:旧 / 新 oid、消息、操作人、时间);侧栏 "Reflog" SidebarSection(复用 TagsPanel 模式);点击复用 `commitLocate` seq 模式在 CommitGraph 定位 | fixture 提交后条目断言;空仓友好空态;仅浏览,不执行恢复(v0.3 M2 做 AI 语义化) |
| hooks 编辑器 | `infrastructure/git/hooks.rs`:list `.git/hooks`(存在 / 可执行标记,Windows 无 exec 位)、读 / 写 hook 内容(路径守卫同 `ignore_path`)、unix chmod +x;Repository 组 "Hooks" → Modal 左列表右 Textarea + 常用模板(pre-commit 等) | 读写 roundtrip 单测;P1 注记:只编辑不执行 hooks |

依赖:无(与 M0 / M1 并行)。

### M3 · Windows 适配 + 版本(工作量:S)

现状:基础已就绪——NSIS-only 打包、`hidden_command`(CREATE_NO_WINDOW)、CI
lint / test / build 三矩阵均含 windows-latest、跨平台 keyring、`open_data_dir`
(OpenerExt 绕 ACL)、macOS-only 代码均 cfg 门控。遗留:`credentials.rs` 自述
`SSH_AUTH_SOCK` 假设偏 Unix;`ssh-add` stdin 为 null,带 passphrase 的 key 必然
失败且报错费解。

| 项 | 范围 | 验收标准 |
|---|---|---|
| SSH Windows 路径 | 补 Windows named pipe(`\\.\pipe\openssh-ssh-agent`)说明 / 回退;ssh-add passphrase 失败的报错文案改善 | Windows 上 SSH list / add / test 行为明确;错误信息可读 |
| 路径与全量验证 | worktree 路径拼接 / 分隔符 / 长路径核查(重点 fs 写入点);cargo test --all-targets、fmt、clippy `-D warnings`、npm typecheck / lint / test / build 全量跑 | 本机(Win)全绿;macOS 侧依赖 CI 矩阵 |
| 版本与升级 | tauri.conf.json / Cargo.toml / package.json 三处 0.1.0 → 0.2.0;新 settings 字段全部 serde default | 0.1 配置文件在新版直接可用(平滑升级) |

依赖:无。完成定义「macOS + Windows 同时可用」「v0.1 全量在 Win 跑通」的落点。

## 3. 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| failover 改 settings 结构 | 旧数据不兼容 | 主字段 `ai_provider` 保持链首;新字段 serde default + roundtrip 单测(与 v0.3 M0 同款缓解) |
| palette LLM JSON 输出不稳 | 误执行 / 不可用 | 容错解析拒绝非法输出;白名单服务端过滤;一切变更需用户确认 |
| git lfs 未安装 | LFS 功能不可用 | version 探测守卫;UI 引导安装而非裸报错 |
| submodule fixture 在 Windows 的复杂性 | 单测不稳 | 复用 test_helpers(autocrlf 已 pin);必要处以本地路径 fixture 规避网络克隆 |
| worktree 新环境冷构建 | 首次验证慢 | npm ci + cargo 首次全量构建一次到位 |
| 范围蔓延(想提前做 GitHub PR / 协作) | 版本失焦 | 决策 2 已锁;PR 随 v0.3 协作评估 |

## 4. 完成定义对照(02-scope §2.2)

| 完成定义 | 落点 |
|---|---|
| macOS + Windows 同时可用 | M3(本机 Win 全量验证 + CI macOS 矩阵) |
| v0.1 全量功能在 Win 上跑通 | M3(审计确认基础就绪 + SSH / 路径遗留修复) |
| AI 进阶能力端到端可用 | M1(M0 保障可靠性) |
| 用户能从 v0.1 平滑升级到 v0.2 | M3 版本号三处 + M0 / M2 新字段 serde default |
