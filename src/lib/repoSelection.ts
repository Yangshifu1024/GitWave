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

/**
 * Merge freshly created repos into a cached repo list: an entry that is
 * already there is replaced in place (keeping its tab position), a new one is
 * appended where the backend appends it.
 *
 * Callers seed the `["repos", workspaceId]` cache right after an add and
 * *before* moving the active selection onto the new repo. Selecting and
 * refetching are two independent round trips, and the selection (a single
 * sqlite write) usually lands first — with the new repo missing from the
 * still-stale list, the tab strip's liveness guard reads the selection as
 * dead and falls back to the first repo.
 */
export function upsertRepos(repos: RepoRef[], incoming: RepoRef[]): RepoRef[] {
  if (incoming.length === 0) return repos;
  const merged = [...repos];
  for (const repo of incoming) {
    const i = merged.findIndex((r) => r.id === repo.id);
    if (i >= 0) merged[i] = repo;
    else merged.push(repo);
  }
  return merged;
}
