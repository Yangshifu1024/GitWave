# GitWave · 主题设计（颜色 · 字体 · 动效）

> 状态：**已实施**（2026-08-28，分支 `feature/theme-design`）  
> 范围：**仅**颜色 / 字体 / 动画（含阴影材质 token）。**不涉及布局**——布局与组件形态见 [`05-visual-redesign.md`](./05-visual-redesign.md)。  
> Palette 体系（native-blue 默认 / tide 可选）沿用 2026-08-27 拍板结论，accent / lanes / 语义色签名全部不动。

---

## 1. 调研：同类软件与工具类软件的主题设计

### 1.1 Git 客户端

| 产品 | 主题策略 | 对 GitWave 的启示 |
|---|---|---|
| **GitKraken** | dark-first；签名 teal accent（品牌色即主题色）；Light / Dark / Custom 主题体系（custom themes 自 11.8.0 起暂停，UI 现代化中） | 主题是产品识别的一部分；GitWave 的 tide palette 同路线 |
| **Sublime Merge** | 仅内置 Light / Dark，theme 绑定 color scheme；性能极致取向，动效近乎即时；内容以 mono 为主 | 工具类动效必须「快」；SHA / 路径 / diff 用 mono |
| **Fork / Tower** | macOS / Windows 双平台原生；系统字体（SF Pro / Segoe UI）；信息密度高；Light/Dark 跟随系统；动效克制 | 原生质感来自「系统字体 + 克制的中性色分层」 |
| **GitHub Desktop**（Primer） | 语义 token 体系；diff 有专属 token（`diffBlob` addition / deletion，light `#E6FFEC` / `#FFEBE9`，dark 为 muted 等价）；系统性对比度审查 | diff 配色应独立成 token，只染底不染字 |
| **lazygit** | 终端 ANSI palette 主题；默认用标准色名（浅色终端冲突是长期 issue）；active border 强调焦点 | lane / 分支色作为签名维度；焦点状态要明确 |

来源：

- GitKraken Themes — <https://help.gitkraken.com/gitkraken-desktop/themes/>
- Sublime Merge theme / color scheme 讨论 — <https://forum.sublimetext.com/t/built-in-support-for-global-text-merge-theme-and-syntax-color-schemes/60274>
- Fork — <https://git-fork.com/> ；Tower — <https://www.git-tower.com/>
- Primer Theme Reference（diffBlob token）— <https://primer.style/product/getting-started/react/theme-reference/>
- primer/primitives（light/dark token 源）— <https://github.com/primer/primitives>
- Primer 色彩使用指南 — <https://primer.style/product/getting-started/foundations/color-usage/>
- lazygit 配置 / 主题 — <https://lazygit.dev/docs/configuration/>；浅色主题问题 — <https://github.com/jesseduffield/lazygit/issues/1339>

### 1.2 动效规范（设计系统共识）

- **Material 3**：duration token 分 short（50–200ms）/ medium（250–400ms）/ long / extra-long；曲线 emphasized `cubic-bezier(0.2, 0, 0, 1)`、decelerate `cubic-bezier(0.05, 0.7, 0.1, 1)`（进入）、accelerate `cubic-bezier(0.3, 0, 0.8, 0.15)`（退出）— <https://m3.material.io/styles/motion/easing-and-duration/tokens-specs>
- **Fluent 2**（Windows 原生参照）：durationFast / Normal / Slow / Gentle ≈ 100–300ms，standard / decelerate / accelerate 曲线 — <https://fluent2.microsoft.design/motion> ；<https://learn.microsoft.com/en-us/windows/apps/design/motion/timing-and-easing>
- **共同结论**：微交互（hover / 色彩）≈ 100–150ms；浮层进出场 150–200ms（菜单偏快、Modal 偏慢）；进入用 decelerate（ease-out）、退出用 accelerate（ease-in）；全部要求 `prefers-reduced-motion` 降级
- **Material dark theme 指南**：dark 不是反色，表面按层级抬升 — <https://m2.material.io/design/color/dark-theme.html>

### 1.3 字体

