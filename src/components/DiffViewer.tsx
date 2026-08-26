import { useEffect, useState } from "react";
import type { DiffSummary, FileDiff, DiffHunk, DiffLine } from "@/lib/api";
import { getCommitDiff, getWorkdirDiff } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/utils";

type DiffViewMode = "unified" | "split";

interface DiffViewerProps {
  /** If provided, show diff for this commit vs its parent */
  commitOid?: string;
  /** If provided, show the working-copy diff */
  workdir?: boolean;
  /** Path to show diff for (when commitOid is set, path specifies which file) */
  path?: string;
}

function getExt(path: string): string {
  const parts = path.split(".");
  return parts.length > 1 ? (parts[parts.length - 1] ?? "") : "";
}

function getLanguage(ext: string): string {
  const map: Record<string, string> = {
    ts: "typescript",
    tsx: "tsx",
    js: "javascript",
    jsx: "jsx",
    rs: "rust",
    py: "python",
    go: "go",
    java: "java",
    c: "c",
    cpp: "cpp",
    md: "markdown",
    json: "json",
    toml: "toml",
    yaml: "yaml",
    yml: "yaml",
    sh: "bash",
    bash: "bash",
    zsh: "bash",
  };
  return map[ext] ?? "text";
}

function DiffLineView({ line, mode }: { line: DiffLine; mode: DiffViewMode }): React.JSX.Element {
  const bgClass =
    line.kind === "added"
      ? "bg-success/10 text-success"
      : line.kind === "removed"
        ? "bg-danger/10 text-danger"
        : "text-text-primary";

  const prefix = line.kind === "added" ? "+" : line.kind === "removed" ? "-" : " ";

  const lineNoLeft =
    line.kind === "added" ? (
      <span className="text-text-muted w-10 text-right pr-2 shrink-0 select-none"> </span>
    ) : (
      <span className="text-text-muted font-mono text-xs w-10 text-right pr-2 shrink-0 select-none">
        {line.old_line_no ?? ""}
      </span>
    );

  const lineNoRight =
    line.kind === "removed" ? (
      <span className="text-text-muted w-10 text-right pr-2 shrink-0 select-none"> </span>
    ) : (
      <span className="text-text-muted font-mono text-xs w-10 text-right pr-2 shrink-0 select-none">
        {line.new_line_no ?? ""}
      </span>
    );

  if (mode === "split") {
    const isRemoved = line.kind === "removed";
    const isAdded = line.kind === "added";
    return (
      <div className={cn("flex text-xs font-mono leading-5", isRemoved && "bg-danger/5")}>
        {lineNoLeft}
        <span
          className={cn(
            "flex-1 px-2",
            bgClass,
            isAdded && "bg-success/5",
            isRemoved && "bg-danger/5",
          )}
        >
          {prefix} {line.content}
        </span>
        {lineNoRight}
        <span
          className={cn(
            "flex-1 px-2",
            bgClass,
            isAdded && "bg-success/5",
            isRemoved && "bg-danger/5",
          )}
        >
          {isAdded ? "+ " : isRemoved ? "- " : "  "}
          {line.content}
        </span>
      </div>
    );
  }

  return (
    <div
      className={cn(
        "flex text-xs font-mono leading-5",
        line.kind === "added" && "bg-success/10",
        line.kind === "removed" && "bg-danger/10",
      )}
    >
      {lineNoLeft}
      {lineNoRight}
      <span className={cn("flex-1 px-2", bgClass)}>
        {prefix} {line.content}
      </span>
    </div>
  );
}

function DiffHunkView({ hunk, mode }: { hunk: DiffHunk; mode: DiffViewMode }): React.JSX.Element {
  return (
    <div className="border border-border-subtle rounded-md overflow-hidden mb-3">
      <div className="bg-bg-secondary px-3 py-1 text-xs text-text-muted font-mono border-b border-border-subtle">
        @@ -{hunk.old_start},{hunk.old_lines} +{hunk.new_start},{hunk.new_lines} @@
      </div>
      {hunk.lines.map((line, i) => (
        <DiffLineView key={i} line={line} mode={mode} />
      ))}
    </div>
  );
}

