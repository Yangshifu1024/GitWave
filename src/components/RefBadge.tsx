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

const REF_NAME_MAX_LEN = 24;

function truncateRefName(name: string): string {
  if (name.length <= REF_NAME_MAX_LEN) return name;
  return `${name.slice(0, REF_NAME_MAX_LEN)}...`;
}

/**
 * Branch / tag badge for a commit, shared by the history rows and the
 * inspector header. Self-drawn (no HeroUI Chip) for a flatter, native feel.
 * Colors follow the history conventions: the current branch (emphasize)
 * wears a current-branch tint, tags a warning tint, remote branches the
 * fixed --color-branch-remote, local branches their lane color — or, when
 * no lane context exists (inspector header), the current-branch color.
 */
export function RefBadge({
  r,
  lane,
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
  const chrome = cn(
    "inline-flex items-center gap-1 shrink-0 min-w-0 max-w-full whitespace-nowrap",
    // Same size as the commit title (text-xs) so refs read as part of the row.
    "rounded-sm px-1.5 py-0.5 text-xs leading-none font-medium shadow-none",
  );
  const displayName = r.kind === "tag" ? r.name : truncate ? truncateRefName(r.name) : r.name;

  if (r.kind === "tag") {
    return (
      <span title={r.name} className={cn(chrome, "bg-[#ffc53d]/15 text-[#ffc53d]")}>
        <Tag size={11} className="shrink-0" />
        <span className="truncate">{displayName}</span>
      </span>
    );
  }

  if (emphasize) {
    return (
      <span
        title={r.name}
        className={cn(chrome, "bg-branch-current/15 text-branch-current font-semibold")}
      >
        {synced && <CircleCheck size={11} className="shrink-0" />}
        <span className="truncate">{displayName}</span>
      </span>
    );
  }

  // Remote branches wear a fixed vivid color (palette blue) so they stay
  // distinguishable from local branches, which follow their lane color.
  const lineColor =
    r.kind === "remote_branch"
      ? "#4096ff"
      : lane === undefined
        ? "var(--color-branch-current)"
        : laneColor(lane);
  return (
    <span
      title={r.name}
      className={cn(chrome)}
      style={{
        backgroundColor: `color-mix(in srgb, ${lineColor} 14%, transparent)`,
        color: lineColor,
      }}
    >
      {r.kind === "remote_branch" ? (
        <Circle size={11} className="shrink-0" />
      ) : synced ? (
        <CircleCheck size={11} className="shrink-0" />
      ) : (
        <GitBranch size={11} className="shrink-0" />
      )}
      <span className="truncate">{displayName}</span>
    </span>
  );
}
