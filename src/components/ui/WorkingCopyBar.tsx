import { Minimize2, Maximize2, ChevronDown, ChevronUp } from "lucide-react";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import { Tooltip } from "@/components/ui/Tooltip";
import { BranchIndicator } from "@/components/ui/BranchIndicator";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { ChangesPanel } from "@/components/ChangesPanel";
import { formatAppError, type WorkingCopy } from "@/lib/api";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { useLayoutStore } from "@/stores/layoutStore";

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
 * Bottom-of-screen bar: branch status when clean; file lists + commit when
 * dirty. Collapsible to a single row and stretchable to half the window via
 * the header controls. Sync lives in sidebar REPOS / BRANCHES sections.
 * Never auto-commits (P1).
 */
export function WorkingCopyBar({
  repoId,
  selectedPath = null,
  selectedStaged = null,
  onSelectFile,
  className,
}: WorkingCopyBarProps): React.JSX.Element | null {
  const wc = useWorkingCopy();
  const wcBarCollapsed = useLayoutStore((s) => s.wcBarCollapsed);
  const toggleWcBarCollapsed = useLayoutStore((s) => s.toggleWcBarCollapsed);
  const wcBarMaximized = useLayoutStore((s) => s.wcBarMaximized);
  const toggleWcBarMaximized = useLayoutStore((s) => s.toggleWcBarMaximized);

  if (repoId === null || !wc.workspaceId) return null;

  const snapshot: WorkingCopy | null = wc.data ?? null;

  const wcAlert =
    wc.actionError ??
    (wc.isError ? (wc.error ? formatAppError(wc.error) : "Failed to load working copy") : null);

  const branchInfo =
    snapshot != null ? (
      <BranchIndicator
        branch={snapshot.branch}
        sha={snapshot.branch === "(detached)" ? snapshot.sha : null}
        upstream={snapshot.upstream}
        ahead={snapshot.ahead}
        behind={snapshot.behind}
        className="text-xs"
      />
    ) : (
      <span className="text-xs text-text-muted italic">Loading…</span>
    );

  if (!wc.isLoading && !wc.isError && !wc.isDirty && snapshot) {
    return (
      <>
        <div
          className={cn(
            "flex items-center justify-between px-3.5",
            "bg-bg-primary border-t border-border-subtle",
            "shadow-[0_-4px_16px_color-mix(in_srgb,var(--color-text-primary)_5%,transparent)]",
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

  // Collapsed: keep branch status visible in a single row, hide the commit box.
  if (wcBarCollapsed) {
    return (
      <>
        <div
          className={cn(
            "flex items-center justify-between px-3.5 gap-2",
            "bg-bg-primary border-t border-border-subtle",
            "shadow-[0_-4px_16px_color-mix(in_srgb,var(--color-text-primary)_5%,transparent)]",
            "text-xs text-text-muted",
            className,
          )}
          style={{ height: CLEAN_HEIGHT }}
        >
          {branchInfo}
          <span className="flex items-center gap-1">
            {snapshot ? (
              <span>
                {wc.unstagedFiles.length} unstaged · {wc.stagedFiles.length} staged
              </span>
            ) : null}
            <Tooltip content="Show commit box">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="p-1.5 text-text-muted hover:text-accent"
                aria-pressed={true}
                aria-label="Show commit box"
                onClick={toggleWcBarCollapsed}
              >
                <ChevronUp size={14} />
              </Button>
            </Tooltip>
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
          "flex flex-col shrink-0 bg-bg-primary border-t border-border-subtle",
          "shadow-[0_-4px_16px_color-mix(in_srgb,var(--color-text-primary)_5%,transparent)]",
          "transition-[height] duration-200 ease-out",
          className,
        )}
        style={{ height: wcBarMaximized ? "50vh" : DIRTY_HEIGHT }}
      >
        <div className="flex items-center gap-2 px-3 h-8 shrink-0 border-b border-border-subtle">
          {branchInfo}
          {snapshot ? (
            <span className="text-xs text-text-muted">
              {wc.unstagedFiles.length} unstaged · {wc.stagedFiles.length} staged
            </span>
          ) : null}
          <span className="ml-auto flex items-center gap-0.5">
            <Tooltip content="Hide commit box">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="p-1.5 text-text-muted hover:text-accent"
                aria-label="Hide commit box"
                onClick={toggleWcBarCollapsed}
              >
                <ChevronDown size={14} />
              </Button>
            </Tooltip>
            <Tooltip
              content={wcBarMaximized ? "Restore commit box height" : "Maximize to half window"}
            >
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="p-1.5 text-text-muted hover:text-accent"
                aria-pressed={wcBarMaximized}
                aria-label={
                  wcBarMaximized ? "Restore commit box height" : "Maximize to half window"
                }
                onClick={toggleWcBarMaximized}
              >
                {wcBarMaximized ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
              </Button>
            </Tooltip>
          </span>
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
