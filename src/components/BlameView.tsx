import { useEffect, useState, useRef } from "react";
import type { BlameLine } from "@/lib/api";
import { formatAppError, getBlame } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { cn } from "@/lib/utils";

function formatTime(time: number): string {
  return new Date(time * 1000).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function shortSha(sha: string): string {
  return sha.slice(0, 7);
}

interface HoverInfo {
  sha: string;
  author: string;
  author_email: string;
  time: number;
  x: number;
  y: number;
}

function BlameGutter({ line }: { line: BlameLine }): React.JSX.Element {
  const [hoverInfo, setHoverInfo] = useState<HoverInfo | null>(null);
  const ref = useRef<HTMLDivElement>(null);

  const handleMouseEnter = () => {
    if (ref.current) {
      const rect = ref.current.getBoundingClientRect();
      setHoverInfo({
        sha: line.sha,
        author: line.author,
        author_email: line.author_email,
        time: line.time,
        x: rect.right + 8,
        y: rect.top,
      });
    }
  };

  return (
    <>
      <div
        ref={ref}
        className={cn(
          "flex items-center gap-2 text-xs font-mono leading-5 px-2 cursor-pointer shrink-0",
          "border-r border-border-subtle hover:bg-accent/5 transition-colors",
        )}
        style={{ width: "200px" }}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={() => setHoverInfo(null)}
      >
        <span className="text-accent font-mono text-xs shrink-0">{shortSha(line.sha)}</span>
        <span className="text-text-muted text-xs truncate shrink-0">{line.author}</span>
        <span className="text-text-muted text-xs ml-auto shrink-0">{formatTime(line.time)}</span>
      </div>

      {hoverInfo && (
        <div
          className={cn(
            "fixed z-popover rounded-lg shadow-modal",
            "bg-bg-elevated border border-border-default",
            "px-3 py-2 min-w-48 max-w-64",
          )}
          style={{ left: `${hoverInfo.x}px`, top: `${hoverInfo.y}px` }}
          onMouseEnter={() => setHoverInfo(hoverInfo)}
          onMouseLeave={() => setHoverInfo(null)}
        >
          <div className="space-y-1">
            <p className="font-mono text-xs text-accent">{shortSha(hoverInfo.sha)}</p>
            <p className="text-sm font-medium text-text-primary">{hoverInfo.author}</p>
            <p className="text-xs text-text-muted">{hoverInfo.author_email}</p>
            <p className="text-xs text-text-muted">{formatTime(hoverInfo.time)}</p>
          </div>
        </div>
      )}
    </>
  );
}

interface BlameLineRowProps {
  line: BlameLine;
  lineNo: number;
}

function BlameLineRow({ line, lineNo }: BlameLineRowProps): React.JSX.Element {
  return (
    <div className="flex text-xs font-mono leading-5 hover:bg-accent/5 transition-colors">
      <BlameGutter line={line} />
      <span className="text-text-muted w-8 text-right pr-2 shrink-0 select-none border-r border-border-subtle">
        {lineNo}
      </span>
      <span className="pl-3 text-text-primary flex-1 break-all select-text">{line.content}</span>
    </div>
  );
}

interface BlameViewProps {
  path: string;
}

export function BlameView({ path }: BlameViewProps): React.JSX.Element {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const [lines, setLines] = useState<BlameLine[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!activeWorkspaceId || !activeRepoId || !path) {
      setLines([]);
      setError(null);
      return;
    }
    setLoading(true);
    setError(null);
    getBlame(activeWorkspaceId, path)
      .then(setLines)
      .catch((e) => setError(formatAppError(e)))
      .finally(() => setLoading(false));
  }, [activeWorkspaceId, activeRepoId, path]);

  if (!activeWorkspaceId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Select a workspace to view blame
      </div>
    );
  }

  if (!activeRepoId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Select a repository to view blame
      </div>
    );
  }

  if (!path) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Select a file to view blame
      </div>
    );
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Loading blame...
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-full text-danger text-sm px-4">
        {error}
      </div>
    );
  }

  if (lines.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        No blame information available
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto">
      {/* File header */}
      <div className="sticky top-0 z-10 px-4 py-2 bg-bg-elevated border-b border-border-subtle">
        <span className="text-sm font-medium text-text-primary">{path}</span>
      </div>

      {/* Lines */}
      <div className="font-mono">
        {lines.map((line) => (
          <BlameLineRow key={line.line_no} line={line} lineNo={line.line_no} />
        ))}
      </div>
    </div>
  );
}
