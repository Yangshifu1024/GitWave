import { useEffect, useState } from "react";
import { ChevronDown, ChevronRight, PanelRightClose, PanelRightOpen } from "lucide-react";
import type { DiffSummary, FileDiff, DiffHunk, DiffLine } from "@/lib/api";
import { formatAppError, getCommitDiff, getWorkdirDiff } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { Button } from "@/components/ui/Button";
import { Chip, Radio, RadioGroup } from "@heroui/react";
import { BlameView } from "@/components/BlameView";
import { filterDiffSummary } from "@/lib/diff";
import { cn } from "@/lib/utils";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";

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
  while (endBefore > start && endAfter > start && before[endBefore - 1] === after[endAfter - 1]) {
    endBefore -= 1;
    endAfter -= 1;
  }
  const text = side === "removed" ? before : after;
  const midStart = start;
  const midEnd = side === "removed" ? endBefore : endAfter;
  return (
    <>
      {text.slice(0, midStart)}
      <span className={side === "removed" ? "bg-diff-del-word" : "bg-diff-add-word"}>
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
  /** Path to show diff for */
  path?: string;
  /** Working-copy only: true = staged (index vs HEAD), false = unstaged (worktree vs index). */
  staged?: boolean | null;
  /** Hide the inspector-maximize button (e.g. inside WorkingCopyModal). */
  hideMaximize?: boolean;
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
            "flex-1 px-2 whitespace-pre-wrap",
            line.kind === "removed" && "bg-diff-del-bg",
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
            "flex-1 px-2 whitespace-pre-wrap",
            line.kind === "added" && "bg-diff-add-bg",
            line.kind === "context" && "text-text-primary",
          )}
        >
          {line.kind === "added" ? `+ ${line.content}` : `  ${line.content}`}
        </span>
      );

    return (
      <div className="flex text-xs font-mono leading-5 border-b border-border-subtle/40">
        <span className="text-text-muted font-mono text-xs w-9 text-right pr-1.5 shrink-0 select-none tabular-nums">
          {line.kind === "added" ? "" : (line.old_line_no ?? "")}
        </span>
        <div className="flex-1 min-w-0 border-r border-border-subtle">{leftContent}</div>
        <span className="text-text-muted font-mono text-xs w-9 text-right pr-1.5 shrink-0 select-none tabular-nums">
          {line.kind === "removed" ? "" : (line.new_line_no ?? "")}
        </span>
        <div className="flex-1 min-w-0">{rightContent}</div>
      </div>
    );
  }

  // Line text stays text-primary (GitHub diffBlob style); only the +/- prefix
  // and word-diff spans carry semantic color.
  const bgClass =
    line.kind === "added"
      ? "bg-diff-add-bg"
      : line.kind === "removed"
        ? "bg-diff-del-bg"
        : "text-text-primary";

  return (
    <div
      className={cn(
        "flex text-xs font-mono leading-5",
        line.kind === "added" && "bg-diff-add-bg",
        line.kind === "removed" && "bg-diff-del-bg",
      )}
    >
      <span className="shrink-0 w-9 text-right pr-1.5 pl-1 bg-bg-elevated border-r border-border-subtle text-text-muted select-none tabular-nums">
        {line.kind === "added" ? "" : (line.old_line_no ?? "")}
      </span>
      <span className="shrink-0 w-9 text-right pr-1.5 pl-1 bg-bg-elevated border-r border-border-subtle text-text-muted select-none tabular-nums">
        {line.kind === "removed" ? "" : (line.new_line_no ?? "")}
      </span>
      <span className={cn("flex-1 px-2 min-w-0 whitespace-pre-wrap", bgClass)}>
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
          <div className="flex text-xs font-mono leading-5 bg-diff-del-bg">
            <span className="text-text-muted font-mono text-xs w-9 text-right pr-1.5 shrink-0 select-none tabular-nums">
              {line.old_line_no ?? ""}
            </span>
            <span className="text-text-muted font-mono text-xs w-9 text-right pr-1.5 shrink-0 select-none tabular-nums" />
            <span className="flex-1 px-2 text-danger whitespace-pre-wrap">
              - <WordDiffSpans before={line.content} after={next.content} side="removed" />
            </span>
          </div>
          <div className="flex text-xs font-mono leading-5 bg-diff-add-bg">
            <span className="text-text-muted font-mono text-xs w-9 text-right pr-1.5 shrink-0 select-none tabular-nums" />
            <span className="text-text-muted font-mono text-xs w-9 text-right pr-1.5 shrink-0 select-none tabular-nums">
              {next.new_line_no ?? ""}
            </span>
            <span className="flex-1 px-2 text-success whitespace-pre-wrap">
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
    <div className="border border-border-subtle overflow-hidden mb-3">
      <div className="bg-diff-hunk-bg px-3 py-1 text-xs text-text-muted font-mono border-b border-border-subtle">
        @@ -{hunk.old_start},{hunk.old_lines} +{hunk.new_start},{hunk.new_lines} @@
      </div>
      {rendered}
    </div>
  );
}

