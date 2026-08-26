import React, { useMemo } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useState } from "react";
import type { CommitRef, CommitSummary } from "@/lib/api";
import { formatAppError, getCommitLog } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { cn } from "@/lib/utils";

const ROW_H = 56;
const LANE_GAP = 16;
const NODE_R = 4.5;
const LANE_COLORS = [
  "var(--color-accent)",
  "var(--color-success)",
  "var(--color-warning)",
  "var(--color-info)",
  "var(--color-branch-current)",
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

function RefBadge({ r, emphasize = false }: { r: CommitRef; emphasize?: boolean }): React.JSX.Element {
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
        "inline-flex items-center shrink-0 px-1.5 py-0.5 rounded text-[10px] font-medium border",
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
            <line
              key={`thru-${lane}`}
              x1={x}
              y1={0}
              x2={x}
              y2={cy - NODE_R}
              stroke={c}
              strokeWidth={1.5}
            />
          );
        }
        return (
          <line
            key={`thru-${lane}`}
            x1={x}
            y1={0}
            x2={x}
            y2={ROW_H}
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
            <line
              key={`edge-${pLane}`}
              x1={cx}
              y1={cy + NODE_R}
              x2={cx}
              y2={ROW_H}
              stroke={color}
              strokeWidth={1.5}
            />
          );
        }
        const midY = cy + (ROW_H - cy) * 0.55;
        return (
          <path
            key={`edge-${pLane}`}
            d={`M ${cx} ${cy + NODE_R} C ${cx} ${midY}, ${px} ${midY}, ${px} ${ROW_H}`}
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
        stroke="var(--color-bg-primary)"
        strokeWidth={1.5}
      />
      {isHead ? (
        <circle
          cx={cx}
          cy={cy}
          r={NODE_R + 4}
          fill="none"
          stroke="var(--color-branch-current)"
          strokeWidth={1.5}
        />
      ) : commit.parents.length > 1 ? (
        <circle
          cx={cx}
          cy={cy}
          r={NODE_R + 2.5}
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
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onSelect(commit.sha)}
      onKeyDown={(e) => e.key === "Enter" && onSelect(commit.sha)}
      aria-current={isHead ? "true" : undefined}
      className={cn(
        "flex items-center gap-3 px-3 py-0 cursor-pointer border-b border-border-subtle",
        "transition-colors duration-fast border-l-2 border-l-transparent",
        // Panel is bg-secondary — hover must use a contrasting surface.
        !isSelected && !isHead && "hover:bg-bg-elevated",
        // Current branch tip
        isHead &&
          !isSelected &&
          "bg-branch-current/10 border-l-branch-current hover:bg-branch-current/20",
        // Selected: stronger accent fill, distinct from hover
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

      <span
        className={cn(
          "font-mono text-xs px-1.5 py-0.5 rounded shrink-0",
          isHead
            ? "text-branch-current bg-branch-current/15 font-semibold"
            : "text-accent bg-accent/10",
        )}
      >
        {shortSha(commit.sha)}
      </span>

      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1.5 min-w-0">
          <p
            className={cn(
              "text-sm truncate leading-tight",
              isHead ? "text-text-primary font-semibold" : "text-text-primary font-medium",
            )}
          >
            {commit.message_summary}
          </p>
          {(commit.refs ?? []).length > 0 ? (
            <span className="flex items-center gap-1 shrink-0 overflow-hidden">
              {(commit.refs ?? []).slice(0, 4).map((r) => (
                <RefBadge key={`${r.kind}:${r.name}`} r={r} emphasize={isHead && r.kind !== "remote_branch"} />
              ))}
              {(commit.refs ?? []).length > 4 ? (
                <span className="text-[10px] text-text-muted">
                  +{(commit.refs ?? []).length - 4}
                </span>
              ) : null}
            </span>
          ) : null}
        </div>
        <p className="text-xs text-text-muted mt-0.5">
          {commit.author} &middot; {formatTime(commit.time)}
          {commit.parents.length > 1 && (
            <span className="ml-1 text-accent">merge · {commit.parents.length} parents</span>
          )}
          {isHead ? <span className="ml-1 text-branch-current font-medium">· current</span> : null}
        </p>
      </div>
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
}

export function CommitGraph({
  onCommitSelect,
  selectedSha: selectedShaProp,
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

  const handleSelect = (sha: string) => {
    setLocalSelected(sha);
    onCommitSelect?.(sha);
  };

  if (!activeWorkspaceId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Select a workspace to view commit history
      </div>
    );
  }

  if (!activeRepoId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Select a repository to view commit history
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
