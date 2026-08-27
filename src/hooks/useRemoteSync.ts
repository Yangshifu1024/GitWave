import { useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { listen } from "@tauri-apps/api/event";

import {
  fetchRemote,
  formatAppError,
  pullRemote,
  pushRemote,
  type PullOptions,
  type SyncProgress,
} from "@/lib/api";
import { useSyncStore } from "@/stores/syncStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

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
  push: () => void;
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

  const handleError = (e: unknown) => {
    useSyncStore.getState().endOp();
    onError?.(formatAppError(e));
  };

  const fetchMut = useMutation({
    mutationFn: () => fetchRemote(workspaceId!),
    onMutate: () => useSyncStore.getState().startOp("fetch"),
    onSuccess: () => {
      invalidate();
      bumpHistory();
    },
    onError: handleError,
    onSettled: () => useSyncStore.getState().endOp(),
  });

  const pullMut = useMutation({
    mutationFn: (options: PullOptions | undefined) => pullRemote(workspaceId!, options),
    onMutate: () => useSyncStore.getState().startOp("pull"),
    onSuccess: () => {
      invalidate();
      bumpHistory();
    },
    onError: handleError,
    onSettled: () => useSyncStore.getState().endOp(),
  });

  const pushMut = useMutation({
    mutationFn: () => pushRemote(workspaceId!),
    onMutate: () => useSyncStore.getState().startOp("push"),
    onSuccess: () => invalidate(),
    onError: handleError,
    onSettled: () => useSyncStore.getState().endOp(),
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
    push: () => {
      if (!workspaceId || isSyncBusy) return;
      pushMut.mutate();
    },
    syncPending: {
      fetch: fetchMut.isPending,
      pull: pullMut.isPending,
      push: pushMut.isPending,
    },
    isSyncBusy,
  };
}
