# feat-wcbar-controls · Code Review 报告

> 审查日期：2026-08-27
> 审查对象：工作区未提交改动（feature/ui-native-studio-v2），对应 [plan.md](./plan.md)
> 审查维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践
> 结论先行：**可以合入**（无 🔴 阻塞项；1 条 🟡 建议合入前顺手处理，其余为 🟢 可选）

## 1. 审查范围与方法

改动文件：

| 文件 | 类型 | 内容 |
|---|---|---|
| `src/stores/layoutStore.ts` | 修改 | 新增 `wcBarCollapsed` / `wcBarMaximized` 两组 state + toggle/set |
| `src/components/ui/WorkingCopyBar.tsx` | 修改 | header 行重构为 `ml-auto` 按钮组、新增 collapsed 渲染分支、height 条件化 |
| `src/App.tsx` | 修改 | reset effect 扩展为同时复位 inspector maximize + wcBar 两状态 |
| `src/stores/layoutStore.test.ts` | 新增 | node 环境直测 store 联动不变式（5 例） |
| `docs/design/04-working-copy.md` | 修改 | §3 增补折叠/最大化控件规格 |
| `docs/tasks/feat-wcbar-controls/plan.md` | 新增 | 任务计划 |

交叉核对的相关文件（未改动）：`src/components/DiffViewer.tsx`（inspector maximize 先例）、`src/components/ui/Button.tsx`、`src/components/ui/Tooltip.tsx`、`src/components/ui/BranchIndicator.tsx`、`src/components/ChangesPanel.tsx`（`layout="bar"` 网格）、`src/hooks/useWorkingCopy.ts`、`src-tauri/tauri.conf.json`（窗口最小尺寸）。

自动化验证（本机复跑）：

| 检查 | 结果 |
|---|---|
| `tsc --noEmit` | 通过（exit 0） |
| `vitest run --passWithNoTests` | 5 files / **29 tests 全过**（含新增 layoutStore 5 例） |
| `eslint`（4 个源码文件） | 0 error / 0 warning |
| `prettier --check`（含两份文档） | 全部通过 |

## 2. 分维度审查

### 2.1 正确性

🟢 **Hooks 顺序安全**。`WorkingCopyBar` 中全部 5 个 hook（`useWorkingCopy` + 4 个 `useLayoutStore` selector）都在 `if (repoId === null || !wc.workspaceId) return null` 早退之前调用，无条件分支下的 hook 违规。

🟢 **zustand selector 用法正确**。均为单字段原子 selector；action 是 `create` 时一次性创建的稳定引用，不会引发无效重渲。App.tsx reset effect 的依赖数组写法规范（setter 入 deps）。

🟢 **三态分支互斥完整**。渲染顺序为 clean → collapsed → dirty 兜底，各守卫条件齐备：

- clean 分支要求 `!isLoading && !isError && !isDirty && snapshot`，排除 loading/error/dirty；
- collapsed 分支随后，dirty 渲染兜底；
- **snapshot 为 null（loading）时**：`branchInfo` 降级为 Loading… 占位，计数 span 因 `snapshot ? … : null` 不渲染——collapsed 态不会出现 "NaN unstaged"，正确；
- **error 态**：clean 分支被 `!wc.isError` 排除，collapsed / dirty 正常渲染，三个分支均在末尾保留 `<ErrorAlert message={wcAlert} …/>`，错误提示不丢失。

🟢 **状态联动规则实现正确**。「收起即退出最大化」在 `toggleWcBarCollapsed` 与 `setWcBarCollapsed(true)` 两个入口都做了维护；展开路径（`set(false)` / toggle 展开）不动最大化字段，符合 plan 中「单向联动」决策。反向不可能态（collapsed && maximized 同时为 true）：UI 层 maximize 按钮仅在 dirty 未收起分支渲染，collapsed 时无触发入口，经现有 action 组合该状态不可达（见 §2.4 的加固建议）。

🟢 **「hide 后经由 clean 回到 dirty 仍保持收起」不是 bug**：用户的 hide 意图跨 clean/dirty 往返持续，直到手动恢复或切 repo/workspace 复位，符合「隐藏」语义。clean 态本身即 32px 单行，无需恢复按钮，按钮只在 dirty 相关联的两态出现，与 plan 一致。

🟡 **D-01 高度切换无过渡动画，与设计文档「smooth animation 200ms」脱节**（`docs/design/04-working-copy.md` §3 末行）。`style={{ height }}` 直写，32px ↔ 220px ↔ 50vh 为硬切。该声明是既有内容（clean↔dirty 本就未动画化），但本次新增了幅度最大的两类跳变（收起、最大化），观感问题被放大。二选一：(a) 给外壳加 `transition-[height] duration-200 ease-out`（注意 collapsed 分支是独立 DOM，需要同时覆盖两处，或统一为一个分支）；(b) 在文档中把该行限定为 clean↔dirty 切换的目标规格并标注未实现。不阻塞合入。