function FileDiffView({
  fileDiff,
  mode,
}: {
  fileDiff: FileDiff;
  mode: DiffViewMode;
}): React.JSX.Element {
  // Language for future shiki integration
  void getLanguage(getExt(fileDiff.path));

  return (
    <div className="mb-6">
      {/* File header */}
      <div className="flex items-center gap-2 px-3 py-2 bg-bg-secondary border border-border-subtle rounded-t-md">
        <span className="text-sm font-medium text-text-primary">{fileDiff.path}</span>
        <span className="text-xs text-text-muted font-mono">
          {fileDiff.old_sha?.slice(0, 7) ?? "0000000"}
          {fileDiff.old_sha && fileDiff.new_sha ? " → " : ""}
          {fileDiff.new_sha?.slice(0, 7) ?? "0000000"}
        </span>
        <div className="ml-auto flex items-center gap-2 text-xs">
          <span className="text-success">+{fileDiff.additions}</span>
          <span className="text-danger">-{fileDiff.deletions}</span>
        </div>
      </div>

      {/* Hunks */}
      <div className="border-x border-b border-border-subtle rounded-b-md px-2 pt-2">
        {fileDiff.hunks.length > 0 ? (
          fileDiff.hunks.map((hunk, i) => <DiffHunkView key={i} hunk={hunk} mode={mode} />)
        ) : (
          // Fallback: show additions/deletions summary when no hunk detail available
          <div className="py-4 text-center text-sm text-text-muted">
            {fileDiff.additions > 0 || fileDiff.deletions > 0 ? (
              <>
                <span className="text-success">+{fileDiff.additions}</span>
                {" / "}
                <span className="text-danger">-{fileDiff.deletions}</span>{" "}
                <span className="text-text-muted">(no hunk detail)</span>
              </>
            ) : (
              "No changes"
            )}
          </div>
        )}
      </div>
    </div>
  );
}

export function DiffViewer({
  commitOid,
  workdir = false,
  path: _path,
}: DiffViewerProps): React.JSX.Element {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const [diff, setDiff] = useState<DiffSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [mode, setMode] = useState<DiffViewMode>("unified");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!activeWorkspaceId) {
      setDiff(null);
      return;
    }
    setLoading(true);
    setError(null);

    const promise = workdir
      ? getWorkdirDiff(activeWorkspaceId)
      : commitOid
        ? getCommitDiff(activeWorkspaceId, commitOid)
        : Promise.resolve(null);

    promise
      .then(setDiff)
      .catch((e) => setError(String(e)))
      .finally(() => setLoading(false));
  }, [activeWorkspaceId, commitOid, workdir]);

  if (!activeWorkspaceId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Select a workspace to view diff
      </div>
    );
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Loading diff...
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-full text-danger text-sm">{error}</div>
    );
  }

  if (!diff || diff.files.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        {workdir ? "No uncommitted changes" : "No diff available"}
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto">
      {/* Toolbar */}
      <div className="sticky top-0 z-10 flex items-center gap-2 px-4 py-2 bg-bg-primary border-b border-border-subtle">
        <span className="text-sm text-text-secondary">
          {diff.files.length} file{diff.files.length !== 1 ? "s" : ""} changed
        </span>
        <span className="text-success text-sm">+{diff.total_additions}</span>
        <span className="text-danger text-sm">-{diff.total_deletions}</span>
        <div className="ml-auto flex gap-1">
          <Button
            variant={mode === "unified" ? "primary" : "secondary"}
            size="sm"
            onClick={() => setMode("unified")}
          >
            Unified
          </Button>
          <Button
            variant={mode === "split" ? "primary" : "secondary"}
            size="sm"
            onClick={() => setMode("split")}
          >
            Split
          </Button>
        </div>
      </div>

      {/* Files */}
      <div className="p-4">
        {diff.files.map((file, i) => (
          <FileDiffView key={i} fileDiff={file} mode={mode} />
        ))}
      </div>
    </div>
  );
}
