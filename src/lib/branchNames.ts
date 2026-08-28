/**
 * Branch name helpers shared by the sidebar branch list.
 */

/**
 * Strip the leading `<remote>/` segment from a remote-tracking branch name
 * (`origin/main` -> `main`, `origin/feat/x` -> `feat/x`). Names without a
 * slash are returned unchanged.
 */
export function remoteShortName(name: string): string {
  const idx = name.indexOf("/");
  return idx === -1 ? name : name.slice(idx + 1);
}

/**
 * Split a display branch name into its first path segment (the prefix
 * folder in the sidebar) and the remainder
 * (`feat/login-flow` -> `feat` + `login-flow`, `main` -> `null` + `main`).
 */
export function splitBranchPrefix(displayName: string): { prefix: string | null; rest: string } {
  const idx = displayName.indexOf("/");
  return idx === -1
    ? { prefix: null, rest: displayName }
    : { prefix: displayName.slice(0, idx), rest: displayName.slice(idx + 1) };
}

interface BranchLike {
  name: string;
  kind: "local" | "remote";
}

/**
 * Drop remote branches whose short name matches an existing local branch
 * (a local `main` hides `origin/main`, `upstream/main`, ...). Local branches
 * always pass through, so the result is exactly the visible branch list.
 */
export function filterRemoteBranches<T extends BranchLike>(branches: T[]): T[] {
  const localNames = new Set(branches.filter((b) => b.kind === "local").map((b) => b.name));
  return branches.filter((b) => b.kind !== "remote" || !localNames.has(remoteShortName(b.name)));
}
