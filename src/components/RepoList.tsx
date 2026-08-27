import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { listen } from "@tauri-apps/api/event";

import {
  addLocalRepo,
  cloneRepo,
  formatAppError,
  initRepo,
  listRepos,
  relinkRepo,
  removeRepo,
  setActiveRepo,
  type CloneProgress,
  type RepoRef,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ListItem } from "@/components/ui/ListItem";
import { Modal } from "@/components/ui/Modal";
import { EmptyState } from "@/components/ui/EmptyState";
import { StatusBadge } from "@/components/ui/StatusBadge";
import { PathInput } from "@/components/ui/PathInput";
import { Link2, Trash2 } from "lucide-react";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { FetchButton, SectionAction } from "@/components/ui/SectionAction";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";

function detectProtocol(url: string): "ssh" | "https" {
  if (url.startsWith("ssh://") || url.startsWith("git@")) return "ssh";
  return "https";
}

function basename(path: string): string {
  const idx = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return idx >= 0 ? path.slice(idx + 1) : path;
}

function deriveDestName(url: string): string {
  const trimmed = url.trim();
  if (trimmed.startsWith("git@")) {
    const after = trimmed.slice(trimmed.indexOf(":") + 1).replace(/\.git$/, "");
    const tail = after.split("/").pop() ?? "repo";
    return tail;
  }
  const noProto = trimmed.replace(/^https?:\/\//, "").replace(/^ssh:\/\//, "");
  const tail = noProto.split("/").pop() ?? "repo";
  return tail.replace(/\.git$/, "");
}

type AddKind = "init" | "clone" | "local" | null;

export function RepoList({ workspaceId }: { workspaceId: string }): React.JSX.Element {
  const queryClient = useQueryClient();
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const setActiveRepoId = useWorkspaceUiStore((s) => s.setActiveRepoId);
  const wc = useWorkingCopy();

  const [adding, setAdding] = useState<AddKind>(null);
  const [relinking, setRelinking] = useState<RepoRef | null>(null);
  const [removing, setRemoving] = useState<RepoRef | null>(null);

  const [initPath, setInitPath] = useState("");
  const [cloneUrl, setCloneUrl] = useState("");
  const [cloneDest, setCloneDest] = useState("");
  const [cloneProgress, setCloneProgress] = useState<CloneProgress | null>(null);
  const [cloneFailed, setCloneFailed] = useState(false);
  const [localPath, setLocalPath] = useState("");
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

  function startAdd(kind: Exclude<AddKind, null>): void {
    setAdding(kind);
    setActionError(null);
    if (kind === "clone" && cloneDest === "") {
      setCloneDest(`./${deriveDestName(cloneUrl)}`);
    }
  }

  function endAdd(): void {
    setAdding(null);
    setActionError(null);
    setCloneProgress(null);
    setCloneFailed(false);
  }

  useEffect(() => {
    if (adding !== "clone") return;
    let unlisten: (() => void) | undefined;
    void listen<CloneProgress>("clone-progress", (event) => {
      setCloneProgress(event.payload);
    }).then((fn) => {
      unlisten = fn;
    });
    return () => {
      unlisten?.();
    };
  }, [adding]);

  const initMut = useMutation({
    mutationFn: ({ path }: { path: string }) => initRepo(workspaceId, path),
    onSuccess: (repo) => {
      refresh();
      setInitPath("");
      endAdd();
      void activateRepo(repo.id);
    },
    onError: (e: unknown) => setActionError(formatAppError(e)),
  });

  const cloneMut = useMutation({
    mutationFn: ({
      url,
      dest,
      replaceDest,
    }: {
      url: string;
      dest: string;
      replaceDest?: boolean;
    }) => cloneRepo(workspaceId, url, dest, replaceDest ?? false),
    onMutate: () => {
      setCloneFailed(false);
      setCloneProgress(null);
      setActionError(null);
    },
    onSuccess: (repo) => {
      refresh();
      setCloneUrl("");
      setCloneDest("");
      endAdd();
      void activateRepo(repo.id);
    },
    onError: (e: unknown) => {
      setActionError(formatAppError(e));
      setCloneFailed(true);
    },
  });
  const localMut = useMutation({
    mutationFn: ({ path }: { path: string }) => addLocalRepo(workspaceId, path),
    onSuccess: (repo) => {
      refresh();
      setLocalPath("");
      endAdd();
      void activateRepo(repo.id);
    },
    onError: (e: unknown) => setActionError(formatAppError(e)),
  });

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

  const headerError = (actionError && !adding && !relinking ? actionError : null) ?? wc.actionError;

  return (
    <>
      <SidebarSection
        title="Repos"
        actions={
          <>
            <FetchButton
              onFetch={wc.fetch}
              disabled={!activeRepoId || wc.isSyncBusy}
              inProgress={wc.syncPending.fetch}
            />
            <SectionAction
              tooltip="Initialize a new repository in a folder"
              onClick={() => startAdd("init")}
            >
              Init
            </SectionAction>
            <SectionAction
              tooltip="Clone a repository from a remote URL"
              onClick={() => startAdd("clone")}
            >
              Clone
            </SectionAction>
            <SectionAction
              tooltip="Add an existing local repository"
              onClick={() => startAdd("local")}
            >
              Add
            </SectionAction>
          </>
        }
      >
        {isLoading ? (
          <p className="px-3 py-2 text-sm text-text-muted">Loading repos…</p>
        ) : error ? (
          <p className="px-3 py-2 text-sm text-text-muted">Failed to load repos.</p>
        ) : repos.length === 0 ? (
          <EmptyState
            title="No repos"
            description="Init, clone, or add a local repo to start."
            className="py-6"
          />
        ) : (
          <ul className="py-1">
            {repos.map((r) => (
              <li key={r.id}>
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
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={(e) => {
                          e.stopPropagation();
                          setRemoving(r);
                        }}
                        className="p-1 text-danger hover:text-danger opacity-0 group-hover:opacity-100 group-focus-within:opacity-100"
                        aria-label="Remove repo"
                      >
                        <Trash2 size={13} />
                      </Button>
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
              </li>
            ))}
          </ul>
        )}
      </SidebarSection>

      {/* Init modal */}
      {adding === "init" && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) endAdd();
          }}
          title="Initialize new repo"
          size="sm"
        >
          <PathInput
            autoFocus
            directory
            value={initPath}
            onChange={setInitPath}
            onKeyDown={(e) => {
              if (e.key === "Enter" && initPath.trim()) initMut.mutate({ path: initPath.trim() });
            }}
            placeholder="Absolute path, e.g. /Users/me/projects/new"
            error={actionError}
          />
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={endAdd}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => initMut.mutate({ path: initPath.trim() })}
              disabled={!initPath.trim() || initMut.isPending}
            >
              Create
            </Button>
          </div>
        </Modal>
      )}

      {/* Clone modal */}
      {adding === "clone" && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) endAdd();
          }}
          title="Clone remote repo"
          size="sm"
        >
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium text-text-secondary" htmlFor="clone-url">
                URL
              </label>
              <Input
                id="clone-url"
                autoFocus
                value={cloneUrl}
                onChange={(v) => {
                  setCloneUrl(v);
                  if (cloneDest === "" || cloneDest.startsWith("./")) {
                    setCloneDest(`./${deriveDestName(v)}`);
                  }
                }}
                placeholder="https://github.com/user/repo.git or git@github.com:user/repo.git"
              />
              <p className="text-xs text-text-muted">
                Detected protocol: {cloneUrl ? detectProtocol(cloneUrl) : "—"}
              </p>
            </div>
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium text-text-secondary" htmlFor="clone-dest">
                Destination path
              </label>
              <PathInput
                id="clone-dest"
                directory
                value={cloneDest}
                onChange={setCloneDest}
                placeholder="./repo"
              />
            </div>
            {actionError && <p className="text-xs text-danger">{actionError}</p>}
            {cloneMut.isPending || cloneProgress ? (
              <div className="flex flex-col gap-1">
                <div className="h-1.5 rounded bg-bg-secondary overflow-hidden">
                  <div
                    className="h-full bg-accent transition-[width] duration-200"
                    style={{
                      width: `${
                        cloneProgress && cloneProgress.totalObjects > 0
                          ? Math.min(
                              100,
                              Math.round(
                                (100 * cloneProgress.receivedObjects) / cloneProgress.totalObjects,
                              ),
                            )
                          : cloneMut.isPending
                            ? 8
                            : 0
                      }%`,
                    }}
                  />
                </div>
                <p className="text-[11px] text-text-muted font-mono">
                  {cloneProgress
                    ? `objects ${cloneProgress.receivedObjects}/${cloneProgress.totalObjects || "?"} · deltas ${cloneProgress.indexedDeltas}/${cloneProgress.totalDeltas || "?"} · ${Math.round(cloneProgress.receivedBytes / 1024)} KiB`
                    : "Starting clone…"}
                </p>
              </div>
            ) : null}
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={endAdd} disabled={cloneMut.isPending}>
              Cancel
            </Button>
            {cloneFailed ? (
              <Button
                variant="primary"
                size="sm"
                onClick={() =>
                  cloneMut.mutate({
                    url: cloneUrl.trim(),
                    dest: cloneDest.trim(),
                    replaceDest: true,
                  })
                }
                disabled={!cloneUrl.trim() || !cloneDest.trim() || cloneMut.isPending}
              >
                Retry
              </Button>
            ) : (
              <Button
                variant="primary"
                size="sm"
                onClick={() => cloneMut.mutate({ url: cloneUrl.trim(), dest: cloneDest.trim() })}
                disabled={!cloneUrl.trim() || !cloneDest.trim() || cloneMut.isPending}
              >
                {cloneMut.isPending ? "Cloning…" : "Clone"}
              </Button>
            )}
          </div>
        </Modal>
      )}
      {/* Add local modal */}
      {adding === "local" && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) endAdd();
          }}
          title="Add existing local repo"
          size="sm"
        >
          <PathInput
            autoFocus
            directory
            value={localPath}
            onChange={setLocalPath}
            onKeyDown={(e) => {
              if (e.key === "Enter" && localPath.trim())
                localMut.mutate({ path: localPath.trim() });
            }}
            placeholder="Absolute path to an existing git working tree"
            error={actionError}
          />
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={endAdd}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => localMut.mutate({ path: localPath.trim() })}
              disabled={!localPath.trim() || localMut.isPending}
            >
              Add
            </Button>
          </div>
        </Modal>
      )}

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
        message={headerError ?? (error ? `Failed to load repos: ${formatAppError(error)}` : null)}
        onDismiss={() => {
          setActionError(null);
          wc.setActionError(null);
        }}
      />
    </>
  );
}
