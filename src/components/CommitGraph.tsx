import React, { useMemo } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useRef, useState } from "react";
import type { CommitRef, CommitSummary } from "@/lib/api";
import { formatAppError, getCommitLog } from "@/lib/api";
import { resolveLocateIndex, type LocateRequest } from "@/lib/commitLocate";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { cn } from "@/lib/utils";
import { EmptyState } from "@/components/ui/EmptyState";
import { FolderOpen } from "lucide-react";

const ROW_H = 28;
const LANE_GAP = 14;
const NODE_R = 3.2;
const LANE_COLORS = [
  "var(--color-lane-1)",
  "var(--color-lane-2)",
  "var(--color-lane-3)",
  "var(--color-lane-4)",
  "var(--color-lane-5)",
];

function formatTime(time: number): string {
  const now = Math.floor(Date.now() / 1000);
  const diff = now - time;
  if (diff < 60) return "just now";
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  if (diff < 604800) return `${Math.floor(diff / 86400)}d ago`;
  return new Date(time * 1000).toLocaleDateString();
}

function shortSha(sha: string): string {
  return sha.slice(0, 7);
}

function laneColor(lane: number): string {
  return LANE_COLORS[lane % LANE_COLORS.length] ?? LANE_COLORS[0]!;
}

function laneX(lane: number): number {
  return LANE_GAP / 2 + lane * LANE_GAP;
}

function RefBadge({
  r,
  emphasize = false,
}: {
  r: CommitRef;
  emphasize?: boolean;
}): React.JSX.Element {
  const styles =
    r.kind === "head"
      ? "bg-branch-current text-text-inverse border-branch-current"
      : r.kind === "local_branch"
        ? emphasize
          ? "bg-branch-current/20 text-branch-current border-branch-current/50 font-semibold"
          : "bg-accent/10 text-accent border-accent/30"
        : r.kind === "tag"
          ? "bg-warning/15 text-warning border-warning/40"
          : "bg-bg-elevated text-text-muted border-border-default";

  return (
    <span
      className={cn(
        "inline-flex items-center shrink-0 px-1 py-0 rounded text-[9px] leading-none font-medium border",
        styles,
      )}
      title={r.kind.replace("_", " ")}
    >
      {r.name}
    </span>
  );
}

/** True when a newer commit's edge keeps `lane` live into this row from above. */
function laneContinuesFromAbove(
  commits: CommitSummary[],
  shaToIndex: Map<string, number>,
  index: number,
  lane: number,
): boolean {
  for (let i = 0; i < index; i++) {
    const c = commits[i];
    if (!c) continue;
    for (const pSha of c.parents) {
      const pIdx = shaToIndex.get(pSha);
      if (pIdx === undefined || pIdx < index) continue;
      // Edge from row i crosses/arrives at rows >= index.
      if (c.lane === lane) return true;
      const parent = commits[pIdx];
      if (parent && parent.lane === lane) return true;
    }
  }
  return false;
}

interface GraphRowProps {
  commit: CommitSummary;
  commits: CommitSummary[];
  shaToIndex: Map<string, number>;
  rowIndex: number;
  maxLane: number;
  activeLanes: number[];
  isHead: boolean;
}

