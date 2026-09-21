import type { TFunction } from "i18next";

/** toLocaleDateString 是 ICU 调用；标签只取决于“哪一天”，按天缓存即可。 */
const dateLabelCache = new Map<string, string>();

/** 缓存键取“本地日历日”：label 由本地时区决定，用 UTC 日桶会把跨本地午夜的
 *  两个时刻误判为同一天（例如 UTC+8 的本地 00:30 与前一天 23:30 落在同一 UTC 桶）。 */
function localDayKey(time: number): string {
  const d = new Date(time * 1000);
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}

export function formatCommitDate(time: number): string {
  const key = localDayKey(time);
  const cached = dateLabelCache.get(key);
  if (cached !== undefined) return cached;
  const label = new Date(time * 1000).toLocaleDateString();
  // 上限保护：缓存不该随仓库历史无限增长
  if (dateLabelCache.size > 512) dateLabelCache.clear();
  dateLabelCache.set(key, label);
  return label;
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
