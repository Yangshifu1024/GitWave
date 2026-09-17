// Small path-list helpers for the batch "add repositories" flow.

/** Trim, drop empties and de-duplicate paths while preserving first-seen
 *  order. Case-sensitive exact matching only — the backend canonicalizes and
 *  is the source of truth for "already in the workspace". */
export function mergeUniquePaths(existing: string[], incoming: string[]): string[] {
  const out = [...existing];
  const seen = new Set(existing);
  for (const raw of incoming) {
    const path = raw.trim();
    if (path === "" || seen.has(path)) continue;
    seen.add(path);
    out.push(path);
  }
  return out;
}
