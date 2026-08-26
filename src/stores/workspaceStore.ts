// UI state for workspaces — kept separate from server state (TanStack
// Query). Zustand for transient UI selections, Query for IPC-derived data.

import { create } from "zustand";

interface WorkspaceUiState {
  activeWorkspaceId: string | null;
  setActiveWorkspaceId: (id: string | null) => void;
}

export const useWorkspaceUiStore = create<WorkspaceUiState>((set) => ({
  activeWorkspaceId: null,
  setActiveWorkspaceId: (id) => set({ activeWorkspaceId: id }),
}));