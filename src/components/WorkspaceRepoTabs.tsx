// Repository tab strip under the ActionBar: switches repositories of the
// active workspace. Workspace switching lives in the ActionBar's workspace
// dropdown; row ops (relink / remove) live on as a right-click menu on the
// repo tabs. Init / clone / add / fetch stay in the Repository menu and
// ActionBar.
//
// Right-click menu notes: the menu content is a controlled HeroUI Popover
// rendered OUTSIDE the HeroTabs tablists — React Aria renders collection
// children in a detached container on mount, and an overlay inside that
// subtree crashes React (createTextNode is not a function). TabList also
// drops non-Tab wrapper elements, so per-tab title / onContextMenu ride on
// TabsTrigger's DOM passthrough.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Header, Menu, Popover, Separator } from "@heroui/react";

import {
  formatAppError,
  listRepos,
  relinkRepo,
  removeRepo,
  setActiveRepo,
  type RepoRef,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { PathInput } from "@/components/ui/PathInput";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/Tabs";
import { ErrorAlert } from "@/components/ui/ErrorAlert";

function basename(path: string): string {
  const idx = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return idx >= 0 ? path.slice(idx + 1) : path;
}

export function WorkspaceRepoTabs(): React.JSX.Element | null {
  const queryClient = useQueryClient();
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const setActiveRepoId = useWorkspaceUiStore((s) => s.setActiveRepoId);

  const [menu, setMenu] = useState<{ repo: RepoRef; x: number; y: number } | null>(null);
  const [relinking, setRelinking] = useState<RepoRef | null>(null);
  const [removing, setRemoving] = useState<RepoRef | null>(null);
  const [relinkPath, setRelinkPath] = useState("");
  const [actionError, setActionError] = useState<string | null>(null);

  const { data: repos = [], error: reposError } = useQuery({
    queryKey: ["repos", activeWorkspaceId],
    queryFn: () => listRepos(activeWorkspaceId!),
    enabled: !!activeWorkspaceId,
  });

  const refresh = (): void => {
    void queryClient.invalidateQueries({ queryKey: ["repos", activeWorkspaceId] });
    void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
  };

  const activateRepo = async (repoId: string): Promise<void> => {
    await setActiveRepo(activeWorkspaceId!, repoId);
    setActiveRepoId(repoId);
    void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
  };

  const removeMut = useMutation({
    mutationFn: (repoId: string) => removeRepo(activeWorkspaceId!, repoId),
    onSuccess: async (_void, repoId) => {
      if (activeRepoId === repoId) {
        await setActiveRepo(activeWorkspaceId!, null);
        setActiveRepoId(null);
      }
      refresh();
      setRemoving(null);
    },
  });

  const relinkMut = useMutation({
    mutationFn: ({ repoId, newPath }: { repoId: string; newPath: string }) =>
      relinkRepo(activeWorkspaceId!, repoId, newPath),
    onSuccess: () => {
      refresh();
      setRelinking(null);
      setRelinkPath("");
      setActionError(null);
    },
    onError: (e: unknown) => setActionError(formatAppError(e)),
  });

  if (!activeWorkspaceId) return null;

  const closeMenu = (): void => setMenu(null);

  return (
    // No container border-b here: each tab carries its own bottom hairline,
    // and the selected tab's segment disappears so it merges with the panes.
    <div className="shrink-0 bg-bg-primary select-none">
      {/* Repositories of the active workspace; workspace switching lives in
          the ActionBar's workspace dropdown. */}
      <div className="flex min-w-0 items-end">
        <Tabs
          value={activeRepoId ?? ""}
          onValueChange={(id) => {
            if (id === activeRepoId) return;
            void activateRepo(id).catch((e: unknown) => setActionError(formatAppError(e)));
          }}
          className="min-w-0 flex-1"
        >
          <TabsList className="h-6 flex-1 rounded-none bg-bg-primary items-end [&>div]:w-full [&_[role=tablist]]:items-end [&_[role=tablist]]:p-0">
            {repos.map((r) => {
              const label = r.nickname ?? basename(r.path);
              return (
                <TabsTrigger
                  key={r.id}
                  value={r.id}
                  disabled={r.status === "missing"}
                  className="h-6 px-3 py-0 text-xs"
                  title={
                    r.status === "missing"
                      ? `${label} — missing, right-click to relink`
                      : r.path
                  }
                  onContextMenu={(e) => {
                    e.preventDefault();
                    setMenu({ repo: r, x: e.clientX, y: e.clientY });
                  }}
                >
                  {label}
                  {r.status === "missing" ? (
                    <span
                      aria-label="missing"
                      className="ml-1.5 size-1.5 rounded-full bg-warning"
                    />
                  ) : null}
                </TabsTrigger>
              );
            })}
            {repos.length === 0 ? (
              <span className="px-3 py-2 text-xs text-text-muted">
                No repositories — init, clone, or add from the Repository menu.
              </span>
            ) : null}
        </TabsList>
      </Tabs>
      </div>

      {/* Repo right-click menu — outside the tablists, see file head. */}
      <Popover isOpen={menu !== null} onOpenChange={(open) => !open && closeMenu()}>
        <Popover.Trigger
          aria-hidden
          className="fixed z-popover h-px w-px overflow-hidden p-0 pointer-events-none"
          style={{ left: menu?.x ?? 0, top: menu?.y ?? 0 }}
        />
        <Popover.Content
          placement="bottom start"
          offset={2}
          className={cn(
            "z-popover min-w-[180px] rounded-lg",
            "bg-bg-elevated border border-border-default shadow-modal",
            "p-1",
          )}
        >
          <Menu className="outline-none">
            {menu ? (
              <>
                <Header
                  title={menu.repo.path}
                  className="px-2 py-1.5 text-xs font-medium text-text-primary truncate"
                >
                  {menu.repo.nickname ?? basename(menu.repo.path)}
                </Header>
                <Separator className="my-1 bg-border-subtle" />
                {menu.repo.status === "missing" ? (
                  <Menu.Item
                    textValue="Relink"
                    onAction={() => {
                      setRelinking(menu.repo);
                      setRelinkPath(menu.repo.path);
                      setActionError(null);
                      closeMenu();
                    }}
                    className="relative flex cursor-pointer select-none items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none text-text-primary"
                  >
                    Relink…
                  </Menu.Item>
                ) : null}
                <Menu.Item
                  textValue="Remove"
                  variant="danger"
                  data-destructive="true"
                  onAction={() => {
                    setRemoving(menu.repo);
                    closeMenu();
                  }}
                  className="relative flex cursor-pointer select-none items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none text-danger"
                >
                  Remove
                </Menu.Item>
              </>
            ) : null}
          </Menu>
        </Popover.Content>
      </Popover>

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
        message={
          actionError ?? (reposError ? `Failed to load repos: ${formatAppError(reposError)}` : null)
        }
        onDismiss={() => setActionError(null)}
      />
    </div>
  );
}
