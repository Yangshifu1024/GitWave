# review · fix-status-area-cancel-button

> 审查人：code-reviewer 代理
> 审查范围：`src/components/SyncStatusArea.tsx`、`src/components/ActionBar.tsx`、
> `docs/tasks/fix-status-area-cancel-button/plan.md`（工作区其余 system-proxy
> 未提交改动已排除）
> 结论：**可合入**（无 🔴 问题；两条 🟡 已在合入前处理，见文末）

## ✅ 优点

- 修复方案精准且最小：`pointer-events-auto` 只加在取消按钮上（而非整个
  HeroCard），正确保住了「窄窗口下卡片非交互区穿透到下层 Fetch/Pull/Push
  按钮」的原始约束。
- 遵循仓库既有模式：与 `AppMenuBar.tsx:192-194`、`Toolbar.tsx:72`（
  pointer-events-none 层之上的交互子层显式 `pointer-events-auto`）一致。
- 两端注释同步：耦合关系在包裹层（ActionBar）与恢复点（SyncStatusArea）双侧
  均有说明，任一端被单独改动时都能看到警告，有效防止回归。
- plan.md 根因链完整，诚实记录了 jsdom 命中测试的局限及自动化门禁思路。

## 关键确认项

1. **加在按钮上是否正确**：正确。CSS `pointer-events` 为可继承属性，祖先为
   `none` 时后代显式 `auto` 可重新进入命中测试。包裹层保持 `pointer-events-none`
   不变，卡片体、文本、进度条继续穿透；仅 24×24 按钮消费点击。窄窗口下按钮恰好
   压住下层按钮时会挡住该点点击，是修复的固有代价，可接受。
2. **无遗漏的交互恢复点**：已通读 `SyncStatusArea.tsx` 全文件——HeroCard 根节点
   无 `isPressable`/`isHoverable`/`onClick`；内层容器与文本 span 纯展示；
   ProgressBar 为 `role="progressbar"` 非交互；底部色条为 `aria-hidden` 展示 div。
   按钮是卡片内唯一交互元素，且 `SyncStatusArea` 全仓库仅 ActionBar 一处挂载。
3. **注释与文档一致性**：两处代码注释与实际行为一致；plan.md 的测试结论表述
   已按下方 🟡-1 修正。

## 🔴 严重问题

无。

## 🟡 一般问题（均已处理）

1. **plan.md 测试结论与工作区事实不符** → 已修正：`npm test` 全量运行时
   `parity.test.ts` 因工作区在途的 system-proxy（F013）未提交 Rust 改动失败
   （`errors.usecases.proxy.url_invalid` 的 en 词条缺失），与本任务无关，
   plan.md 已如实记录。
2. **缺自动化防回归门禁** → 已补充 `src/components/statusAreaHitTesting.test.ts`：
   沿用仓库 `parity.test.ts` 的 plain vitest + `node:fs` 静态断言模式，
   断言「包裹层存在 `pointer-events-none` 时 `SyncStatusArea.tsx` 含
   `pointer-events-auto`」，拦截「卡片内新增交互元素忘记恢复命中」这类回归。
   （未引入 Testing Library 渲染测试：jsdom 不做真实命中测试，测不出本缺陷，
   为单个修复引入整套渲染测试基建不成比例。）

## 🟢 优化建议（可选，未在本任务处理）

- `pointer-events-auto` 位于 className 首部、定位类之前，与 Tailwind 常见类排序
  不一致；功能无影响，后续若引入 `prettier-plugin-tailwindcss` 会自动归位。
- 取消按钮无可见 tooltip（对比 ActionBarButton 均带 `title`），建议后续
  a11y/打磨任务补 `title={t(...)}` 与设计 token 化的 `focus-visible:` 焦点环
  （当前原生焦点环可见，无障碍底线达标）。

## 验证记录

- `npm run typecheck` / `npm run lint` 全绿。
- `src/components/statusAreaHitTesting.test.ts` 通过。
- `npm test` 全量：1 个失败为 `parity.test.ts`（在途 system-proxy 改动所致，
  与本任务无关），其余 20 个测试文件全部通过。
- 交互行为依赖手动冒烟，要点见 plan.md「测试」一节。
