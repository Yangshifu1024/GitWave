import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  addLocalRepo,
  cloneRepo,
  formatAppError,
  initRepo,
  listRepos,
  relinkRepo,
  removeRepo,
  type RepoRef,
} from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ListItem } from "@/components/ui/ListItem";
import { Modal } from "@/components/ui/Modal";
import { EmptyState } from "@/components/ui/EmptyState";
import { StatusBadge } from "@/components/ui/StatusBadge";
import { PathInput } from "@/components/ui/PathInput";
import { FolderPlus, GitBranch, FolderInput, Link2, Trash2 } from "lucide-react";

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

export function RepoList({
  workspaceId,
}: {
  workspaceId: string;
}): React.JSX.Element {
  const queryClient = useQueryClient();

  const [adding, setAdding] = useState<AddKind>(null);
  const [relinking, setRelinking] = useState<RepoRef | null>(null);
  const [removing, setRemoving] = useState<RepoRef | null>(null);

  const [initPath, setInitPath] = useState("");
  const [cloneUrl, setCloneUrl] = useState("");
  const [cloneDest, setCloneDest] = useState("");
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
  };

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
  }

  const initMut = useMutation({
    mutationFn: ({ path }: { path: string }) => initRepo(workspaceId, path),
    onSuccess: () => {
      refresh();
      setInitPath("");
      endAdd();
    },
    onError: (e: unknown) => setActionError(formatAppError(e)),
  });

  const cloneMut = useMutation({
    mutationFn: ({ url, dest }: { url: string; dest: string }) =>
      cloneRepo(workspaceId, url, dest),
    onSuccess: () => {
      refresh();
      setCloneUrl("");
      setCloneDest("");
      endAdd();
    },
    onError: (e: unknown) => setActionError(formatAppError(e)),
  });

  const localMut = useMutation({
    mutationFn: ({ path }: { path: string }) => addLocalRepo(workspaceId, path),
    onSuccess: () => {
      refresh();
      setLocalPath("");
      endAdd();
    },
    onError: (e: unknown) => setActionError(formatAppError(e)),
  });

  const removeMut = useMutation({
    mutationFn: (repoId: string) => removeRepo(workspaceId, repoId),
    onSuccess: () => {
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

  return (
    <div className="flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-border-subtle">
        <h2 className="text-xs font-semibold text-text-muted uppercase tracking-wide">
          Repos
        </h2>
        <span className="flex items-center gap-1">
          <Button variant="ghost" size="sm" onClick={() => startAdd("init")} aria-label="Init repo">
            <GitBranch size={14} />
          </Button>
          <Button variant="ghost" size="sm" onClick={() => startAdd("clone")} aria-label="Clone repo">
            <FolderPlus size={14} />
          </Button>
          <Button variant="ghost" size="sm" onClick={() => startAdd("local")} aria-label="Add local repo">
            <FolderInput size={14} />
          </Button>
        </span>
      </div>

      {/* List */}
      {isLoading ? (
        <p className="px-3 py-2 text-sm text-text-muted">Loading repos…</p>
      ) : error ? (
        <p className="px-3 py-2 text-sm text-danger">
          Failed to load repos: {formatAppError(error)}
        </p>
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
                selected={false}
                leading={null}
                trailing={
                  <span className="flex items-center gap-1">
                    {r.status === "missing" && (
                      <StatusBadge variant="missing" />
                    )}
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
                      className="p-1 text-danger hover:text-danger"
                      aria-label="Remove repo"
                    >
                      <Trash2 size={13} />
                    </Button>
                  </span>
                }
              >
                <span
                  className="truncate font-mono text-xs text-text-secondary"
                  title={r.path}
                >
                  {basename(r.path)}
                </span>
              </ListItem>
            </li>
          ))}
        </ul>
      )}

      {/* Init modal */}
      {adding === "init" && (
        <Modal
          open={true}
          onOpenChange={(open) => { if (!open) endAdd(); }}
          title="Initialize new repo"
          size="sm"
        >
          <PathInput
            autoFocus
            directory
            value={initPath}
            onChange={setInitPath}
            onKeyDown={(e) => {
              if (e.key === "Enter" && initPath.trim())
                initMut.mutate({ path: initPath.trim() });
            }}
            placeholder="Absolute path, e.g. /Users/me/projects/new"
            error={actionError}
          />
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={endAdd}>Cancel</Button>
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
          onOpenChange={(open) => { if (!open) endAdd(); }}
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
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={endAdd}>Cancel</Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() =>
                cloneMut.mutate({ url: cloneUrl.trim(), dest: cloneDest.trim() })
              }
              disabled={!cloneUrl.trim() || !cloneDest.trim() || cloneMut.isPending}
            >
              Clone
            </Button>
          </div>
        </Modal>
      )}

      {/* Add local modal */}
      {adding === "local" && (
        <Modal
          open={true}
          onOpenChange={(open) => { if (!open) endAdd(); }}
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
            <Button variant="secondary" size="sm" onClick={endAdd}>Cancel</Button>
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
          onOpenChange={(open) => { if (!open) setRelinking(null); }}
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
            <Button variant="secondary" size="sm" onClick={() => setRelinking(null)}>Cancel</Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() =>
                relinkMut.mutate({ repoId: relinking.id, newPath: relinkPath.trim() })
              }
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
          onOpenChange={(open) => { if (!open) setRemoving(null); }}
          title={`Remove "${basename(removing.path)}"?`}
          description={removing.path}
          size="sm"
        >
          <p className="text-sm text-text-secondary">
            Removes the workspace reference.
          </p>
          <p className="text-sm text-text-secondary">
            The local directory and its .git/ folder are not touched.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setRemoving(null)}>Cancel</Button>
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
    </div>
  );
}
