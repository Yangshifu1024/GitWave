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

// Seeded by wall clock + a random suffix so an HMR module reload (which
// re-runs this module) cannot recycle ids of still in-flight operations
// from before the reload — the clock seed alone only makes reuse unlikely.
let syncRequestSeq = Date.now() % 1_000_000;
const idSalt = Math.random().toString(36).slice(2, 8);

/** Caller-generated operation instance id. The starter passes it both to
 * `startOp` and to the backend invoke options, so progress events (stamped
 * server-side with the same id) attribute to exactly one slot occupant —
 * overlapping operations can no longer overwrite each other's progress. */
export function nextSyncRequestId(op: ActiveOperation): string {
  syncRequestSeq += 1;
  return `${op}-${syncRequestSeq}-${idSalt}`;
}

/** Human label for an in-flight operation ("Pulling changes…"). Translated at
 * the generation point: in-flight labels are finalized when the op starts and
 * are not replayed on language switch. `remote` interpolates into labels that
 * name their target ("Pushing to {remote}…"); ops without one (fetch-all,
 * UI operations) ignore it. */
export function operationLabel(op: ActiveOperation | null, remote?: string | null): string | null {
  if (!op) return null;
  const t = i18next.t.bind(i18next);
  switch (op) {
    case "fetch":
      return t("status.sync.fetching");
    case "pull":
      return t("status.sync.pulling");
    case "push":
      return remote ? t("status.sync.pushing", { remote }) : t("status.sync.remoteOp");
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
  /** Remote name attached to the in-flight op, for label interpolation. */
  activeRemote: string | null;
  /** Backend invocation id of the slot occupant; events from other
   * instances are dropped instead of overwriting the slot. */
  activeRequestId: string | null;
  receivedObjects: number;
  totalObjects: number;
  receivedBytes: number;
  fading: boolean;
  startOp: (op: ActiveOperation, remote?: string | null, requestId?: string | null) => void;
  updateProgress: (progress: SyncProgress) => void;
  /** End the matching op; a no-op if another op took over the slot. When
   * `requestId` is given, the slot must still hold that instance. */
  endOp: (op: ActiveOperation, requestId?: string | null) => void;
  isBusy: () => boolean;
}

export const useSyncStore = create<SyncStoreState>((set, get) => ({
  activeOp: null,
  activeRemote: null,
  activeRequestId: null,
  receivedObjects: 0,
  totalObjects: 0,
  receivedBytes: 0,
  fading: false,
  startOp: (op, remote = null, requestId = null) =>
    set({
      activeOp: op,
      activeRemote: remote,
      activeRequestId: requestId,
      receivedObjects: 0,
      totalObjects: 0,
      receivedBytes: 0,
      fading: false,
    }),
  updateProgress: (progress) => {
    const { activeOp, activeRequestId, fading } = get();
    // Backend progress events only belong to in-flight sync ops. Ignore
    // stragglers after endOp (would revive the op and stick the status area
    // in "sync" forever) and events arriving under a UI-started op.
    if (fading || activeOp === null || UI_OPERATIONS.includes(activeOp)) return;
    // Instance check: a superseded operation's late events must not
    // overwrite the current occupant's progress (or flip activeOp back).
    // An empty id matches anything — belt and braces for unstamped events.
    if (
      activeRequestId !== null &&
      progress.requestId !== "" &&
      progress.requestId !== activeRequestId
    ) {
      return;
    }
    set({
      activeOp: progress.operation,
      receivedObjects: progress.receivedObjects,
      totalObjects: progress.totalObjects,
      receivedBytes: progress.receivedBytes,
    });
  },
  endOp: (op, requestId = null) => {
    const { activeOp, activeRequestId } = get();
    if (activeOp !== op) return;
    if (requestId !== null && activeRequestId !== null && activeRequestId !== requestId) {
      return;
    }
    set({ fading: true });
    const endedRequestId = activeRequestId;
    setTimeout(() => {
      // `endedRequestId` guards a back-to-back same-name op: without it, the
      // old timer firing between the new op's own endOp and its fade would
      // clear the new occupant's slot early.
      if (get().fading && get().activeOp === op && get().activeRequestId === endedRequestId) {
        set({
          activeOp: null,
          activeRemote: null,
          activeRequestId: null,
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
