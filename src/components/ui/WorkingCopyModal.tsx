import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/Button";
import { BranchIndicator } from "@/components/ui/BranchIndicator";
import { EmptyState } from "@/components/ui/EmptyState";
import { Modal } from "@/components/ui/Modal";
import { ChangesPanel } from "@/components/ChangesPanel";
import { DiffViewer } from "@/components/DiffViewer";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";

export interface WorkingCopyModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Working-copy modal opened from the toolbar's Local Changes button: file
 * lists + commit box on the left, the clicked file's diff on the right.
 * Selection is local to the modal — it never drives the inspector pane.
 */
export function WorkingCopyModal({
  open,
  onOpenChange,
}: WorkingCopyModalProps): React.JSX.Element | null {
  const { t } = useTranslation();
  const wc = useWorkingCopy();
  const [selected, setSelected] = useState<{ path: string; staged: boolean } | null>(null);

  const containerRef = useRef<HTMLDivElement>(null);
  const [ratio, setRatio] = useState(() => {
    try {
      const value = Number(localStorage.getItem("gitwave.workingCopy.ratio") ?? 40);
      return Number.isFinite(value) ? Math.max(25, Math.min(65, value)) : 40;
    } catch {
      return 40;
    }
  });
  const [expanded, setExpanded] = useState(false);
  const resize = (value: number) => {
    const next = Math.max(25, Math.min(65, value));
    setRatio(next);
    try {
      localStorage.setItem("gitwave.workingCopy.ratio", String(next));
    } catch {
      /* Session still works without storage. */
    }
  };

  // Repo switches reset the in-modal selection.
  useEffect(() => {
    setSelected(null);
    setExpanded(false);
  }, [wc.workspaceId, wc.repoId]);

  // Keep the selection from pointing at a file that just got committed away.
  useEffect(() => {
    if (
      selected &&
      wc.data &&
      !wc.data.files.some((f) => f.path === selected.path && f.staged === selected.staged)
    ) {
      setSelected(null);
    }
  }, [wc.data, selected]);

  if (!open || !wc.repoId) return null;

  const snapshot = wc.data ?? null;
  // The image diff needs the selected change's kind: a deleted file's new
  // side has no OID and no worktree file to read.
  const selectedKind = selected
    ? wc.data?.files.find((f) => f.path === selected.path && f.staged === selected.staged)?.kind
    : undefined;

  return (
    <Modal open={open} onOpenChange={onOpenChange} title={t("changes.panel.title")} size="xl">
      {snapshot ? (
        <div className="flex items-center gap-2 text-xs text-text-muted">
          <BranchIndicator
            branch={snapshot.branch}
            sha={snapshot.branch === "(detached)" ? snapshot.sha : null}
            upstream={snapshot.upstream}
            ahead={snapshot.ahead}
            behind={snapshot.behind}
          />
          <span>
            {t("changes.panel.statusSummary", {
              unstaged: wc.unstagedFiles.length,
              staged: wc.stagedFiles.length,
            })}
          </span>
        </div>
      ) : null}

      <div className="flex justify-end">
        <Button
          variant="ghost"
          size="sm"
          disabled={!selected && !expanded}
          aria-pressed={expanded}
          onClick={() => setExpanded((value) => !value)}
        >
          {t(expanded ? "changes.panel.restoreLayout" : "changes.panel.expandDiff")}
        </Button>
      </div>
      <div
        ref={containerRef}
        style={{
          gridTemplateColumns: expanded ? "minmax(0, 1fr)" : ratio + "% 6px minmax(0, 1fr)",
        }}
        className="h-[62vh] min-h-0 grid border border-border-subtle rounded-md overflow-hidden bg-bg-primary"
      >
        <div className="min-h-0 min-w-0" hidden={expanded}>
          <ChangesPanel
            selectedPath={selected?.path ?? null}
            selectedStaged={selected?.staged ?? null}
            onSelectFile={(path, staged) => setSelected({ path, staged })}
            layout="modal"
            onCommitted={() => onOpenChange(false)}
          />
        </div>
        {!expanded ? (
          <div
            role="separator"
            aria-label={t("changes.panel.resize")}
            aria-orientation="vertical"
            aria-valuemin={25}
            aria-valuemax={65}
            aria-valuenow={Math.round(ratio)}
            tabIndex={0}
            className="cursor-col-resize touch-none bg-border-subtle hover:bg-accent focus-visible:bg-accent outline-none"
            onKeyDown={(event) => {
              if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
              event.preventDefault();
              resize(ratio + (event.key === "ArrowLeft" ? -2 : 2));
            }}
            onPointerDown={(event) => {
              event.preventDefault();
              event.currentTarget.setPointerCapture(event.pointerId);
            }}
            onPointerMove={(event) => {
              if (!event.currentTarget.hasPointerCapture(event.pointerId)) return;
              const rect = containerRef.current?.getBoundingClientRect();
              if (rect) resize(((event.clientX - rect.left) / rect.width) * 100);
            }}
            onPointerUp={(event) => {
              if (event.currentTarget.hasPointerCapture(event.pointerId))
                event.currentTarget.releasePointerCapture(event.pointerId);
            }}
          />
        ) : null}
        <div className="min-h-0 min-w-0 overflow-hidden">
          {selected ? (
            <DiffViewer
              workdir
              path={selected.path}
              staged={selected.staged}
              workdirKind={selectedKind}
              hideMaximize
            />
          ) : (
            <EmptyState
              title={t("changes.panel.noFileSelected")}
              description={t("changes.panel.noFileSelectedDescription")}
              className="h-full"
            />
          )}
        </div>
      </div>
    </Modal>
  );
}
