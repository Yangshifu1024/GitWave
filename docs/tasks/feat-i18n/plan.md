# feat-i18n · 完整国际化实施计划

> 关联：[F010-i18n](../../pm/features/F010-i18n.md) · [ADR-0006](../../tech/decisions/00-overview.md)
> 分支：`feature/i18n` · 影响：v0.6.0

## 范围

1. 前端 UI 全量双语（zh-CN / en）：组件 + 菜单常量表 + hooks/stores 状态消息
2. Settings → General 两个语言选项：UI language（中/英，即时生效）、AI language（中/日/韩/英）
3. Rust 错误 key 化：`AppError` 增 code + params，前端按 code 翻译
4. AI 回复语言：AI 命令加 `language` 参数，prompt 追加回复语言指令
5. `README.zh-CN.md` + site 中文页按浏览器语言自动适应

## 前端架构

```
src/i18n/
  index.ts          # i18next 初始化 + 语言解析 + html lang 同步
  resources.ts      # 汇总各域 JSON 为 resources
  locales/
    en/<域>.json
    zh-CN/<域>.json
```

域文件清单：`common` / `app` / `menu` / `palette` / `workspace` / `changes` / `commits` / `branches` / `conflicts` / `remotes` / `submodules` / `health` / `settings` / `ssh` / `ai` / `updater` / `status` / `errors`。

## Key 命名约定（迁移必须遵守）

- key 结构 `<域>.<名词>.<描述>`，camelCase：`changes.panel.stageAll`
- 插值 i18next 默认语法：`t("status.checkedOut", { name })` ↔ JSON `"Checked out {{name}}"`
- UI store / hooks 中的状态消息同样 key 化，t() 在**渲染点或消息生成点**调用（statusArea 消息在生成时定稿，语言切换不回放历史消息，属可接受行为）
- en JSON 是 key 的**事实来源**；zh-CN 必须逐 key 对齐（parity 测试强制）
- 不翻译：品牌名 GitWave、字体预览样例串（`FONT_PREVIEW_SAMPLE`）、AI prompt 主体
- 日期格式化用 `i18n.language` 替代硬编码 `en-US`（BlameView / CommitInfoHeader）
- `aria-label` / `title` / `placeholder` 等无障碍与提示属性一律翻译

## 中文术语表

- 译：提交 commit · 拉取 pull · 推送 push · 获取 fetch · 合并 merge · 变基 rebase · 分支 branch · 标签 tag · 冲突 conflict · 暂存 stage · 贮藏 stash · 检出 checkout / 工作区 workspace · 仓库 repo / 远程 remote / 子模块 submodule
- 保留英文：GitWave（品牌）、worktree、LFS、hook、diff、blame、SSH、URL、AI、PR、commit message（作名词短语时）、shallow / detached HEAD 等状态词可译（游离 HEAD）
- 措辞风格：简洁命令式（按钮「新建」非「新建一个」）；英文提示句以句号结尾，中文以句号结尾

## 错误 key 化约定

- code 为 dot.case 双段以上：`workspace.name_empty` / `rebase.conflict` / `git.raw`
- `src-tauri/src/domain/error_codes/` 模块常量单一来源（按区域分文件：usecases / git / infra / cmds）；`AppError` 为 struct 变体 `{code, message, params}`，序列化恒含 `code`、空 `params` 省略
- 英文原文保留为 `message` 字段作 fallback；前端 `formatAppError`：有 code → `t("errors.<code 各段>", params)`，无 → `category: message`
- locale errors 按区域拆文件（`errors-usecases.json` / `errors-git.json` / `errors-infra.json` / `errors-cmds.json`），resources.ts 深合并为单一 `errors` 子树，叶子为纯字符串展示文案
- vitest parity 测试：en/zh-CN 全域 key 集合一致；`error_codes.rs` 常量 ↔ 两 locale `errors` 组双向覆盖

## AI 语言约定

- 可接受值：`zh` / `ja` / `ko` / `en`；Rust 侧 sanitize，其余视为 None
- None：prompt 不变（现状）；有值：system prompt 尾部追加 `Always respond in <Language> (<native name>).`
- 前端 localStorage key：`gitwave.aiLanguage`；UI 语言 localStorage key：`gitwave.language`
- `cmd_ai_palette_intent` 不加语言参数（意图解析无散文回复）

## 批次

| # | 内容 | 状态 |
|---|---|---|
| 1 | F010 + ADR-0006 + 本计划 | ✅ |
| 2 | i18n 基础设施 + General 两语言选择器 + parity 测试 | ✅ |
| 3 | 菜单链路（appMenuSpec / nativeMenuBuild / CommandPalette）| ✅ |
| 4 | ui/ 基础组件迁移 | ✅ |
| 5 | 业务组件迁移（34 个 .tsx） | ✅ |
| 6 | hooks/stores 状态消息（61 处） | ✅ |
| 7 | 日期 locale 收尾（BlameView / CommitInfoHeader / DiffViewer） | ✅ |
| 8 | Rust AI 语言注入 | ✅ |
| 9 | Rust 错误 key 化（172 处构造点 / 168 常量） | ✅ |
| 10 | formatAppError + errors 译文 | ✅ |
| 11 | README.zh-CN + site 中文页 | ✅ |
| 12 | 全量验证 | ✅ |
| 13 | code-reviewer 审查 → review.md（🔴 0 / 🟡 5 已修 / 🟢 记录） | ✅ |

## 验收清单（用户手动）

- [ ] 切换 UI 语言：界面 / 菜单 / 命令面板 / toast / 错误提示全部即时切换，重启后保持
- [ ] 首次启动无设置时跟随系统语言
- [ ] AI language 设为中文：commit message / PR 描述 / Explain / 冲突解释以中文回复；设回英文恢复
- [ ] Rust 错误（如推拉失败）在中文界面显示中文
- [ ] 官网 gitwave.work 中文浏览器自动显示中文页
