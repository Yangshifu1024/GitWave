import { useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { listen } from "@tauri-apps/api/event";
import i18next from "i18next";

import {
  fetchRemote,
  formatAppError,
  isCancelledSyncError,
  pullRemote,
  pushRemote,
  type PullOptions,
  type PushOptions,
  type SyncProgress,
} from "@/lib/api";
import { useSyncStore } from "@/stores/syncStore";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

/** Short result lines written to the ActionBar status area; the area keeps
 * showing the last one until the next operation overwrites it. Translated at
 * the generation point: status messages are finalized when emitted and are
 * not replayed on language switch. */
const SUCCESS_KEYS = {
  fetch: "status.sync.fetched",
  pull: "status.sync.pulled",
  push: "status.sync.pushed",
} as const;

const FAILURE_KEYS = {
  fetch: "status.sync.fetchFailed",
  pull: "status.sync.pullFailed",
  push: "status.sync.pushFailed",
} as const;

let syncListenerReady = false;

function ensureSyncProgressListener(): void {
  if (syncListenerReady) return;
  syncListenerReady = true;
  void listen<SyncProgress>("sync-progress", (event) => {
    useSyncStore.getState().updateProgress(event.payload);
  });
}

export interface UseRemoteSyncResult {
  fetch: () => void;
  pull: (options?: PullOptions) => void;
  push: (options?: PushOptions) => void;
  syncPending: { fetch: boolean; pull: boolean; push: boolean };
  isSyncBusy: boolean;
}

export function useRemoteSync(onError?: (message: string) => void): UseRemoteSyncResult {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const queryClient = useQueryClient();
  const activeOp = useSyncStore((s) => s.activeOp);
  const fading = useSyncStore((s) => s.fading);
  const t = i18next.t.bind(i18next);

  useEffect(() => {
    ensureSyncProgressListener();
  }, []);

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["working-copy", workspaceId, repoId] });
  };

  const handleError = (e: unknown, op: "fetch" | "pull" | "push") => {
    useSyncStore.getState().endOp(op);
    if (isCancelledSyncError(e)) {
      // User-initiated abort: report neutrally instead of as a failure.
      useStatusAreaStore.getState().setStatus(t("status.sync.cancelled"), "info");
      return;
    }
    useStatusAreaStore.getState().setStatus(t(FAILURE_KEYS[op]), "danger");
    onError?.(formatAppError(e));
  };

  const fetchMut = useMutation({
    mutationFn: () => fetchRemote(workspaceId!),
    onMutate: () => useSyncStore.getState().startOp("fetch"),
    onSuccess: () => {
      useStatusAreaStore.getState().setStatus(t(SUCCESS_KEYS.fetch), "success");
      invalidate();
      bumpHistory();
    },
    onError: (e) => handleError(e, "fetch"),
    onSettled: () => useSyncStore.getState().endOp("fetch"),
  });

  const pullMut = useMutation({
    mutationFn: (options: PullOptions | undefined) => pullRemote(workspaceId!, options),
    onMutate: (options) => useSyncStore.getState().startOp("pull", options?.remote),
    onSuccess: (_data, options) => {
      useStatusAreaStore
        .getState()
        .setStatus(t(SUCCESS_KEYS.pull, { remote: options?.remote }), "success");
      invalidate();
      bumpHistory();
    },
    onError: (e) => handleError(e, "pull"),
    onSettled: () => useSyncStore.getState().endOp("pull"),
  });

  const pushMut = useMutation({
    mutationFn: (options: PushOptions | undefined) => pushRemote(workspaceId!, options),
    onMutate: (options) => useSyncStore.getState().startOp("push", options?.remote),
    onSuccess: (summary, options) => {
      if (summary.skippedTags.length > 0) {
        // Tag recovery pushed the branch and the non-conflicting tags; the
        // diverged ones need an explicit force push to land.
        useStatusAreaStore.getState().setStatus(
          t("status.sync.pushedSkipped", {
            remote: options?.remote,
            tags: summary.skippedTags.join(", "),
          }),
          "info",
        );
      } else {
        useStatusAreaStore
          .getState()
          .setStatus(t(SUCCESS_KEYS.push, { remote: options?.remote }), "success");
      }
      invalidate();
      bumpHistory();
    },
    onError: (e) => handleError(e, "push"),
    onSettled: () => useSyncStore.getState().endOp("push"),
  });

  const isSyncBusy = activeOp !== null && !fading;

  return {
    fetch: () => {
      if (!workspaceId || isSyncBusy) return;
      fetchMut.mutate();
    },
    pull: (options?: PullOptions) => {
      if (!workspaceId || isSyncBusy) return;
      pullMut.mutate(options);
    },
    push: (options?: PushOptions) => {
      if (!workspaceId || isSyncBusy) return;
      pushMut.mutate(options);
    },
    syncPending: {
      fetch: fetchMut.isPending,
      pull: pullMut.isPending,
      push: pushMut.isPending,
    },
    isSyncBusy,
  };
}
