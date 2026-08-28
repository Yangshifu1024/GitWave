import { useCallback, useEffect, useState } from "react";
import type { ConflictFile } from "@/lib/api";
import { abortMerge, listConflicts, mergeInProgress } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

export interface MergeConflictsState {
  /** A merge (MERGE_HEAD) is in progress for the active repo. */
  active: boolean;
  /** Paths still conflicted in the index. Empty while active = all resolved. */
  files: ConflictFile[];
  /** Re-read merge state + conflict list (after resolve / abort / poll tick). */
  refresh: () => Promise<void>;
  /** Abort the in-progress merge (hard reset to HEAD). */
  abort: () => Promise<void>;
}

/**
 * Single owner of merge-conflict polling. `MergeBanner` and `ConflictPanel`
 * both consume one instance so the 3s poll runs once per app.
 *
 * Poll failures are deliberately silent: the banner simply stays hidden and
 * the next tick retries. Action failures (resolve / abort / explain) surface
 * through the components that own them.
 */
export function useMergeConflicts(): MergeConflictsState {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);

  const [active, setActive] = useState(false);
  const [files, setFiles] = useState<ConflictFile[]>([]);

  const refresh = useCallback(async () => {
    if (!workspaceId || !repoId) {
      setActive(false);
      setFiles([]);
      return;
    }
    const inProgress = await mergeInProgress(workspaceId);
    setActive(inProgress);
    setFiles(inProgress ? await listConflicts(workspaceId) : []);
  }, [workspaceId, repoId]);

  useEffect(() => {
    refresh().catch(() => undefined);
    const t = window.setInterval(() => {
      void refresh().catch(() => undefined);
    }, 3000);
    return () => window.clearInterval(t);
  }, [refresh]);

  const abort = useCallback(async () => {
    if (!workspaceId) return;
    await abortMerge(workspaceId);
    await refresh();
  }, [workspaceId, refresh]);

  return { active, files, refresh, abort };
}
