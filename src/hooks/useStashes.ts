// Shared stashes query. The backend resolves commands against the workspace's
// *active* repo, so `repoId` must be part of the query key: switching repos
// changes the key (refetch against the new active repo) while `enabled` keeps
// the query parked until both ids are known. Without repoId the sidebar kept
// showing the previous repo's stash list, and drop / apply / pop then hit the
// new repo with a stale index (wrong entry or a range error).
//
// Consumers: StashPanel (sidebar card) and ActionBar (save-stash invalidation).

import { useQuery, useQueryClient, type UseQueryResult } from "@tanstack/react-query";

import { listStashes, type StashEntry } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

/** Query key for the stash list of one (workspace, repo) pair. */
export function stashesQueryKey(
  workspaceId: string | null,
  repoId: string | null,
): readonly ["stashes", string | null, string | null] {
  return ["stashes", workspaceId, repoId];
}

export function useStashes(): UseQueryResult<StashEntry[], Error> & {
  workspaceId: string | null;
  repoId: string | null;
  /** Invalidate the stash list for the currently active repo. */
  invalidate: () => void;
} {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const queryClient = useQueryClient();

  const query = useQuery<StashEntry[], Error>({
    queryKey: stashesQueryKey(workspaceId, repoId),
    queryFn: () => listStashes(workspaceId!),
    enabled: Boolean(workspaceId && repoId),
  });

  return {
    ...query,
    workspaceId,
    repoId,
    // Invalidate the exact active (workspaceId, repoId) pair.
    invalidate: () => {
      void queryClient.invalidateQueries({ queryKey: stashesQueryKey(workspaceId, repoId) });
    },
  };
}
