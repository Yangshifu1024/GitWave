import type { TFunction } from "i18next";
import { describe, expect, it } from "vitest";

import { formatCommitDate, formatCommitTime } from "./commitTime";

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