function splitPath(path: string): { dir: string; name: string } {
  const idx = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  if (idx < 0) return { dir: "", name: path };
  return { dir: path.slice(0, idx + 1), name: path.slice(idx + 1) };
}

function fileChangeKey(f: { path: string; staged?: boolean | null }): string {
  return `${f.staged ? "s" : "u"}:${f.path}`;
}

/** Compact iOS-style segmented control. */
function SegmentedControl({
  segments,
  active,
  onSelect,
  "aria-label": ariaLabel,
}: {
  segments: { key: string; label: string }[];
  active: string | null;
  onSelect: (key: string) => void;
  "aria-label": string;
}): React.JSX.Element {
  return (
    <RadioGroup
      value={active ?? ""}
      onChange={onSelect}
      orientation="horizontal"
      aria-label={ariaLabel}
      className="flex items-center gap-0 rounded-md border border-border-subtle bg-bg-primary p-0.5 [&_[data-slot=radio]]:mt-0"
    >
      {segments.map((s) => (
        <Radio key={s.key} value={s.key}>
          <Radio.Content
            className={cn(
              "h-auto rounded-sm border-0 px-2 py-0.5 text-xs shadow-none transition-colors duration-fast",
              active === s.key
                ? "bg-accent font-medium text-text-inverse"
                : "bg-transparent text-text-secondary hover:text-text-primary",
            )}
          >
            {s.label}
          </Radio.Content>
        </Radio>
      ))}
    </RadioGroup>
  );
}

function FileDiffView({
  fileDiff,
  mode,
  collapsed,
  onToggleCollapsed,
  onBlame,
}: {
  fileDiff: FileDiff;
  mode: DiffViewMode;
  collapsed: boolean;
  onToggleCollapsed: () => void;
  onBlame?: (path: string) => void;
}): React.JSX.Element {
  // Language for future shiki integration
  void getLanguage(getExt(fileDiff.path));
  const { dir, name } = splitPath(fileDiff.path);

  return (
    <div className="mb-3 min-w-0">
      <div className="min-w-0 px-3 py-2 bg-bg-elevated border-b border-border-subtle">
        <div className="flex min-w-0 items-center gap-2" title={fileDiff.path}>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            aria-expanded={!collapsed}
            aria-label={collapsed ? "Expand file diff" : "Collapse file diff"}
            onClick={onToggleCollapsed}
            className="h-auto shrink-0 p-0 text-text-muted hover:text-text-secondary border-0 shadow-none bg-transparent"
          >
            {collapsed ? <ChevronRight size={14} /> : <ChevronDown size={14} />}
          </Button>
          <span className="shrink-0 rounded-sm bg-bg-elevated px-2 py-0.5 text-xs font-mono font-medium text-text-primary">
            {name}
          </span>
          {dir ? (
            <span className="min-w-0 truncate text-xs font-mono text-text-muted">{dir}</span>
          ) : null}
          {fileDiff.staged === true ? (
            <Chip
              size="sm"
              className="ml-2 shrink-0 rounded-sm px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-accent/15 text-accent shadow-none"
            >
              <Chip.Label>Staged</Chip.Label>
            </Chip>
          ) : null}
          {fileDiff.staged === false ? (
            <Chip
              size="sm"
              className="ml-2 shrink-0 rounded-sm px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-text-muted/15 text-text-secondary shadow-none"
            >
              <Chip.Label>Unstaged</Chip.Label>
            </Chip>
          ) : null}
        </div>
        <div className="mt-1 flex items-center gap-2 text-xs">
          <span className="min-w-0 truncate font-mono text-text-muted">
            {fileDiff.old_sha?.slice(0, 7) ?? "0000000"}
            {fileDiff.old_sha && fileDiff.new_sha ? " → " : ""}
            {fileDiff.new_sha?.slice(0, 7) ?? "0000000"}
          </span>
          <div className="ml-auto flex shrink-0 items-center gap-2">
            <span className="text-success">+{fileDiff.additions}</span>
            <span className="text-danger">-{fileDiff.deletions}</span>
            {onBlame ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => onBlame(fileDiff.path)}
              >
                Blame
              </Button>
            ) : null}
          </div>
        </div>
      </div>

      {/* Hunks */}
      {!collapsed ? (
        <div className="border-x border-b border-border-subtle px-0 pt-0">
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
      ) : null}
    </div>
  );
}

