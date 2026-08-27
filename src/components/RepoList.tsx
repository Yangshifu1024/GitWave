// Sidebar repository list — pure navigation (activate) with row-scoped ops
// (relink / remove via context menu). Init / clone / add / fetch live in the
// ActionBar below the top bar.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link2 } from "lucide-react";

import {
  formatAppError,
  listRepos,
  relinkRepo,
  removeRepo,
  setActiveRepo,
  type RepoRef,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { ListItem } from "@/components/ui/ListItem";
import { Modal } from "@/components/ui/Modal";
import { EmptyState } from "@/components/ui/EmptyState";
import { StatusBadge } from "@/components/ui/StatusBadge";
import { PathInput } from "@/components/ui/PathInput";
import { SidebarSection } from "@/components/ui/SidebarSection";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/ContextMenu";
import { ErrorAlert } from "@/components/ui/ErrorAlert";

function basename(path: string): string {
  const idx = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return idx >= 0 ? path.slice(idx + 1) : path;
}

export function RepoList({ workspaceId }: { workspaceId: string }): React.JSX.Element {
  const queryClient = useQueryClient();
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const setActiveRepoId = useWorkspaceUiStore((s) => s.setActiveRepoId);

  const [relinking, setRelinking] = useState<RepoRef | null>(null);
  const [removing, setRemoving] = useState<RepoRef | null>(null);
  const [relinkPath, setRelinkPath] = useState("");
  const [actionError, setActionError] = useState<string | null>(null);

  const {
    data: repos = [],
    isLoading,
    error,
  } = useQuery({
    queryKey: ["repos", workspaceId],
    queryFn: () => listRepos(workspaceId),
    enabled: !!workspaceId,
  });

  const refresh = (): void => {
    void queryClient.invalidateQueries({ queryKey: ["repos", workspaceId] });
    void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
  };

  async function activateRepo(repoId: string): Promise<void> {
    await setActiveRepo(workspaceId, repoId);
    setActiveRepoId(repoId);
    void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
  }

  const removeMut = useMutation({
    mutationFn: (repoId: string) => removeRepo(workspaceId, repoId),
    onSuccess: async (_void, repoId) => {
      if (activeRepoId === repoId) {
        await setActiveRepo(workspaceId, null);
        setActiveRepoId(null);
      }
      refresh();
      setRemoving(null);
    },
  });

  const relinkMut = useMutation({
    mutationFn: ({ repoId, newPath }: { repoId: string; newPath: string }) =>
      relinkRepo(workspaceId, repoId, newPath),
    onSuccess: () => {
      refresh();
      setRelinking(null);
      setRelinkPath("");
      setActionError(null);
    },
    onError: (e: unknown) => setActionError(formatAppError(e)),
  });

  const listError = actionError && !relinking ? actionError : null;

  return (
    <>
      <SidebarSection title="Repos">
        {isLoading ? (
          <p className="px-3 py-2 text-sm text-text-muted">Loading repos…</p>
        ) : error ? (
          <p className="px-3 py-2 text-sm text-text-muted">Failed to load repos.</p>
        ) : repos.length === 0 ? (
          <EmptyState
            title="No repos"
            description="Init, clone, or add a local repo from the toolbar."
            className="py-6"
          />
        ) : (
          <ul className="py-1">
            {repos.map((r) => (
              <li key={r.id}>
                <ContextMenu>
                  <ContextMenuTrigger asChild>
                    <div>
                      <ListItem
                        selected={r.id === activeRepoId}
                        onClick={() => {
                          if (r.status === "missing" || r.id === activeRepoId) return;
                          void activateRepo(r.id).catch((e: unknown) =>
                            setActionError(formatAppError(e)),
                          );
                        }}
                        leading={null}
                        trailing={
                          <span className="flex items-center gap-1">
                            {r.status === "missing" && <StatusBadge variant="missing" />}
                            {r.status === "missing" ? (
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  setRelinking(r);
                                  setRelinkPath(r.path);
                                  setActionError(null);
                                }}
                                className="p-1"
                                aria-label="Relink repo"
                              >
                                <Link2 size={13} />
                              </Button>
                            ) : null}
                          </span>
                        }
                      >
                        <span
                          className={
                            r.id === activeRepoId
                              ? "truncate font-mono text-xs text-text-primary font-medium"
                              : "truncate font-mono text-xs text-text-secondary"
                          }
                          title={r.path}
                        >
                          {basename(r.path)}
                        </span>
                      </ListItem>
                    </div>
                  </ContextMenuTrigger>
                  <ContextMenuContent className="max-w-[240px]">
                    <ContextMenuLabel title={r.path}>{basename(r.path)}</ContextMenuLabel>
                    <ContextMenuSeparator />
                    <ContextMenuItem destructive onSelect={() => setRemoving(r)}>
                      Remove
                    </ContextMenuItem>
                  </ContextMenuContent>
                </ContextMenu>
              </li>
            ))}
          </ul>
        )}
      </SidebarSection>

      {/* Relink modal */}
      {relinking && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) setRelinking(null);
          }}
          title={`Relink "${relinking.path}"`}
          size="sm"
        >
          <PathInput
            autoFocus
            directory
            value={relinkPath}
            onChange={setRelinkPath}
            onKeyDown={(e) => {
              if (e.key === "Enter" && relinkPath.trim())
                relinkMut.mutate({ repoId: relinking.id, newPath: relinkPath.trim() });
            }}
            placeholder="New path to a valid git working tree"
            error={actionError}
          />
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setRelinking(null)}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => relinkMut.mutate({ repoId: relinking.id, newPath: relinkPath.trim() })}
              disabled={!relinkPath.trim() || relinkMut.isPending}
            >
              Relink
            </Button>
          </div>
        </Modal>
      )}

      {/* Remove modal */}
      {removing && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) setRemoving(null);
          }}
          title={`Remove "${basename(removing.path)}"?`}
          description={`Path: ${removing.path}`}
          size="sm"
        >
          <p className="text-sm text-text-secondary">Removes the workspace reference.</p>
          <p className="text-sm text-text-secondary">
            The local directory and its .git/ folder are not touched.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setRemoving(null)}>
              Cancel
            </Button>
            <Button
              variant="danger"
              size="sm"
              onClick={() => removeMut.mutate(removing.id)}
              disabled={removeMut.isPending}
            >
              Remove
            </Button>
          </div>
        </Modal>
      )}
      <ErrorAlert
        message={listError ?? (error ? `Failed to load repos: ${formatAppError(error)}` : null)}
        onDismiss={() => setActionError(null)}
      />
    </>
  );
}
