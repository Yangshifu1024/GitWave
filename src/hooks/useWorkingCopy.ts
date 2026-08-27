import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import {
  commit,
  discardChanges,
  formatAppError,
  getWorkingCopy,
  ignorePath,
  stageFiles,
  unstageFiles,
  type FileChange,
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
  commitMessage: (message: string, options?: { onSuccess?: () => void }) => void;
  fetch: () => void;
  pull: (options?: PullOptions) => void;
  push: (options?: PushOptions) => void;
  commitPending: boolean;
  syncPending: { fetch: boolean; pull: boolean; push: boolean };
  isSyncBusy: boolean;
}

export function useWorkingCopy(): UseWorkingCopyResult {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const queryClient = useQueryClient();
  const [actionError, setActionError] = useState<string | null>(null);

  const remoteSync = useRemoteSync((message) => setActionError(message));

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["working-copy", workspaceId, repoId],
    queryFn: () => getWorkingCopy(workspaceId!),
    enabled: Boolean(workspaceId && repoId),
    refetchInterval: 2000,
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["working-copy", workspaceId, repoId] });
  };

  const stageMut = useMutation({
    mutationFn: (paths: string[]) => stageFiles(workspaceId!, paths),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });

  const unstageMut = useMutation({
    mutationFn: (paths: string[]) => unstageFiles(workspaceId!, paths),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });

  const discardMut = useMutation({
    mutationFn: (paths: string[]) => discardChanges(workspaceId!, paths),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });

  const ignoreMut = useMutation({
    mutationFn: (pattern: string) => ignorePath(workspaceId!, pattern),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });

  const commitMut = useMutation({
    mutationFn: (msg: string) => commit(workspaceId!, msg),
    onSuccess: () => {
      setActionError(null);
      invalidate();
      bumpHistory();
    },
    onError: (e) => setActionError(formatAppError(e)),
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
    stage: (paths) => stageMut.mutate(paths),
    unstage: (paths) => unstageMut.mutate(paths),
    discard: (paths) => discardMut.mutate(paths),
    ignore: (pattern) => ignoreMut.mutate(pattern),
    commitMessage: (message, options) =>
      commitMut.mutate(message, { onSuccess: options?.onSuccess }),
    fetch: remoteSync.fetch,
    pull: remoteSync.pull,
    push: remoteSync.push,
    commitPending: commitMut.isPending,
    syncPending: remoteSync.syncPending,
    isSyncBusy: remoteSync.isSyncBusy,
  };
}
