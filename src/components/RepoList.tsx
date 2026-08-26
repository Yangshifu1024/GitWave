import { useEffect, useRef, useState } from "react";
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

function Modal({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
}): React.JSX.Element {
  const dialogRef = useRef<HTMLDialogElement | null>(null);
  useEffect(() => {
    dialogRef.current?.showModal();
  }, []);
  return (
    <dialog
      ref={dialogRef}
      onClose={onClose}
      onClick={(e) => {
        if (e.target === dialogRef.current) onClose();
      }}
    >
      <div className="modal-inner">
        <h3>{title}</h3>
        {children}
      </div>
    </dialog>
  );
}

function detectProtocol(url: string): "ssh" | "https" {
  if (url.startsWith("ssh://") || url.startsWith("git@")) return "ssh";
  return "https";
}

function deriveDestName(url: string): string {
  // For git@github.com:user/repo.git -> user/repo
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
    <section className="repo-list">
      <header>
        <h2>Repos</h2>
        <span className="add-group">
          <button type="button" onClick={() => startAdd("init")}>
            Init
          </button>
          <button type="button" onClick={() => startAdd("clone")}>
            Clone
          </button>
          <button type="button" onClick={() => startAdd("local")}>
            Add Local
          </button>
        </span>
      </header>

      {isLoading ? (
        <p>Loading repos…</p>
      ) : error ? (
        <p className="error">Failed to load repos: {formatAppError(error)}</p>
      ) : repos.length === 0 ? (
        <p className="empty">
          No repos in this workspace yet — Init / Clone / Add Local to start.
        </p>
      ) : (
        <ul>
          {repos.map((r) => (
            <li key={r.id} className={r.status === "missing" ? "missing" : undefined}>
              <span className="path">{r.path}</span>
              {r.status === "missing" ? (
                <span className="badge">missing</span>
              ) : null}
              <span className="actions">
                {r.status === "missing" ? (
                  <button
                    type="button"
                    onClick={() => {
                      setRelinking(r);
                      setRelinkPath(r.path);
                      setActionError(null);
                    }}
                  >
                    relink
                  </button>
                ) : null}
                <button
                  type="button"
                  onClick={() => setRemoving(r)}
                  className="danger"
                >
                  remove
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}

      {adding === "init" ? (
        <Modal title="Initialize new repo" onClose={endAdd}>
          <input
            autoFocus
            value={initPath}
            onChange={(e) => setInitPath(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && initPath.trim())
                initMut.mutate({ path: initPath.trim() });
            }}
            placeholder="Absolute path, e.g. /Users/me/projects/new"
          />
          {actionError ? <p className="error">{actionError}</p> : null}
          <div className="modal-actions">
            <button type="button" onClick={endAdd}>
              Cancel
            </button>
            <button
              type="button"
              onClick={() => initMut.mutate({ path: initPath.trim() })}
              disabled={!initPath.trim() || initMut.isPending}
            >
              Create
            </button>
          </div>
        </Modal>
      ) : null}

      {adding === "clone" ? (
        <Modal title="Clone remote repo" onClose={endAdd}>
          <label>
            URL
            <input
              autoFocus
              value={cloneUrl}
              onChange={(e) => {
                setCloneUrl(e.currentTarget.value);
                if (cloneDest === "" || cloneDest.startsWith("./")) {
                  setCloneDest(`./${deriveDestName(e.currentTarget.value)}`);
                }
              }}
              placeholder="https://github.com/user/repo.git or git@github.com:user/repo.git"
            />
            <small>
              Detected protocol: {cloneUrl ? detectProtocol(cloneUrl) : "—"}
            </small>
          </label>
          <label>
            Destination path
            <input
              value={cloneDest}
              onChange={(e) => setCloneDest(e.currentTarget.value)}
              placeholder="./repo"
            />
          </label>
          {actionError ? <p className="error">{actionError}</p> : null}
          <div className="modal-actions">
            <button type="button" onClick={endAdd}>
              Cancel
            </button>
            <button
              type="button"
              onClick={() =>
                cloneMut.mutate({
                  url: cloneUrl.trim(),
                  dest: cloneDest.trim(),
                })
              }
              disabled={!cloneUrl.trim() || !cloneDest.trim() || cloneMut.isPending}
            >
              Clone
            </button>
          </div>
        </Modal>
      ) : null}

      {adding === "local" ? (
        <Modal title="Add existing local repo" onClose={endAdd}>
          <input
            autoFocus
            value={localPath}
            onChange={(e) => setLocalPath(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && localPath.trim())
                localMut.mutate({ path: localPath.trim() });
            }}
            placeholder="Absolute path to an existing git working tree"
          />
          {actionError ? <p className="error">{actionError}</p> : null}
          <div className="modal-actions">
            <button type="button" onClick={endAdd}>
              Cancel
            </button>
            <button
              type="button"
              onClick={() => localMut.mutate({ path: localPath.trim() })}
              disabled={!localPath.trim() || localMut.isPending}
            >
              Add
            </button>
          </div>
        </Modal>
      ) : null}

      {relinking ? (
        <Modal title={`Relink "${relinking.path}"`} onClose={() => setRelinking(null)}>
          <input
            autoFocus
            value={relinkPath}
            onChange={(e) => setRelinkPath(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && relinkPath.trim())
                relinkMut.mutate({
                  repoId: relinking.id,
                  newPath: relinkPath.trim(),
                });
            }}
            placeholder="New path to a valid git working tree"
          />
          {actionError ? <p className="error">{actionError}</p> : null}
          <div className="modal-actions">
            <button type="button" onClick={() => setRelinking(null)}>
              Cancel
            </button>
            <button
              type="button"
              onClick={() =>
                relinkMut.mutate({
                  repoId: relinking.id,
                  newPath: relinkPath.trim(),
                })
              }
              disabled={!relinkPath.trim() || relinkMut.isPending}
            >
              Relink
            </button>
          </div>
        </Modal>
      ) : null}

      {removing ? (
        <Modal title={`Remove "${removing.path}"?`} onClose={() => setRemoving(null)}>
          <p>
            Removes the workspace reference. The local directory and its
            .git/ folder are not touched.
          </p>
          <div className="modal-actions">
            <button type="button" onClick={() => setRemoving(null)}>
              Cancel
            </button>
            <button
              type="button"
              className="danger"
              onClick={() => removeMut.mutate(removing.id)}
              disabled={removeMut.isPending}
            >
              Remove
            </button>
          </div>
        </Modal>
      ) : null}
    </section>
  );
}