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
import { FolderTree, Plus, Trash2, ArrowRightLeft } from "lucide-react";

export function WorktreePanel(): React.JSX.Element {
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
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Select a repository to manage worktrees
      </div>
    );
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Loading worktrees...
      </div>
    );
  }

  return (
    <div className="h-full min-h-0 flex flex-col overflow-hidden">
      <div className="shrink-0 flex items-center gap-2 px-3 py-2 border-b border-border-subtle">
        <Button variant="secondary" size="sm" disabled={busy} onClick={() => setShowCreate((v) => !v)}>
          <Plus size={14} />
          New worktree
        </Button>
      </div>

      {showCreate ? (
        <div className="shrink-0 flex flex-col gap-2 px-3 py-2 border-b border-border-subtle bg-bg-elevated">
          <Input placeholder="Name (e.g. feature-x)" value={name} onChange={setName} disabled={busy} />
          <Input
            placeholder="Path (absolute worktree directory)"
            value={path}
            onChange={setPath}
            disabled={busy}
          />
          <Input
            placeholder="Branch (default: same as name, created from HEAD)"
            value={branch}
            onChange={setBranch}
            disabled={busy}
          />
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

      <div className="flex-1 min-h-0 overflow-auto">
        {items.map((wt) => (
          <ListItem
            key={wt.name}
            leading={<FolderTree size={14} className="text-accent shrink-0" />}
            trailing={
              <div className="flex items-center gap-1">
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
                  <span className="text-xs text-accent font-medium">main</span>
                )}
              </div>
            }
          >
            <div className="flex flex-col min-w-0">
              <span className="text-sm text-text-primary truncate">
                {wt.name}
                {wt.branch ? (
                  <span className="text-text-muted font-normal"> · {wt.branch}</span>
                ) : null}
              </span>
              <span className="text-xs text-text-muted font-mono truncate" title={wt.path}>
                {wt.path}
              </span>
            </div>
          </ListItem>
        ))}
      </div>
    </div>
  );
}
