import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { cn } from "@/lib/utils";
import { BranchIndicator } from "@/components/ui/BranchIndicator";
import { FileListItem } from "@/components/ui/FileListItem";
import { CommitMessageBox } from "@/components/ui/CommitMessageBox";
import { SyncButtons } from "@/components/ui/SyncButtons";
import {
  commit,
  fetchRemote,
  formatAppError,
  getWorkingCopy,
  pullRemote,
  pushRemote,
  stageFiles,
  unstageFiles,
  type WorkingCopy,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

export interface WorkingCopyBarProps {
  /** Current active repo id; null renders nothing */
  repoId: string | null;
  initialHeight?: number;
  className?: string;
}

const CLEAN_HEIGHT = 32;
const DIRTY_MIN_HEIGHT = 80;

/**
 * Bottom-of-screen bar for working copy state: stage / unstage / commit.
 * Polls every 2s. Never auto-commits (P1).
 */
export function WorkingCopyBar({
  repoId,
  initialHeight = 120,
  className,
}: WorkingCopyBarProps): React.JSX.Element | null {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const queryClient = useQueryClient();

  const [height, setHeight] = useState(initialHeight);
  const [message, setMessage] = useState("");
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [isDragging, setIsDragging] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["working-copy", workspaceId, repoId],
    queryFn: () => getWorkingCopy(workspaceId!),
    enabled: Boolean(workspaceId && repoId),
    refetchInterval: 2000,
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["working-copy", workspaceId, repoId] });
  };

  const stageMut = useMutation({
    mutationFn: (paths: string[]) => stageFiles(workspaceId!, paths),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });

  const unstageMut = useMutation({
    mutationFn: (paths: string[]) => unstageFiles(workspaceId!, paths),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });

  const commitMut = useMutation({
    mutationFn: (msg: string) => commit(workspaceId!, msg),
    onSuccess: () => {
      setMessage("");
      setActionError(null);
      invalidate();
      bumpHistory();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });

  const fetchMut = useMutation({
    mutationFn: () => fetchRemote(workspaceId!),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });
  const pullMut = useMutation({
    mutationFn: () => pullRemote(workspaceId!),
    onSuccess: () => {
      setActionError(null);
      invalidate();
      bumpHistory();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });
  const pushMut = useMutation({
    mutationFn: () => pushRemote(workspaceId!),
    onSuccess: () => {
      setActionError(null);
      invalidate();
    },
    onError: (e) => setActionError(formatAppError(e)),
  });

  const syncButtons = (ahead: number, behind: number) => (
    <SyncButtons
      ahead={ahead}
      behind={behind}
      onFetch={() => fetchMut.mutate()}
      onPull={() => pullMut.mutate()}
      onPush={() => pushMut.mutate()}
      inProgress={{
        fetch: fetchMut.isPending,
        pull: pullMut.isPending,
        push: pushMut.isPending,
      }}
    />
  );

  const unstagedFiles = data?.files.filter((f) => !f.staged) ?? [];
  const stagedFiles = data?.files.filter((f) => f.staged) ?? [];
  const isDirty = unstagedFiles.length > 0 || stagedFiles.length > 0;

  if (repoId === null || !workspaceId) return null;

  const displayHeight = isDirty ? Math.max(height, DIRTY_MIN_HEIGHT) : CLEAN_HEIGHT;
  const wc: WorkingCopy | null = data ?? null;

  const handleResizeMouseDown = (e: React.MouseEvent<HTMLDivElement>) => {
    e.preventDefault();
    setIsDragging(true);
    const startY = e.clientY;
    const startHeight = height;
    const onMouseMove = (moveEvent: MouseEvent) => {
      const delta = startY - moveEvent.clientY;
      setHeight(Math.max(DIRTY_MIN_HEIGHT, Math.min(280, startHeight + delta)));
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

  if (!isLoading && !isError && !isDirty && wc) {
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
          branch={wc.branch}
          sha={wc.sha}
          upstream={wc.upstream}
          ahead={wc.ahead}
          behind={wc.behind}
        />
        <div className="flex items-center gap-3">
          {syncButtons(wc.ahead, wc.behind)}
          <span>
            clean · {wc.ahead} ↑ {wc.behind} ↓
          </span>
        </div>
      </div>
    );
  }

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

      {(actionError || isError) && (
        <div className="shrink-0 px-3 py-1 text-xs text-danger border-b border-border-subtle">
          {actionError ?? (error ? formatAppError(error) : "Failed to load working copy")}
        </div>
      )}

      <div className="flex flex-1 overflow-hidden">
        <div className="flex flex-col flex-1 min-w-0 overflow-hidden border-r border-border-subtle">
          <div className="shrink-0 px-3 py-1.5 border-b border-border-subtle flex items-center justify-between gap-2">
            {wc ? (
              <>
                <BranchIndicator
                  branch={wc.branch}
                  upstream={wc.upstream}
                  sha={wc.sha}
                  ahead={wc.ahead}
                  behind={wc.behind}
                />
                {syncButtons(wc.ahead, wc.behind)}
              </>
            ) : (
              <span className="text-xs text-text-muted italic">Loading…</span>
            )}
          </div>

          <div className="flex flex-1 overflow-auto">
            <div className="flex-1 min-w-0 border-r border-border-subtle">
              <div className="px-3 py-1 text-xs font-medium text-text-muted uppercase tracking-wide">
                Unstaged ({unstagedFiles.length})
              </div>
              <div className="px-1 pb-2">
                {unstagedFiles.length === 0 ? (
                  <p className="px-2 py-1 text-xs text-text-muted italic">No unstaged changes</p>
                ) : (
                  unstagedFiles.map((f) => (
                    <FileListItem
                      key={`u-${f.path}`}
                      change={f}
                      selected={selectedFile === f.path}
                      onClick={() => setSelectedFile(f.path)}
                      onStageToggle={() => stageMut.mutate([f.path])}
                    />
                  ))
                )}
              </div>
            </div>

            <div className="flex-1 min-w-0">
              <div className="px-3 py-1 text-xs font-medium text-text-muted uppercase tracking-wide">
                Staged ({stagedFiles.length})
              </div>
              <div className="px-1 pb-2">
                {stagedFiles.length === 0 ? (
                  <p className="px-2 py-1 text-xs text-text-muted italic">No staged changes</p>
                ) : (
                  stagedFiles.map((f) => (
                    <FileListItem
                      key={`s-${f.path}`}
                      change={f}
                      selected={selectedFile === f.path}
                      onClick={() => setSelectedFile(f.path)}
                      onStageToggle={() => unstageMut.mutate([f.path])}
                    />
                  ))
                )}
              </div>
            </div>
          </div>
        </div>

        <div className="w-72 shrink-0 p-3">
          {wc ? (
            <CommitMessageBox
              value={message}
              onChange={setMessage}
              onSubmit={() => {
                if (stagedFiles.length === 0 || !message.trim()) return;
                commitMut.mutate(message);
              }}
              disabled={stagedFiles.length === 0 || commitMut.isPending}
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
