import { type useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { cn } from "@/lib/utils";
import { BranchIndicator } from "@/components/ui/BranchIndicator";
import { FileListItem, type FileChange } from "@/components/ui/FileListItem";
import { CommitMessageBox } from "@/components/ui/CommitMessageBox";

// ─── Stub types (Sprint 4: real IPC integration) ───────────────────────────

/** Placeholder for TanStack Query result — replace with real query in Sprint 4 */
type StubWorkingCopy = {
  branch: string;
  upstream: string | null;
  sha: string;
  ahead: number;
  behind: number;
  files: FileChange[];
};

type StubQueryResult = ReturnType<typeof useQuery<StubWorkingCopy>>;

// ─── Props ──────────────────────────────────────────────────────────────────

export interface WorkingCopyBarProps {
  /** Current active repo id; null renders nothing */
  repoId: string | null;
  initialHeight?: number;
  // Sprint 4: replace `stubQuery` with `repoId`-based query
  stubQuery?: StubQueryResult;
  className?: string;
}

// ─── Component ───────────────────────────────────────────────────────────────

const CLEAN_HEIGHT = 32;
const DIRTY_MIN_HEIGHT = 80;

/**
 * Bottom-of-screen composite bar for working copy state.
 *
 * **Sprint 3/4 note**: This component uses a stub query for now.
 * Real IPC (`cmd_get_working_copy`) + TanStack Query polling (2s) comes in Sprint 4.
 *
 * Layout (dirty state):
 * ┌────────────────────────────────────────────────────┐
 * │ [BranchIndicator]                    [Commit msg] │
 * │ [Unstaged list] [Staged list]                      │
 * └────────────────────────────────────────────────────┘
 */
export function WorkingCopyBar({
  repoId,
  initialHeight = 120,
  stubQuery,
  className,
}: WorkingCopyBarProps): React.JSX.Element | null {
  const [height, setHeight] = useState(initialHeight);
  const [message, setMessage] = useState("");
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [isDragging, setIsDragging] = useState(false);

  // Stub data when no real query is provided
  const data = stubQuery?.data ?? null;
  const isLoading = stubQuery?.isLoading ?? false;
  const isError = stubQuery?.isError ?? false;

  const unstagedFiles = data?.files.filter((f) => !f.staged) ?? [];
  const stagedFiles = data?.files.filter((f) => f.staged) ?? [];
  const isDirty = unstagedFiles.length > 0 || stagedFiles.length > 0;

  if (repoId === null) return null;

  const displayHeight = isDirty ? Math.max(height, DIRTY_MIN_HEIGHT) : CLEAN_HEIGHT;

  // ─── Resize handle drag ──────────────────────────────────────────────────

  const handleResizeMouseDown = (e: React.MouseEvent<HTMLDivElement>) => {
    e.preventDefault();
    setIsDragging(true);

    const startY = e.clientY;
    const startHeight = height;

    const onMouseMove = (moveEvent: MouseEvent) => {
      const delta = startY - moveEvent.clientY;
      const newHeight = Math.max(DIRTY_MIN_HEIGHT, Math.min(280, startHeight + delta));
      setHeight(newHeight);
    };

    const onMouseUp = () => {
      setIsDragging(false);
      document.removeEventListener("mousemove", onMouseMove);
      document.removeEventListener("mouseup", onMouseUp);
      document.body.style.userSelect = "";
    };

    document.addEventListener("mousemove", onMouseMove);
    document.addEventListener("mouseup", onMouseUp);
    document.body.style.userSelect = "none";
  };

  // ─── Clean state ──────────────────────────────────────────────────────────

  if (!isLoading && !isError && !isDirty && data) {
    return (
      <div
        className={cn(
          "flex items-center justify-between px-4",
          "bg-bg-secondary border-t border-border-subtle",
          "text-xs text-text-muted",
          className,
        )}
        style={{ height: CLEAN_HEIGHT }}
      >
        <BranchIndicator
          branch={data.branch}
          sha={data.sha}
          upstream={data.upstream}
          ahead={data.ahead}
          behind={data.behind}
        />
        <span>clean · {data.ahead} ↑ {data.behind} ↓</span>
      </div>
    );
  }

  // ─── Dirty / expanded state ────────────────────────────────────────────────

  return (
    <div
      className={cn(
        "flex flex-col bg-bg-secondary border-t border-border-subtle",
        "transition-colors duration-200",
        isDragging && "select-none",
        className,
      )}
      style={{ height: displayHeight }}
    >
      {/* Resize handle */}
      <div
        onMouseDown={handleResizeMouseDown}
        className={cn(
          "shrink-0 h-1 cursor-row-resize bg-border-subtle",
          "hover:bg-accent transition-colors",
          isDragging && "bg-accent",
        )}
        title="Drag to resize"
        role="separator"
        aria-orientation="horizontal"
        tabIndex={0}
      />

      <div className="flex flex-1 overflow-hidden">
        {/* Left: Branch indicator + file lists */}
        <div className="flex flex-col flex-1 min-w-0 overflow-hidden border-r border-border-subtle">
          {/* Branch row */}
          <div className="shrink-0 px-3 py-1.5 border-b border-border-subtle">
            {data ? (
              <BranchIndicator
                branch={data.branch}
                upstream={data.upstream}
                sha={data.sha}
                ahead={data.ahead}
                behind={data.behind}
              />
            ) : (
              <span className="text-xs text-text-muted italic">Loading…</span>
            )}
          </div>

          {/* File lists */}
          <div className="flex flex-1 overflow-auto">
            {/* Unstaged */}
            <div className="flex-1 min-w-0 border-r border-border-subtle">
              <div className="px-3 py-1 text-xs font-medium text-text-muted uppercase tracking-wide">
                Unstaged ({unstagedFiles.length})
              </div>
              <div className="px-1 pb-2">
                {unstagedFiles.length === 0 ? (
                  <p className="px-2 py-1 text-xs text-text-muted italic">
                    Nounstaged changes
                  </p>
                ) : (
                  unstagedFiles.map((f) => (
                    <FileListItem
                      key={f.path}
                      change={f}
                      selected={selectedFile === f.path}
                      onClick={() => setSelectedFile(f.path)}
                      onStageToggle={() => {
                        // Sprint 4: call stage/unstage IPC
                      }}
                    />
                  ))
                )}
              </div>
            </div>

            {/* Staged */}
            <div className="flex-1 min-w-0">
              <div className="px-3 py-1 text-xs font-medium text-text-muted uppercase tracking-wide">
                Staged ({stagedFiles.length})
              </div>
              <div className="px-1 pb-2">
                {stagedFiles.length === 0 ? (
                  <p className="px-2 py-1 text-xs text-text-muted italic">
                    No staged changes
                  </p>
                ) : (
                  stagedFiles.map((f) => (
                    <FileListItem
                      key={f.path}
                      change={f}
                      selected={selectedFile === f.path}
                      onClick={() => setSelectedFile(f.path)}
                      onStageToggle={() => {
                        // Sprint 4: call stage/unstage IPC
                      }}
                    />
                  ))
                )}
              </div>
            </div>
          </div>
        </div>

        {/* Right: Commit message box */}
        <div className="w-72 shrink-0 p-3">
          {data ? (
            <CommitMessageBox
              value={message}
              onChange={setMessage}
              onSubmit={() => {
                // Sprint 4: call commit IPC
              }}
              disabled={stagedFiles.length === 0}
            />
          ) : (
            <div className="flex items-center justify-center h-full">
              <span className="text-xs text-text-muted italic">
                {isLoading ? "Loading…" : "No repo"}
              </span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