### 2.2 安全

🟢 无问题。纯前端 UI 布局状态：无用户输入拼接进 DOM（Tooltip/aria 均为静态英文文案，lucide 图标静态引入）；不触及 git 凭证、diff 数据出境等隐私约束；按钮仅改本地 store，不影响「禁止自动 commit/push」（P1）。无新增 IPC、无权限面变化。

### 2.3 性能

🟢 订阅粒度合理。组件侧按字段订阅 4 个切片，重渲范围最小；App.tsx 仅订阅 setter（稳定引用），mount 时两次幂等 `set(false)` 虽会生成新 store 快照，但选中值未变，zustand 订阅方不重渲，开销可忽略。

🟢 `useWorkingCopy` 的 2s 轮询导致的重渲为既有行为，本次改动未叠加成本；collapsed 分支比 dirty 分支渲染更少节点，收起反而降低常态开销。

🟢 50vh 下 ChangesPanel 不会被挤压异常：maximize 只增加纵向空间（grid-cols-[1fr_1fr_280px] 各列原本就在 220px 高度内工作）；列宽固定 280px 的 commit 列有 `h-full` 自适应，FileSection 内部已有滚动容器。窗口极矮使 50vh < 220px 的场景已被 `tauri.conf.json` 的 `minHeight: 600` 排除（50vh ≥ 300px > 220px）。

### 2.4 可维护性

🟢 与既有先例对称：命名完全镜像 `inspectorMaximized`（`wcBar*` / `toggleWcBar*` / `setWcBar*`），reset 复位策略同处一个 effect，后来者容易推断全貌。

🟢 联动规则单一出处：约束只写在 store action 内部（附注释 "A collapsed bar has no maximized body to restore"），组件层无散落的状态修补，且有测试锚定。

🟢 `branchInfo` 提取消除了 collapsed / dirty 两分支间的 `BranchIndicator` 六属性重复（此前 diff 中该块是复制粘贴的），props 顺序还顺带修正为与组件签名一致，纯改进。

🟢 **E-01 双布尔 + 不变式的长期维护提醒（🟢）**：目前 `(collapsed ⇒ ¬maximized)` 这条不变式靠每个新调用点自觉遵守；若未来某处直接 `setWcBarMaximized(true)` 而 bar 处于收起态，store 不阻止（渲染上 collapsed 分支优先，最大化被静默忽略，不会崩溃，只是静默失效）。两个低成本加固选项，任选其一即可：(a) 将两个字段合并为单枚举 `wcBarMode: 'normal' | 'collapsed' | 'maximized'`，让非法态在类型层不可表达；(b) 在 `setWcBarMaximized` 中对 `wcBarCollapsed` 做 no-op 守卫并以注释明示。当前规模（两个布尔 + 5 条测试）维持现状完全可接受。

🟢 三分支外壳共享的 `bg-bg-secondary border-t border-border-subtle shadow-[…]` 串重复三次，可提为模块级常量；量级很小，不强求。

### 2.5 可读性

🟢 渲染分支带意图注释（"Collapsed: keep branch status visible…"），JSDoc 同步更新了折叠/最大化描述；collapsed 分支整体复刻 clean 行的视觉语言（同高 32px、同 `justify-between px-3.5 text-xs text-text-muted`），一致性直观。

🟢 collapsed 行恢复按钮 `aria-pressed={true}` 用字面量表达「收起开关处于按下态」。语义正确；若希望更显式，可在合入后续改为绑定 `wcBarCollapsed` 变量（此时恒真），或在组件顶部解构出布尔再引用——纯风格问题。

🟢 `Minimize2, Maximize2, ChevronDown, ChevronUp` import 未按字母序；项目内 lint 未启用 sort-imports 规则，不构成问题。

### 2.6 测试覆盖

🟢 store 测试选点有效，锚定的正是本次核心不变式：初值、`toggleWcBarMaximized` 不干扰 collapsed（独立性）、collapse 清 maximize（联动）、再次展开不复原 maximize（联动单向性）、`setWcBarCollapsed(true)` 重复调用幂等。beforeEach 归零避免用例间串扰。node 环境直测 zustand store 是该项目现有测试基建（无 jsdom / testing-library）下的正确形态。

