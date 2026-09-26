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
import type { FileDiff, FileStatusKind, DiffPreview, DiffPreviewRequest } from "@/lib/api";
import { formatAppError, getDiffPreview, getImageContent, isImageTooLargeError } from "@/lib/api";
import { useTranslation } from "react-i18next";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { Button } from "@/components/ui/Button";
import { Chip } from "@heroui/react";
import { BlameView } from "@/components/BlameView";
import { filterDiffSummary, imageMimeFromPath, isImagePath } from "@/lib/diff";
import { DiffText } from "@/components/DiffText";

type DiffViewMode = "unified" | "split";
type PanelMode = "diff" | "blame";

export interface DiffViewerProps {
  /** If provided, show diff of this stash entry. Wins over workdir / commitOid */
  stashOid?: string;
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
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const query = useQuery({
    queryKey: ["diff-image", workspaceId, repoId, path, oid ?? "<worktree>"],
    queryFn: () => getImageContent(workspaceId, path, oid ?? undefined, repoId ?? undefined),
    staleTime: oid ? Infinity : 0,
    refetchInterval: oid ? false : 2000,
    // A missing version (deleted file, unreadable bytes) is final — surface
    // the error state instead of burning ~7s on react-query's default retries.
    retry: false,
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
        <span className="rounded-sm bg-bg-elevated px-2 py-0.5 text-xs text-text-muted">
          {hint}
        </span>
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
  // The unstaged side of the working copy carries a "ghost" OID: libgit2
  // hashes the worktree content into the delta but never writes that blob to
  // the ODB, so find_blob on it fails — read the working-tree file instead
  // (staged entries come from the index and commit diffs from trees; both
  // are materialized). Deleted workdir files have no usable side either, and
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
          oid={fileDiff.staged === false ? null : fileDiff.new_sha}
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
            {fileDiff.hunks.length > 0 ? (
              <>
                <span className="text-success">+{fileDiff.additions}</span>
                <span className="text-danger">-{fileDiff.deletions}</span>
              </>
            ) : null}
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
                <DiffText file={fileDiff} mode={mode} />
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
                    t("diff.preview.noText")
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
  stashOid,
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
  const [expanded, setExpanded] = useState(false);
  const [mode, setMode] = useState<DiffViewMode>("unified");
  const [panel, setPanel] = useState<PanelMode>("diff");
  const [blamePath, setBlamePath] = useState<string | null>(null);
  const [collapsedFiles, setCollapsedFiles] = useState<Set<string>>(new Set());
  const request: DiffPreviewRequest = {
    path,
    staged,
    commit_oid: commitOid,
    stash_oid: stashOid,
    expanded,
  };
  const query = useQuery({
    queryKey: ["diff-preview", activeWorkspaceId, activeRepoId, request],
    queryFn: () => getDiffPreview(activeWorkspaceId!, activeRepoId!, request),
    enabled: Boolean(activeWorkspaceId && activeRepoId && (workdir || commitOid || stashOid)),
    // A successful status poll can have identical metadata after content edits.
    // Poll only the selected file, independent of path/status/line-count changes.
    refetchInterval: workdir ? 2000 : false,
    retry: false,
  });
  const diff = query.data?.diff ?? null;
  const loading = query.isPending;
  const error = query.error ? formatAppError(query.error) : null;
  useEffect(() => {
    setExpanded(false);
    setCollapsedFiles(new Set());
    setPanel("diff");
    setBlamePath(null);
  }, [activeWorkspaceId, activeRepoId, commitOid, stashOid, path, staged]);

  useEffect(() => {
    setPanel("diff");
    setBlamePath(null);
  }, [path, staged]);

  const visible = diff ? filterDiffSummary(diff, path, staged) : null;
  const fileKeys = visible ? visible.files.map(fileChangeKey) : [];
  const anyCollapsed = fileKeys.some((k) =>
    path ? collapsedFiles.has(k) : !collapsedFiles.has(k),
  );

  if (!activeWorkspaceId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        {t("repo.blame.selectWorkspace")}
      </div>
    );
  }

  if (!activeRepoId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        {t("repo.blame.selectRepo")}
      </div>
    );
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        {t("diff.preview.loading")}
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
            {t("diff.preview.back")}
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
    // A stash viewer always passes a `path` (StashDetailModal mounts DiffViewer
    // only once a file is selected), so the stash empty state is the `noForPath`
    // arm — a bare `stashOid` arm would be unreachable and is gone.
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
          {!query.data?.too_large ? (
            <span className="ml-auto text-xs text-text-muted tabular-nums">
              +{visible.total_additions} / -{visible.total_deletions}
            </span>
          ) : null}
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
        {path && !query.data?.too_large ? (
          <>
            <span className="text-success text-sm">+{visible.total_additions}</span>
            <span className="text-danger text-sm">-{visible.total_deletions}</span>
          </>
        ) : null}
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
                anyCollapsed === Boolean(path)
                  ? new Set()
                  : new Set(visible.files.map(fileChangeKey)),
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

      {query.data && path ? (
        <PreviewNotice
          preview={query.data}
          expanded={expanded}
          onExpand={() => setExpanded(true)}
        />
      ) : null}
      {/* Files */}
      <div className="pb-2 select-text">
        {visible.files.map((file) => (
          <LazyFileDiffView
            workspaceId={activeWorkspaceId}
            repoId={activeRepoId}
            request={request}
            alreadyLoaded={Boolean(path)}
            key={fileChangeKey(file)}
            fileDiff={file}
            mode={mode}
            workdir={workdir}
            workdirKind={workdirKind}
            collapsed={
              path
                ? collapsedFiles.has(fileChangeKey(file))
                : !collapsedFiles.has(fileChangeKey(file))
            }
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

function PreviewNotice({
  preview,
  expanded,
  onExpand,
}: {
  preview: DiffPreview;
  expanded: boolean;
  onExpand: () => void;
}): React.JSX.Element | null {
  const { t } = useTranslation();
  if (!preview.truncated) return null;
  return (
    <div className="p-3 text-xs text-text-muted bg-bg-elevated" role="status">
      {t(preview.too_large ? "diff.preview.tooLarge" : "diff.preview.truncated")}
      {!expanded ? (
        <Button size="sm" variant="secondary" onClick={onExpand}>
          {t("diff.preview.loadMore")}
        </Button>
      ) : (
        <span className="ml-2">{t("diff.preview.limit")}</span>
      )}
    </div>
  );
}

function LazyFileDiffView({
  workspaceId,
  repoId,
  request,
  alreadyLoaded,
  ...props
}: React.ComponentProps<typeof FileDiffView> & {
  workspaceId: string;
  repoId: string;
  request: DiffPreviewRequest;
  alreadyLoaded: boolean;
}): React.JSX.Element {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const selectedRequest = {
    ...request,
    path: props.fileDiff.path,
    staged: props.fileDiff.staged,
    expanded,
  };
  const query = useQuery({
    queryKey: ["diff-preview", workspaceId, repoId, selectedRequest],
    queryFn: () => getDiffPreview(workspaceId, repoId, selectedRequest),
    enabled: !alreadyLoaded && !props.collapsed,
    retry: false,
    refetchInterval: props.workdir && !props.collapsed ? 2000 : false,
  });
  const file = alreadyLoaded ? props.fileDiff : (query.data?.diff.files[0] ?? props.fileDiff);
  return (
    <>
      <FileDiffView
        {...props}
        fileDiff={file}
        collapsed={props.collapsed || (!alreadyLoaded && !query.data)}
      />
      {!props.collapsed && !alreadyLoaded ? (
        <>
          {query.isPending ? (
            <p className="p-3 text-text-muted">{t("diff.preview.loading")}</p>
          ) : query.error ? (
            <p className="p-3 text-danger">{formatAppError(query.error)}</p>
          ) : query.data ? (
            <PreviewNotice
              preview={query.data}
              expanded={expanded}
              onExpand={() => setExpanded(true)}
            />
          ) : null}
        </>
      ) : null}
    </>
  );
}
