// Writes freshly created repos straight into the ["repos", workspaceId]
// query cache.
//
// Why this exists: adding a repo does two independent round trips — moving
// the selection (`setActiveRepo`, a single sqlite write) and refetching the
// list (`listRepos`, which stats every repo path). The selection usually
// lands first, and until the list catches up the new repo is missing from
// `repos`. WorkspaceRepoTabs' liveness guard reads an active id that is
// absent from the rendered list as "the selection is dead" and falls back to
// the first repo — so a fresh add lands on tab 1 instead of the new tab.
//
// Seeding closes that window and, as a bonus, renders the new tab without
// waiting for the refetch. The refetch still runs and wins on ordering and
// fields; this only keeps the list from lagging behind the selection.

import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";

import type { RepoRef } from "@/lib/api";
import { upsertRepos } from "@/lib/repoSelection";

export type SeedRepos = (workspaceId: string, incoming: RepoRef[]) => void;

export function useSeedRepos(): SeedRepos {
  const queryClient = useQueryClient();
  return useCallback(
    (workspaceId: string, incoming: RepoRef[]) => {
      if (incoming.length === 0) return;
      queryClient.setQueryData<RepoRef[]>(["repos", workspaceId], (prev) =>
        // No cache yet means the list is about to be fetched for the first
        // time — it will already contain the new repo.
        prev ? upsertRepos(prev, incoming) : prev,
      );
    },
    [queryClient],
  );
}