🟡 **D-02 两个已声明的行为缺少断言**：(a) `setWcBarCollapsed(false)` 不触碰 `wcBarMaximized`——目前只测了正向联动，反向「展开不清最大化标志」虽由实现保证，但没有回归网；(b) reset effect（切 workspace/repo 复位）位于 App.tsx，现有基建测不到，plan 冒烟清单已把它列为人工项，可接受。建议至少补 (a)（一行用例）：

```ts
it("expanding via setter keeps maximized cleared", () => {
  const s = useLayoutStore.getState();
  s.setWcBarMaximized(true);
  s.setWcBarCollapsed(true);
  s.setWcBarCollapsed(false);
  expect(useLayoutStore.getState().wcBarMaximized).toBe(false);
});
```

🟢 组件级行为（三态渲染、clean 态按钮缺席、aria 属性、Tooltip 文案随状态切换）目前无法在 node 环境覆盖，属已知盲区且 plan 已说明。将来若引入 jsdom + `@testing-library/react`，`WorkingCopyBar` 的三态快照/交互测试是首个候补对象。

### 2.7 最佳实践

🟢 **样式与交互对齐先例**：Hide/Maximize 按钮与 `DiffViewer.tsx:459-473` 的 inspector maximize 按钮逐项一致——`variant="ghost" size="sm"`、`p-1.5 text-text-muted hover:text-accent`、图标 `size={14}`、`Maximize2/Minimize2` 图标对、动态 Tooltip 文案、动态 `aria-label` + `aria-pressed`。Tooltip（Radix `asChild` + Button forwardRef）组合与现有用法同构，Provider 嵌套无害。

🟢 **a11y 完备**：两态四枚按钮均有 `aria-label`，且与 Tooltip content 文案一致（视障读屏与悬停提示不冲突）；maximize 符合 WAI-ARIA toggle button 模式（pressed 反映状态、label 随状态变化）；collapsed 的 Show 按钮以 `aria-pressed=true` 表达 hide 开关按下态可接受。焦点环来自 Button 基类 `focus-visible:outline-*`。

🟢 动态高度走 inline style（数字常量 + vh 单位混排）是该场景的务实选择；50vh 与 220px 都作用于同一元素，无双源漂移。

🟢 文档随码更新：设计规格写入 `docs/design/04-working-copy.md`（跨任务工程文档归属正确，符合 AGENTS.md 的 tech/tasks 目录约定），按钮位置/图标/联动/复位策略均有据可查。

🟢 附带的仓库卫生提醒（与本 feature 无关）：`git status` 显示存在未跟踪的 `.zcode/` 目录且 `.gitignore` 无对应条目，后续 `git add` 时注意勿误提交，建议单独加一条 ignore。

## 3. 问题汇总

| 编号 | 维度 | 级别 | 摘要 | 建议 |
|---|---|---|---|---|
| D-01 | 正确性（文档一致性） | 🟡 | 高度切换无 200ms 过渡动画，与设计文档 §3 声明不符 | 补 `transition-[height] duration-200` 或修订文档限定动画适用范围 |
| D-02 | 测试覆盖 | 🟡 | 「`set(false)` 不清 maximize」方向缺回归断言 | 补一行 store 用例 |
| E-01 | 可维护性 | 🟢 | 双布尔不变式靠调用点自律，非法态静默忽略 | 可选：单枚举模式或在 setter 加守卫 |
| E-02 | 可维护性 | 🟢 | 三分支外壳 class 重复 | 可提取常量 |
| E-03 | 可读性 | 🟢 | collapsed 按钮 `aria-pressed` 用字面量 `true` | 可绑定为 `wcBarCollapsed` |
| E-04 | 仓库卫生 | 🟢 | `.zcode/` 未忽略 | 加入 `.gitignore`（独立 chore 提交） |

🔴 阻塞项：无。

## 4. 结论

**可以合入。**

实现忠实落地了 plan 中已认可的取舍：隐藏=折叠为 32px 状态条并保留恢复入口、收起联动清最大化、切 workspace/repo 复位、控件样式对齐 DiffViewer 先例、clean 态不出按钮。三态渲染互斥完备，loading/error/snapshot-null 边界处理正确，a11y 属性齐全，store 联动有测试锚定，typecheck / test(29) / lint / format 全部本机复跑通过。

D-01（动画/文档一致性）与 D-02（补一条反向联动断言）工作量都在十分钟以内，建议合入前顺手处理；即便留待后续小修也不构成风险。E 组为可选打磨。

人工冒烟项（无法在本环境替代，沿用 plan 清单）：dirty 态两枚按钮出现 → Hide 折叠/恢复往返 → Max 半窗高往返且 ChangesPanel 三列正常 → 切 repo 后自动展开复原。
