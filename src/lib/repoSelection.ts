import type { RepoRef } from "./api";

/**
 * Pick the repo to restore a selection onto: keep `target` when it still
 * resolves to a repo that opens, else the first non-missing repo, else null.
 * Shared by startup restore (App.tsx), workspace switching
 * (useValidatedWorkspaceSwitch) and the tab strip's liveness guard, so every
 * path lands on the same fallback instead of a missing repo that would
 * error-loop every panel query.
 */
export function pickRestoredRepo(repos: RepoRef[], target: string | null): string | null {
  if (target != null && repos.some((r) => r.id === target && r.status !== "missing")) {
    return target;
  }
  return repos.find((r) => r.status !== "missing")?.id ?? null;
}
