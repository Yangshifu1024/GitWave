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

export const useWorkspaceUiStore = create<WorkspaceUiState>((set) => ({
  activeWorkspaceId: null,
  activeRepoId: null,
  historyEpoch: 0,
  setActiveWorkspaceId: (id) =>
    set((state) => ({
      activeWorkspaceId: id,
      activeRepoId: id === null || id !== state.activeWorkspaceId ? null : state.activeRepoId,
    })),
  setActiveRepoId: (id) => set({ activeRepoId: id }),
  bumpHistoryEpoch: () => set((state) => ({ historyEpoch: state.historyEpoch + 1 })),
  selectWorkspace: (workspaceId, lastActiveRepoId = null) =>
    set({
      activeWorkspaceId: workspaceId,
      activeRepoId: workspaceId ? (lastActiveRepoId ?? null) : null,
    }),
}));