- 原生阵营（Fork / Tower / GitHub Desktop）一律**系统字体栈**：macOS SF Pro、Windows Segoe UI Variable / Segoe UI
- 跨平台打包字体（Roboto，GitKraken / Chromium 系）换来一致性但牺牲原生质感
- mono 用于 SHA / 路径 / diff，优先平台自带：SF Mono（macOS）、Cascadia Mono（Windows 11 随 Windows Terminal 分发）、Consolas（Windows 兜底）

---

## 2. GitWave 现状对照（调研发现的问题）

| # | 问题 | 细节 |
|---|---|---|
| 1 | **弹层动画整体失效** | `animate-in / fade-in-0 / zoom-in-95 / slide-in-from-*` 是 tailwindcss-animate（Tailwind v3 生态）的类；本项目 Tailwind v4 且未装任何 animate 插件 → Modal / Tooltip / ContextMenu / Toast / Tabs 的进出场类**不生成任何 CSS**，弹层瞬现瞬失 |
| 2 | **dark 材质层次消失** | light 三层分离（panel `#F8F8F8` > 窗口 `#ECECEE` > chrome `#DFDFDF`）；dark 下 panel == 窗口 == `#262628`，三栏糊成一片 |
| 3 | **diff 配色非 token 化** | `bg-success/10` 半透明 tint 随底层表面偏色；整行 `text-success` 染色降低代码可读性（GitHub 模式是只染底不染字） |
| 4 | **动效无 token 体系** | 只有 `--duration-fast`；组件散写 120 / 150 / 200ms；无 easing 约定；无 reduced-motion 支持 |
| 5 | **字体栈与原生方向不符** | Roboto 全平台打包且优先——05 文档 §5 已写明「SF Pro Text / system」方向；Windows 无 Roboto 时才偶然回落 Segoe UI |

---

## 3. 设计决策

### 3.1 颜色

**不动**：双 palette 的全部中性色、accent、lanes、语义色、交互态（hover / 选中）、阴影。

**改动一 · 新增 diff 语义 token**（GitHub `diffBlob` 模式，light 用实体浅 tint、dark 用语义色透明 tint）：

| Token | Light | Dark |
|---|---|---|
| `--color-diff-add-bg` | `#E6F2EB` | `rgb(62 207 142 / 0.14)`（= dark success 14%） |
| `--color-diff-add-word` | `#BFE0CE` | `rgb(62 207 142 / 0.30)` |
| `--color-diff-del-bg` | `#F7E9E8` | `rgb(224 90 90 / 0.14)`（= dark danger 14%） |
| `--color-diff-del-word` | `#EFC6C4` | `rgb(224 90 90 / 0.30)` |
| `--color-diff-hunk-bg` | `#F0F0F3` | `rgb(255 255 255 / 0.05)` |

diff 行文字恢复 `text-primary`；仅 `+` / `-` 前缀与 word-diff span 保留语义色。

**改动二 · dark 材质层次修复**（唯一中性色改动，两 palette 同规则——panel 提到 primary 与 elevated 之间）：

```
native dark:  secondary #202022 < primary #262628 < panel #2A2A2D < elevated #313134
tide dark:    #12161A < #161B20 < panel #191F25 < #1C2329
light：全部不变（层次已成立）
```

滚动条 track 跟随面板的既有机制自动生效。

### 3.2 字体

sans 平台优先 + 中文回退（必含）；mono 以打包的 Roboto Mono 首选（跨平台一致）：

```
font-sans:
  -apple-system, BlinkMacSystemFont, "SF Pro Text",
  "Segoe UI Variable Text", "Segoe UI",
  "Helvetica Neue", Arial,
  "Microsoft YaHei", "PingFang SC", "Noto Sans CJK SC", sans-serif

font-mono:
  "Roboto Mono", "Cascadia Mono", "SF Mono", Menlo, Consolas,
  "Microsoft YaHei", "PingFang SC", "Noto Sans CJK SC", monospace
```

- `@fontsource/roboto` 已移除（Roboto 不再进任何字体栈）
- `@fontsource/roboto-mono` 打包保留（mono 首选，保证跨平台一致）
- 全局 `font-variant-numeric: tabular-nums`（ahead / behind 计数、行号对齐）

