import React from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useEffect, useState } from "react";
import type { CommitSummary } from "@/lib/api";
import { formatAppError, getCommitLog } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { cn } from "@/lib/utils";

// Lane colors for the graph — matches the design tokens for branch colors
const LANE_COLORS = ["bg-accent", "bg-success", "bg-warning", "bg-info", "bg-branch-current"];

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

interface CommitRowProps {
  commit: CommitSummary;
  onSelect: (sha: string) => void;
  isSelected: boolean;
}

function CommitRow({ commit, onSelect, isSelected }: CommitRowProps): React.JSX.Element {
  const laneColor = LANE_COLORS[commit.lane % LANE_COLORS.length];
  const leftPct = Math.min((commit.lane / 4) * 100, 75);

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onSelect(commit.sha)}
      onKeyDown={(e) => e.key === "Enter" && onSelect(commit.sha)}
      className={cn(
        "flex items-center gap-3 px-4 py-2 cursor-pointer border-b border-border-subtle",
        "hover:bg-bg-secondary transition-colors duration-fast",
        isSelected && "bg-accent/10 border-l-2 border-l-accent",
      )}
      style={{ height: "56px" }}
    >
      {/* Lane indicator column */}
      <div className="relative w-10 h-full shrink-0">
        {/* Lane dots for merge parents */}
        {commit.parents.slice(1).map((_, i) => (
          <div
            key={i}
            className={cn("absolute top-1/2 w-2 h-2 rounded-full -translate-y-1/2", laneColor)}
            style={{ left: `${leftPct + (i + 1) * 12}%` }}
          />
        ))}
        {/* Primary lane dot */}
        <div
          className={cn("absolute top-1/2 w-2.5 h-2.5 rounded-full -translate-y-1/2", laneColor)}
          style={{ left: `${leftPct}%` }}
        />
        {/* Vertical lane line */}
        {commit.lane > 0 && (
          <div
            className="absolute top-0 bottom-0 w-px bg-border-subtle"
            style={{ left: `${leftPct}%` }}
          />
        )}
      </div>

      {/* SHA badge */}
      <span className="font-mono text-xs text-accent bg-accent/10 px-1.5 py-0.5 rounded shrink-0">
        {shortSha(commit.sha)}
      </span>

      {/* Message */}
      <div className="flex-1 min-w-0">
        <p className="text-sm text-text-primary truncate font-medium leading-tight">
          {commit.message_summary}
        </p>
        <p className="text-xs text-text-muted mt-0.5">
          {commit.author} &middot; {formatTime(commit.time)}
          {commit.parents.length > 1 && (
            <span className="ml-1 text-accent">+{commit.parents.length - 1}</span>
          )}
        </p>
      </div>
    </div>
  );
}

interface CommitGraphProps {
  onCommitSelect?: (sha: string) => void;
}

export function CommitGraph({ onCommitSelect }: CommitGraphProps): React.JSX.Element {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const [commits, setCommits] = useState<CommitSummary[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedSha, setSelectedSha] = useState<string | null>(null);
  const scrollRef = React.useRef<HTMLDivElement>(null);

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
  }, [activeWorkspaceId, activeRepoId]);

  const virtualizer = useVirtualizer({
    count: commits.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 56,
    overscan: 10,
  });

  const handleSelect = (sha: string) => {
    setSelectedSha(sha);
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
              key={virtualRow.index}
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
                onSelect={handleSelect}
                isSelected={selectedSha === commit.sha}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}
