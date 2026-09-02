export type CheckoutGate =
  | { kind: "noop" }
  | { kind: "proceed" }
  | { kind: "dirty"; fileCount: number }
  | { kind: "blocked"; reason: "merge" | "rebase" | "worktree"; message: string };

export interface CheckoutGateInput {
  isCurrent: boolean;
  dirtyCount: number;
  mergeInProgress: boolean;
  rebasePaused: boolean;
  occupiedWorktree: string | null;
}

/**
 * Decide whether a branch switch can proceed, needs a dirty-work dialog, or
 * is blocked. Remote-tracking branches are resolved to their local DWIM
 * target (F012) by the caller before gating, so this only ever sees local
 * branch checks.
 */
export function gateCheckout(input: CheckoutGateInput): CheckoutGate {
  if (input.isCurrent) return { kind: "noop" };
  if (input.mergeInProgress) {
    return {
      kind: "blocked",
      reason: "merge",
      message: "A merge is in progress. Finish or abort it before switching branches.",
    };
  }
  if (input.rebasePaused) {
    return {
      kind: "blocked",
      reason: "rebase",
      message: "An interactive rebase is paused. Continue or abort it before switching branches.",
    };
  }
  if (input.occupiedWorktree) {
    return {
      kind: "blocked",
      reason: "worktree",
      message: `Branch is already checked out in worktree “${input.occupiedWorktree}”.`,
    };
  }
  if (input.dirtyCount > 0) {
    return { kind: "dirty", fileCount: input.dirtyCount };
  }
  return { kind: "proceed" };
}
