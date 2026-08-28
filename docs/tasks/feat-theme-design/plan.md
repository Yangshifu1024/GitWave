# feat-theme-design · 主题设计落地（颜色 · 字体 · 动效）

> 分支：`feature/theme-design`（用户明确要求新分支；**不 commit、不 push**）  
> 设计依据：[`docs/design/07-theme.md`](../../design/07-theme.md)（含同类软件调研）  
> 范围约束：仅颜色 / 字体 / 动画 token，不涉及布局。

## 目标

1. 修复弹层动画整体失效（tailwindcss-animate 类在 Tailwind v4 下无来源）
2. 建立动效 token 体系（duration / ease）并统一组件散写的时长
3. diff 配色 token 化（GitHub diffBlob 模式）
4. dark 模式材质层次修复（panel 与窗口分层）
5. 字体栈平台化（SF Pro / Segoe UI 优先，Roboto 兜底，中文回退必含）

## 实施步骤

| # | 文件 | 改动 | 状态 |
|---|---|---|---|
| 1 | `src/styles/tokens.css` | 字体栈 / motion token / diff token / dark panel / animate-in/out @utility / reduced-motion / tabular-nums | ✅ |
| 2 | `src/components/ui/Modal.tsx` | 移除 v3 slide hack；duration-base | ✅ |
| 3 | `src/components/ui/Toast.tsx` | duration token 化 | ✅ |
| 4 | `src/components/ui/DropdownMenu.tsx` | 补 content 进出场类；item duration-fast | ✅ |
| 5 | `src/components/ui/ContextMenu.tsx` / `Tooltip.tsx` | item duration-fast | ✅ |
| 6 | `src/components/ui/Tabs.tsx` | trigger duration-base | ✅ |
| 7 | `src/components/ui/Button.tsx` / `Input.tsx` | duration-base | ✅ |
| 8 | `src/components/DiffViewer.tsx` | diff token + 行文字恢复 text-primary | ✅ |
| 9 | `src/main.tsx` | Roboto 导入注释 | ✅ |
| 10 | `docs/design/01-tokens.md` / `06-color-palettes.md` | token 现状同步 | ✅ |

## 验证

- `pnpm typecheck`、`pnpm build`、`pnpm test`
- 构建产物 CSS 中确认 `animate-in` 等类已生成
- 审查报告：`review.md`

## 不做

- 不改 palette 体系 / accent / lanes / 语义色
- 不改布局、间距、组件形态
- 不 commit / push（AI 代理禁止 + 用户明确要求）
