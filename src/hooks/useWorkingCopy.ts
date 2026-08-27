import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import {
  commit,
  fetchRemote,
  formatAppError,
  getWorkingCopy,
  pullRemote,
  pushRemote,
  stageFiles,
  unstageFiles,
  type FileChange,
  type WorkingCopy,
} from "@/lib/api";
import { partitionFileChanges } from "@/lib/diff";
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
  commitMessage: (message: string, options?: { onSuccess?: () => void }) => void;
  fetch: () => void;
  pull: () => void;
  push: () => void;
  commitPending: boolean;
  syncPending: { fetch: boolean; pull: boolean; push: boolean };
}

export function useWorkingCopy(): UseWorkingCopyResult {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const queryClient = useQueryClient();
  const [actionError, setActionError] = useState<string | null>(null);

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

  const commitMut = useMutation({
    mutationFn: (msg: string) => commit(workspaceId!, msg),
    onSuccess: () => {
      setActionError(null);
      invalidate();
      bumpHistory();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });

  const fetchMut = useMutation({
    mutationFn: () => fetchRemote(workspaceId!),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });

  const pullMut = useMutation({
    mutationFn: () => pullRemote(workspaceId!),
    onSuccess: () => {
      setActionError(null);
      invalidate();
      bumpHistory();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });

  const pushMut = useMutation({
    mutationFn: () => pushRemote(workspaceId!),
    onSuccess: () => {
      setActionError(null);
      invalidate();
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
    commitMessage: (message, options) => commitMut.mutate(message, { onSuccess: options?.onSuccess }),
    fetch: () => fetchMut.mutate(),
    pull: () => pullMut.mutate(),
    push: () => pushMut.mutate(),
    commitPending: commitMut.isPending,
    syncPending: {
      fetch: fetchMut.isPending,
      pull: pullMut.isPending,
      push: pushMut.isPending,
    },
  };
}
