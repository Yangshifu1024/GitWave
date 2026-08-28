// UI state for workspaces — kept separate from server state (TanStack
// Query). Zustand for transient UI selections, Query for IPC-derived data.

import { create } from "zustand";

interface WorkspaceUiState {
  activeWorkspaceId: string | null;
  activeRepoId: string | null;
  /** Bumped when git graph tips change (checkout / branch CRUD) so History refetches. */
  historyEpoch: number;
  setActiveWorkspaceId: (id: string | null) => void;
  setActiveRepoId: (id: string | null) => void;
  bumpHistoryEpoch: () => void;
  /** Switch workspace and sync active repo from persisted last_active_repo_id. */
  selectWorkspace: (workspaceId: string | null, lastActiveRepoId?: string | null) => void;
}

// PM 1.4: "重启后状态完整恢复" — the workspace/repo records live in sqlite;
// the active-selection pointer is UI state and rides in localStorage.
const LAST_WS_KEY = "gitwave.lastActiveWorkspaceId";
const LAST_REPO_KEY = "gitwave.lastActiveRepoId";

export function rememberActive(workspaceId: string | null, repoId: string | null): void {
  try {
    if (workspaceId) window.localStorage.setItem(LAST_WS_KEY, workspaceId);
    else window.localStorage.removeItem(LAST_WS_KEY);
    if (repoId) window.localStorage.setItem(LAST_REPO_KEY, repoId);
    else window.localStorage.removeItem(LAST_REPO_KEY);
  } catch {
    // Storage unavailable (private mode etc.) — restore simply won't happen.
  }
}

export function readLastActive(): { workspaceId: string | null; repoId: string | null } {
  try {
    return {
      workspaceId: window.localStorage.getItem(LAST_WS_KEY),
      repoId: window.localStorage.getItem(LAST_REPO_KEY),
    };
  } catch {
    return { workspaceId: null, repoId: null };
  }
}

export const useWorkspaceUiStore = create<WorkspaceUiState>((set) => ({
  activeWorkspaceId: null,
  activeRepoId: null,
  historyEpoch: 0,
  setActiveWorkspaceId: (id) =>
    set((state) => {
      const next: Partial<WorkspaceUiState> = {
        activeWorkspaceId: id,
        activeRepoId: id === null || id !== state.activeWorkspaceId ? null : state.activeRepoId,
      };
      rememberActive(id, next.activeRepoId ?? null);
      return next;
    }),
  setActiveRepoId: (id) => {
    rememberActive(useWorkspaceUiStore.getState().activeWorkspaceId, id);
    set({ activeRepoId: id });
  },
  bumpHistoryEpoch: () => set((state) => ({ historyEpoch: state.historyEpoch + 1 })),
  selectWorkspace: (workspaceId, lastActiveRepoId = null) => {
    rememberActive(workspaceId, workspaceId ? (lastActiveRepoId ?? null) : null);
    set({
      activeWorkspaceId: workspaceId,
      activeRepoId: workspaceId ? (lastActiveRepoId ?? null) : null,
    });
  },
}));
