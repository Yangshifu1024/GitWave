# review: 字体设置（F006）

审查对象：`feat/font-settings` 分支 F006 范围内改动（不含同工作区 F005 文件）。
审查方式：code-reviewer 代理，7 维度（正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践）；级联结论对生产构建产物（`dist/assets/*.css`）核实。

## 范围

- 新增：`src/lib/fonts.ts`、`src/lib/fonts.test.ts`、`src/hooks/useFonts.ts`、`docs/pm/features/F006-font-settings.md`、本任务 plan
- 修改：`src/styles/tokens.css`（字体链拆 `-fallback` 变量）、`src/main.tsx`（挂载前应用）、`src/components/SettingsModal.tsx`（Fonts 区块）、`src/components/ui/Input.tsx`（新增可选 `label`/`description`，见下）、`docs/design/07-theme.md` §3.2

## 结论

**可合入**（无 🔴 问题）。

## 核实要点

- **消毒**：`style.setProperty` 以 name/value 分离传入，结构上无法注入额外声明；`FORBIDDEN_CHARS` 剥离可逃出双引号包裹的字符（`"` `'` `\` `;` `{}` `<>`），`\p{C}` 覆盖控制符 / 零宽格式符 / lone surrogate；放行字符（括号、`!` 等）经逐段双引号包裹后语法合法，仅表现为未安装字体静默回退。存储读回值二次消毒（幂等）作纵深防御
- **级联**：`--font-sans: var(--font-sans-fallback)` 在产物中唯一且覆盖 HeroUI 自带默认；HeroUI preflight 经 `--default-font-family: var(--font-sans)` 跟随；61 处 `font-mono` 工具类产物为 `font-family: var(--font-mono)`；`src/` 无硬编码 font-family
- **状态边界**：双击 Save 不可达（保存后草稿对齐消毒值 → 禁用）；justSaved 计时器由 effect cleanup 管理；草稿随组件卸载重置，不跨弹窗开合残留
- **FOUC**：`applyInitialPreferences()` 在 `createRoot` 前同步执行，与 theme/palette 既有取舍一致

## 发现与处理

| 级别 | 问题 | 处理 |
|---|---|---|
| 🟡 | FontField 可见 label 是无关联 `<span>`，读屏只有 placeholder；hint 未关联 | **已修复**：`ui/Input` 新增可选 `label` / `description` props（HeroUI TextField 组合式自动关联，对现有调用零影响），FontField 改用 |
| 🟡 | `previewFontFamily` 无测试；sanitize 不折叠段内连续空格（带引号名会匹配失败） | **已修复**：补 `previewFontFamily` 2 断言 + 空白折叠 1 断言（sanitize 中 `\s+ → " "`） |
| 🟡 | `applyFont` 直接解引用 `document`，与 `readStoredFont` 的 localStorage 守卫风格不一 | 接受现状（Tauri WebView 不触发，与 palette.ts 同风格） |
| 🟢 | `DEFAULT_FONT_LEADS.mono` 与 tokens.css 默认链首选存在手工同步 | **已修复**：补注释声明仅文案用途及同步目标 |
| 🟢 | `storeFonts` reset 路径（removeItem/removeProperty）无自动化覆盖 | 接受：项目无 jsdom，以 plan.md 手动冒烟清单兜底 |
| 🟢 | 多窗口 localStorage 不跨 WebView 传播（无 storage 事件） | 接受：与 palette 同限制，MVP 边界内 |
| 🟢 | 保存提示 2000ms 魔法数字 | 接受：单一使用点 |

## 验证记录

- `npm run typecheck` ✔
- `npm run lint` ✔（初版 `no-control-regex` 报错，已改用 `\p{C}`）
- `npm run test`：81 通过（含 fonts 13 用例）
- `npm run build` 产物核实：`--font-*-fallback` 正常输出（未被 Tailwind 裁剪）、`var()` 引用链完整
- 手动冒烟（待用户执行）：Save 后正文与 mono 场景即时换字体；重启保持；清空 + Save 恢复默认
