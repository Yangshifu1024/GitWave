import { useEffect, useState } from "react";

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
  const wc = useWorkingCopy();
  const [selected, setSelected] = useState<{ path: string; staged: boolean } | null>(null);

  // Repo switches reset the in-modal selection.
  useEffect(() => {
    setSelected(null);
  }, [wc.repoId]);

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

  return (
    <Modal open={open} onOpenChange={onOpenChange} title="Local Changes" size="xl">
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
            {wc.unstagedFiles.length} unstaged · {wc.stagedFiles.length} staged
          </span>
        </div>
      ) : null}

      <div className="h-[62vh] min-h-0 grid grid-cols-2 border border-border-subtle rounded-md overflow-hidden bg-bg-primary">
        <div className="min-h-0 border-r border-border-subtle">
          <ChangesPanel
            selectedPath={selected?.path ?? null}
            selectedStaged={selected?.staged ?? null}
            onSelectFile={(path, staged) => setSelected({ path, staged })}
            layout="modal"
          />
        </div>
        <div className="min-h-0 overflow-hidden">
          {selected ? (
            <DiffViewer workdir path={selected.path} staged={selected.staged} hideMaximize />
          ) : (
            <EmptyState
              title="No file selected"
              description="Click a file on the left to see its diff."
              className="h-full"
            />
          )}
        </div>
      </div>
    </Modal>
  );
}
