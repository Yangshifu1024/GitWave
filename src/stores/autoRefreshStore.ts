// Persisted auto-refresh preference shared across top-level components:
// the settings modal writes the toggle + interval and the App-level refresh
// loop reads them to arm, disarm and size the timer, so both must observe
// one shared value. Persistence to localStorage is best-effort.

import { create } from "zustand";

const ENABLED_STORAGE_KEY = "gitwave-auto-refresh";
const INTERVAL_STORAGE_KEY = "gitwave-auto-refresh-interval";

export const DEFAULT_INTERVAL_MINUTES = 5;
export const MIN_INTERVAL_MINUTES = 1;
export const MAX_INTERVAL_MINUTES = 1440;

/** Clamp an arbitrary stored/typed value to a whole number of minutes in
 *  `[MIN, MAX]`, falling back to the default when it is not a finite number. */
export function sanitizeIntervalMinutes(raw: unknown): number {
  if (raw === null || raw === undefined || raw === "") return DEFAULT_INTERVAL_MINUTES;
  const n = typeof raw === "number" ? raw : Number(raw);
  if (!Number.isFinite(n)) return DEFAULT_INTERVAL_MINUTES;
  const whole = Math.round(n);
  if (whole < MIN_INTERVAL_MINUTES) return MIN_INTERVAL_MINUTES;
  if (whole > MAX_INTERVAL_MINUTES) return MAX_INTERVAL_MINUTES;
  return whole;
}

function readStoredAutoRefresh(): boolean {
  try {
    return window.localStorage.getItem(ENABLED_STORAGE_KEY) === "true";
  } catch {
    return false;
  }
}

function readStoredIntervalMinutes(): number {
  try {
    return sanitizeIntervalMinutes(window.localStorage.getItem(INTERVAL_STORAGE_KEY));
  } catch {
    return DEFAULT_INTERVAL_MINUTES;
  }
}

interface AutoRefreshState {
  autoRefresh: boolean;
  intervalMinutes: number;
  setAutoRefresh: (enabled: boolean) => void;
  setIntervalMinutes: (minutes: number) => void;
}

export const useAutoRefreshStore = create<AutoRefreshState>()((set) => ({
  autoRefresh: readStoredAutoRefresh(),
  intervalMinutes: readStoredIntervalMinutes(),
  setAutoRefresh: (autoRefresh) => {
    try {
      window.localStorage.setItem(ENABLED_STORAGE_KEY, String(autoRefresh));
    } catch {
      // Persistence is best-effort; the in-memory value still applies.
    }
    set({ autoRefresh });
  },
  setIntervalMinutes: (minutes) => {
    const intervalMinutes = sanitizeIntervalMinutes(minutes);
    try {
      window.localStorage.setItem(INTERVAL_STORAGE_KEY, String(intervalMinutes));
    } catch {
      // Persistence is best-effort; the in-memory value still applies.
    }
    set({ intervalMinutes });
  },
}));
