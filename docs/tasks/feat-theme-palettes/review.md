# feat-theme-palettes · Code Review

> 审查日期：2026-08-27
> 分支：`feature/ui-native-studio-v2`（未提交工作区改动，全部属同一 feature）
> 审查范围：`src/styles/tokens.css`、`src/components/CommitGraph.tsx`、`src/components/Toolbar.tsx`、`src/main.tsx`、新增 `src/lib/palette.ts(.test.ts)` / `src/hooks/usePalette.ts` / `src/components/SettingsModal.tsx`、docs/design 三份文档 + mockup + 本任务 plan.md
> 结论：**可以合入**（无 🔴 严重问题；两项 🟡 建议在 squash 提交前顺手处理）

## 复验记录（审查者本机执行）

| 检查 | 结果 |
|---|---|
| `npm run typecheck` | 通过 |
| `npm test` | 24/24（含新增 palette 5 例） |
| `npm run lint`（eslint） | 通过 |
| `npx prettier --check <改动文件>` | **palette.test.ts 未通过**（见 最佳实践-1） |
| `npm run build` | 通过；并对产物 `dist/assets/index-*.css` 做了逐项核查 |

产物 CSS 核查要点（层叠结论的实证依据）：

- 层声明顺序为 `@layer properties → theme → base → components → utilities`；`@theme` 编译进 `theme` 层（`:root,:host{...}`），用户 `@layer base` 追加到 `base` 层。**跨层胜负由层序决定：base 内任何规则都会压过 theme 层的同名变量，与特异性无关**；dark/tide 各形态之间的胜负才真正由「同处 base 层」的 特异性 + 源序 决定。
- 三个 tide 选择器均按预期输出：`html[data-palette=tide]`、`html.dark[data-palette=tide]`、media 内 `html:not(.light)[data-palette=tide]`。
- 每个 `--color-lane-N` 在产物中出现 6 处定义（native :root / native dark / native media / tide light / tide dark / tide media），完整未丢。
- `--color-scrollbar-track` 仅 `:root` 一处定义（绑定 `var(--color-bg-secondary)`），旧双主题显式值恰好等于各自 bg-secondary，行为等价。

---

## 一、正确性 ✅ 无 🔴

### CSS 层叠矩阵逐 case 推演（已对照产物确认源序）

| 场景 | 命中链 | 结果 |
|---|---|---|
| 显式 light（`.light`）+ 系统暗 + tide | media 块要求 `:not(.light)` 不命中；`html.dark[data-palette=tide]` 缺 `.dark` 不命中 → 仅 `html[data-palette=tide]` (0,1,1) | Tide light ✓ |
| 显式 dark（`.dark`）+ 系统亮 + tide | `html.dark[data-palette=tide]` (0,2,1) > `html[data-palette=tide]` (0,1,1)，也 > 基座 `:root.dark` (0,2,0) | Tide dark ✓ |
| 显式 dark + 系统暗同时成立 + tide | B 与 C 同特异性 (0,2,1)；C（media 块）源序在后胜出，但两者值完全相同 | Tide dark ✓（无风险） |
| follow-mode + 系统暗 + tide | C (0,2,1) > 基座 media `:root:not(.light)` (0,2,0) > A (0,1,1) | Tide dark ✓ |
| native-blue 显式 dark | `:root.dark` (0,2,0) > base `:root` (0,1,0) > theme 层基座 | Native dark ✓ |
| 共享语义色 / shadows / tide-dark 的 bg-overlay | tide 块刻意不声明 → 天然回落 `:root.dark` / media 的共享值 | 与文档 §1.3「= 共享」一致 ✓ |

tide 覆盖块放在全部 dark 块之后 + 组合选择器提升特异性的设计是正确且必要的；plan.md §CSS 层叠设计的特异性算式与实际一致。

### Tailwind v4 prune 规避（lane 变量）

- lane 变量置于 `@layer base :root` 属用户手写 CSS，构建管线原样透传，不受「@theme 变量仅在被 utility 引用时才输出」的默认裁剪影响——产物中 6 处定义齐全是硬证据。
- CommitGraph 以字面量 `var(--color-lane-1..5)` 用于 SVG `stroke/fill`（与改动前 `var(--color-accent)` 同机制）：palette 切换时只换 `<html>` 属性，SVG 即时重着色且无需 React 重渲染，这个取舍是对的。
- 对照组佐证裁剪机制真实存在：`text-text-muted` 编译为 `color:var(--color-text-muted)`（被引用故存活），而 `--duration-fast` 在产物中定义数为 0（详见 最佳实践-4 的存量问题）。
- scrollbar-track 绑定链正确：var() 在 computed-value 阶段解析，「赢家后的 bg-secondary」（含 dark / tide 各形态）自动传导给 track，dark/tide 均无需再显式声明。

