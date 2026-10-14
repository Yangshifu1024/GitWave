import { Circle, CircleCheck, GitBranch, Tag } from "lucide-react";

import type { CommitRef } from "@/lib/api";
import { cn } from "@/lib/utils";

const LANE_COLORS = [
  "var(--color-lane-1)",
  "var(--color-lane-2)",
  "var(--color-lane-3)",
  "var(--color-lane-4)",
  "var(--color-lane-5)",
  "var(--color-lane-6)",
  "var(--color-lane-7)",
  "var(--color-lane-8)",
  "var(--color-lane-9)",
];

export function laneColor(lane: number): string {
  return LANE_COLORS[lane % LANE_COLORS.length] ?? LANE_COLORS[0]!;
}

const REF_NAME_MAX_LEN = 20;

function truncateRefName(name: string): string {
  if (name.length <= REF_NAME_MAX_LEN) return name;
  return `${name.slice(0, REF_NAME_MAX_LEN)}...`;
}

/**
 * Quiet branch / tag label shared by history and the inspector. Shape
 * distinguishes ref kinds; only the current branch uses the accent.
 */
export function RefBadge({
  r,
  emphasize = false,
  truncate = true,
  synced = false,
}: {
  r: CommitRef;
  lane?: number;
  emphasize?: boolean;
  /** History rows truncate long names; the inspector header shows them in full. */
  truncate?: boolean;
  /** Local branch whose tracked remote branch lives on this commit too
   *  (Fork-style single badge; the icon becomes a check-in-circle). */
  synced?: boolean;
}): React.JSX.Element {
  const Icon =
    r.kind === "tag" ? Tag : r.kind === "remote_branch" ? Circle : synced ? CircleCheck : GitBranch;
  const displayName = truncate ? truncateRefName(r.name) : r.name;
  return (
    <span
      title={r.name}
      className={cn(
        "inline-flex min-w-0 max-w-full items-center gap-1 rounded px-1.5 py-0.5 text-[11px] leading-none",
        emphasize ? "bg-accent/10 font-medium text-accent" : "bg-text-muted/5 text-text-secondary",
      )}
    >
      <Icon size={11} className="shrink-0" />
      <span className={truncate ? "truncate" : "[overflow-wrap:anywhere]"}>{displayName}</span>
    </span>
  );
}
