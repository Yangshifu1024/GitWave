// Lightweight read of the active repo's HEAD state (current branch + tip
// sha), shared by the commit-graph context menus to disable menu items that
// don't apply (e.g. "Checkout Commit" on the current HEAD, "Reset" while
// detached). Shares the ["working-copy"] query key with useWorkingCopy so
// both observers read the same cache.

import { useQuery } from "@tanstack/react-query";

import { getWorkingCopy } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

export interface ActiveRepoHeadState {
  /** Current branch name, or null while detached / unborn / unknown. */
  currentBranch: string | null;
  /** Sha the working copy currently points at, or null while unknown. */
  headSha: string | null;
}

export function useActiveRepoState(): ActiveRepoHeadState {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const { data } = useQuery({
    queryKey: ["working-copy", workspaceId, repoId],
    queryFn: () => getWorkingCopy(workspaceId!),
    enabled: Boolean(workspaceId && repoId),
  });
  const branch = data?.branch;
  return {
    currentBranch: branch && branch !== "(detached)" && branch !== "(unborn)" ? branch : null,
    headSha: data?.sha ?? null,
  };
}
