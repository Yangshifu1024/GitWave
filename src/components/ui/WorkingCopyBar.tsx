import { useState } from "react";
import { cn } from "@/lib/utils";
import { BranchIndicator } from "@/components/ui/BranchIndicator";
import { CommitMessageBox } from "@/components/ui/CommitMessageBox";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { SyncButtons } from "@/components/ui/SyncButtons";
import { formatAppError, generateCommitMessage, type WorkingCopy } from "@/lib/api";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";

export interface WorkingCopyBarProps {
  /** Current active repo id; null renders nothing */
  repoId: string | null;
  className?: string;
}

const CLEAN_HEIGHT = 32;

/**
 * Bottom-of-screen bar: branch / sync + commit message. File lists live in the Changes tab.
 * Polls every 2s. Never auto-commits (P1).
 */
export function WorkingCopyBar({ repoId, className }: WorkingCopyBarProps): React.JSX.Element | null {
  const wc = useWorkingCopy();
  const [message, setMessage] = useState("");
  const [aiBusy, setAiBusy] = useState(false);

  const handleAiGenerate = () => {
    if (!wc.workspaceId || aiBusy) return;
    setAiBusy(true);
    wc.setActionError(null);
    generateCommitMessage(wc.workspaceId)
      .then((msg) => setMessage(msg))
      .catch((e) => wc.setActionError(formatAppError(e)))
      .finally(() => setAiBusy(false));
  };

  if (repoId === null || !wc.workspaceId) return null;

  const snapshot: WorkingCopy | null = wc.data ?? null;

  const syncButtons = (ahead: number, behind: number) => (
    <SyncButtons
      ahead={ahead}
      behind={behind}
      onFetch={wc.fetch}
      onPull={wc.pull}
      onPush={wc.push}
      inProgress={wc.syncPending}
    />
  );

  const wcAlert =
    wc.actionError ?? (wc.isError ? (wc.error ? formatAppError(wc.error) : "Failed to load working copy") : null);

  if (!wc.isLoading && !wc.isError && !wc.isDirty && snapshot) {
    return (
      <>
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
          branch={snapshot.branch}
          sha={snapshot.sha}
          upstream={snapshot.upstream}
          ahead={snapshot.ahead}
          behind={snapshot.behind}
        />
        <div className="flex items-center gap-3">
          {syncButtons(snapshot.ahead, snapshot.behind)}
          <span>
            clean · {snapshot.ahead} ↑ {snapshot.behind} ↓
          </span>
        </div>
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
    >
      <div className="flex items-center justify-between gap-2 px-3 py-1.5">
        {snapshot ? (
          <>
            <BranchIndicator
              branch={snapshot.branch}
              upstream={snapshot.upstream}
              sha={snapshot.sha}
              ahead={snapshot.ahead}
              behind={snapshot.behind}
            />
            {syncButtons(snapshot.ahead, snapshot.behind)}
          </>
        ) : (
          <span className="text-xs text-text-muted italic">Loading…</span>
        )}
      </div>

      <div className="px-3 pb-3">
        {snapshot ? (
          <CommitMessageBox
            value={message}
            onChange={setMessage}
            onSubmit={() => {
              if (wc.stagedFiles.length === 0 || !message.trim()) return;
              wc.commitMessage(message, { onSuccess: () => setMessage("") });
            }}
            onAiGenerate={handleAiGenerate}
            disabled={wc.stagedFiles.length === 0 || wc.commitPending || aiBusy}
          />
        ) : (
          <span className="text-xs text-text-muted italic">
            {wc.isLoading ? "Loading…" : "No repo"}
          </span>
        )}
      </div>
    </div>
    <ErrorAlert message={wcAlert} onDismiss={() => wc.setActionError(null)} />
    </>
  );
}
