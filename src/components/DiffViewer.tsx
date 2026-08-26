import { useEffect, useState } from "react";
import type { DiffSummary, FileDiff, DiffHunk, DiffLine } from "@/lib/api";
import { formatAppError, getCommitDiff, getWorkdirDiff } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { BlameView } from "@/components/BlameView";
import { cn } from "@/lib/utils";

type DiffViewMode = "unified" | "split";
type PanelMode = "diff" | "blame";

/** Highlight character-level changes between old and new strings. */
function WordDiffSpans({
  before,
  after,
  side,
}: {
  before: string;
  after: string;
  side: "removed" | "added";
}): React.JSX.Element {
  // Longest common prefix/suffix → middle is the changed span.
  let start = 0;
  while (start < before.length && start < after.length && before[start] === after[start]) {
    start += 1;
  }
  let endBefore = before.length;
  let endAfter = after.length;
  while (
    endBefore > start &&
    endAfter > start &&
    before[endBefore - 1] === after[endAfter - 1]
  ) {
    endBefore -= 1;
    endAfter -= 1;
  }
  const text = side === "removed" ? before : after;
  const midStart = start;
  const midEnd = side === "removed" ? endBefore : endAfter;
  return (
    <>
      {text.slice(0, midStart)}
      <span className={side === "removed" ? "bg-danger/30" : "bg-success/30"}>
        {text.slice(midStart, midEnd)}
      </span>
      {text.slice(midEnd)}
    </>
  );
}

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
  const prefix = line.kind === "added" ? "+" : line.kind === "removed" ? "-" : " ";

  if (mode === "split") {
    const leftContent =
      line.kind === "added" ? (
        <span className="flex-1 px-2 text-text-muted select-none">&nbsp;</span>
      ) : (
        <span
          className={cn(
            "flex-1 px-2",
            line.kind === "removed" && "bg-danger/10 text-danger",
            line.kind === "context" && "text-text-primary",
          )}
        >
          {line.kind === "removed" ? `- ${line.content}` : `  ${line.content}`}
        </span>
      );

    const rightContent =
      line.kind === "removed" ? (
        <span className="flex-1 px-2 text-text-muted select-none">&nbsp;</span>
      ) : (
        <span
          className={cn(
            "flex-1 px-2",
            line.kind === "added" && "bg-success/10 text-success",
            line.kind === "context" && "text-text-primary",
          )}
        >
          {line.kind === "added" ? `+ ${line.content}` : `  ${line.content}`}
        </span>
      );

    return (
      <div className="flex text-xs font-mono leading-5 border-b border-border-subtle/40">
        <span className="text-text-muted font-mono text-xs w-10 text-right pr-2 shrink-0 select-none tabular-nums">
          {line.kind === "added" ? "" : (line.old_line_no ?? "")}
        </span>
        <div className="flex-1 min-w-0 border-r border-border-subtle">{leftContent}</div>
        <span className="text-text-muted font-mono text-xs w-10 text-right pr-2 shrink-0 select-none tabular-nums">
          {line.kind === "removed" ? "" : (line.new_line_no ?? "")}
        </span>
        <div className="flex-1 min-w-0">{rightContent}</div>
      </div>
    );
  }

  const bgClass =
    line.kind === "added"
      ? "bg-success/10 text-success"
      : line.kind === "removed"
        ? "bg-danger/10 text-danger"
        : "text-text-primary";

  return (
    <div
      className={cn(
        "flex text-xs font-mono leading-5",
        line.kind === "added" && "bg-success/10",
        line.kind === "removed" && "bg-danger/10",
      )}
    >
      <span className="text-text-muted font-mono text-xs w-10 text-right pr-2 shrink-0 select-none tabular-nums">
        {line.kind === "added" ? "" : (line.old_line_no ?? "")}
      </span>
      <span className="text-text-muted font-mono text-xs w-10 text-right pr-2 shrink-0 select-none tabular-nums">
        {line.kind === "removed" ? "" : (line.new_line_no ?? "")}
      </span>
      <span className={cn("flex-1 px-2", bgClass)}>
        {prefix} {line.content}
      </span>
    </div>
  );
}