### React 正确性

- `usePalette` 与 `useTheme` 结构同构（init 读存储 / setter 内持久化）；差异点（palette 在 setter 内同步 apply DOM，theme 经 effect apply class）已在 JSDoc 注明，属有意为之，无行为问题。
- `SettingsModal` 受控用法标准（`open/onOpenChange` 直通 radix Dialog.Root）；无条件挂载、closed 态 Portal 为空，成本可忽略。
- Toolbar 接线（菜单项 `onSelect={() => setSettingsOpen(true)}` + 组件挂 header 尾部）与既有 SshKeyManagerModal 完全同模式；radix DropdownMenu → Dialog 组合在本 app 已有线上路径。

### 边界场景

- localStorage 非法值：`normalizePalette` 单测覆盖 null / undefined / "" / 未知字符串，缺省一律回落 native-blue ✓。
- FOUC：`applyInitialPreferences()` 在 main.tsx 模块体内同步执行（此时 tokens.css import 已求值注入），先于 `createRoot().render()`，首帧即携带 theme class + `data-palette`；生产模式下浏览器渲染等待样式表完成，机制与既有 theme 预热同级，无闪色路径。
- 系统暗偏好 live 切换：纯 CSS media 自动重算，accent / lanes / scrollbar track 全部即时联动，无需 JS 参与 ✓。
- 切回 native-blue：storePalette 先写存储再改属性，选择器立即停止匹配 ✓；`data-palette="native-blue"` 常驻属性无副作用。

---

## 二、安全 ✅ 无问题

- 所有颜色值为静态 hex/rgb 字面量；DOM 操作仅 `dataset.palette = <受枚举约束的字符串>`，无任何 innerHTML / 注入面。
- localStorage 键常量私有，写入值经 `Palette` 枚举收敛，无原型链 / 越权读写风险。
- 未触碰 Tauri IPC、文件系统、凭证逻辑；符合 PM 约束（纯 UI 维度变更，不涉隐私边界）。

## 三、性能 ✅ 良好

- palette 切换 = 根元素单属性变更，触发一次 ~35 个自定义属性的重算 + 重绘，单次用户动作，可忽略。
- 启动增加一次同步 `localStorage.getItem`（微秒级），换来零 FOUC，划算。
- SettingsModal 选用静态 swatch 内联背景色而非强制重挂整棵树切换，即点即生效的实现方式开销最小。
- 🟢 可选：SettingsModal 关闭后仍持有 usePalette state；未来若 Settings 扩容再考虑按需挂载即可，当前无必要。

## 四、可维护性 🟡 有改进空间

1. 🟡 **四份近重复的暗色块**。`tokens.css` 中 native-blue 的 `:root.dark` 与 media `:root:not(.light)` 两块 ~30 行完全一致；tide 的两形态又重复一遍同一结构。本次人工比对未发现漂移，但每加一套 palette 就是 ×2 的复制成本与漂移风险（语义色/shadows 尚共享，铬色部分无法共享）。建议后续任务收敛，可选方向：
   - 构建期从单一来源（如 TS meta 或一份中间 token 表）生成三形态；
   - 或迁移到 CSS `color-scheme` + `light-dark()`（需先确认 Tauri WebView(WebKit/WKWebView 与 WebView2)版本支持度）；
   - 至少在同文件加注释指明「修改任一暗色块必须同步另一块」。
2. 🟡 **swatch 数据与 token 存在未注明偏差**。`src/lib/palette.ts` 中 tide `swatch.lanes[0] = "#2a7b8c"`，而实际 `--color-lane-1`(tide light) = `#1a8f8a`；其余 canvas/sidebar/accent 均精确镜像 token。单独 swatch 是可接受的「预览美化」，但应有注释说明或改为派生自 token，否则下次调色板时极易漏改。（tests 只校验 hex 格式，拦不住这类漂移。）
3. 🟢 文档可在 `01-tokens.md` §1.0 补一句：base 层对 theme 层的覆盖由 cascade layer 顺序保证，组合选择器只在 base 层内部竞争时起作用——把这次推演出的关键前提写下来，防止后人误判。

## 五、可读性 ✅ 良好

