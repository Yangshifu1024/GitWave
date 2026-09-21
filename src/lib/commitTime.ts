import type { TFunction } from "i18next";

/** One absolute-time format: the exact Intl call its call site used before the
 *  refactor, plus the coarsest cache key that is still safe for that label. */
interface AbsoluteStyle {
  /** 沿用调用点原有的方法：带 options 时 toLocaleDateString 与 toLocaleString 输出一致，
   *  但这是引擎相关行为（本机只在 V8 验证过），逐点保持原方法最稳。 */
  method: "toLocaleDateString" | "toLocaleString";
  options: Intl.DateTimeFormatOptions | undefined;
  /** 缓存粒度必须 ≥ 标签的最细字段：纯日期 → 本地日；含时分 → 分钟；含秒 → 秒。
   *  粒度粗于标签会让同一天的不同时刻互相串味（与 fix-history-scroll-perf 的 🔴 R1 同类陷阱）。 */
  granularity: "day" | "minute" | "second";
}

/** 全仓绝对时间样式表：每个调用点的 method/options/粒度只在此定义一处。
 *  options 逐点照抄原调用点，含年份 / 不含年份、含秒 / 不含秒的差异都必须保留。 */
const ABSOLUTE_STYLES = {
  /** 相对时间末档（formatCommitDate）：系统 locale 的纯日期。 */
  date: { method: "toLocaleDateString", options: undefined, granularity: "day" },
  /** BlameView gutter / hover：`2026年9月21日`（locale 由调用点传入，通常为应用语言）。 */
  ymd: {
    method: "toLocaleDateString",
    options: { year: "numeric", month: "short", day: "numeric" },
    granularity: "day",
  },
  /** CommitInfoHeader 作者行：`2026年9月21日 14:05`（应用语言）。 */
  "ymd-hm": {
    method: "toLocaleString",
    options: {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    },
    granularity: "minute",
  },
  /** ReflogPanel 时间线：`9月21日 14:05`（系统 locale，刻意不含年份）。 */
  "md-hm": {
    method: "toLocaleString",
    options: { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" },
    granularity: "minute",
  },
  /** BranchList tooltip：`2026/9/21 14:05:33`（系统 locale，无 options，含秒）。 */
  "date-time": { method: "toLocaleString", options: undefined, granularity: "second" },
} as const satisfies Record<string, AbsoluteStyle>;

export type AbsoluteTimeStyle = keyof typeof ABSOLUTE_STYLES;

/** toLocaleDateString 是 ICU 调用；标签只取决于“哪一天”，按天缓存即可。 */
const dateLabelCache = new Map<string, string>();

/** 缓存键取“本地日历日”：label 由本地时区决定，用 UTC 日桶会把跨本地午夜的
 *  两个时刻误判为同一天（例如 UTC+8 的本地 00:30 与前一天 23:30 落在同一 UTC 桶）。 */
function localDayKey(time: number): string {
  const d = new Date(time * 1000);
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}

/** 缓存粒度对应的键片段。粒度是样式表的一部分，不是这里的实现细节。 */
function cacheStamp(time: number, granularity: AbsoluteStyle["granularity"]): string {
  if (granularity === "day") return localDayKey(time);
  if (granularity === "minute") return String(Math.floor(time / 60));
  return String(time);
}

/** 按样式表原样发起 Intl 调用：method 与 options 的写法逐点照抄原调用点。
 *  `locale` 为 undefined 时等价于不传 locale（系统 locale）。 */
function renderAbsolute(date: Date, spec: AbsoluteStyle, locale: string | undefined): string {
  if (spec.method === "toLocaleDateString") {
    return spec.options === undefined
      ? date.toLocaleDateString(locale)
      : date.toLocaleDateString(locale, spec.options);
  }
  return spec.options === undefined
    ? date.toLocaleString(locale)
    : date.toLocaleString(locale, spec.options);
}

/** 绝对时间：按样式表渲染并缓存。缓存键必须同时带上样式与 locale —— 只按时间键
 *  会让不同样式 / 不同语言互相覆盖，返回上一个调用点的标签。 */
export function formatAbsoluteTime(
  time: number,
  style: AbsoluteTimeStyle,
  locale?: string,
): string {
  const spec: AbsoluteStyle = ABSOLUTE_STYLES[style];
  const stamp = cacheStamp(time, spec.granularity);
  const key = `${style}|${locale ?? ""}|${stamp}`;
  const cached = dateLabelCache.get(key);
  if (cached !== undefined) return cached;
  const label = renderAbsolute(new Date(time * 1000), spec, locale);
  // 上限保护：缓存不该随仓库历史无限增长
  if (dateLabelCache.size > 512) dateLabelCache.clear();
  dateLabelCache.set(key, label);
  return label;
}

export function formatCommitDate(time: number): string {
  return formatAbsoluteTime(time, "date");
}

export function formatCommitTime(
  time: number,
  t: TFunction,
  now: number = Math.floor(Date.now() / 1000),
): string {
  const diff = now - time;
  if (diff < 60) return t("branches.time.justNow");
  if (diff < 3600) return t("branches.time.minutesAgo", { n: Math.floor(diff / 60) });
  if (diff < 86400) return t("branches.time.hoursAgo", { n: Math.floor(diff / 3600) });
  if (diff < 604800) return t("branches.time.daysAgo", { n: Math.floor(diff / 86400) });
  return formatCommitDate(time);
}
