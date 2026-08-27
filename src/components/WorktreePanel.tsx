import { useCallback, useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { WorktreeInfo } from "@/lib/api";
import {
  addLocalRepo,
  addWorktree,
  formatAppError,
  listWorktrees,
  removeWorktree,
  setActiveRepo,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ListItem } from "@/components/ui/ListItem";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { cn } from "@/lib/utils";
import { FolderTree, Plus, Trash2, ArrowRightLeft } from "lucide-react";

export function WorktreePanel({ compact = false }: { compact?: boolean }): React.JSX.Element {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const setActiveRepoId = useWorkspaceUiStore((s) => s.setActiveRepoId);
  const queryClient = useQueryClient();

  const [items, setItems] = useState<WorktreeInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [name, setName] = useState("");
  const [path, setPath] = useState("");
  const [branch, setBranch] = useState("");

  const refresh = useCallback(async () => {
    if (!workspaceId) return;
    setItems(await listWorktrees(workspaceId));
  }, [workspaceId]);

  useEffect(() => {
    if (!workspaceId || !repoId) {
      setItems([]);
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
    } catch (e) {
      setError(formatAppError(e));
    } finally {
      setBusy(false);
    }
  };

  const handleCreate = () =>
    void run(async () => {
      const n = name.trim();
      const p = path.trim();
      const b = branch.trim() || n;
      if (!n || !p) throw new Error("Name and path are required");
      await addWorktree(workspaceId!, n, p, b, true);
      setShowCreate(false);
      setName("");
      setPath("");
      setBranch("");
    });

  /** Switch = add worktree path as a Workspace repo and make it active. */
  const handleSwitch = (wt: WorktreeInfo) =>
    void run(async () => {
      const repo = await addLocalRepo(workspaceId!, wt.path);
      await setActiveRepo(workspaceId!, repo.id);
      setActiveRepoId(repo.id);
      void queryClient.invalidateQueries({ queryKey: ["repos", workspaceId] });
    });

  if (!workspaceId || !repoId) {
    return (
      <p
        className={cn(
          "text-text-muted",
          compact ? "px-3 py-1.5 text-xs" : "flex items-center justify-center h-full text-sm",
        )}
      >
        Select a repository to manage worktrees
      </p>
    );
  }

  if (loading) {
    return (
      <p
        className={cn(
          "text-text-muted italic",
          compact ? "px-3 py-1.5 text-xs" : "flex items-center justify-center h-full text-sm",
        )}
      >
        Loading worktrees…
      </p>
    );
  }

  return (
    <div className={cn("min-h-0 flex flex-col", !compact && "h-full overflow-hidden")}>
      <div
        className={cn(
          "shrink-0",
          compact ? "px-2 py-1" : "px-3 py-2 border-b border-border-subtle",
        )}
      >
        <Button
          variant="secondary"
          size="sm"
          disabled={busy}
          onClick={() => setShowCreate((v) => !v)}
        >
          <Plus size={14} />
          {compact ? "New" : "New worktree"}
        </Button>
      </div>

      {showCreate ? (
        <div
          className={cn(
            "shrink-0 flex flex-col gap-1.5 bg-bg-elevated",
            compact ? "px-2 py-1.5" : "px-3 py-2 border-b border-border-subtle gap-2",
          )}
        >
          <Input placeholder="Name" value={name} onChange={setName} disabled={busy} />
          <Input placeholder="Path" value={path} onChange={setPath} disabled={busy} />
          {!compact ? (
            <Input
              placeholder="Branch (default: same as name, created from HEAD)"
              value={branch}
              onChange={setBranch}
              disabled={busy}
            />
          ) : (
            <Input
              placeholder="Branch (optional)"
              value={branch}
              onChange={setBranch}
              disabled={busy}
            />
          )}
          <div className="flex justify-end">
            <Button
              variant="primary"
              size="sm"
              disabled={busy || !name.trim() || !path.trim()}
              onClick={handleCreate}
            >
              Create
            </Button>
          </div>
        </div>
      ) : null}

      {error ? <ErrorAlert message={error} onDismiss={() => setError(null)} /> : null}

      <div className={cn("min-h-0 overflow-auto", compact ? "max-h-52" : "flex-1")}>
        {items.length === 0 ? (
          <p className="px-3 py-1.5 text-xs text-text-muted">No worktrees</p>
        ) : (
          items.map((wt) => (
            <ListItem
              key={wt.name}
              leading={<FolderTree size={14} className="text-accent shrink-0" />}
              trailing={
                <div className="flex items-center gap-0.5">
                  {!wt.is_main ? (
                    <>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="p-1"
                        disabled={busy}
                        title="Switch: add path to Workspace and activate"
                        onClick={() => handleSwitch(wt)}
                      >
                        <ArrowRightLeft size={12} />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="p-1 text-danger hover:bg-danger/10"
                        disabled={busy}
                        title="Remove worktree"
                        onClick={() => void run(async () => removeWorktree(workspaceId, wt.name))}
                      >
                        <Trash2 size={12} />
                      </Button>
                    </>
                  ) : (
                    <span className="text-[10px] text-accent font-medium">main</span>
                  )}
                </div>
              }
            >
              <div className="flex flex-col min-w-0">
                <span className={cn("text-text-primary truncate", compact ? "text-xs" : "text-sm")}>
                  {wt.name}
                  {wt.branch ? (
                    <span className="text-text-muted font-normal"> · {wt.branch}</span>
                  ) : null}
                </span>
                <span className="text-[10px] text-text-muted font-mono truncate" title={wt.path}>
                  {wt.path}
                </span>
              </div>
            </ListItem>
          ))
        )}
      </div>
    </div>
  );
}