function DiffHunkView({ hunk, mode }: { hunk: DiffHunk; mode: DiffViewMode }): React.JSX.Element {
  const rendered: React.JSX.Element[] = [];
  const lines = hunk.lines;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!;
    const next = lines[i + 1];
    if (mode === "unified" && line.kind === "removed" && next?.kind === "added") {
      rendered.push(
        <div key={`w-${i}`}>
          <div className="flex text-xs font-mono leading-5 bg-danger/10">
            <span className="text-text-muted font-mono text-xs w-10 text-right pr-2 shrink-0 select-none tabular-nums">
              {line.old_line_no ?? ""}
            </span>
            <span className="text-text-muted font-mono text-xs w-10 text-right pr-2 shrink-0 select-none tabular-nums" />
            <span className="flex-1 px-2 text-danger">
              - <WordDiffSpans before={line.content} after={next.content} side="removed" />
            </span>
          </div>
          <div className="flex text-xs font-mono leading-5 bg-success/10">
            <span className="text-text-muted font-mono text-xs w-10 text-right pr-2 shrink-0 select-none tabular-nums" />
            <span className="text-text-muted font-mono text-xs w-10 text-right pr-2 shrink-0 select-none tabular-nums">
              {next.new_line_no ?? ""}
            </span>
            <span className="flex-1 px-2 text-success">
              + <WordDiffSpans before={line.content} after={next.content} side="added" />
            </span>
          </div>
        </div>,
      );
      i += 1;
      continue;
    }
    rendered.push(<DiffLineView key={i} line={line} mode={mode} />);
  }

  return (
    <div className="border border-border-subtle rounded-md overflow-hidden mb-3">
      <div className="bg-bg-secondary px-3 py-1 text-xs text-text-muted font-mono border-b border-border-subtle">
        @@ -{hunk.old_start},{hunk.old_lines} +{hunk.new_start},{hunk.new_lines} @@
      </div>
      {rendered}
    </div>
  );
}

function FileDiffView({
  fileDiff,
  mode,
  onBlame,
}: {
  fileDiff: FileDiff;
  mode: DiffViewMode;
  onBlame?: (path: string) => void;
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
          {onBlame ? (
            <Button type="button" variant="ghost" size="sm" onClick={() => onBlame(fileDiff.path)}>
              Blame
            </Button>
          ) : null}
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
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const [diff, setDiff] = useState<DiffSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [mode, setMode] = useState<DiffViewMode>("unified");
  const [panel, setPanel] = useState<PanelMode>("diff");
  const [blamePath, setBlamePath] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!activeWorkspaceId || !activeRepoId) {
      setDiff(null);
      setError(null);
      return;
    }
    setLoading(true);
    setError(null);
    setPanel("diff");
    setBlamePath(null);

    const promise = workdir
      ? getWorkdirDiff(activeWorkspaceId)
      : commitOid
        ? getCommitDiff(activeWorkspaceId, commitOid)
        : Promise.resolve(null);

    promise
      .then(setDiff)
      .catch((e) => setError(formatAppError(e)))
      .finally(() => setLoading(false));
  }, [activeWorkspaceId, activeRepoId, commitOid, workdir]);

  if (!activeWorkspaceId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Select a workspace to view diff
      </div>
    );
  }

  if (!activeRepoId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Select a repository to view diff
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
      <div className="flex items-center justify-center h-full text-danger text-sm px-4 text-center">
        {error}
      </div>
    );
  }

  if (panel === "blame" && blamePath) {
    return (
      <div className="h-full min-h-0 flex flex-col overflow-hidden">
        <div className="shrink-0 flex items-center gap-2 px-4 py-2 border-b border-border-subtle">
          <Button type="button" variant="secondary" size="sm" onClick={() => setPanel("diff")}>
            Back to diff
          </Button>
          <span className="text-sm text-text-secondary font-mono truncate">{blamePath}</span>
        </div>
        <div className="flex-1 min-h-0">
          <BlameView path={blamePath} />
        </div>
      </div>
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
    <div className="h-full min-h-0 overflow-auto">
      {/* Toolbar */}
      <div className="sticky top-0 z-10 flex items-center gap-2 px-4 py-2 bg-bg-primary border-b border-border-subtle">
        <span className="text-sm text-text-secondary">
          {diff.files.length} file{diff.files.length !== 1 ? "s" : ""} changed
        </span>
        <span className="text-success text-sm">+{diff.total_additions}</span>
        <span className="text-danger text-sm">-{diff.total_deletions}</span>
        <div className="ml-auto flex gap-1">
          <Button
            type="button"
            variant={mode === "unified" ? "primary" : "secondary"}
            size="sm"
            aria-pressed={mode === "unified"}
            onClick={() => setMode("unified")}
          >
            Unified
          </Button>
          <Button
            type="button"
            variant={mode === "split" ? "primary" : "secondary"}
            size="sm"
            aria-pressed={mode === "split"}
            onClick={() => setMode("split")}
          >
            Split
          </Button>
        </div>
      </div>

      {/* Files */}
      <div className="p-4">
        {diff.files.map((file, i) => (
          <FileDiffView
            key={i}
            fileDiff={file}
            mode={mode}
            onBlame={(p) => {
              setBlamePath(p);
              setPanel("blame");
            }}
          />
        ))}
      </div>
    </div>
  );
}
