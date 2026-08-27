import { useCallback, useEffect, useState } from "react";
import type { DiffSummary, StashEntry } from "@/lib/api";
import {
  applyStash,
  dropStash,
  formatAppError,
  getStashDiff,
  listStashes,
  popStash,
  saveStash,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ListItem } from "@/components/ui/ListItem";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { cn } from "@/lib/utils";
import { Archive, Eye, Play, Trash2, Upload } from "lucide-react";

export function StashPanel({ compact = false }: { compact?: boolean }): React.JSX.Element {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);

  const [entries, setEntries] = useState<StashEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState("");
  const [selectedOid, setSelectedOid] = useState<string | null>(null);
  const [diff, setDiff] = useState<DiffSummary | null>(null);

  const refresh = useCallback(async () => {
    if (!workspaceId) return;
    const list = await listStashes(workspaceId);
    setEntries(list);
  }, [workspaceId]);

  useEffect(() => {
    if (!workspaceId || !repoId) {
      setEntries([]);
      setDiff(null);
      return;
    }
    setLoading(true);
    setError(null);
    refresh()
      .catch((e) => setError(formatAppError(e)))
      .finally(() => setLoading(false));
  }, [workspaceId, repoId, refresh]);

  const run = async (fn: () => Promise<void>) => {
    if (!workspaceId || busy) return;
    setBusy(true);
    setError(null);
    try {
      await fn();
      await refresh();
      bumpHistory();
    } catch (e) {
      setError(formatAppError(e));
    } finally {
      setBusy(false);
    }
  };

  const handleSave = () =>
    void run(async () => {
      await saveStash(workspaceId!, message.trim() || undefined);
      setMessage("");
    });

  const handleDiff = async (oid: string) => {
    if (!workspaceId) return;
    setSelectedOid(oid);
    setError(null);
    try {
      setDiff(await getStashDiff(workspaceId, oid));
    } catch (e) {
      setDiff(null);
      setError(formatAppError(e));
    }
  };

  if (!workspaceId || !repoId) {
    return (
      <p className={cn("text-text-muted", compact ? "px-3 py-1.5 text-xs" : "flex items-center justify-center h-full text-sm")}>
        Select a repository to manage stashes
      </p>
    );
  }

  if (loading) {
    return (
      <p className={cn("text-text-muted italic", compact ? "px-3 py-1.5 text-xs" : "flex items-center justify-center h-full text-sm")}>
        Loading stashes…
      </p>
    );
  }

  return (
    <div className={cn("min-h-0 flex flex-col", !compact && "h-full overflow-hidden")}>
      <div
        className={cn(
          "shrink-0 flex items-center gap-1.5",
          compact ? "px-2 py-1" : "px-3 py-2 border-b border-border-subtle",
        )}
      >
        <div className="flex-1 min-w-0">
          <Input
            placeholder={compact ? "Message (optional)" : "Stash message (optional)"}
            value={message}
            onChange={setMessage}
            disabled={busy}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleSave();
            }}
          />
        </div>
        <Button variant="primary" size="sm" disabled={busy} onClick={handleSave}>
          <Archive size={14} />
          {compact ? null : "Stash"}
        </Button>
      </div>

      {error ? <ErrorAlert message={error} onDismiss={() => setError(null)} /> : null}

      <div
        className={cn(
          "min-h-0 overflow-auto",
          compact ? "max-h-52" : "flex flex-1 min-h-0 overflow-hidden",
        )}
      >
        <div className={cn("min-w-0", !compact && "flex-1 overflow-auto border-r border-border-subtle")}>
          {entries.length === 0 ? (
            <p className={cn("text-text-muted", compact ? "px-3 py-1.5 text-xs" : "flex items-center justify-center h-full text-sm px-4 text-center")}>
              {compact ? "No stashes" : "No stashes yet. Save uncommitted work with Stash."}
            </p>
          ) : (
            entries.map((e) => (
              <ListItem
                key={`${e.index}-${e.oid}`}
                selected={selectedOid === e.oid}
                onClick={() => {
                  void handleDiff(e.oid);
                }}
                leading={<Archive size={14} className="text-accent shrink-0" />}
                trailing={
                  <div className="flex items-center gap-0.5">
                    {!compact ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="p-1"
                        disabled={busy}
                        title="View diff"
                        onClick={(ev) => {
                          ev.stopPropagation();
                          void handleDiff(e.oid);
                        }}
                      >
                        <Eye size={12} />
                      </Button>
                    ) : null}
                    <Button
                      variant="ghost"
                      size="sm"
                      className="p-1"
                      disabled={busy}
                      title="Apply"
                      onClick={(ev) => {
                        ev.stopPropagation();
                        void run(async () => applyStash(workspaceId, e.index));
                      }}
                    >
                      <Play size={12} />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="p-1"
                      disabled={busy}
                      title="Pop"
                      onClick={(ev) => {
                        ev.stopPropagation();
                        void run(async () => popStash(workspaceId, e.index));
                      }}
                    >
                      <Upload size={12} />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="p-1 text-danger hover:bg-danger/10"
                      disabled={busy}
                      title="Drop"
                      onClick={(ev) => {
                        ev.stopPropagation();
                        void run(async () => dropStash(workspaceId, e.index));
                      }}
                    >
                      <Trash2 size={12} />
                    </Button>
                  </div>
                }
              >
                <div className="flex flex-col min-w-0">
                  <span className={cn("text-text-primary truncate", compact ? "text-xs" : "text-sm")}>
                    {`stash@{${e.index}}`} · {e.message || "(no message)"}
                  </span>
                  <span className="text-[10px] text-text-muted font-mono">{e.oid.slice(0, 7)}</span>
                </div>
              </ListItem>
            ))
          )}
        </div>

        {!compact ? (
          <div className="w-80 shrink-0 overflow-auto p-3">
            {!diff ? (
              <p className="text-xs text-text-muted">Select a stash to preview its diff.</p>
            ) : (
              <div className="space-y-2">
                <p className="text-xs text-text-secondary">
                  {diff.files.length} file(s) · +{diff.total_additions} −{diff.total_deletions}
                </p>
                {diff.files.map((f) => (
                  <div
                    key={f.path}
                    className="rounded border border-border-subtle px-2 py-1.5 text-xs font-mono"
                  >
                    <div className="text-text-primary truncate">{f.path}</div>
                    <div className="text-text-muted">
                      <span className="text-success">+{f.additions}</span>{" "}
                      <span className="text-danger">−{f.deletions}</span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        ) : null}
      </div>
    </div>
  );
}
