// Shared tags query. The backend resolves commands against the workspace's
// *active* repo, so `repoId` must be part of the query key: switching repos
// changes the key (refetch against the new active repo) while `enabled` keeps
// the query parked until both ids are known. Repo switches are safe — every
// switch site awaits `set_active_repo` (backend pointer) before updating the
// UI store, so a refetch never races ahead of the pointer.
//
// Consumers: TagsPanel (sidebar card) and CommitInfoHeader (tag manager).

import { useQuery, useQueryClient, type UseQueryResult } from "@tanstack/react-query";

import { listTags, type TagInfo } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

export function useTags(): UseQueryResult<TagInfo[], Error> & {
  workspaceId: string | null;
  repoId: string | null;
  /** Invalidate the tags query for the currently active repo. */
  invalidate: () => void;
} {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const queryClient = useQueryClient();

  const query = useQuery<TagInfo[], Error>({
    queryKey: ["tags", workspaceId, repoId],
    queryFn: () => listTags(workspaceId!),
    enabled: Boolean(workspaceId && repoId),
  });

  return {
    ...query,
    workspaceId,
    repoId,
    // Invalidate the exact active (workspaceId, repoId) pair.
    invalidate: () => {
      void queryClient.invalidateQueries({ queryKey: ["tags", workspaceId, repoId] });
    },
  };
}
