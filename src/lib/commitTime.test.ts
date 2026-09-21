import type { TFunction } from "i18next";
import { describe, expect, it } from "vitest";

import { formatAbsoluteTime, formatCommitDate, formatCommitTime } from "./commitTime";
import type { AbsoluteTimeStyle } from "./commitTime";

/** Minimal stand-in for i18next's `t`: key plus the `n` interpolation. */
const t = ((key: string, opts?: { n?: number }) =>
  opts?.n === undefined ? key : `${key}:${opts.n}`) as unknown as TFunction;

/** Fixed "now" so buckets are asserted without touching the wall clock. */
const NOW = 1_700_000_000;

function ago(seconds: number): string {
  return formatCommitTime(NOW - seconds, t, NOW);
}

describe("formatCommitTime buckets", () => {
  it("reports just-now below one minute", () => {
    expect(ago(0)).toBe("branches.time.justNow");
    expect(ago(59)).toBe("branches.time.justNow");
  });

  it("reports minutes from 60s up to (but excluding) one hour", () => {
    expect(ago(60)).toBe("branches.time.minutesAgo:1");
    expect(ago(3599)).toBe("branches.time.minutesAgo:59");
  });

  it("reports hours from one hour up to (but excluding) one day", () => {
    expect(ago(3600)).toBe("branches.time.hoursAgo:1");
    expect(ago(86399)).toBe("branches.time.hoursAgo:23");
  });

  it("reports days from one day up to (but excluding) one week", () => {
    expect(ago(86400)).toBe("branches.time.daysAgo:1");
    expect(ago(604799)).toBe("branches.time.daysAgo:6");
  });

  it("falls back to an absolute date from one week on", () => {
    const time = NOW - 604800;
    const label = formatCommitTime(time, t, NOW);
    expect(label).not.toBe("");
    expect(label).toBe(new Date(time * 1000).toLocaleDateString());
  });
});

describe("formatCommitDate", () => {
  it("labels two instants inside one local calendar day identically", () => {
    // 本地日语义的*行为*断言在 UTC 宿主（CI 默认 TZ=UTC）上对旧实现（UTC 日桶）同样成立，
    // 因此它无法单独守住该缺陷；真正的守卫是
    // src/components/CommitGraph.renderGuards.test.ts 里的源码级断言。
    const morning = new Date(2024, 4, 17, 9, 0, 0).getTime() / 1000; // 本地 09:00
    const lateSameDay = new Date(2024, 4, 17, 23, 45, 0).getTime() / 1000; // 本地 23:45
    const nextDayEarly = new Date(2024, 4, 18, 0, 15, 0).getTime() / 1000; // 次日本地 00:15

    const label = formatCommitDate(morning);
    expect(label).toBeTypeOf("string");
    expect(label).not.toBe("");
    // 同一本地日的两个不同时刻 → 同一个 label
    expect(formatCommitDate(lateSameDay)).toBe(label);
    // 不同本地日 → 不同 label
    expect(formatCommitDate(nextDayEarly)).not.toBe(label);
  });

  it("shares one label for two instants inside the same local day", () => {
    const today = new Date();
    const oneThirty =
      new Date(today.getFullYear(), today.getMonth(), today.getDate(), 1, 30, 0).getTime() / 1000;
    expect(formatCommitDate(oneThirty)).toBe(formatCommitDate(oneThirty + 3600));
    expect(formatCommitDate(oneThirty)).toBe(new Date(oneThirty * 1000).toLocaleDateString());
  });

  it("keeps local days apart across the local midnight boundary", () => {
    // 本地日边界：本地“当天 00:30”与“前一天 23:30”相差 1 小时但属于不同本地日，
    // 必须得到不同 label（旧实现用 UTC 日桶，在非 UTC 时区会把两者混为一天）。
    const today = new Date();
    const todayEarly =
      new Date(today.getFullYear(), today.getMonth(), today.getDate(), 0, 30, 0).getTime() / 1000;
    const yesterdayLate = todayEarly - 3600;
    expect(formatCommitDate(todayEarly)).not.toBe(formatCommitDate(yesterdayLate));
    expect(formatCommitDate(todayEarly)).toBe(new Date(todayEarly * 1000).toLocaleDateString());
    expect(formatCommitDate(yesterdayLate)).toBe(
      new Date(yesterdayLate * 1000).toLocaleDateString(),
    );
  });
});

describe("formatCommitTime now injection", () => {
  it("buckets the same commit differently as now advances", () => {
    const time = NOW;
    expect(formatCommitTime(time, t, time + 30)).toBe("branches.time.justNow");
    expect(formatCommitTime(time, t, time + 7200)).toBe("branches.time.hoursAgo:2");
    expect(formatCommitTime(time, t, time + 3 * 86400)).toBe("branches.time.daysAgo:3");
  });

  it("treats a future commit time (clock skew) as just-now", () => {
    // now < time → negative diff: must stay in the just-now bucket, as before.
    expect(formatCommitTime(NOW + 3600, t, NOW)).toBe("branches.time.justNow");
  });
});

