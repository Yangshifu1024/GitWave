import { create } from "zustand";
import i18next from "i18next";
import type { SyncOperation, SyncProgress } from "@/lib/api";

/** Fade-out window before an ended op clears; SyncStatusArea's
 * `duration-150` transition must stay in sync with this. */
export const OP_FADE_MS = 150;

/** UI-initiated operations (outside the toolbar's fetch/pull/push) that drive
 * the status area's in-flight indicator. "remote-op" may still hit the
 * network (per-remote fetch), but never emits backend progress events. */
export type UiOperation =
  "checkout" | "delete" | "merge" | "rebase" | "stash" | "worktree" | "remote-op";

export type ActiveOperation = SyncOperation | UiOperation;

const UI_OPERATIONS: readonly ActiveOperation[] = [
  "checkout",
  "delete",
  "merge",
  "rebase",
  "stash",
  "worktree",
  "remote-op",
];

/** Human label for an in-flight operation ("Pulling changes…"). Translated at
 * the generation point: in-flight labels are finalized when the op starts and
 * are not replayed on language switch. */
export function operationLabel(op: ActiveOperation | null): string | null {
  if (!op) return null;
  const t = i18next.t.bind(i18next);
  switch (op) {
    case "fetch":
      return t("status.sync.fetching");
    case "pull":
      return t("status.sync.pulling");
    case "push":
      return t("status.sync.pushing");
    case "checkout":
      return t("status.sync.checkingOut");
    case "delete":
      return t("status.sync.deletingBranch");
    case "merge":
      return t("status.sync.merging");
    case "rebase":
      return t("status.sync.rebasing");
    case "stash":
      return t("status.sync.savingStash");
    case "worktree":
      return t("status.sync.creatingWorktree");
    case "remote-op":
      return t("status.sync.remoteOp");
  }
}

interface SyncStoreState {
  activeOp: ActiveOperation | null;
  receivedObjects: number;
  totalObjects: number;
  receivedBytes: number;
  fading: boolean;
  startOp: (op: ActiveOperation) => void;
  updateProgress: (progress: SyncProgress) => void;
  /** End the matching op; a no-op if another op took over the slot. */
  endOp: (op: ActiveOperation) => void;
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
  updateProgress: (progress) => {
    const { activeOp, fading } = get();
    // Backend progress events only belong to in-flight sync ops. Ignore
    // stragglers after endOp (would revive the op and stick the status area
    // in "sync" forever) and events arriving under a UI-started op.
    if (fading || activeOp === null || UI_OPERATIONS.includes(activeOp)) return;
    set({
      activeOp: progress.operation,
      receivedObjects: progress.receivedObjects,
      totalObjects: progress.totalObjects,
      receivedBytes: progress.receivedBytes,
    });
  },
  endOp: (op) => {
    if (get().activeOp !== op) return;
    set({ fading: true });
    setTimeout(() => {
      if (get().fading && get().activeOp === op) {
        set({
          activeOp: null,
          receivedObjects: 0,
          totalObjects: 0,
          receivedBytes: 0,
          fading: false,
        });
      }
    }, OP_FADE_MS);
  },
  isBusy: () => get().activeOp !== null && !get().fading,
}));
