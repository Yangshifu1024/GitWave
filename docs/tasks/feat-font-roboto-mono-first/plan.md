# feat: 移除 Roboto，Roboto Mono 成为 mono 栈首选

状态：已实现

## 需求来源

用户 2026-08-28：移除 Roboto，保留 Roboto Mono 并作为首选字体。经确认范围为**仅 mono 栈首选**（UI 文字仍用平台字体）。分支：`feature/theme-design`（沿用）。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 移除范围 | `@fontsource/roboto` 依赖卸载 + main.tsx 三个 face 导入删除 + `--font-sans` 去掉 Roboto | bundle 减小；sans 仍平台字体打头（Segoe UI / SF Pro），中文回退不变 |
| mono 首选 | `--font-mono` 改为 "Roboto Mono", "Cascadia Mono", "SF Mono", Menlo, Consolas, CJK… | Roboto Mono 已打包，跨平台等宽一致；Cascadia 降为回退 |
| 文档同步 | `docs/design/07-theme.md` §3.2 字体栈与说明更新 | 07 文档曾与 tokens 同步提交，保持一致 |

## 改动清单

- `src/styles/tokens.css`：两个字体栈 + 注释
- `src/main.tsx`：仅保留 roboto-mono 400/500 导入
- `package.json` / `package-lock.json`：卸载 `@fontsource/roboto`
- `docs/design/07-theme.md` §3.2

## 验证

- 残留检查：src / tokens / package.json 无 Roboto（sans）引用
- `npm run build` / typecheck / eslint / vitest 43 全绿
- 真机：diff / commit message / sha 等等宽场景应显示 Roboto Mono；UI 文字仍为 Segoe UI
