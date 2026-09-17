// User preference + global timer for auto-refreshing repository data.
// The preference lives in the autoRefreshStore so the settings modal
// (writer) and the App-level loop (reader) observe one shared value.
// The loop owns the single timer: every N minutes (configurable) it refreshes
// the whole active workspace — bumping the history epoch re-walks the commit
// graph and branch list of the active repo, invalidating all react-query
// caches refreshes every panel served through react-query, and fetching every
// repo in the workspace keeps their remote-tracking refs current. Panels with
// manual effects (Remotes / Worktrees / Submodules) re-run because their
// refresh callbacks depend on the epoch.

import { useCallback, useEffect } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import i18next from "i18next";
import { fetchRemote, fetchWorkspaceRepos, formatAppError, isCancelledSyncError } from "@/lib/api";
import { useAutoRefreshStore } from "@/stores/autoRefreshStore";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { nextSyncRequestId, useSyncStore } from "@/stores/syncStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

export interface UseAutoRefreshReturn {
  autoRefresh: boolean;
  intervalMinutes: number;
  setAutoRefresh: (enabled: boolean) => void;
  setIntervalMinutes: (minutes: number) => void;
}

export function useAutoRefresh(): UseAutoRefreshReturn {
  const autoRefresh = useAutoRefreshStore((s) => s.autoRefresh);
  const intervalMinutes = useAutoRefreshStore((s) => s.intervalMinutes);
  const setAutoRefresh = useAutoRefreshStore((s) => s.setAutoRefresh);
  const setIntervalMinutes = useAutoRefreshStore((s) => s.setIntervalMinutes);
  return { autoRefresh, intervalMinutes, setAutoRefresh, setIntervalMinutes };
}

/** Bump the history epoch (commit graph + branch list re-walk) and invalidate
 *  every react-query cache (working copy, health, tags, stashes, reflog…). */
async function refreshLocalCaches(queryClient: QueryClient): Promise<void> {
  useWorkspaceUiStore.getState().bumpHistoryEpoch();
  await queryClient.invalidateQueries();
}

/**
 * Refresh the active repo now (manual ⌘R / Ctrl+R path): fetch from its
 * remote (so history reflects origin, not just local refs), bump the history
 * epoch and invalidate every react-query cache. Local re-read happens first
 * for fast feedback, then again after a successful fetch so fetched tips show
 * up in the same refresh. Fetch is skipped while another sync op owns the
 * pipeline and on failure the local-only refresh result stands. Pull / push
 * are never run.
 */
export function useRefreshRepo(): () => void {
  const queryClient = useQueryClient();

  return useCallback(() => {
    // Status messages are finalized when emitted (not replayed on language
    // switch), so they translate at the generation point.
    const t = i18next.t.bind(i18next);
    const setStatus = useStatusAreaStore.getState().setStatus;
    setStatus(t("status.sync.refreshing"), "info");

    void (async () => {
      try {
        await refreshLocalCaches(queryClient);
        const { activeWorkspaceId } = useWorkspaceUiStore.getState();
        const sync = useSyncStore.getState();
        if (!activeWorkspaceId || sync.isBusy()) {
          setStatus(t("status.sync.refreshed"));
          return;
        }
        const requestId = nextSyncRequestId("fetch");
        sync.startOp("fetch", null, requestId);
        try {
          await fetchRemote(activeWorkspaceId, { requestId });
          // Fetched tips landed in the repo — re-read so they show up now.
          await refreshLocalCaches(queryClient);
          setStatus(t("status.sync.refreshed"));
        } catch (e) {
          // Local refresh already applied; report the fetch problem. A
          // user-initiated cancel is neutral, not a failure — same as the
          // manual sync path in useRemoteSync.
          if (isCancelledSyncError(e)) {
            setStatus(t("status.sync.cancelled"), "info");
          } else {
            setStatus(formatAppError(e), "danger");
          }
        } finally {
          sync.endOp("fetch", requestId);
        }
      } catch (e) {
        setStatus(formatAppError(e), "danger");
      }
    })();
  }, [queryClient]);
}

/**
 * Refresh the whole active workspace (auto-refresh path): same local re-read
 * as {@link useRefreshRepo}, but fetches every repo in the workspace
 * best-effort. The status area reports how many repos succeeded; a partial
 * result is informational, not an error — a background sweep must not turn
 * red because one repo needs credentials. Local data for non-active repos is
 * re-read when the user switches to them (their query keys include the repo
 * id).
 */
export function useRefreshWorkspaceRepos(): () => void {
  const queryClient = useQueryClient();

  return useCallback(() => {
    const t = i18next.t.bind(i18next);
    const setStatus = useStatusAreaStore.getState().setStatus;
    setStatus(t("status.sync.refreshing"), "info");

    void (async () => {
      try {
        await refreshLocalCaches(queryClient);
        const { activeWorkspaceId } = useWorkspaceUiStore.getState();
        const sync = useSyncStore.getState();
        if (!activeWorkspaceId || sync.isBusy()) {
          setStatus(t("status.sync.refreshed"));
          return;
        }
        const requestId = nextSyncRequestId("fetch");
        sync.startOp("fetch", null, requestId);
        try {
          const summary = await fetchWorkspaceRepos(activeWorkspaceId, { requestId });
          // Fetched tips landed in the repos — re-read so they show up now.
          await refreshLocalCaches(queryClient);
          if (summary.failed === 0) {
            setStatus(t("status.sync.refreshedAll", { count: summary.succeeded }));
          } else {
            setStatus(
              t("status.sync.refreshedAllPartial", {
                ok: summary.succeeded,
                total: summary.total,
                failed: summary.failed,
              }),
              "info",
            );
          }
        } catch (e) {
          if (isCancelledSyncError(e)) {
            setStatus(t("status.sync.cancelled"), "info");
          } else {
            setStatus(formatAppError(e), "danger");
          }
        } finally {
          sync.endOp("fetch", requestId);
        }
      } catch (e) {
        setStatus(formatAppError(e), "danger");
      }
    })();
  }, [queryClient]);
}

/** Global single timer: refresh the active workspace's repos every N minutes. */
export function useAutoRefreshLoop(): void {
  const { autoRefresh, intervalMinutes } = useAutoRefresh();
  const refreshWorkspace = useRefreshWorkspaceRepos();

  useEffect(() => {
    if (!autoRefresh) return undefined;
    const timer = window.setInterval(() => {
      // The guard above only runs when the effect is (re)created; re-check
      // at tick time so the toggle wins even with a stale closure.
      if (useAutoRefreshStore.getState().autoRefresh) refreshWorkspace();
    }, intervalMinutes * 60_000);
    return () => window.clearInterval(timer);
  }, [autoRefresh, intervalMinutes, refreshWorkspace]);
}
