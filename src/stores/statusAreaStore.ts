import { create } from "zustand";

export type StatusVariant = "success" | "danger" | "info";

export interface StatusEntry {
  text: string;
  variant: StatusVariant;
}

/** How long a status result stays before the area falls back to its idle
 * content (branch name). Restarted by every setStatus. */
export const STATUS_TTL_MS = 15_000;

interface StatusAreaState {
  /** Last operation result; auto-clears after STATUS_TTL_MS idle. */
  status: StatusEntry | null;
  setStatus: (text: string, variant?: StatusVariant) => void;
  clearStatus: () => void;
}

let ttlTimer: ReturnType<typeof setTimeout> | null = null;

export const useStatusAreaStore = create<StatusAreaState>((set) => ({
  status: null,
  setStatus: (text, variant = "success") => {
    set({ status: { text, variant } });
    if (ttlTimer !== null) clearTimeout(ttlTimer);
    ttlTimer = setTimeout(() => {
      ttlTimer = null;
      set({ status: null });
    }, STATUS_TTL_MS);
  },
  clearStatus: () => {
    if (ttlTimer !== null) {
      clearTimeout(ttlTimer);
      ttlTimer = null;
    }
    set({ status: null });
  },
}));