export function DiffViewer({
  commitOid,
  workdir = false,
  path,
  staged = null,
  hideMaximize = false,
}: DiffViewerProps): React.JSX.Element {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const inspectorMaximized = useLayoutStore((s) => s.inspectorMaximized);
  const toggleInspectorMaximized = useLayoutStore((s) => s.toggleInspectorMaximized);
  const { data: workingCopy } = useWorkingCopy();
  const fileSignature = workdir
    ? (workingCopy?.files.map((f) => `${f.staged}:${f.path}:${f.kind}`).join("|") ?? "")
    : "";
  const [diff, setDiff] = useState<DiffSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [mode, setMode] = useState<DiffViewMode>("unified");
  const [panel, setPanel] = useState<PanelMode>("diff");
  const [blamePath, setBlamePath] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [collapsedFiles, setCollapsedFiles] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (!activeWorkspaceId || !activeRepoId) {
      setDiff(null);
      setError(null);
      return;
    }
    let cancelled = false;
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
      .then((result) => {
        if (!cancelled) setDiff(result);
      })
      .catch((e) => {
        if (!cancelled) setError(formatAppError(e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [activeWorkspaceId, activeRepoId, commitOid, workdir, fileSignature]);

  useEffect(() => {
    setPanel("diff");
    setBlamePath(null);
  }, [path, staged]);

  const visible = diff ? filterDiffSummary(diff, path, staged) : null;
  const fileKeys = visible ? visible.files.map(fileChangeKey) : [];
  const allCollapsed = fileKeys.length > 0 && fileKeys.every((k) => collapsedFiles.has(k));
  const anyCollapsed = fileKeys.some((k) => collapsedFiles.has(k));

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

  if (!visible || visible.files.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm px-4 text-center">
        {path
          ? staged === true
            ? `No staged diff for ${path}`
            : staged === false
              ? `No unstaged diff for ${path}`
              : `No diff for ${path}`
          : workdir
            ? "No uncommitted changes"
            : "No diff available"}
      </div>
    );
  }

  return (
    <div className="h-full min-h-0 overflow-auto flex flex-col">
      {path && visible.files.length === 1 ? (
        <div className="shrink-0 flex items-center gap-2 px-3 py-2 bg-bg-elevated border-b border-border-subtle">
          <span className="shrink-0 rounded-sm bg-bg-elevated px-2 py-0.5 text-xs font-mono font-medium text-text-primary">
            {splitPath(path).name}
          </span>
          <span className="min-w-0 truncate text-xs font-mono text-text-muted">
            {splitPath(path).dir}
          </span>
          <span className="ml-auto text-xs text-text-muted tabular-nums">
            +{visible.total_additions} / -{visible.total_deletions}
          </span>
        </div>
      ) : null}
      {/* Toolbar */}
      <div className="sticky top-0 z-10 flex items-center gap-2 px-4 py-2 bg-bg-elevated border-b border-border-subtle">
        <span className="text-sm text-text-secondary">
          {visible.files.length} file{visible.files.length !== 1 ? "s" : ""} changed
        </span>
        {workdir && staged === true ? (
          <span className="rounded-sm px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-accent/15 text-accent">
            Staged
          </span>
        ) : null}
        {workdir && staged === false ? (
          <span className="rounded-sm px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-text-muted/15 text-text-secondary">
            Unstaged
          </span>
        ) : null}
        <span className="text-success text-sm">+{visible.total_additions}</span>
        <span className="text-danger text-sm">-{visible.total_deletions}</span>
        <div className="ml-auto flex items-center gap-1.5">
          <SegmentedControl
            aria-label="File fold"
            segments={[
              { key: "expand", label: "Expand" },
              { key: "collapse", label: "Collapse" },
            ]}
            active={allCollapsed ? "collapse" : anyCollapsed ? null : "expand"}
            onSelect={(key) =>
              setCollapsedFiles(
                key === "collapse" ? new Set(visible.files.map(fileChangeKey)) : new Set(),
              )
            }
          />
          <SegmentedControl
            aria-label="Diff view"
            segments={[
              { key: "unified", label: "Unified" },
              { key: "split", label: "Split" },
            ]}
            active={mode}
            onSelect={(key) => setMode(key as DiffViewMode)}
          />
          {!hideMaximize ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="p-1.5 text-text-muted hover:text-accent"
              aria-pressed={inspectorMaximized}
              aria-label={inspectorMaximized ? "Restore panel" : "Expand panel"}
              title={inspectorMaximized ? "Restore panel" : "Expand panel"}
              onClick={toggleInspectorMaximized}
            >
              {inspectorMaximized ? <PanelRightClose size={14} /> : <PanelRightOpen size={14} />}
            </Button>
          ) : null}
        </div>
      </div>

      {/* Files */}
      <div className="pb-2 select-text">
        {visible.files.map((file) => (
          <FileDiffView
            key={fileChangeKey(file)}
            fileDiff={file}
            mode={mode}
            collapsed={collapsedFiles.has(fileChangeKey(file))}
            onToggleCollapsed={() =>
              setCollapsedFiles((prev) => {
                const next = new Set(prev);
                const key = fileChangeKey(file);
                if (next.has(key)) {
                  next.delete(key);
                } else {
                  next.add(key);
                }
                return next;
              })
            }
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
