import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ChevronDown,
  ChevronRight,
  Columns2,
  FoldVertical,
  PanelRightClose,
  PanelRightOpen,
  Square,
  UnfoldVertical,
} from "lucide-react";
import type { DiffSummary, FileDiff, DiffHunk, DiffLine, FileStatusKind } from "@/lib/api";
import {
  formatAppError,
  getCommitDiff,
  getImageContent,
  getWorkdirDiff,
  isImageTooLargeError,
} from "@/lib/api";
import { useTranslation } from "react-i18next";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { Button } from "@/components/ui/Button";
import { Chip } from "@heroui/react";
import { BlameView } from "@/components/BlameView";
import { filterDiffSummary, imageMimeFromPath, isImagePath } from "@/lib/diff";
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
  /** Working-copy only: kind of the selected change — the image diff uses it
   * to detect deletions (their workdir side has no OID and no file). */
  workdirKind?: FileStatusKind;
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
            "flex-1 px-2 whitespace-pre",
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
            "flex-1 px-2 whitespace-pre",
            line.kind === "added" && "bg-diff-add-bg",
            line.kind === "context" && "text-text-primary",
          )}
        >
          {line.kind === "added" ? `+ ${line.content}` : `  ${line.content}`}
        </span>
      );

    return (
      <div className="flex text-xs font-mono leading-5 border-b border-border-subtle/40">
        <span className="sticky left-0 z-10 bg-bg-elevated text-text-muted font-mono text-xs w-9 text-right pr-1.5 shrink-0 select-none tabular-nums">
          {line.kind === "added" ? "" : (line.old_line_no ?? "")}
        </span>
        <div className="flex-1 min-w-0 border-r border-border-subtle">{leftContent}</div>
        <span className="sticky left-9 z-10 bg-bg-elevated text-text-muted font-mono text-xs w-9 text-right pr-1.5 shrink-0 select-none tabular-nums">
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
      <span className="sticky left-0 z-10 shrink-0 w-9 text-right pr-1.5 pl-1 bg-bg-elevated border-r border-border-subtle text-text-muted select-none tabular-nums">
        {line.kind === "added" ? "" : (line.old_line_no ?? "")}
      </span>
      <span className="sticky left-9 z-10 shrink-0 w-9 text-right pr-1.5 pl-1 bg-bg-elevated border-r border-border-subtle text-text-muted select-none tabular-nums">
        {line.kind === "removed" ? "" : (line.new_line_no ?? "")}
      </span>
      <span className={cn("flex-1 px-2 min-w-0 whitespace-pre", bgClass)}>
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
            <span className="sticky left-0 z-10 bg-diff-del-bg-solid text-text-muted font-mono text-xs w-9 text-right pr-1.5 shrink-0 select-none tabular-nums">
              {line.old_line_no ?? ""}
            </span>
            <span className="sticky left-9 z-10 bg-diff-del-bg-solid text-text-muted font-mono text-xs w-9 text-right pr-1.5 shrink-0 select-none tabular-nums" />
            <span className="flex-1 px-2 text-danger whitespace-pre">
              - <WordDiffSpans before={line.content} after={next.content} side="removed" />
            </span>
          </div>
          <div className="flex text-xs font-mono leading-5 bg-diff-add-bg">
            <span className="sticky left-0 z-10 bg-diff-add-bg-solid text-text-muted font-mono text-xs w-9 text-right pr-1.5 shrink-0 select-none tabular-nums" />
            <span className="sticky left-9 z-10 bg-diff-add-bg-solid text-text-muted font-mono text-xs w-9 text-right pr-1.5 shrink-0 select-none tabular-nums">
              {next.new_line_no ?? ""}
            </span>
            <span className="flex-1 px-2 text-success whitespace-pre">
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
    <div className="border border-border-subtle mb-3">
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

/** One pane of the image diff: fetch a version (blob OID or working tree)
 * and render it as a `data:` URL (CSP already allows img-src data:).
 * Committed OIDs are content-addressed so they cache forever; worktree reads
 * refetch whenever the pane remounts. */
function ImageDiffPane({
  workspaceId,
  path,
  oid,
  mime,
  label,
}: {
  workspaceId: string;
  path: string;
  /** Blob OID of this version; null reads the working-tree file. */
  oid: string | null;
  mime: string;
  label: string;
}): React.JSX.Element {
  const { t } = useTranslation();
  const [broken, setBroken] = useState(false);
  const query = useQuery({
    queryKey: ["diff-image", workspaceId, path, oid ?? "<worktree>"],
    queryFn: () => getImageContent(workspaceId, path, oid ?? undefined),
    staleTime: oid ? Infinity : 0,
  });

  useEffect(() => {
    setBroken(false);
  }, [query.data]);

  let body: React.ReactNode;
  if (query.isPending) {
    body = <span className="text-xs text-text-muted">{t("diff.image.loading")}</span>;
  } else if (query.isError) {
    body = (
      <span className="max-w-full text-center text-xs text-text-muted">
        {isImageTooLargeError(query.error)
          ? t("diff.image.tooLarge")
          : t("diff.image.cannotDisplay")}
      </span>
    );
  } else if (broken) {
    // E.g. LFS pointer files or corrupt bytes the backend cannot detect.
    body = <span className="text-xs text-text-muted">{t("diff.image.cannotDisplay")}</span>;
  } else {
    body = (
      <img
        src={`data:${mime};base64,${query.data.base64}`}
        alt={label}
        className="max-h-[420px] max-w-full object-contain"
        onError={() => setBroken(true)}
      />
    );
  }

  return (
    <figure className="flex min-w-0 flex-col">
      <figcaption className="shrink-0 px-3 py-1.5 text-[11px] font-medium uppercase tracking-wide text-text-muted">
        {label}
      </figcaption>
      <div className="flex min-h-[140px] flex-1 items-center justify-center overflow-auto p-3">
        {body}
      </div>
    </figure>
  );
}

function ImageDiffEmptyPane({ label, hint }: { label: string; hint: string }): React.JSX.Element {
  return (
    <figure className="flex min-w-0 flex-col">
      <figcaption className="shrink-0 px-3 py-1.5 text-[11px] font-medium uppercase tracking-wide text-text-muted">
        {label}
      </figcaption>
      <div className="flex min-h-[140px] flex-1 items-center justify-center p-3">
        <span className="rounded-sm bg-bg-elevated px-2 py-0.5 text-xs text-text-muted">{hint}</span>
      </div>
    </figure>
  );
}

/** F016: side-by-side image compare replacing the hunk area for image files.
 * Left = old version, right = new version; a missing side collapses to a
 * single pane with an Added / Deleted marker. */
function ImageDiffView({
  fileDiff,
  workdir,
  workdirKind,
}: {
  fileDiff: FileDiff;
  workdir: boolean;
  workdirKind?: FileStatusKind;
}): React.JSX.Element {
  const { t } = useTranslation();
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  if (!activeWorkspaceId) {
    return (
      <div className="border-x border-b border-border-subtle py-4 text-center text-sm text-text-muted">
        {t("diff.image.cannotDisplay")}
      </div>
    );
  }
  const mime = imageMimeFromPath(fileDiff.path);
  // Commit diffs carry OIDs on both existing sides. In the working copy the
  // new side has no OID (git2 does not hash workdir content), so it is read
  // from the working tree — unless the file was deleted there, in which case
  // the selected kind is the only signal (both sides report no OID).
  const hasOld = fileDiff.old_sha != null;
  const hasNew = workdir ? workdirKind !== "deleted" : fileDiff.new_sha != null;
  return (
    <div
      className="grid min-w-0 divide-x divide-border-subtle border-b border-border-subtle"
      style={{ gridTemplateColumns: hasOld && hasNew ? "1fr 1fr" : "1fr" }}
    >
      {hasOld ? (
        <ImageDiffPane
          workspaceId={activeWorkspaceId}
          path={fileDiff.path}
          oid={fileDiff.old_sha}
          mime={mime}
          label={t("diff.image.old")}
        />
      ) : (
        <ImageDiffEmptyPane label={t("diff.image.old")} hint={t("diff.image.added")} />
      )}
      {hasNew ? (
        <ImageDiffPane
          workspaceId={activeWorkspaceId}
          path={fileDiff.path}
          oid={fileDiff.new_sha}
          mime={mime}
          label={t("diff.image.new")}
        />
      ) : (
        <ImageDiffEmptyPane label={t("diff.image.new")} hint={t("diff.image.deleted")} />
      )}
    </div>
  );
}

function FileDiffView({
  fileDiff,
  mode,
  workdir,
  workdirKind,
  collapsed,
  onToggleCollapsed,
  onBlame,
}: {
  fileDiff: FileDiff;
  mode: DiffViewMode;
  /** Working-copy context: how the image view resolves the new version. */
  workdir: boolean;
  /** Kind of the selected working-copy change (deleted images show no "new"). */
  workdirKind?: FileStatusKind;
  collapsed: boolean;
  onToggleCollapsed: () => void;
  onBlame?: (path: string) => void;
}): React.JSX.Element {
  const { t } = useTranslation();
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
            aria-label={t(collapsed ? "diff.file.expand" : "diff.file.collapse")}
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
              <Chip.Label>{t("diff.chip.staged")}</Chip.Label>
            </Chip>
          ) : null}
          {fileDiff.staged === false ? (
            <Chip
              size="sm"
              className="ml-2 shrink-0 rounded-sm px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-text-muted/15 text-text-secondary shadow-none"
            >
              <Chip.Label>{t("diff.chip.unstaged")}</Chip.Label>
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
                {t("diff.file.blame")}
              </Button>
            ) : null}
          </div>
        </div>
      </div>

      {/* Hunks — image files render a side-by-side compare instead (F016). */}
      {!collapsed ? (
        isImagePath(fileDiff.path) ? (
          <ImageDiffView fileDiff={fileDiff} workdir={workdir} workdirKind={workdirKind} />
        ) : (
          <div className="isolate overflow-x-auto border-x border-b border-border-subtle px-0 pt-0">
            {/* w-max wrapper: every hunk stretches to the widest hunk's width so
                borders stay continuous while scrolling horizontally. */}
            <div className="w-max min-w-full">
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
                      <span className="text-text-muted">{t("diff.file.noHunkDetail")}</span>
                    </>
                  ) : (
                    t("diff.file.noChanges")
                  )}
                </div>
              )}
            </div>
          </div>
        )
      ) : null}
    </div>
  );
}

