import { useIsMutating, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useWorkingCopyErrorStore } from "@/stores/workingCopyErrorStore";
import {
  amendCommit,
  commit,
  discardChanges,
  formatAppError,
  getWorkingCopy,
  ignorePath,
  stageFiles,
  unstageFiles,
  type FileChange,
  type FetchOptions,
  type PullOptions,
  type PushOptions,
  type WorkingCopy,
} from "@/lib/api";
import { partitionFileChanges } from "@/lib/diff";
import { useRemoteSync } from "@/hooks/useRemoteSync";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

export interface UseWorkingCopyResult {
  workspaceId: string | null;
  repoId: string | null;
  data: WorkingCopy | undefined;
  isLoading: boolean;
  isError: boolean;
  error: unknown;
  actionError: string | null;
  setActionError: (message: string | null) => void;
  unstagedFiles: FileChange[];
  stagedFiles: FileChange[];
  isDirty: boolean;
  stage: (paths: string[]) => void;
  unstage: (paths: string[]) => void;
  /** Discard unstaged worktree changes (destructive; caller confirms first). */
  discard: (paths: string[]) => void;
  /** Append a pattern to the repo-root `.gitignore`. */
  ignore: (pattern: string) => void;
  commitMessage: (message: string, options?: { onSuccess?: () => void; amend?: boolean }) => void;
  fetch: (options?: FetchOptions) => void;
  pull: (options?: PullOptions) => void;
  push: (options?: PushOptions) => void;
  commitPending: boolean;
  syncPending: { fetch: boolean; pull: boolean; push: boolean };
  isSyncBusy: boolean;
}

export function useWorkingCopy({
  writeErrors = true,
}: { writeErrors?: boolean } = {}): UseWorkingCopyResult {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const queryClient = useQueryClient();
  const scope = JSON.stringify([workspaceId, repoId]);
  const [failure, setFailure] = useState<{ scope: string; message: string | null } | null>(null);
  const writeError = useWorkingCopyErrorStore((state) => state.errors[scope]);
  const setWriteError = useWorkingCopyErrorStore((state) => state.setError);
  const actionError =
    (writeErrors ? writeError : null) ?? (failure?.scope === scope ? failure.message : null);
  const setActionError = (message: string | null) => {
    setFailure({ scope, message });
    if (message === null) setWriteError(scope, null);
  };
  const commitPending =
    useIsMutating({ mutationKey: ["working-copy-write", workspaceId, repoId] }) > 0;

  const remoteSync = useRemoteSync((message) => setActionError(message));

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["working-copy", workspaceId, repoId],
    queryFn: () => getWorkingCopy(workspaceId!, repoId!),
    enabled: Boolean(workspaceId && repoId),
    refetchInterval: 2000,
  });

  type Write = { workspaceId: string; repoId: string; onSuccess?: () => void } & (
    | { op: "stage" | "unstage" | "discard"; paths: string[] }
    | { op: "ignore"; pattern: string }
    | { op: "commit"; message: string; amend?: boolean }
  );
  const write = useMutation({
    mutationKey: ["working-copy-write", workspaceId, repoId],
    mutationFn: (request: Write) => {
      const { workspaceId: ws, repoId: repo } = request;
      switch (request.op) {
        case "stage":
          return stageFiles(ws, request.paths, repo);
        case "unstage":
          return unstageFiles(ws, request.paths, repo);
        case "discard":
          return discardChanges(ws, request.paths, repo);
        case "ignore":
          return ignorePath(ws, request.pattern, repo);
        case "commit":
          return (request.amend ? amendCommit : commit)(ws, request.message, repo).then(
            () => undefined,
          );
      }
    },
    onSuccess: (_result, request) => {
      setWriteError(JSON.stringify([request.workspaceId, request.repoId]), null);
      void queryClient.invalidateQueries({
        queryKey: ["working-copy", request.workspaceId, request.repoId],
      });
      void queryClient.invalidateQueries({
        queryKey: ["diff-preview", request.workspaceId, request.repoId],
        predicate: (query) => {
          const source = query.queryKey[3] as
            { commit_oid?: string; stash_oid?: string } | undefined;
          return !source?.commit_oid && !source?.stash_oid;
        },
      });
      void queryClient.invalidateQueries({ queryKey: ["health", request.workspaceId] });
      if (request.op === "commit") bumpHistory();
      // Mutation-level callback survives observer unmounts/repository switches.
      request.onSuccess?.();
    },
    onError: (error, request) =>
      setWriteError(JSON.stringify([request.workspaceId, request.repoId]), formatAppError(error)),
  });

  const { unstaged, staged } = partitionFileChanges(data?.files ?? []);

  return {
    workspaceId,
    repoId,
    data,
    isLoading,
    isError,
    error,
    actionError,
    setActionError,
    unstagedFiles: unstaged,
    stagedFiles: staged,
    isDirty: unstaged.length > 0 || staged.length > 0,
    stage: (paths) => {
      if (workspaceId && repoId) write.mutate({ op: "stage", workspaceId, repoId, paths });
    },
    unstage: (paths) => {
      if (workspaceId && repoId) write.mutate({ op: "unstage", workspaceId, repoId, paths });
    },
    discard: (paths) => {
      if (workspaceId && repoId) write.mutate({ op: "discard", workspaceId, repoId, paths });
    },
    ignore: (pattern) => {
      if (workspaceId && repoId) write.mutate({ op: "ignore", workspaceId, repoId, pattern });
    },
    commitMessage: (message, options) => {
      if (workspaceId && repoId)
        write.mutate({
          op: "commit",
          workspaceId,
          repoId,
          message,
          amend: options?.amend,
          onSuccess: options?.onSuccess,
        });
    },
    fetch: remoteSync.fetch,
    pull: remoteSync.pull,
    push: remoteSync.push,
    commitPending,
    syncPending: remoteSync.syncPending,
    isSyncBusy: remoteSync.isSyncBusy,
  };
}
