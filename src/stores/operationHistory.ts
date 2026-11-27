import { create } from "zustand";
import type { QueryClient } from "@tanstack/react-query";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { isCancelledSyncError, type RepoRef } from "@/lib/api";
export type OperationTarget = { workspaceId: string; repoId: string; label: string };
export type Operation =
  "fetch" | "pull" | "push" | "ignore" | "stage" | "unstage" | "discard" | "commit";
export type Outcome = "success" | "failed" | "cancelled" | "partial";
type Entry = OperationTarget & { id: number; at: number; operation: Operation; outcome: Outcome };
let sequence = 0;
export const useOperationHistory = create<{ entries: Entry[] }>(() => ({ entries: [] }));
export function operationTarget(
  client: QueryClient,
  workspaceId: string | null,
  repoId: string | null,
): OperationTarget | null {
  if (!workspaceId || !repoId) return null;
  const repo = client.getQueryData<RepoRef[]>(["repos", workspaceId])?.find((r) => r.id === repoId);
  return { workspaceId, repoId, label: repo?.nickname || repo?.path || repoId };
}
export function recordOperation(
  target: OperationTarget | null | undefined,
  operation: Operation,
  outcome: Outcome,
) {
  if (!target) return;
  const entry = { ...target, id: ++sequence, at: Date.now(), operation, outcome };
  useOperationHistory.setState((state) => ({ entries: [entry, ...state.entries].slice(0, 200) }));
}

export function isCurrentTarget(target: OperationTarget | null | undefined) {
  const current = useWorkspaceUiStore.getState();
  return (
    !!target &&
    target.workspaceId === current.activeWorkspaceId &&
    target.repoId === current.activeRepoId
  );
}

export async function trackFetch(target: OperationTarget | null, run: () => Promise<void>) {
  try {
    await run();
    recordOperation(target, "fetch", "success");
  } catch (error) {
    recordOperation(target, "fetch", isCancelledSyncError(error) ? "cancelled" : "failed");
    throw error;
  }
}
