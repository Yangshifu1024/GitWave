# fix-status-area-cancel-button · 状态区取消操作按钮无法按下

> 状态：已修复（待冒烟）
> 需求（用户）：状态指示区域的取消操作按钮无法按下。

## 现象

fetch / pull / push 进行中，状态卡右侧出现取消按钮（X），按钮可见但鼠标悬停无任何
hover 反馈，点击无响应——「看得见、按不下」。

## 根因

`pointer-events` 继承：

1. `src/components/ActionBar.tsx`（状态区包裹层）：
   `<div className="absolute inset-x-0 flex justify-center pointer-events-none">`。
   该层 `pointer-events-none` 是有意为之——窄窗口下状态卡与 Fetch / Pull / Push
   按钮重叠时，卡片非交互区域必须穿透点击，不能挡住按钮。
2. `src/components/SyncStatusArea.tsx` 的取消按钮是卡片内唯一可交互元素，但
   className 未显式恢复 `pointer-events-auto`。CSS `pointer-events` 为可继承属性，
   按钮继承 `none` 后浏览器命中测试永远不把它作为事件目标：hover 样式不触发、
   `onClick={handleCancel}` 永不执行，点击直接落到下层元素。

按钮渲染条件 `cancellable = syncing && !fading && isCancellableOp(activeOp) &&
!cancelRequested` 在 fetch/pull/push 进行中恒为真，排除「按钮未渲染 / 提前卸载」路径；
`tokens.css` 全局 `button { cursor: pointer }` 正常，排除其他交互样式干扰。

仓库内正确范例：`AppMenuBar.tsx` / `Toolbar.tsx` 在 `pointer-events-none` 拖拽层之上的
交互子层均显式加了 `pointer-events-auto`。本缺陷是该模式在 SyncStatusArea 的遗漏。

## 改动清单

- `src/components/SyncStatusArea.tsx` 取消按钮 className 追加 `pointer-events-auto`
  （仅加在按钮上）。**不**加在整个 `HeroCard` 上，否则窄窗口下卡片会重新挡住下层按钮，
  破坏 ActionBar 包裹层注释声明的原始约束。
- `src/components/ActionBar.tsx` 包裹层注释同步修正：卡片非交互区穿透不变，取消按钮
  经 `pointer-events-auto` 自行恢复命中。
- `src/components/statusAreaHitTesting.test.ts` 静态断言守卫（与 `parity.test.ts`
  同款 plain vitest + `node:fs` 模式）：包裹层存在 `pointer-events-none` 时，
  `SyncStatusArea.tsx` 必须含 `pointer-events-auto`，拦截「卡片内新增交互元素忘记
  恢复命中」这类回归。jsdom 的 `userEvent.click` 不做真实命中测试，测不出本缺陷，
  故采用静态断言而非渲染测试。

hover（`hover:bg-danger/10 hover:text-danger`）、cursor（全局按钮样式）、aria
（`aria-label`）现状已合规，无需连带修改。

## 测试

- `npm run typecheck` / `npm run lint` 全绿；新增的 `statusAreaHitTesting.test.ts`
  通过。
- `npm test` 全量运行时有 1 个与本任务无关的失败：
  `src/i18n/locales/parity.test.ts` 报 `errors.usecases.proxy.url_invalid` 的 en
  词条缺失——由工作区在途的 system-proxy（F013）未提交 Rust 改动引入，其 locale
  词条尚未同步；本任务改动不涉及。
- 手动冒烟要点：
  - fetch/pull/push 进行中：悬停取消按钮出现 danger 底色反馈；点击后调用
    `cancelSync`、状态卡过渡到结果态（修复前悬停无反馈可作快速判别）
  - 窄窗口下状态卡与 Fetch/Pull/Push 按钮重叠：卡片文本/卡体/进度条上的悬停与
    点击仍穿透到下层按钮（下层按钮 hover 高亮、点击生效）；仅取消按钮消费点击
  - 键盘：Tab 聚焦取消按钮，Enter/Space 触发取消
  - checkout/stash/worktree 等不可取消操作不渲染按钮；点击一次后按钮消失，
    新操作开始时恢复