**用户字体覆盖**（F006）：设置中可分别配置 UI / Mono 字体，作为字体链首选，保留上述回退链（含 CJK）。实现：默认链声明为 `:root` 上的 `--font-sans-fallback` / `--font-mono-fallback`（`@theme` 外，避免 Tailwind 裁剪），`--font-sans` / `--font-mono` 引用它们；用户设置由 `src/lib/fonts.ts` 以 inline style 写 `"<用户字体>", var(--font-*-fallback)` 覆盖 html 上的主变量，localStorage 持久化，挂载前应用防 FOUC。

### 3.3 动效

**Token**（Tailwind v4 命名空间，自动生成 `duration-*` / `ease-*` 工具类）：

| Token | 值 | 用途 |
|---|---|---|
| `--duration-fast` | `120ms` | hover / 色彩过渡 / 菜单 |
| `--duration-base` | `200ms` | 控件 / Modal / Tab 切换 |
| `--duration-slow` | `300ms` | 大面积切换（预留） |
| `--ease-standard` | `cubic-bezier(0.2, 0, 0, 1)` | 过渡默认（M3 emphasized） |
| `--ease-enter` | `cubic-bezier(0, 0, 0.2, 1)` | 浮层进入（decelerate） |
| `--ease-exit` | `cubic-bezier(0.3, 0, 0.8, 0.15)` | 浮层退出（accelerate） |

**自实现 tailwindcss-animate 子集**（不新增依赖，符合本地优先）：

- `@utility animate-in / animate-out` + 变量修饰类：`fade-in-0` `fade-out-0` `fade-out-80` `zoom-in-95` `zoom-out-95` `slide-in-from-right-full` `slide-out-to-right-full`
- keyframes 用**独立变换属性** `translate` / `scale`（不用 `transform`），避免与 Tailwind v4 的 `translate` 工具类（Modal 居中定位）叠加冲突；duration 跟随 `--tw-duration`（`duration-*` 类可覆盖），默认 `--duration-fast`
- Modal 移除 v3 时代的 `slide-from-left-1/2` 居中 hack（v4 下语义失效）
- 修复后生效的动画：Modal / Overlay fade + zoom；Tooltip / ContextMenu / DropdownMenu（补上类）fade + zoom；Toast 右滑入出 + fade；Tabs 内容 fade
- `prefers-reduced-motion: reduce` → 全局动画 / 过渡即时完成

---

## 4. 实施对照

| 文件 | 改动 |
|---|---|
| `src/styles/tokens.css` | 字体栈、motion token、diff token、dark panel、animate-in/out 工具类、reduced-motion、tabular-nums |
| `src/components/ui/Modal.tsx` | 移除 v3 slide hack 类；`duration-base` |
| `src/components/ui/Toast.tsx` | duration token 化（动画类经工具类修复后自然生效） |
| `src/components/ui/DropdownMenu.tsx` | 补 content 进出场类；item `duration-fast` |
| `src/components/ui/ContextMenu.tsx` / `Tooltip.tsx` | item `duration-fast`（动画类经工具类修复后生效） |
| `src/components/ui/Tabs.tsx` | trigger `duration-base` |
| `src/components/ui/Button.tsx` / `Input.tsx` | `duration-base` |
| `src/components/DiffViewer.tsx` | 改用 diff token；行文字恢复 `text-primary` |
| `src/main.tsx` | Roboto 导入注释（Linux 兜底语义） |
| `docs/design/01-tokens.md` · `06-color-palettes.md` | 同步 token 现状 |

任务执行记录见 [`docs/tasks/feat-theme-design/plan.md`](../tasks/feat-theme-design/plan.md)。

## 5. 关联

- [`01-tokens.md`](./01-tokens.md) — token 落地规范（§1.5 diff 色、§6 Motion 本次同步）
- [`05-visual-redesign.md`](./05-visual-redesign.md) — 布局与组件形态（本次不变）
- [`06-color-palettes.md`](./06-color-palettes.md) — palette 候选与已落地记录
