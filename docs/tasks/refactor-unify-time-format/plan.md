# refactor-unify-time-format · 时间格式化统一到 commitTime.ts

> 状态：已实现，复审通过（无 🔴），待真机验收；改动未提交
> 分支：`refactor/unify-time-format`（基线 `fix/history-scroll-perf@1728526`）
> 审查报告：`docs/tasks/refactor-unify-time-format/review.md` · 测试与收尾：`.codewave/tasks/20260921-193320-unify-time-format/report.md`
> 需求：`.codewave/tasks/20260921-193320-unify-time-format/requirement.md`（含 S1 调研 + S2 需求分析）
> 上游：`docs/tasks/fix-history-scroll-perf/review.md:62`（🟢 留档建议）

## 目标

把全仓 5 处各自实现的时间格式化收敛到 `src/lib/commitTime.ts`，**输出逐字节不变**。交付的价值是一致性与可维护性，不是「格式看起来更统一」——验收核心是等价移植。

本轮纳入 4 个组件共 6 处调用点（含用户确认纳入的两处）：
`BranchList.tsx:207`（相对时间）、`BranchList.tsx:195`（tooltip，含秒）、`ReflogPanel.tsx:199`（绝对时间，无年份）、`BlameView.tsx:63`+`:82`（绝对日期，应用语言）、`CommitInfoHeader.tsx:181`（绝对日期+时间，应用语言）。

## 设计

### 1. 共享模块新增「绝对时间」能力（`src/lib/commitTime.ts`）

各调用点的 `method / options / 缓存粒度` 收成一张常量表，避免每个调用点自建 options 对象（也让缓存键可预计算）：

```ts
interface AbsoluteStyle {
  /** 沿用调用点原有的方法：带 options 时 toLocaleDateString 与 toLocaleString 输出一致，
   *  但这是引擎相关行为（本机只在 V8 验证过），逐点保持原方法最稳。 */
  method: "toLocaleDateString" | "toLocaleString";
  options?: Intl.DateTimeFormatOptions;
  /** 缓存粒度必须 ≥ 标签的最细字段：纯日期 → 本地日；含时分 → 分钟；含秒 → 秒。
   *  粒度粗于标签会串味（同一天不同时刻显示同一结果）——与 fix-history-scroll-perf 的 🔴 R1 同类。 */
  granularity: "day" | "minute" | "second";
}

const ABSOLUTE_STYLES = {
  /** 相对时间末档（formatCommitDate）：系统 locale 的纯日期。 */
  date:      { method: "toLocaleDateString", granularity: "day" },
  /** BlameView gutter / hover：`2026年9月21日`（应用语言）。 */
  ymd:       { method: "toLocaleDateString", options: { year: "numeric", month: "short", day: "numeric" }, granularity: "day" },
  /** CommitInfoHeader 作者行：`2026年9月21日 14:05`（应用语言）。 */
  "ymd-hm":  { method: "toLocaleString", options: { year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }, granularity: "minute" },
  /** ReflogPanel 时间线：`9月21日 14:05`（系统 locale，刻意不含年份）。 */
  "md-hm":   { method: "toLocaleString", options: { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }, granularity: "minute" },
  /** BranchList tooltip：`2026/9/21 14:05:33`（系统 locale，无 options，含秒）。 */
  "date-time": { method: "toLocaleString", granularity: "second" },
} as const;

export type AbsoluteTimeStyle = keyof typeof ABSOLUTE_STYLES;

export function formatAbsoluteTime(time: number, style: AbsoluteTimeStyle, locale?: string): string
```

- **缓存键**：`${style}|${locale ?? ""}|${stamp}`，`stamp` 按 `granularity` 取 `localDayKey(time)` / `Math.floor(time / 60)` / `time`。共用同一个 `Map`（`> 512` 全清）。
- `formatCommitDate(time)` 改为 `formatAbsoluteTime(time, "date")` —— 导出与行为不变（`toLocaleDateString()` 无参、系统 locale）。
- `formatCommitTime` 及其分档**完全不动**。
- **约束**：`localDayKey` 必须继续用本地日历分量（`getFullYear/getMonth/getDate`），不得退回 UTC 日桶 —— 既有源码守卫 `CommitGraph.renderGuards.test.ts` 会拦（见验证）。

### 2. 调用点替换（逐字节等价）

| 文件:行 | 现状 | 改为 |
|---|---|---|
| `BranchList.tsx:81-90`（定义）/`:207`（调用） | 本地 `formatTime`，相对分档 + `time <= 0 → ""` | 删除本地定义；`:207` 用 `formatCommitTime(branch.last_commit_time, t)`（该调用点已有 `> 0` 守卫，空串语义不变） |
| `BranchList.tsx:195` | `new Date(...).toLocaleString()` 内联 | `formatAbsoluteTime(branch.last_commit_time, "date-time")` |
| `ReflogPanel.tsx:41-48`（定义）/`:199` | 本地 `formatTime`，`toLocaleString(undefined, {month,day,hour,minute})` | 删除本地定义；`formatAbsoluteTime(e.time, "md-hm")` |
| `BlameView.tsx:8-14`（定义）/`:63`、`:82` | 本地 `formatTime(time, locale)`，`toLocaleDateString(locale, {year,month,day})` | 删除本地定义；`formatAbsoluteTime(line.time, "ymd", i18n.language)` |
| `CommitInfoHeader.tsx:25-33`（定义）/`:181` | 本地 `formatDateTime(time, locale)` | 删除本地定义；`formatAbsoluteTime(time, "ymd-hm", locale)` |

