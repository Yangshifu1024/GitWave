import { create } from "zustand";

export type WorkbenchMode = "history" | "changes";
export interface ChangeSelection {
  path: string;
  staged: boolean;
}
export interface WorkbenchSession {
  mode: WorkbenchMode;
  commit: string | null;
  lastCreatedCommit: string | null;
  historyFile: { commit: string; path: string } | null;
  change: ChangeSelection | null;
}
export const EMPTY_SESSION: WorkbenchSession = {
  mode: "history",
  commit: null,
  lastCreatedCommit: null,
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

/** Keep the current side, then its surviving neighbours, then the file's other side. */
export function reconcileChangeSelection(
  selected: ChangeSelection | null,
  files: readonly ChangeSelection[],
  previous: readonly ChangeSelection[] = [],
): ChangeSelection | null {
  if (!selected) return null;
  const unchanged = files.find(
    (file) => file.path === selected.path && file.staged === selected.staged,
  );
  if (unchanged) return unchanged;
  const group = previous.filter((file) => file.staged === selected.staged);
  const index = group.findIndex((file) => file.path === selected.path);
  if (index >= 0) {
    const remaining = new Set(
      files.filter((file) => file.staged === selected.staged).map((file) => file.path),
    );
    const next = group.slice(index + 1).find((file) => remaining.has(file.path));
    if (next) return next;
    const previousFile = group
      .slice(0, index)
      .reverse()
      .find((file) => remaining.has(file.path));
    if (previousFile) return previousFile;
  }
  return files.find((file) => file.path === selected.path) ?? null;
}