/** 直调对照表：与 src/lib/commitTime.ts 的 ABSOLUTE_STYLES 逐点对应但独立写下，
 *  这样样式表里的 method/options 被改动时，字节级对比会立刻失败。 */
const DIRECT: Record<AbsoluteTimeStyle, (time: number, locale?: string) => string> = {
  // `date` / `date-time` 的调用点本来就不传 locale：undefined 走无参形式。
  date: (time, locale) =>
    locale === undefined
      ? new Date(time * 1000).toLocaleDateString()
      : new Date(time * 1000).toLocaleDateString(locale),
  ymd: (time, locale) =>
    new Date(time * 1000).toLocaleDateString(locale, {
      year: "numeric",
      month: "short",
      day: "numeric",
    }),
  "ymd-hm": (time, locale) =>
    new Date(time * 1000).toLocaleString(locale, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    }),
  "md-hm": (time, locale) =>
    new Date(time * 1000).toLocaleString(locale, {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    }),
  "date-time": (time, locale) =>
    locale === undefined
      ? new Date(time * 1000).toLocaleString()
      : new Date(time * 1000).toLocaleString(locale),
};

const STYLES = Object.keys(DIRECT) as AbsoluteTimeStyle[];

/** undefined = 系统 locale（调用点不传 locale 的情形）。 */
const LOCALES: (string | undefined)[] = [undefined, "en", "zh-CN"];

describe("formatAbsoluteTime byte parity with the call sites", () => {
  it("matches the literal Intl call for every style and locale", () => {
    for (const style of STYLES) {
      for (const locale of LOCALES) {
        expect(formatAbsoluteTime(NOW, style, locale)).toBe(DIRECT[style](NOW, locale));
      }
    }
  });

  it("returns the same string for repeated identical inputs", () => {
    for (const style of STYLES) {
      const first = formatAbsoluteTime(NOW, style, "en");
      expect(formatAbsoluteTime(NOW, style, "en")).toBe(first);
    }
  });
});

describe("formatAbsoluteTime cache keys", () => {
  it("keeps locales apart, whichever one is formatted first", () => {
    const enFirst = formatAbsoluteTime(NOW, "ymd", "en");
    const zhSecond = formatAbsoluteTime(NOW, "ymd", "zh-CN");
    expect(enFirst).toBe(DIRECT.ymd(NOW, "en"));
    expect(zhSecond).toBe(DIRECT.ymd(NOW, "zh-CN"));
    expect(zhSecond).not.toBe(enFirst);

    // 反向顺序、另一个本地日 → 全新的缓存条目，同样不得串味。
    const otherDay = NOW + 3 * 86400;
    const zhFirst = formatAbsoluteTime(otherDay, "ymd", "zh-CN");
    const enSecond = formatAbsoluteTime(otherDay, "ymd", "en");
    expect(zhFirst).toBe(DIRECT.ymd(otherDay, "zh-CN"));
    expect(enSecond).toBe(DIRECT.ymd(otherDay, "en"));
    expect(enSecond).not.toBe(zhFirst);
  });

  it("keeps different times of one local day apart for time-bearing styles", () => {
    // 同一本地日的两个时刻：粒度粗于标签时（按本地日缓存）会得到同一个 label，
    // 这正是 fix-history-scroll-perf 🔴 R1 的同类陷阱，必须被拦住。
    const morning = new Date(2024, 8, 21, 9, 0, 0).getTime() / 1000;
    const afternoon = new Date(2024, 8, 21, 17, 45, 0).getTime() / 1000;

    expect(formatAbsoluteTime(morning, "md-hm")).not.toBe(formatAbsoluteTime(afternoon, "md-hm"));
    expect(formatAbsoluteTime(morning, "ymd-hm", "en")).not.toBe(
      formatAbsoluteTime(afternoon, "ymd-hm", "en"),
    );
    // 秒粒度：相差 1 秒也必须不同
    expect(formatAbsoluteTime(morning, "date-time")).not.toBe(
      formatAbsoluteTime(morning + 1, "date-time"),
    );
    // 而纯日期粒度：同一本地日的不同时刻必须相同
    expect(formatAbsoluteTime(morning, "date")).toBe(formatAbsoluteTime(afternoon, "date"));
    expect(formatAbsoluteTime(morning, "ymd", "en")).toBe(
      formatAbsoluteTime(afternoon, "ymd", "en"),
    );
  });
});

describe("formatAbsoluteTime edge inputs", () => {
  it("returns a string instead of throwing for NaN / non-finite times", () => {
    for (const style of STYLES) {
      expect(typeof formatAbsoluteTime(Number.NaN, style)).toBe("string");
      expect(typeof formatAbsoluteTime(Number.POSITIVE_INFINITY, style)).toBe("string");
      expect(typeof formatAbsoluteTime(Number.NEGATIVE_INFINITY, style)).toBe("string");
    }
  });
});
