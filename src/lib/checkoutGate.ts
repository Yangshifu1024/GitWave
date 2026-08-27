export type CheckoutGate =
  | { kind: "noop" }
  | { kind: "proceed" }
  | { kind: "dirty"; fileCount: number }
  | { kind: "blocked"; reason: "remote" | "merge" | "rebase" | "worktree"; message: string };

export interface CheckoutGateInput {
  isCurrent: boolean;
  branchKind: "local" | "remote";
  dirtyCount: number;
  mergeInProgress: boolean;
  rebasePaused: boolean;
  occupiedWorktree: string | null;
}

/** Decide whether a branch switch can proceed, needs a dirty-work dialog, or is blocked. */
export function gateCheckout(input: CheckoutGateInput): CheckoutGate {
  if (input.isCurrent) return { kind: "noop" };
  if (input.branchKind === "remote") {
    return {
      kind: "blocked",
      reason: "remote",
      message:
        "Create or check out a local branch. Remote-tracking branches cannot be checked out directly.",
    };
  }
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
