# feat-theme-palettes · 双配色体系 + macOS 原生蓝默认

> 状态：实施完成（待 code review）
> 需求（用户，2026-08-27）：保留现有主题颜色；新增一套 macOS 原生蓝配色；默认使用原生蓝；在设置页面可选择切换。
> 分支：`feature/ui-native-studio-v2`

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| palette 范围 | 整套 macOS 系统风 | 用户选定：除 accent 外画布/侧栏/悬浮层也向系统窗口灰阶靠拢（方案 B）|
| 设置入口 | Toolbar ⋯ → Settings… 模态 | 与 SSH Keys modal 同模式；后续设置项可在此扩展分区 |
| 默认值 | native-blue | 无 localStorage 偏好（存量与新用户）一律回落 native-blue，无迁移脚本 |
| 与 light/dark 关系 | 正交维度 | palette 用 `<html data-palette>`；明暗沿用 `.light/.dark` + `prefers-color-scheme` |
| 原则冲突处理 | 「不用系统蓝」为显式例外 | 已更新 `00-overview.md` / `06-color-palettes.md` 并注明拍板日期 |

## 改动清单

### 代码

- `src/styles/tokens.css` — 重构：Native Blue 成为默认基座（light/dark/media 三形态）；Tide Studio 迁移为 `html[data-palette="tide"]` 覆盖块（light/dark/media 三形态）；新增 `--color-lane-1..5`（置于 `@layer base :root`，避开 Tailwind v4 对未被 utility 引用的 theme 变量的 prune）与 `--color-scrollbar-track` 绑定 `var(--color-bg-secondary)`
- `src/components/CommitGraph.tsx` — LANE_COLORS 全部改引 lane 变量（原两个硬编码色 `#4a5fa8`/`#5b56a8` 入 token）
- `src/lib/palette.ts` — 新增：palette 常量/normalize/localStorage 读写/启动应用；swatch meta 供设置 UI
- `src/hooks/usePalette.ts` — 新增：组件态同步（仿 useTheme）
- `src/main.tsx` — 启动预热扩展为 theme + palette
- `src/components/SettingsModal.tsx` — 新增：Settings 模态（Appearance → Color palette 单选卡片，即点即生效）
- `src/components/Toolbar.tsx` — ⋯ 菜单新增 "Settings…" 项（SlidersHorizontal 图标）
- `src/lib/palette.test.ts` — 新增：normalize 回落 / 注册表完整性

### 文档

- `docs/design/01-tokens.md` §1 重写（双 palette + lane token + 运行时机制）
- `docs/design/00-overview.md`（IA 决策行 / 视觉基调色板行 / 焦点态措辞）
- `docs/design/06-color-palettes.md`(状态头/选型原则例外/K·Native Blue 章节)

## 关键值（Native Blue）

- Light：canvas `#ECECEC` · sidebar `#DFDFDF` · elevated `#F4F4F5` · ink `#1B1B1D` · accent `#007AFF`
- Dark：canvas `#262628` · sidebar `#202022` · elevated `#313134` · ink `#EFEFF1` · accent `#0A84FF`
- 共享语义色不变(success/warning/danger/info/status/ahead/behind/conflict/branch-remote);branch-local/current = accent
- Lanes = Apple system colors: blue/teal/indigo/purple/gray(成对值见 06 文档 K 章节)

## CSS 层叠设计

1. `@theme`(layer theme)= native-blue light 基座;`:root.dark` 与 media fallback(layer base)覆盖 dark
2. tide 覆盖块放 dark 块之后,组合选择器特异性压过 `:root.dark`:
   - `html[data-palette="tide"]` (0,1,1) > `:root` (0,1,0)
   - `html.dark[data-palette="tide"]` (0,2,1) > `:root.dark` (0,2,0)
   - media 内 `html:not(.light)[data-palette="tide"]` (0,2,1) 同理
3. JS 保证启动前(`applyInitialPreferences`)同时写入 theme class 与 data-palette,消除 FOUC;CSS 兜底(media 无属性时= native dark)

## 验收

- [x] `npm run typecheck` 通过
- [x] `npm test` 24/24(含新增 5 个 palette 测试)
- [x] `npm run lint` 通过
- [x] `npm run build` 通过;产物 CSS 含四形态 lane 变量、tide override ×3、native accent
- [ ] 手动冒烟(dev 运行):默认呈现原生蓝;Settings 切 Tide 即时变色并重启保留;dark/system 组合正确 —— GUI 自动化授权缺失,由用户目验
- [x] code-reviewer 审查通过(review.md):无 🔴 阻塞项,结论「可以合入」;🟡 建议(tide swatch lane 色对齐落地 token、test 文件格式)已修复
