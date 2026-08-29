import { useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { listen } from "@tauri-apps/api/event";

import {
  fetchRemote,
  formatAppError,
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
 * showing the last one until the next operation overwrites it. */
const SUCCESS_LABELS = {
  fetch: "Fetched from origin",
  pull: "Pulled from origin",
  push: "Pushed to origin",
} as const;

const FAILURE_LABELS = {
  fetch: "Fetch failed",
  pull: "Pull failed",
  push: "Push failed",
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

  useEffect(() => {
    ensureSyncProgressListener();
  }, []);

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["working-copy", workspaceId, repoId] });
  };

  const handleError = (e: unknown, op: "fetch" | "pull" | "push") => {
    useSyncStore.getState().endOp(op);
    useStatusAreaStore.getState().setStatus(FAILURE_LABELS[op], "danger");
    onError?.(formatAppError(e));
  };

  const fetchMut = useMutation({
    mutationFn: () => fetchRemote(workspaceId!),
    onMutate: () => useSyncStore.getState().startOp("fetch"),
    onSuccess: () => {
      useStatusAreaStore.getState().setStatus(SUCCESS_LABELS.fetch, "success");
      invalidate();
      bumpHistory();
    },
    onError: (e) => handleError(e, "fetch"),
    onSettled: () => useSyncStore.getState().endOp("fetch"),
  });

  const pullMut = useMutation({
    mutationFn: (options: PullOptions | undefined) => pullRemote(workspaceId!, options),
    onMutate: () => useSyncStore.getState().startOp("pull"),
    onSuccess: () => {
      useStatusAreaStore.getState().setStatus(SUCCESS_LABELS.pull, "success");
      invalidate();
      bumpHistory();
    },
    onError: (e) => handleError(e, "pull"),
    onSettled: () => useSyncStore.getState().endOp("pull"),
  });

  const pushMut = useMutation({
    mutationFn: (options: PushOptions | undefined) => pushRemote(workspaceId!, options),
    onMutate: () => useSyncStore.getState().startOp("push"),
    onSuccess: () => {
      useStatusAreaStore.getState().setStatus(SUCCESS_LABELS.push, "success");
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
