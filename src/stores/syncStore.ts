import { create } from "zustand";
import type { SyncOperation, SyncProgress } from "@/lib/api";

/** Human label for an in-flight sync operation ("Pulling changes…"). */
export function syncOperationLabel(op: SyncOperation | null): string | null {
  if (!op) return null;
  switch (op) {
    case "fetch":
      return "Fetching from origin…";
    case "pull":
      return "Pulling changes…";
    case "push":
      return "Pushing to origin…";
  }
}

interface SyncStoreState {
  activeOp: SyncOperation | null;
  receivedObjects: number;
  totalObjects: number;
  receivedBytes: number;
  fading: boolean;
  startOp: (op: SyncOperation) => void;
  updateProgress: (progress: SyncProgress) => void;
  endOp: () => void;
  isBusy: () => boolean;
}

export const useSyncStore = create<SyncStoreState>((set, get) => ({
  activeOp: null,
  receivedObjects: 0,
  totalObjects: 0,
  receivedBytes: 0,
  fading: false,
  startOp: (op) =>
    set({
      activeOp: op,
      receivedObjects: 0,
      totalObjects: 0,
      receivedBytes: 0,
      fading: false,
    }),
  updateProgress: (progress) =>
    set({
      activeOp: progress.operation,
      receivedObjects: progress.receivedObjects,
      totalObjects: progress.totalObjects,
      receivedBytes: progress.receivedBytes,
    }),
  endOp: () => {
    set({ fading: true });
    window.setTimeout(() => {
      if (get().fading) {
        set({
          activeOp: null,
          receivedObjects: 0,
          totalObjects: 0,
          receivedBytes: 0,
          fading: false,
        });
      }
    }, 150);
  },
  isBusy: () => get().activeOp !== null && !get().fading,
}));