- `tokens.css` 头部注释完整交代双 palette 机制与覆盖顺序意图；lane 变量「为何不放 @theme」就地注释，避免后人好心搬回去；scrollbar-track 绑定语义有成句说明。
- `palette.ts` 结构清晰（PALETTES 元组 → 类型自动推导 → Record 保证 META 穷尽），JSDoc 标注了与 main.tsx 预热的协作关系。
- 文档三处更新（00-overview 决策行 / 01-tokens §1 重写 / 06 新增 K 章节 + 原则例外注记拍板日期）与代码数值逐一核对一致（抽查 canvas/sidebar/accent/lanes 明暗全对）。

## 六、测试覆盖 🟡 合理但有盲区

- ✅ node 环境下的纯函数覆盖得当：normalize 回落矩阵（含 null/undefined）、注册表唯一性、swatch 格式完整性。
- 🟡 **CSS 侧无任何自动化守护**。层叠正确性目前只能靠人眼 + 构建（我以产物 grep 代替）。建议后续低成本补充其一：
  - build 后跑一段脚本断言产物 CSS 含 `html.dark[data-palette=tide]` 等关键选择器与 6 组 lane 定义（防回归到被 prune 的状态）；
  - 项目引入 happy-dom/jsdom 后补 `readStoredPalette` / `applyInitialPalette`（会写 document/localStorage）路径的 hook 测试。当前设施缺失下不强求。
- 🟢 可加一条 meta↔token 一致性断言（读取 tokens.css 正则比对 swatch），一并解决 四-2 的漂移问题。
- 提醒：plan.md 验收中「手动冒烟（GUI 目验）」仍未勾选，合入前请按清单过一遍四形态 × 双 palette。

## 七、最佳实践 🟡 个别清理项

1. 🟡 **`src/lib/palette.test.ts` 未通过 prettier**：import 块应合并为一行（项目存在 `format:check` 脚本）。一行修复，建议 squash 前执行 `npx prettier --write src/lib/palette.test.ts`。
2. 🟢 radiogroup 建议补 roving tabindex / 方向键导航，或直接改用 Radix RadioGroup，达成 WAI-ARIA radio pattern 完整键盘模型；当前 Tab + 按钮 focusable 已可用、分组标签有 aria-label，非阻塞。
3. 🟢 未来若出现第二个 palette 消费者，考虑 storage event 或共享 store（zustand 已在依赖中）避免多实例状态分叉；单消费者现状没问题。
4. 🟢 **存量问题（非本 PR 引入，建议另开 fix 任务）**：`CommitGraph.tsx:258` 使用 `duration-fast` 工具类，但 Tailwind v4 产物中没有该类、`--duration-fast` 也被整体裁剪——该行 transition 时长实际从未生效（HEAD 版本已如此）。另注意 `.zcode/` 目前未被 .gitignore 忽略，提交本 feature 时留意范围，勿误入暂存区。
5. ✅ 流程合规：分支命名对齐任务目录；PM「AI 不代做 commit/push」约束未被触碰；plan.md 决策记录完整（含「不用系统蓝」原则的显式例外与拍板日期），文档归属（design vs tasks）划分正确。

---

## 问题汇总

| 级别 | 位置 | 问题 | 处理建议 |
|---|---|---|---|
| 🔴 | — | 无 | — |
| 🟡 | `src/lib/palette.test.ts` | prettier 不过 | squash 前 `prettier --write` |
| 🟡 | `src/lib/palette.ts` tide.swatch | lanes[0] 与实际 lane-1 不符且无注释 | 对齐 token 或加注释说明偏差 |
| 🟡 | `src/styles/tokens.css` | 四份近重复暗色块，长期漂移风险 | 记入后续重构任务（生成化 / light-dark() / 同步注释） |
| 🟡 | 测试 | CSS 形态与启动应用路径无自动化守护 | build 断言脚本或引入 happy-dom（后续） |
| 🟢 | `src/components/SettingsModal.tsx` | radiogroup 缺方向键模型 | Radix RadioGroup / roving tabindex |
| 🟢 | `src/hooks/usePalette.ts` | 多实例 / storage event 同步缺位 | 出现第二消费者时处理 |
| 🟢 | `src/styles/tokens.css` + CommitGraph | 存量死类 `duration-fast`；`.zcode/` 未忽略 | 另立 fix 任务 / 注意提交范围 |

## 结论

**可以合入。**

- 5 个重点核查方向全部落实：CSS 层叠经特异性+源序+产物三重验证无误；lane 变量规避 prune 经产物证实可靠；React 三件套模式一致、接线正确；FOUC/非法值/暗系统切换边界均有妥善处理；测试覆盖在现有设施下合理。
- 无 🔴 阻塞项。建议在 squash 提交前顺手完成两个一分钟级修复（🟡-1 prettier、🟡-2 swatch 对齐或注释），其余 🟡/🟢 作为后续改进项跟踪。
