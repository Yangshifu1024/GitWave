// Pure helpers behind the commit-graph context menus (F011): remote ref
// parsing for badge actions, clipboard payload for "Copy Commit Info", and
// the safety gate for detached commit checkout.

import type { CommitSummary } from "@/lib/api";

export interface RemoteBranchRef {
  remote: string;
  branch: string;
}

/** "origin/main" → { remote: "origin", branch: "main" }; null when unparsable. */
export function parseRemoteBranchName(name: string): RemoteBranchRef | null {
  const slash = name.indexOf("/");
  if (slash <= 0 || slash === name.length - 1) return null;
  return { remote: name.slice(0, slash), branch: name.slice(slash + 1) };
}

/** Plain-text block copied by "Copy Commit Info" (Fork-style). */
export function copyCommitInfoText(
  commit: Pick<CommitSummary, "sha" | "author" | "time" | "message_summary">,
): string {
  const date = new Date(commit.time * 1000).toLocaleString();
  return `${commit.sha}\nAuthor: ${commit.author}\nDate: ${date}\n\n${commit.message_summary}`;
}

/** Write to the system clipboard; returns false when the API is unavailable. */
export function copyToClipboard(text: string): Promise<boolean> {
  if (!navigator.clipboard) return Promise.resolve(false);
  return navigator.clipboard
    .writeText(text)
    .then(() => true)
    .catch(() => false);
}

export type CommitCheckoutGate =
  | { kind: "noop" }
  | { kind: "proceed" }
  | { kind: "dirty"; fileCount: number }
  | { kind: "blocked"; reason: "merge" | "rebase" };

export interface CommitCheckoutGateInput {
  isHead: boolean;
  dirtyCount: number;
  mergeInProgress: boolean;
  rebasePaused: boolean;
}

/**
 * Decide whether a detached checkout of a commit can proceed, needs the
 * dirty-work dialog, or is blocked — the commit-side twin of
 * `gateCheckout` (worktree occupancy / remote-kind checks don't apply).
 * Blocked reasons render localized via the caller (unlike `gateCheckout`,
 * whose English literals predate i18n).
 */
export function gateCommitCheckout(input: CommitCheckoutGateInput): CommitCheckoutGate {
  if (input.isHead) return { kind: "noop" };
  if (input.mergeInProgress) return { kind: "blocked", reason: "merge" };
  if (input.rebasePaused) return { kind: "blocked", reason: "rebase" };
  if (input.dirtyCount > 0) {
    return { kind: "dirty", fileCount: input.dirtyCount };
  }
  return { kind: "proceed" };
}
