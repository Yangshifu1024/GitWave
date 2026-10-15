import { create } from "zustand";

export type WorkbenchMode = "history" | "changes";
export interface ChangeSelection {
  path: string;
  staged: boolean;
}
export interface WorkbenchSession {
  mode: WorkbenchMode;
  commit: string | null;
  historyFile: { commit: string; path: string } | null;
  change: ChangeSelection | null;
}
export const EMPTY_SESSION: WorkbenchSession = {
  mode: "history",
  commit: null,
  historyFile: null,
  change: null,
};
export const workbenchKey = (workspaceId: string | null, repoId: string | null): string =>
  JSON.stringify([workspaceId, repoId]);

/** Lightweight session state survives navigation without keeping every repo mounted. */
export const useWorkbenchStore = create<{
  sessions: Record<string, WorkbenchSession>;
  update: (key: string, patch: Partial<WorkbenchSession>) => void;
}>((set) => ({
  sessions: {},
  update: (key, patch) =>
    set((state) => ({
      sessions: {
        ...state.sessions,
        [key]: { ...(state.sessions[key] ?? EMPTY_SESSION), ...patch },
      },
    })),
}));

/** Follow a stage/unstage to its other side; preserve the side for partially staged files. */
export function reconcileChangeSelection(
  selected: ChangeSelection | null,
  files: readonly ChangeSelection[],
): ChangeSelection | null {
  if (!selected) return null;
  return (
    files.find((file) => file.path === selected.path && file.staged === selected.staged) ??
    files.find((file) => file.path === selected.path) ??
    null
  );
}
