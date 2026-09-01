// Persisted auto-refresh preference shared across top-level components:
// the settings modal writes the toggle and the App-level refresh loop
// reads it to arm and disarm the 60s timer, so both must observe one
// shared value. Persistence to localStorage is best-effort.

import { create } from "zustand";

const STORAGE_KEY = "gitwave-auto-refresh";

function readStoredAutoRefresh(): boolean {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "true";
  } catch {
    return false;
  }
}

interface AutoRefreshState {
  autoRefresh: boolean;
  setAutoRefresh: (enabled: boolean) => void;
}

export const useAutoRefreshStore = create<AutoRefreshState>()((set) => ({
  autoRefresh: readStoredAutoRefresh(),
  setAutoRefresh: (autoRefresh) => {
    try {
      window.localStorage.setItem(STORAGE_KEY, String(autoRefresh));
    } catch {
      // Persistence is best-effort; the in-memory value still applies.
    }
    set({ autoRefresh });
  },
}));
