import { useCallback, useEffect, useRef, useState } from "react";
import type { ConflictFile } from "@/lib/api";
import { abortMerge, listConflicts, mergeInProgress } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

export interface MergeConflictsState {
  /** A merge is in progress or unresolved index entries need attention. */
  active: boolean;
  /** MERGE_HEAD exists; only then is abort merge available. */
  mergeInProgress: boolean;
  /** Paths still conflicted in the index. Empty during a merge = all resolved. */
  files: ConflictFile[];
  /** Re-read merge state + conflict list (after resolve / abort / poll tick). */
  refresh: () => Promise<void>;
  /** Abort the in-progress merge (hard reset to HEAD). */
  abort: () => Promise<void>;
}

/**
 * Single owner of conflict polling. `MergeBanner` and `ConflictPanel`
 * both consume one instance so the 3s poll runs once per app.
 *
 * Poll failures are deliberately silent: the banner retains its previous state and
 * the next tick retries. Action failures (resolve / abort / explain) surface
 * through the components that own them.
 */
export function useMergeConflicts(): MergeConflictsState {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);

  const repoKey = `${workspaceId ?? ""}\0${repoId ?? ""}`;
  const requestSeq = useRef(0);
  const [snapshot, setSnapshot] = useState<{
    repoKey: string;
    mergeInProgress: boolean;
    files: ConflictFile[];
  }>({ repoKey: "", mergeInProgress: false, files: [] });
  // Never expose another repository's conflicts during a tab switch.
  const current = snapshot.repoKey === repoKey ? snapshot : null;
  const files = current?.files ?? [];
  const mergeActive = current?.mergeInProgress ?? false;
  const active = mergeActive || files.length > 0;

  const refresh = useCallback(async () => {
    const seq = ++requestSeq.current;
    if (!workspaceId || !repoId) {
      setSnapshot({ repoKey, mergeInProgress: false, files: [] });
      return;
    }
    const [inProgress, conflicts] = await Promise.all([
      mergeInProgress(workspaceId),
      listConflicts(workspaceId),
    ]);
    if (seq === requestSeq.current) {
      setSnapshot({ repoKey, mergeInProgress: inProgress, files: conflicts });
    }
  }, [workspaceId, repoId, repoKey]);

  useEffect(() => {
    refresh().catch(() => undefined);
    const t = window.setInterval(() => {
      void refresh().catch(() => undefined);
    }, 3000);
    return () => {
      requestSeq.current += 1;
      window.clearInterval(t);
    };
  }, [refresh]);

  const abort = useCallback(async () => {
    if (!workspaceId) return;
    await abortMerge(workspaceId);
    await refresh();
  }, [workspaceId, refresh]);

  return { active, mergeInProgress: mergeActive, files, refresh, abort };
}