**不动的**：`lib/commitMenu.ts:26`（剪贴板文本 + 其测试）、`CommitGraph.tsx`、相对时间分档、i18n 资源、Rust。

### 3. 文件范围（S6 分包，互斥）

- **包 1（共享模块 + 守卫）**：`src/lib/commitTime.ts`、`src/lib/commitTime.test.ts`、`src/components/timeFormatCallSites.test.ts`（新建）
- **包 2（调用点替换）**：`src/components/BranchList.tsx`、`src/components/ReflogPanel.tsx`、`src/components/BlameView.tsx`、`src/components/CommitInfoHeader.tsx`

包 2 依赖包 1 的接口 ⇒ **顺序派发**（先包 1 后包 2），不做并行。

## 验证

### 自动（开发机）

```bash
pnpm format:check && pnpm lint && pnpm typecheck && pnpm test
```

新增测试：

- `commitTime.test.ts`（行为，覆盖 AC-1/AC-2/AC-4）：
  - 每个样式与直调 `toLocaleXxx(locale, options)` **逐字节一致**（locale 取 `undefined`/`"en"`/`"zh-CN"`）；
  - **跨 locale 不串味**：同一 time、同一样式，`"en"` 与 `"zh-CN"` 结果不同且各自正确；
  - **跨粒度不串味（R1 同类陷阱）**：含时分的样式（`md-hm`/`ymd-hm`）对同一本地日的两个不同时刻必须不同；含秒样式（`date-time`）相差 1 秒必须不同；
  - 纯日期样式（`date`/`ymd`）同一本地日不同时刻 → 相同；
  - `formatCommitDate` 仍等于 `new Date(t*1000).toLocaleDateString()`（既有断言不破）；
  - `time <= 0` / `now < time` / `NaN` 的现状行为不变。
- `src/components/timeFormatCallSites.test.ts`（新建，静态源码守卫，仿 `reactVersionParity.test.ts` / `CommitGraph.renderGuards.test.ts`）：
  - 4 个组件源码**不含** `toLocaleDateString(` 与 `toLocaleString(`（内联实现不得回流）；
  - 4 个组件源码**含** `formatAbsoluteTime(` / `formatCommitTime(`；
  - `commitTime.ts` 的缓存键含 style 与 locale（断言 `ABSOLUTE_STYLES` 存在、且键模板引用两者）；
  - `commitTime.ts` 仍用本地日历分量、不含 `Math.floor(time / 86400)`。

### 真机/手工（macOS）

- 非 UTC 时区（`TZ=Asia/Shanghai` 或真机）跑一遍，而非只在 CI 的 UTC 下绿。
- `zh-CN` / `en` 各一遍；**切语言不重载**后 blame / inspector 头部立即变新语言、无旧语言残留。
- ReflogPanel 时间**不含年份**；`BranchList` tooltip 完整串（`sha · 时间`，含秒、分隔符不变）；`time = 0` / 无 upstream 分支不出现孤立 ` · `。
- 造「同一本地日、多个不同时刻」的 reflog 数据（多次 reset/checkout），确认不串味。
- blame 千行文件滚动无卡顿回退。

## 风险与回滚

| 风险 | 缓解 |
|---|---|
| 含时间字段的样式被按日缓存 → 显示错时间 | 样式表内声明 `granularity`，键按粒度取 stamp；专门的跨粒度测试 |
| locale 被一把抓 → 语言混排 | helper 的 `locale` 为可选参数、默认 `undefined`（系统 locale）；调用点各自传入原值；跨 locale 测试 |
| 方法互换（`toLocaleDateString` ↔ `toLocaleString`）在非 V8 引擎上输出不同 | 每个样式显式声明 `method`，逐点沿用原方法；测试逐个断言与直调结果一致 |
| ReflogPanel 多出年份 / tooltip 丢秒 | 样式表逐点照抄原 options；`md-hm` 无 `year`、`date-time` 无 options；验收清单已列 |
| 缓存键改动破坏既有守卫 | `localDayKey` 不动、仍用本地日历分量；改完跑 `CommitGraph.renderGuards.test.ts` |
| 内联实现回流 | 新增 `timeFormatCallSites.test.ts` 源码守卫 |

**回滚**：改动集中在 1 个 lib + 4 个组件 + 3 个测试文件；无依赖、无 i18n 资源、无 Rust 变更 —— 丢弃分支即可。

## 非目标

见 `requirement.md` §「非目标（本轮明确不做）」7 条；其中 `lib/commitMenu.ts` 与「相对时间 tick」两项已在方案与评审中显式排除。
