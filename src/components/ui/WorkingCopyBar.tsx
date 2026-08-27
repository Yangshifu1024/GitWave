import { cn } from "@/lib/utils";
import { BranchIndicator } from "@/components/ui/BranchIndicator";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { ChangesPanel } from "@/components/ChangesPanel";
import { formatAppError, type WorkingCopy } from "@/lib/api";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";

export interface WorkingCopyBarProps {
  /** Current active repo id; null renders nothing */
  repoId: string | null;
  selectedPath?: string | null;
  selectedStaged?: boolean | null;
  onSelectFile?: (path: string, staged: boolean) => void;
  className?: string;
}

const CLEAN_HEIGHT = 32;
const DIRTY_HEIGHT = 220;

/**
 * Bottom-of-screen bar: branch status when clean; file lists + commit when dirty.
 * Sync lives in the toolbar. Never auto-commits (P1).
 */
export function WorkingCopyBar({
  repoId,
  selectedPath = null,
  selectedStaged = null,
  onSelectFile,
  className,
}: WorkingCopyBarProps): React.JSX.Element | null {
  const wc = useWorkingCopy();

  if (repoId === null || !wc.workspaceId) return null;

  const snapshot: WorkingCopy | null = wc.data ?? null;

  const wcAlert =
    wc.actionError ??
    (wc.isError ? (wc.error ? formatAppError(wc.error) : "Failed to load working copy") : null);

  if (!wc.isLoading && !wc.isError && !wc.isDirty && snapshot) {
    return (
      <>
        <div
          className={cn(
            "flex items-center justify-between px-3.5",
            "bg-bg-secondary border-t border-border-subtle",
            "text-xs text-text-muted",
            className,
          )}
          style={{ height: CLEAN_HEIGHT }}
        >
          <BranchIndicator
            branch={snapshot.branch}
            sha={snapshot.branch === "(detached)" ? snapshot.sha : null}
            upstream={snapshot.upstream}
            ahead={snapshot.ahead}
            behind={snapshot.behind}
            className="text-xs"
          />
          <span>
            clean · {snapshot.ahead} ↑ {snapshot.behind} ↓
          </span>
        </div>
        <ErrorAlert message={wcAlert} onDismiss={() => wc.setActionError(null)} />
      </>
    );
  }

  return (
    <>
      <div
        className={cn(
          "flex flex-col shrink-0 bg-bg-secondary border-t border-border-subtle",
          className,
        )}
        style={{ height: DIRTY_HEIGHT }}
      >
        <div className="flex items-center justify-between gap-2 px-3 h-8 shrink-0 border-b border-border-subtle">
          {snapshot ? (
            <>
              <BranchIndicator
                branch={snapshot.branch}
                upstream={snapshot.upstream}
                sha={snapshot.branch === "(detached)" ? snapshot.sha : null}
                ahead={snapshot.ahead}
                behind={snapshot.behind}
                className="text-xs"
              />
              <span className="text-xs text-text-muted">
                {wc.unstagedFiles.length} unstaged · {wc.stagedFiles.length} staged
              </span>
            </>
          ) : (
            <span className="text-xs text-text-muted italic">Loading…</span>
          )}
        </div>
        <div className="flex-1 min-h-0 overflow-hidden">
          <ChangesPanel
            selectedPath={selectedPath}
            selectedStaged={selectedStaged}
            onSelectFile={onSelectFile ?? (() => undefined)}
            layout="bar"
          />
        </div>
      </div>
      <ErrorAlert message={wcAlert} onDismiss={() => wc.setActionError(null)} />
    </>
  );
}
