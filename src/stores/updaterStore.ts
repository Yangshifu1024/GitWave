// UI-facing mirror of the updater flow (src/hooks/useUpdater.ts). The
// plugin's Update object carries download methods, so it lives in a module
// slot inside the hook — this store only holds what the render layer needs.

import { create } from "zustand";

export type UpdaterPhase =
  /** Nothing checked yet (fresh app start, silent check not run/failed). */
  | "idle"
  | "checking"
  /** Update found; this install can download and apply it in-app. */
  | "available"
  /** Update found but the install kind (deb/rpm) must download manually. */
  | "manual-download"
  | "downloading"
  /** Installed; relaunch applies it (macOS) / finishes setup (Windows, AppImage). */
  | "ready"
  /** Manifest has nothing newer than the running version. */
  | "up-to-date"
  | "error";

export interface UpdaterState {
  phase: UpdaterPhase;
  modalOpen: boolean;
  currentVersion: string | null;
  newVersion: string | null;
  downloadedBytes: number;
  totalBytes: number | null;
  error: string | null;
  /** Monotonic id of the latest check; late results from an older check are dropped. */
  checkEpoch: number;
  /** Bump the epoch without touching the phase (silent startup checks stay invisible). */
  startCheckEpoch: () => number;
  beginCheck: () => number;
  markUpToDate: (currentVersion: string) => void;
  markAvailable: (info: { currentVersion: string; newVersion: string; manual: boolean }) => void;
  beginDownload: () => void;
  setProgress: (downloadedBytes: number, totalBytes: number | null) => void;
  markReady: () => void;
  fail: (error: string) => void;
  setModalOpen: (open: boolean) => void;
}

export const useUpdaterStore = create<UpdaterState>()((set, get) => ({
  phase: "idle",
  modalOpen: false,
  currentVersion: null,
  newVersion: null,
  downloadedBytes: 0,
  totalBytes: null,
  error: null,
  checkEpoch: 0,
  startCheckEpoch: () => {
    const checkEpoch = get().checkEpoch + 1;
    set({ checkEpoch });
    return checkEpoch;
  },
  beginCheck: () => {
    const checkEpoch = get().checkEpoch + 1;
    set({ phase: "checking", error: null, checkEpoch });
    return checkEpoch;
  },
  markUpToDate: (currentVersion) => set({ phase: "up-to-date", currentVersion, error: null }),
  markAvailable: ({ currentVersion, newVersion, manual }) =>
    set({
      phase: manual ? "manual-download" : "available",
      currentVersion,
      newVersion,
      modalOpen: true,
      downloadedBytes: 0,
      totalBytes: null,
      error: null,
    }),
  beginDownload: () => set({ phase: "downloading", downloadedBytes: 0, error: null }),
  setProgress: (downloadedBytes, totalBytes) => set({ downloadedBytes, totalBytes }),
  // Re-open the modal: the user may have closed it mid-download, and the
  // relaunch affordance must not go unnoticed.
  markReady: () => set({ phase: "ready", modalOpen: true }),
  fail: (error) =>
    set({ phase: "error", error, newVersion: null, downloadedBytes: 0, totalBytes: null }),
  setModalOpen: (modalOpen) => set({ modalOpen }),
}));
