// Sidebar reflog browser — newest-first HEAD movement history. Clicking an
// entry selects and centers that commit in the history graph. Read-only:
// recovery actions belong to v0.3's AI-assisted recovery.

import { useCallback, useEffect, useState } from "react";
import { History } from "lucide-react";
import { formatAppError, listReflog, type ReflogEntry } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";

export function ReflogPanel({ onSelect }: { onSelect?: (sha: string) => void }): React.JSX.Element {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);

  const [entries, setEntries] = useState<ReflogEntry[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!workspaceId || !repoId) {
      setEntries([]);
      return;
    }
    setBusy(true);
    try {
      setEntries(await listReflog(workspaceId));
      setError(null);
    } catch (e) {
      setError(formatAppError(e));
    } finally {
      setBusy(false);
    }
  }, [workspaceId, repoId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const select = (sha: string): void => {
    if (busy) return;
    onSelect?.(sha);
  };

  if (!workspaceId || !repoId) return <></>;

  return (
    <div className="flex flex-col gap-1 px-1">
      {error ? <p className="px-2 text-xs text-danger">{error}</p> : null}
      {entries.length === 0 ? (
        <p className="px-2 py-1 text-xs text-text-muted italic">No reflog entries</p>
      ) : (
        entries.slice(0, 50).map((entry, i) => (
          <Button
            key={`${entry.new_sha}-${i}`}
            variant="ghost"
            size="sm"
            className="flex w-full items-center gap-2 rounded-md px-2 py-1 text-left"
            onClick={() => select(entry.new_sha)}
            title={`${entry.message ?? "(no message)"} · ${entry.new_sha}`}
          >
            <History size={12} className="shrink-0 text-text-muted" />
            <span className="min-w-0 flex-1">
              <span className="block truncate text-xs text-text-secondary">
                {entry.message ?? "(no message)"}
              </span>
              <span className="block truncate text-[11px] text-text-muted">
                <span className="font-mono">{entry.new_sha.slice(0, 7)}</span>
                {entry.time > 0
                  ? ` · ${new Date(entry.time * 1000).toLocaleString(undefined, {
                      month: "short",
                      day: "numeric",
                      hour: "2-digit",
                      minute: "2-digit",
                    })}`
                  : ""}
              </span>
            </span>
          </Button>
        ))
      )}
    </div>
  );
}