export function DiffViewer({
  commitOid,
  workdir = false,
  path,
  staged = null,
  workdirKind,
  hideMaximize = false,
}: DiffViewerProps): React.JSX.Element {
  const { t } = useTranslation();
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const inspectorMaximized = useLayoutStore((s) => s.inspectorMaximized);
  const toggleInspectorMaximized = useLayoutStore((s) => s.toggleInspectorMaximized);
  const { data: workingCopy } = useWorkingCopy();
  // Content-aware signature: an in-place edit keeps staged/path/kind
  // identical, so additions/deletions (+tree sha) must participate or the
  // 2s working-copy poll never refreshes the panel.
  const fileSignature = workdir
    ? `${workingCopy?.sha ?? ""}|` +
      (workingCopy?.files
        .map((f) => `${f.staged}:${f.path}:${f.kind}:${f.additions}:${f.deletions}`)
        .join("|") ?? "")
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
            ? t("diff.empty.noStaged", { path })
            : staged === false
              ? t("diff.empty.noUnstaged", { path })
              : t("diff.empty.noForPath", { path })
          : workdir
            ? t("diff.empty.noUncommitted")
            : t("diff.empty.noAvailable")}
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
          {t("diff.toolbar.filesChanged", { count: visible.files.length })}
        </span>
        {workdir && staged === true ? (
          <span className="rounded-sm px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-accent/15 text-accent">
            {t("diff.chip.staged")}
          </span>
        ) : null}
        {workdir && staged === false ? (
          <span className="rounded-sm px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-text-muted/15 text-text-secondary">
            {t("diff.chip.unstaged")}
          </span>
        ) : null}
        <span className="text-success text-sm">+{visible.total_additions}</span>
        <span className="text-danger text-sm">-{visible.total_deletions}</span>
        <div className="ml-auto flex items-center gap-1.5">
          {/* Icon toggles styled like the panel button: the icon shows the
              state clicking switches to (action semantics). */}
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="p-1.5 text-text-muted hover:text-accent"
            disabled={visible.files.length === 0}
            aria-pressed={anyCollapsed}
            aria-label={t(anyCollapsed ? "diff.toolbar.expandAll" : "diff.toolbar.collapseAll")}
            title={t(anyCollapsed ? "diff.toolbar.expandAll" : "diff.toolbar.collapseAll")}
            onClick={() =>
              setCollapsedFiles(
                anyCollapsed ? new Set() : new Set(visible.files.map(fileChangeKey)),
              )
            }
          >
            {anyCollapsed ? <UnfoldVertical size={14} /> : <FoldVertical size={14} />}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="p-1.5 text-text-muted hover:text-accent"
            aria-pressed={mode === "split"}
            aria-label={t(
              mode === "unified" ? "diff.toolbar.splitView" : "diff.toolbar.unifiedView",
            )}
            title={t(mode === "unified" ? "diff.toolbar.splitView" : "diff.toolbar.unifiedView")}
            onClick={() => setMode(mode === "unified" ? "split" : "unified")}
          >
            {mode === "unified" ? <Columns2 size={14} /> : <Square size={14} />}
          </Button>
          {!hideMaximize ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="p-1.5 text-text-muted hover:text-accent"
              aria-pressed={inspectorMaximized}
              aria-label={t(
                inspectorMaximized ? "diff.toolbar.restorePanel" : "diff.toolbar.expandPanel",
              )}
              title={t(
                inspectorMaximized ? "diff.toolbar.restorePanel" : "diff.toolbar.expandPanel",
              )}
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
            workdir={workdir}
            workdirKind={workdirKind}
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
