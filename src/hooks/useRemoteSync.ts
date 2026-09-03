import { useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { listen } from "@tauri-apps/api/event";
import i18next from "i18next";

import {
  errorParam,
  fetchRemote,
  formatAppError,
  isAuthError,
  isCancelledSyncError,
  pullRemote,
  pushRemote,
  type CredentialStorageOutcome,
  type FetchOptions,
  type InlineAuth,
  type PullOptions,
  type PushOptions,
  type SyncProgress,
} from "@/lib/api";
import { useSyncStore } from "@/stores/syncStore";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useAuthPromptStore } from "@/stores/authPromptStore";
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
  // How accepted credentials were persisted (F012 fix: the system helper
  // can silently drop them). The backend confirms with `stored` — the
  // common case stays quiet; anything else gets a status-area note so a
  // persistence failure never resurfaces as a mysterious prompt loop.
  void listen<CredentialStorageOutcome>("credential-storage", (event) => {
    const t = i18next.t.bind(i18next);
    const setStatus = useStatusAreaStore.getState().setStatus;
    if (event.payload === "fallback") {
      setStatus(t("status.sync.credentialFallback"), "info");
    } else if (event.payload === "failed") {
      setStatus(t("status.sync.credentialSaveFailed"), "danger");
    }
  });
}

export interface UseRemoteSyncResult {
  fetch: (options?: FetchOptions) => void;
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

  // On authentication failure the F012 auth prompt collects credentials in
  // the app and re-runs the operation through `retry` — no terminal detour.
  // `canPrompt` is false on a retry that failed auth again: the promise is
  // at most one prompt per operation, so a wrong token surfaces as a plain
  // status-area error instead of looping.
  const handleError = (
    e: unknown,
    op: "fetch" | "pull" | "push",
    retry: (auth: InlineAuth) => void,
    remote?: string,
    canPrompt = true,
  ) => {
    useSyncStore.getState().endOp(op);
    if (isCancelledSyncError(e)) {
      // User-initiated abort: report neutrally instead of as a failure.
      useStatusAreaStore.getState().setStatus(t("status.sync.cancelled"), "info");
      return;
    }
    if (isAuthError(e) && canPrompt) {
      useAuthPromptStore.getState().show(remote ?? "origin", retry);
      return;
    }
    useStatusAreaStore.getState().setStatus(t(FAILURE_KEYS[op]), "danger");
    onError?.(formatAppError(e));
  };

  const fetchMut = useMutation({
    mutationFn: (options: FetchOptions | undefined) => fetchRemote(workspaceId!, options),
    onMutate: (options) => useSyncStore.getState().startOp("fetch", options?.remote),
    onSuccess: () => {
      useStatusAreaStore.getState().setStatus(t(SUCCESS_KEYS.fetch), "success");
      invalidate();
      bumpHistory();
    },
    onError: (e, variables) => {
      // Fetch-all fans out over every remote: credentials are host-scoped,
      // so a retry must target only the remote that actually challenged
      // (the backend names it in the error params).
      const failedRemote = errorParam(e, "remote");
      handleError(
        e,
        "fetch",
        (auth) =>
          fetchMut.mutate(
            failedRemote ? { ...variables, remote: failedRemote, auth } : { ...variables, auth },
          ),
        variables?.remote ?? failedRemote,
        variables?.auth === undefined,
      );
    },
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
    onError: (e, variables) =>
      handleError(
        e,
        "pull",
        (auth) => pullMut.mutate({ ...(variables ?? {}), auth }),
        variables?.remote,
        variables?.auth === undefined,
      ),
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
    onError: (e, variables) =>
      handleError(
        e,
        "push",
        (auth) => pushMut.mutate({ ...(variables ?? {}), auth }),
        variables?.remote,
        variables?.auth === undefined,
      ),
    onSettled: () => useSyncStore.getState().endOp("push"),
  });

  const isSyncBusy = activeOp !== null && !fading;

  return {
    fetch: (options?: FetchOptions) => {
      if (!workspaceId || isSyncBusy) return;
      fetchMut.mutate(options);
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
