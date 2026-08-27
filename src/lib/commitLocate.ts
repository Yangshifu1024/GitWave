/**
 * One-shot locate requests for the history list (sidebar branch click).
 */

/** Scroll request for CommitGraph; `seq` increases per click so repeats re-fire. */
export interface LocateRequest {
  repoId: string;
  sha: string;
  seq: number;
}

/**
 * Resolve the list index a locate request points at, or null when the request
 * is stale (already handled), targets another repo, or the commit is missing
 * from the loaded log window. A null from a missing commit is retryable: the
 * caller's effect re-runs when the log reloads and may resolve then.
 */
export function resolveLocateIndex(
  request: LocateRequest | null | undefined,
  handledSeq: number,
  activeRepoId: string | null | undefined,
  shaToIndex: Map<string, number>,
): number | null {
  if (!request || request.repoId !== activeRepoId) return null;
  if (request.seq === handledSeq) return null;
  const index = shaToIndex.get(request.sha);
  return index === undefined ? null : index;
}
