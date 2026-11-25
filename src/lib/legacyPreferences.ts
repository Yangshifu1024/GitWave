import { invoke, isTauri } from "@tauri-apps/api/core";

const preferenceKeys = [
  "gitwave.language",
  "gitwave.aiLanguage",
  "gitwave-theme",
  "gitwave-palette",
  "gitwave-font-sans",
  "gitwave-font-mono",
  "gitwave-font-sans-size",
  "gitwave-font-mono-size",
  "gitwave.split.navigator",
  "gitwave.split.history-details",
  "gitwave.split.history-files",
  "gitwave.split.changes-files",
  "gitwave.lastActiveWorkspaceId",
  "gitwave.lastActiveRepoId",
  "gitwave-auto-refresh",
  "gitwave-auto-refresh-interval",
  "gitwave-auto-update",
] as const;

let enabled = false;
let pending: ReturnType<typeof setTimeout> | undefined;
let writes: Promise<void> = Promise.resolve();

export function startLegacyPreferenceExport(): void {
  if (enabled || !isTauri()) return;
  enabled = true;
  void flushLegacyPreferences().catch(() => undefined);
}

// A drag can save dozens of intermediate values. Coalesce these writes.
export function queueLegacyPreferenceExport(): void {
  if (!enabled || pending !== undefined) return;
  pending = setTimeout(() => void flushLegacyPreferences().catch(() => undefined), 150);
}

export function flushLegacyPreferences(): Promise<void> {
  if (!enabled) return writes;
  clearTimeout(pending);
  pending = undefined;
  const values: Record<string, string> = {};
  try {
    for (const key of preferenceKeys) {
      const value = localStorage.getItem(key);
      if (value !== null) values[key] = value;
    }
  } catch {
    return Promise.reject(new Error("Could not read the previous version's desktop preferences"));
  }
  // A later sample must always commit after an older in-flight write.
  const write = writes.then(() => invoke<void>("cmd_write_legacy_preferences", { values }));
  writes = write.catch(() => undefined);
  return write;
}

export async function finishLegacyPreferencesBeforeQuit(): Promise<void> {
  let deadline: ReturnType<typeof setTimeout> | undefined;
  try {
    await Promise.race([
      flushLegacyPreferences(),
      new Promise<void>((resolve) => {
        deadline = setTimeout(resolve, 500);
      }),
    ]);
  } catch {
    // Quitting remains possible if the previous state database is unavailable.
  } finally {
    clearTimeout(deadline);
  }
}
