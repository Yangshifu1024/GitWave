# feat: 字体设置（UI 字体 + Mono 字体）

关联提案：[F006-font-settings](../../pm/features/F006-font-settings.md)

## 目标

Settings → Appearance 支持 UI 字体与 Mono 字体配置：自由文本输入 + 实时预览样张 + 显式 Save；保存后全局即时生效并持久化，重启保持；可重置默认。

## 技术方案

### 字体链拆分（tokens.css，单一事实来源不变）

`--font-sans` / `--font-mono`（`tokens.css:71-76`）已是全应用字体唯一入口：`body` 正文走 `var(--font-sans)`（L313），61 处 `font-mono` 工具类走 `var(--font-mono)`。为支持运行时覆盖，把现有两条链拆为兜底变量，主变量改为引用：

```css
:root { /* 放 @theme 外，沿用 L112 lane colors 先例，避免 Tailwind v4 裁剪未使用的变量 */
  --font-sans-fallback: -apple-system, …;
  --font-mono-fallback: "Roboto Mono", …;
}
@theme {
  --font-sans: var(--font-sans-fallback);
  --font-mono: var(--font-mono-fallback);
}
```

### 覆盖机制（inline style，项目首个先例）

用户可输入任意字体名，无法预声明 `data-*` CSS 块，改用 `documentElement.style.setProperty("--font-sans", '"用户字体", var(--font-sans-fallback)')`：

- inline 声明在 html 元素上覆盖 `:root` 样式表声明，后代全部继承；`var()` 在各元素计算值阶段解析，正文与 `font-mono` 工具类自动跟随
- 应用值引用 `-fallback` 变量而非 `--font-sans` 自身，无循环引用
- 字体名逐段包双引号（空格名安全）；清空设置时 `removeProperty` 恢复 tokens.css 默认链

### 存储与应用（纯前端，无后端改动）

外观偏好沿用主题/调色板模式：localStorage + 挂载前应用防 FOUC，不进 Rust/SQLite。

- **`src/lib/fonts.ts`（新，镜像 `palette.ts`）**：keys `gitwave-font-sans` / `gitwave-font-mono`（空串 = 默认，removeItem）；`sanitizeFontList()` 按逗号分段，剥离引号/反斜杠/分号/花括号/控制字符，空段丢弃，全空 = 默认；`readStoredFonts()` / `storeFonts()`（持久化 + 即时应用）/ `applyInitialFonts()`
- **`src/hooks/useFonts.ts`（新，镜像 `usePalette.ts`）**：`fonts` 状态 + `saveFonts()`；初始应用由 main.tsx 完成，hook 只管组件侧
- **`src/main.tsx`**：`applyInitialPreferences()` 末尾加 `applyInitialFonts()`

### 设置界面（SettingsModal · AppearanceSection）

Theme、Color palette 之下新增 Fonts 区块：

- 两行 HeroUI TextField（与 AiProviderSettings 输入用法一致）：**UI font** / **Mono font**，placeholder 显示默认首选（System / Roboto Mono）
- 每行下方预览样张（中英文 + 数字，如 `Aa Bb 0123 — 中文字体预览 0O1lI`），用草稿字体链渲染：输入即时变预览但不落盘；未安装字体静默回退，预览可直接看出（自由文本方案的已知取舍）
- 每行 Reset 按钮清空输入（= 默认值）
- 单个 **Save** 按钮：草稿与已存值一致时禁用；点击 `saveFonts()` 全局即时生效（弹窗背后可见），短暂 Saved 反馈后回禁用态；未保存关闭弹窗草稿丢弃

### 修改点

| 层 | 文件 | 改动 |
|---|---|---|
| token | `src/styles/tokens.css` | 字体链拆 `-fallback` 变量 + var() 引用 |
| lib | `src/lib/fonts.ts`（新） | 存储 / 消毒 / 应用 |
| 测试 | `src/lib/fonts.test.ts`（新） | sanitize + apply 值构造 |
| hook | `src/hooks/useFonts.ts`（新） | 组件侧读写 |
| 入口 | `src/main.tsx` | 预挂载 `applyInitialFonts()` |
| ui | `src/components/SettingsModal.tsx` | AppearanceSection Fonts 区块 |
| 设计文档 | `docs/design/07-theme.md` §3.2 | 补用户字体覆盖说明 |

### 不做（MVP 边界）

系统字体枚举下拉（需 Rust 跨平台字体枚举）、字号设置、per-workspace 字体、后端持久化。

## 验证

- `npm run typecheck && npm run lint && npm run test`
- 手动冒烟：Save 后正文与 mono 场景（diff / blame / 提交图 / 冲突编辑器）立即换字体；重启保持；清空 + Save 恢复默认；带空格字体名正确加引号；非法字符被剥离
