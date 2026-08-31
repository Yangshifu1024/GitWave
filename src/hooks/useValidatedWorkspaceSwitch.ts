import { useQueryClient } from "@tanstack/react-query";

import type { RepoRef } from "@/lib/api";
import { pickRestoredRepo } from "@/lib/repoSelection";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

/**
 * Switch workspace without landing on a missing repo: when the target
 * workspace's repos are already cached, revalidate lastActiveRepoId against
 * ["repos", id] synchronously (pickRestoredRepo fallback). On a cache miss
 * switch optimistically — the tab strip's guard effect corrects the
 * selection once its own repos query lands.
 */
export function useValidatedWorkspaceSwitch(): (
  workspaceId: string,
  lastActiveRepoId: string | null,
) => void {
  const queryClient = useQueryClient();
  const selectWorkspace = useWorkspaceUiStore((s) => s.selectWorkspace);
  return (workspaceId, lastActiveRepoId) => {
    const cached = queryClient.getQueryData<RepoRef[]>(["repos", workspaceId]);
    selectWorkspace(
      workspaceId,
      cached ? pickRestoredRepo(cached, lastActiveRepoId) : lastActiveRepoId,
    );
  };
}