/** Per-row SVG: through-lines, merge elbows, commit node (newest-first). */
function GraphRow({
  commit,
  commits,
  shaToIndex,
  rowIndex,
  maxLane,
  activeLanes,
  isHead,
}: GraphRowProps): React.JSX.Element {
  const width = laneX(maxLane) + LANE_GAP / 2;
  const cy = ROW_H / 2;
  const cx = laneX(commit.lane);
  const color = isHead ? "var(--color-branch-current)" : laneColor(commit.lane);

  const parentEdges = commit.parents
    .map((pSha) => {
      const pIdx = shaToIndex.get(pSha);
      if (pIdx === undefined) return null;
      const parent = commits[pIdx];
      if (!parent) return null;
      return { lane: parent.lane, color: laneColor(parent.lane) };
    })
    .filter((e): e is { lane: number; color: string } => e !== null);

  const parentLanes = [...new Map(parentEdges.map((e) => [e.lane, e])).values()];
  // Tip: nothing above → no upper stub. Root / orphan parent: no lower stub.
  const fromAbove = laneContinuesFromAbove(commits, shaToIndex, rowIndex, commit.lane);

  return (
    <svg width={width} height={ROW_H} className="shrink-0 overflow-visible" aria-hidden>
      {activeLanes.map((lane) => {
        const x = laneX(lane);
        const c = laneColor(lane);
        if (lane === commit.lane) {
          // Branch tip: circle is the terminus — skip upper half.
          if (!fromAbove) return null;
          return (
            <path
              key={`thru-${lane}`}
              d={`M ${x} 0 C ${x} ${cy * 0.35}, ${x} ${cy * 0.65}, ${x} ${cy - NODE_R}`}
              fill="none"
              stroke={c}
              strokeWidth={1.5}
            />
          );
        }
        return (
          <path
            key={`thru-${lane}`}
            d={`M ${x} 0 C ${x} ${ROW_H * 0.35}, ${x} ${ROW_H * 0.65}, ${x} ${ROW_H}`}
            fill="none"
            stroke={c}
            strokeWidth={1.5}
          />
        );
      })}

      {/* Only draw downward stubs/curves when a parent is in the loaded log. */}
      {parentLanes.map(({ lane: pLane, color: pColor }) => {
        const px = laneX(pLane);
        if (pLane === commit.lane) {
          return (
            <path
              key={`edge-${pLane}`}
              d={`M ${cx} ${cy + NODE_R} C ${cx} ${cy + (ROW_H - cy) * 0.45}, ${cx} ${ROW_H * 0.85}, ${cx} ${ROW_H}`}
              fill="none"
              stroke={color}
              strokeWidth={1.5}
            />
          );
        }
        const midY = cy + (ROW_H - cy) * 0.62;
        const wave = (px - cx) * 0.18;
        return (
          <path
            key={`edge-${pLane}`}
            d={`M ${cx} ${cy + NODE_R} C ${cx + wave} ${midY}, ${px - wave} ${midY}, ${px} ${ROW_H}`}
            fill="none"
            stroke={pColor}
            strokeWidth={1.5}
          />
        );
      })}

      <circle
        cx={cx}
        cy={cy}
        r={isHead ? NODE_R + 1 : NODE_R}
        fill={color}
        stroke="var(--color-bg-panel)"
        strokeWidth={1.5}
      />
      {isHead ? (
        <circle
          cx={cx}
          cy={cy}
          r={NODE_R + 3}
          fill="none"
          stroke="var(--color-branch-current)"
          strokeWidth={1.5}
        />
      ) : commit.parents.length > 1 ? (
        <circle
          cx={cx}
          cy={cy}
          r={NODE_R + 2}
          fill="none"
          stroke={color}
          strokeWidth={1}
          opacity={0.5}
        />
      ) : null}
    </svg>
  );
}

interface CommitRowProps {
  commit: CommitSummary;
  commits: CommitSummary[];
  shaToIndex: Map<string, number>;
  rowIndex: number;
  maxLane: number;
  activeLanes: number[];
  onSelect: (sha: string) => void;
  isSelected: boolean;
  isHead: boolean;
}

function CommitRow({
  commit,
  commits,
  shaToIndex,
  rowIndex,
  maxLane,
  activeLanes,
  onSelect,
  isSelected,
  isHead,
}: CommitRowProps): React.JSX.Element {
  const refs = commit.refs ?? [];

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onSelect(commit.sha)}
      onKeyDown={(e) => e.key === "Enter" && onSelect(commit.sha)}
      aria-current={isHead ? "true" : undefined}
      className={cn(
        "flex items-center gap-2 px-2 py-0 cursor-pointer",
        "transition-colors duration-fast border-l-2 border-l-transparent",
        !isSelected && !isHead && "hover:bg-bg-elevated",
        isHead && !isSelected && "bg-accent/10 border-l-accent hover:bg-accent/20",
        isSelected && "bg-accent/20 border-l-accent hover:bg-accent/30",
      )}
      style={{ height: `${ROW_H}px` }}
    >
      <GraphRow
        commit={commit}
        commits={commits}
        shaToIndex={shaToIndex}
        rowIndex={rowIndex}
        maxLane={maxLane}
        activeLanes={activeLanes}
        isHead={isHead}
      />

      <span className="font-mono text-[10px] text-text-muted w-14 shrink-0 tabular-nums">
        {shortSha(commit.sha)}
      </span>

      {refs.length > 0 ? (
        <span className="flex items-center gap-0.5 shrink-0 max-w-[28%] overflow-hidden">
          {refs.slice(0, 2).map((r) => (
            <RefBadge
              key={`${r.kind}:${r.name}`}
              r={r}
              emphasize={isHead && r.kind !== "remote_branch"}
            />
          ))}
          {refs.length > 2 ? (
            <span className="text-[9px] text-text-muted shrink-0">+{refs.length - 2}</span>
          ) : null}
        </span>
      ) : null}

      <p
        className={cn(
          "flex-1 min-w-0 text-xs leading-none truncate text-text-primary",
          isHead ? "font-semibold" : "font-medium",
        )}
        title={commit.message_summary}
      >
        {commit.message_summary}
      </p>

      <span className="shrink-0 text-[10px] text-text-muted whitespace-nowrap tabular-nums">
        {commit.author} &middot; {formatTime(commit.time)}
        {commit.parents.length > 1 ? <span className="ml-1 text-accent">merge</span> : null}
        {isHead ? <span className="ml-1 text-accent font-medium">HEAD</span> : null}
      </span>
    </div>
  );
}

