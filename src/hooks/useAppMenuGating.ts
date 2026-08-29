// Enable/disable inputs for the app menu, shared by the in-app menu bar and
// the macOS native menu so both gate identically. Memoized on primitives:
// the native menu rebuilds on identity change, and the working-copy poll
// (2s) must not trigger a rebuild every tick.

import { useMemo } from "react";

import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import type { MenuGating } from "@/lib/appMenuSpec";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

export function useAppMenuGating(): MenuGating {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const wc = useWorkingCopy();

  return useMemo(
    () => ({
      noWorkspace: !activeWorkspaceId,
      noRepo: !activeRepoId,
      detached: wc.data?.branch === "(detached)",
      hasSha: Boolean(wc.data?.sha),
      syncBusy: wc.isSyncBusy,
    }),
    [activeWorkspaceId, activeRepoId, wc.data?.branch, wc.data?.sha, wc.isSyncBusy],
  );
}
