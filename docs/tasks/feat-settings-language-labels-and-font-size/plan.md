# feat: 设置界面语言下拉框补 label + 界面/等宽字体大小分别设置

## 目标

1. 设置 → 通用：界面语言、AI 语言两个下拉框补可见 label（当前只有 `aria-label`，无可见文字）。
2. 设置 → 外观 → 字体：新增两个独立的字体大小设置——界面字体大小（全局等比缩放，12–20，默认 16）与等宽字体大小（仅缩放 mono 文本，10–20，默认 12）；大小输入框与字体输入框同行右侧；删 Save 按钮，输入框回车或失焦即生效，可重置。

## 现状与根因

- `SettingsModal.tsx` `LanguageSection`：两个 `<Select>` 只设了 `aria-label`；i18n key（`settings.general.uiLanguage` / `aiLanguage`）早已存在，只是未渲染。
- 项目此前无任何字号机制：文字全走 Tailwind rem 基工具类，html 无显式 font-size（WebView 默认 16px）。

## 技术方案

### 界面字体大小（root font-size 全局缩放）

`documentElement.style.fontSize = Npx`。Tailwind v4 文字/间距/控件尺寸全 rem 基，等比缩放；`text-[10px]/[11px]` 微标签保持像素。清空 = `removeProperty` 恢复 WebView 默认 16px。

### 等宽字体大小（`--font-mono-scale`，仅 mono 文本）

新增 CSS 变量，运行时设 `scale = 值 / 12`（12 为 mono 主导字号 text-xs 的默认值）。`tokens.css` 末尾 unlayered 枚举代码库中实际存在的 mono+字号组合（grep 确认仅 4 种）：

```css
.font-mono.text-xs { font-size: calc(0.75rem * var(--font-mono-scale, 1)); }
.font-mono.text-sm { font-size: calc(0.875rem * var(--font-mono-scale, 1)); }
.font-mono.text-\[10px\] { font-size: calc(10px * var(--font-mono-scale, 1)); }
.font-mono.text-\[11px\] { font-size: calc(11px * var(--font-mono-scale, 1)); }
```

rem 基使 mono 随界面缩放再叠加自身系数；8 处不带字号类的嵌入式 mono（sha/URL 等）随父级字号走。维护约定：新增 mono+字号组合需同步补规则。

### 存储（复刻 fonts 模式）

`FontPreferences` 扩展 `sansSize` / `monoSize`（string，"" = 默认）；localStorage keys `gitwave-font-sans-size` / `gitwave-font-mono-size`；`sanitizeSizeInput(input, min, max)`：parseInt、clamp、空/非法 → ""。`applyInitialFonts()` 挂载前应用防 FOUC（main.tsx 已调用，无需改）。

### 交互（无 Save）

- 字体区 4 字段共用 draft；任一输入框回车或失焦提交：`saveFonts(draft)`（sanitize + 持久化 + 即时应用），sanitized 值回写 draft。
- 守卫：sanitized 后与已存值相同则跳过（不重写 localStorage、不闪「已保存」）。
- 保留「已保存——全局生效」2 秒反馈；删除 dirty 与 Save 按钮。
- 「重置」（清字体）/「重置大小」（回默认）仅非默认时显示，点击立即提交。
- 预览样张按 draft 字号实时渲染（display-only）。

## 布局

每行 = 字体输入框（flex-1，label + hint）+ 大小输入框（定宽容器 `w-24`，label「大小」，`inputMode="numeric"`，placeholder = 默认值）。重置按钮图标化（`RotateCcw`，PathInput 同款样式）放进各自输入框的 suffix 插槽：字体重置在字体输入框尾部，`px` 单位与「重置大小」图标在大小输入框尾部；均仅非默认时显示。预览行只留样张，按 draft 字号实时渲染。

注意（布局三个坑，均已修复）：① `InputGroup.Input` 原本只有 `flex-1` 无 `min-w-0`，窄容器里输入框按固有宽度（~170px）溢出边框、把 suffix 顶到外面——已在共享 `Input` 组件的内层 input 补 `min-w-0`（`Input.tsx`）；② `fullWidth` prop 给组加 `--full-width` 类（`w-full`），HeroUI 无 layer 样式压过 Tailwind 工具类，直接给 `Input` 传宽度类无效——宽度须经外层定宽容器（`w-28`）约束；③ 行内 flex 项是 `Input` 外层的 `TextField`，无 `flex-1` 时宽度取内容 max-content（被下方 hint 文字长度决定），须外包 `flex-1 min-w-0` 容器才能两行等宽。

## 改动清单

| 文件 | 内容 |
|---|---|
| `src/lib/fonts.ts` | size 字段、sanitize/apply/store 扩展 |
| `src/components/SettingsModal.tsx` | 语言 Label 包裹；字体区重构（Label 行、回车/失焦提交、预览字号联动） |
| `src/styles/tokens.css` | 4 条 mono 缩放规则 |
| `src/i18n/locales/{en,zh-CN}/settings.json` | `settings.appearance.size` / `resetSize` |
| `src/lib/fonts.test.ts` | `sanitizeSizeInput` 用例 |

## 验证

- `./node_modules/.bin/vitest run` 全量绿；lint 过。
- `make dev` 人工验收：label 显示；两个大小独立缩放且互不影响；回车/失焦/重置/越界 clamp；重启保持；启动无 FOUC。
