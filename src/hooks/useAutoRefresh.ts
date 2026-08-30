// User preference + global timer for auto-refreshing repository data.
// The preference follows the app's hook + localStorage convention (see
// usePalette); the loop owns the single 60s timer: bumping the history
// epoch re-walks the commit graph and branch list, invalidating all
// react-query caches refreshes every panel served through react-query.
// Panels with manual effects (Remotes / Worktrees / Submodules) re-run
// because their refresh callbacks depend on the epoch.

import { useCallback, useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import i18next from "i18next";
import { fetchRemote, formatAppError } from "@/lib/api";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useSyncStore } from "@/stores/syncStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

const STORAGE_KEY = "gitwave-auto-refresh";
const INTERVAL_MS = 60_000;

function readStoredAutoRefresh(): boolean {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === "true";
  } catch {
    return false;
  }
}

export interface UseAutoRefreshReturn {
  autoRefresh: boolean;
  setAutoRefresh: (enabled: boolean) => void;
}

export function useAutoRefresh(): UseAutoRefreshReturn {
  const [autoRefresh, setAutoRefreshState] = useState(readStoredAutoRefresh);

  const setAutoRefresh = useCallback((enabled: boolean) => {
    try {
      window.localStorage.setItem(STORAGE_KEY, String(enabled));
    } catch {
      // Persistence is best-effort; the in-memory value still applies.
    }
    setAutoRefreshState(enabled);
  }, []);

  return { autoRefresh, setAutoRefresh };
}

/**
 * Refresh repository data now: fetch from the remote (so history reflects
 * origin, not just local refs), bump the history epoch (commit graph +
 * branch list re-walk) and invalidate every react-query cache (working
 * copy, health, tags, stashes, reflog…). Local re-read happens first for
 * fast feedback, then again after a successful fetch so fetched tips show
 * up in the same refresh. Fetch is skipped while another sync op owns the
 * pipeline and on failure the local-only refresh result stands. Pull /
 * push are never run. The action is surfaced in the status area, whether
 * triggered by the auto-refresh timer or the ⌘R / Ctrl+R shortcut.
 */
export function useRefreshRepo(): () => void {
  const queryClient = useQueryClient();

  return useCallback(() => {
    // Status messages are finalized when emitted (not replayed on language
    // switch), so they translate at the generation point.
    const t = i18next.t.bind(i18next);
    const setStatus = useStatusAreaStore.getState().setStatus;
    setStatus(t("status.sync.refreshing"), "info");

    const refreshLocal = async (): Promise<void> => {
      useWorkspaceUiStore.getState().bumpHistoryEpoch();
      await queryClient.invalidateQueries();
    };

    void (async () => {
      try {
        await refreshLocal();
        const { activeWorkspaceId } = useWorkspaceUiStore.getState();
        const sync = useSyncStore.getState();
        if (!activeWorkspaceId || sync.isBusy()) {
          setStatus(t("status.sync.refreshed"));
          return;
        }
        sync.startOp("fetch");
        try {
          await fetchRemote(activeWorkspaceId);
          // Fetched tips landed in the repo — re-read so they show up now.
          await refreshLocal();
          setStatus(t("status.sync.refreshed"));
        } catch (e) {
          // Local refresh already applied; report the fetch problem.
          setStatus(formatAppError(e), "danger");
        } finally {
          sync.endOp("fetch");
        }
      } catch (e) {
        setStatus(formatAppError(e), "danger");
      }
    })();
  }, [queryClient]);
}

/** Global single timer: refresh everything every minute while enabled. */
export function useAutoRefreshLoop(): void {
  const { autoRefresh } = useAutoRefresh();
  const refreshRepo = useRefreshRepo();

  useEffect(() => {
    if (!autoRefresh) return undefined;
    const timer = window.setInterval(() => refreshRepo(), INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [autoRefresh, refreshRepo]);
}
