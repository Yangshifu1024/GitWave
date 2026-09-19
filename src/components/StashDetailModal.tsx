import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Surface } from "@heroui/react";

import type { FileDiff, StashEntry } from "@/lib/api";
import { formatAppError, getStashDiff } from "@/lib/api";
import { fileDiffKind, stashTitle } from "@/lib/stashActions";
import { DiffViewer } from "@/components/DiffViewer";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Modal } from "@/components/ui/Modal";
import { StatusIcon } from "@/components/ui/StatusIcon";
import { Tooltip } from "@/components/ui/Tooltip";
import { cn } from "@/lib/utils";

export interface StashDetailModalProps {
  workspaceId: string;
  entry: StashEntry;
  /** True while the panel runs an apply / pop / drop — locks every action. */
  busy: boolean;
  onClose: () => void;
  onApply: () => void;
  onPop: () => void;
  onDrop: () => void;
}

/**
 * Stash inspect + act modal: file list on the left, the selected file's diff on
 * the right — the WorkingCopyModal layout applied to one stash entry. The
 * selected path never leaves this modal.
 */
export function StashDetailModal({
  workspaceId,
  entry,
  busy,
  onClose,
  onApply,
  onPop,
  onDrop,
}: StashDetailModalProps): React.JSX.Element {
  const { t } = useTranslation();
  const [pickedPath, setPickedPath] = useState<string | null>(null);

  // The file list is fetched here while DiffViewer fetches its own diff: the
  // list needs every file, DiffViewer only the selected one.
  const {
    data: summary,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["stash-diff", workspaceId, entry.oid],
    queryFn: () => getStashDiff(workspaceId, entry.oid),
  });

  // No effect resetting the selection: the panel keys this modal by the entry
  // oid, so another stash mounts a fresh instance.
  const files = summary?.files ?? [];
  // Derived rather than stored: a refresh that drops the picked file falls back
  // to the first entry instead of leaving the right pane on a stale path.
  const selectedPath =
    pickedPath && files.some((f) => f.path === pickedPath) ? pickedPath : (files[0]?.path ?? null);

  return (
    <Modal
      open
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={stashTitle(entry, t("changes.stash.noMessage"))}
      size="xl"
      footer={
        <div className="flex justify-end gap-2">
          <Tooltip content={t("changes.stash.dropTooltip")}>
            <Button variant="danger" size="sm" disabled={busy} onClick={onDrop}>
              {t("changes.stash.drop")}
            </Button>
          </Tooltip>
          <Tooltip content={t("changes.stash.popTooltip")}>
            <Button variant="secondary" size="sm" disabled={busy} onClick={onPop}>
              {t("changes.stash.pop")}
            </Button>
          </Tooltip>
          <Tooltip content={t("changes.stash.applyTooltip")}>
            <Button variant="primary" size="sm" disabled={busy} onClick={onApply}>
              {t("changes.stash.apply")}
            </Button>
          </Tooltip>
        </div>
      }
    >
      <div className="h-[62vh] min-h-0 grid grid-cols-2 border border-border-subtle rounded-md overflow-hidden bg-bg-primary">
        <div className="min-h-0 flex flex-col border-r border-border-subtle">
          <div className="shrink-0 border-b border-border-subtle px-3 py-2 text-xs text-text-muted tabular-nums">
            {summary
              ? t("changes.stash.diffSummary", {
                  files: summary.files.length,
                  additions: summary.total_additions,
                  deletions: summary.total_deletions,
                })
              : null}
          </div>
          <div className="flex-1 min-h-0 overflow-auto p-1" role="listbox">
            {isLoading ? (
              <p className="px-2 py-1 text-xs italic text-text-muted">
                {t("changes.stash.detailLoading")}
              </p>
            ) : error ? (
              <p className="px-2 py-1 text-xs text-danger">{formatAppError(error)}</p>
            ) : files.length === 0 ? (
              <p className="px-2 py-1 text-xs italic text-text-muted">
                {t("changes.stash.detailEmptyFiles")}
              </p>
            ) : (
              files.map((f) => (
                <StashFileRow
                  key={f.path}
                  file={f}
                  selected={f.path === selectedPath}
                  onClick={() => setPickedPath(f.path)}
                />
              ))
            )}
          </div>
        </div>

        <div className="min-h-0 overflow-hidden">
          {selectedPath ? (
            <DiffViewer stashOid={entry.oid} path={selectedPath} hideMaximize />
          ) : (
            <EmptyState
              title={t("changes.stash.detailNoFileSelected")}
              description={t("changes.stash.detailNoFileSelectedDescription")}
              className="h-full"
            />
          )}
        </div>
      </div>
    </Modal>
  );
}

/**
 * Stash file row. FileListItem is not reused verbatim: its StatusIcon is a
 * stage/unstage button, and a stash entry has nothing to stage.
 */
function StashFileRow({
  file,
  selected,
  onClick,
}: {
  file: FileDiff;
  selected: boolean;
  onClick: () => void;
}): React.JSX.Element {
  const { t } = useTranslation();

  return (
    <Surface
      variant="transparent"
      role="option"
      aria-selected={selected}
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === "Enter") onClick();
      }}
      className={cn(
        "flex items-center gap-2 rounded-md px-2 py-1.5 text-xs",
        "cursor-pointer shadow-none transition-colors duration-fast",
        selected ? "bg-accent/10" : "hover:bg-bg-secondary",
      )}
    >
      <StatusIcon kind={fileDiffKind(file)} />
      <span className="min-w-0 flex-1 truncate text-text-primary" title={file.path}>
        {file.path}
      </span>
      {file.untracked === true ? (
        <span className="shrink-0 rounded-sm bg-accent/15 px-1 py-0.5 text-[10px] font-medium text-accent">
          {t("changes.stash.untracked")}
        </span>
      ) : null}
      <span className="shrink-0 flex items-center gap-1.5 font-mono">
        <span className="text-status-active">+{file.additions}</span>
        <span className="text-danger">-{file.deletions}</span>
      </span>
    </Surface>
  );
}
