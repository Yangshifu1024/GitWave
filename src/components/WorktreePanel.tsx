import { useCallback, useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { WorktreeInfo } from "@/lib/api";
import {
  addLocalRepo,
  formatAppError,
  listWorktrees,
  removeWorktree,
  setActiveRepo,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { ListItem } from "@/components/ui/ListItem";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { cn } from "@/lib/utils";
import { FolderTree, Trash2, ArrowRightLeft } from "lucide-react";

export function WorktreePanel({ compact = false }: { compact?: boolean }): React.JSX.Element {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const setActiveRepoId = useWorkspaceUiStore((s) => s.setActiveRepoId);
  const queryClient = useQueryClient();

  const [items, setItems] = useState<WorktreeInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // epoch in deps: auto-refresh bumps it to re-run this manual effect.
  const historyEpoch = useWorkspaceUiStore((s) => s.historyEpoch);

  const refresh = useCallback(async () => {
    void historyEpoch; // re-run trigger: auto-refresh bumps the epoch.
    if (!workspaceId) return;
    setItems(await listWorktrees(workspaceId));
  }, [workspaceId, historyEpoch]);

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

  /** Switch = add worktree path as a Workspace repo and make it active. */
  const handleSwitch = (wt: WorktreeInfo) =>
    void run(async () => {
      const repo = await addLocalRepo(workspaceId!, wt.path);
      await setActiveRepo(workspaceId!, repo.id);
      setActiveRepoId(repo.id);
      void queryClient.invalidateQueries({ queryKey: ["repos", workspaceId] });
    });

  // Empty dataset = static header (nothing to expand); worktrees are created
  // from the Repository menu ("New worktree").
  const collapsible = !loading && items.length > 0;

  return (
    <SidebarSection title="Worktrees" collapsible={collapsible}>
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
                        onClick={() => void run(async () => removeWorktree(workspaceId!, wt.name))}
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
    </SidebarSection>
  );
}