function computeActiveLanes(
  commits: CommitSummary[],
  shaToIndex: Map<string, number>,
  index: number,
): number[] {
  const lanes = new Set<number>();
  const commit = commits[index];
  if (!commit) return [];

  lanes.add(commit.lane);

  for (let i = 0; i < index; i++) {
    const c = commits[i];
    if (!c) continue;
    for (const pSha of c.parents) {
      const pIdx = shaToIndex.get(pSha);
      if (pIdx !== undefined && pIdx >= index) {
        const parent = commits[pIdx];
        if (parent) lanes.add(parent.lane);
        if (pIdx > index) lanes.add(c.lane);
      }
    }
  }

  return [...lanes].sort((a, b) => a - b);
}

interface CommitGraphProps {
  onCommitSelect?: (sha: string) => void;
  selectedSha?: string | null;
  /** One-shot request to scroll a commit to the viewport center. */
  locateRequest?: LocateRequest | null;
}

export function CommitGraph({
  onCommitSelect,
  selectedSha: selectedShaProp,
  locateRequest,
}: CommitGraphProps): React.JSX.Element {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const historyEpoch = useWorkspaceUiStore((s) => s.historyEpoch);
  const [commits, setCommits] = useState<CommitSummary[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [localSelected, setLocalSelected] = useState<string | null>(null);
  const scrollRef = React.useRef<HTMLDivElement>(null);

  const selectedSha = selectedShaProp !== undefined ? selectedShaProp : localSelected;

  // Backend returns newest-first — do not reverse.
  const shaToIndex = useMemo(() => {
    const m = new Map<string, number>();
    commits.forEach((c, i) => m.set(c.sha, i));
    return m;
  }, [commits]);

  const maxLane = useMemo(() => commits.reduce((m, c) => Math.max(m, c.lane), 0), [commits]);

  const activeLanesByIndex = useMemo(
    () => commits.map((_, i) => computeActiveLanes(commits, shaToIndex, i)),
    [commits, shaToIndex],
  );

  useEffect(() => {
    if (!activeWorkspaceId || !activeRepoId) {
      setCommits([]);
      setError(null);
      return;
    }
    setLoading(true);
    setError(null);
    getCommitLog(activeWorkspaceId, 200)
      .then(setCommits)
      .catch((e) => {
        setCommits([]);
        setError(formatAppError(e));
      })
      .finally(() => setLoading(false));
  }, [activeWorkspaceId, activeRepoId, historyEpoch]);

  const virtualizer = useVirtualizer({
    count: commits.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_H,
    overscan: 10,
  });

  // Locate request from the sidebar (branch click): center the commit in the
  // viewport. One-shot per seq — history refreshes must not yank the scroll
  // position back. A commit missing from the log window (async load still in
  // flight) stays unhandled and retries when shaToIndex updates.
  const handledLocateSeq = useRef(-1);
  useEffect(() => {
    if (!locateRequest) return;
    const index = resolveLocateIndex(
      locateRequest,
      handledLocateSeq.current,
      activeRepoId,
      shaToIndex,
    );
    if (index === null) return;
    handledLocateSeq.current = locateRequest.seq;
    virtualizer.scrollToIndex(index, { align: "center" });
  }, [locateRequest, activeRepoId, shaToIndex, virtualizer]);

  const handleSelect = (sha: string) => {
    setLocalSelected(sha);
    onCommitSelect?.(sha);
  };

  if (!activeWorkspaceId) {
    return (
      <div className="flex items-center justify-center h-full">
        <EmptyState
          icon={<FolderOpen size={22} />}
          title="Select a workspace"
          description="Choose a workspace in the toolbar to view commit history."
        />
      </div>
    );
  }

  if (!activeRepoId) {
    return (
      <div className="flex items-center justify-center h-full">
        <EmptyState
          icon={<FolderOpen size={22} />}
          title="Select a repository"
          description="Pick a repository from the sidebar to view commit history."
        />
      </div>
    );
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Loading history...
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-full text-danger text-sm px-4 text-center">
        {error}
      </div>
    );
  }

  if (commits.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        No commits yet
      </div>
    );
  }

  return (
    <div ref={scrollRef} className="h-full overflow-auto">
      <div
        style={{
          height: `${virtualizer.getTotalSize()}px`,
          width: "100%",
          position: "relative",
        }}
      >
        {virtualizer.getVirtualItems().map((virtualRow) => {
          const commit = commits[virtualRow.index];
          if (!commit) return null;
          return (
            <div
              key={commit.sha}
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                width: "100%",
                height: `${virtualRow.size}px`,
                transform: `translateY(${virtualRow.start}px)`,
              }}
            >
              <CommitRow
                commit={commit}
                commits={commits}
                shaToIndex={shaToIndex}
                rowIndex={virtualRow.index}
                maxLane={maxLane}
                activeLanes={activeLanesByIndex[virtualRow.index] ?? [commit.lane]}
                onSelect={handleSelect}
                isSelected={selectedSha === commit.sha}
                isHead={(commit.refs ?? []).some((r) => r.kind === "head")}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}
