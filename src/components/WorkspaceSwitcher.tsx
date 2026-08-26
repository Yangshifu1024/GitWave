import { useEffect, useRef, useState } from "react";
import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import {
  createWorkspace,
  deleteWorkspace,
  formatAppError,
  listWorkspaces,
  renameWorkspace,
  type AppError,
  type WorkspaceSummary,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

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

export function WorkspaceSwitcher(): React.JSX.Element {
  const queryClient = useQueryClient();
  const activeId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const setActiveId = useWorkspaceUiStore((s) => s.setActiveWorkspaceId);

  const [showCreate, setShowCreate] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createError, setCreateError] = useState<string | null>(null);
  const [renaming, setRenaming] = useState<WorkspaceSummary | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [deleting, setDeleting] = useState<WorkspaceSummary | null>(null);

  const {
    data: workspaces = [],
    isLoading,
    error,
  } = useQuery({
    queryKey: ["workspaces"],
    queryFn: listWorkspaces,
  });

  const createMut = useMutation({
    mutationFn: (name: string) => createWorkspace(name),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      setShowCreate(false);
      setCreateName("");
      setCreateError(null);
    },
    onError: (err: unknown) => {
      setCreateError(formatAppError(err));
    },
  });

  const renameMut = useMutation({
    mutationFn: ({ id, newName }: { id: string; newName: string }) =>
      renameWorkspace(id, newName),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      setRenaming(null);
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteWorkspace(id),
    onSuccess: (_void, id) => {
      void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      if (activeId === id) setActiveId(null);
      setDeleting(null);
    },
  });

  function openRename(ws: WorkspaceSummary): void {
    setRenaming(ws);
    setRenameValue(ws.name);
  }

  function submitCreate(): void {
    const name = createName.trim();
    if (!name) return;
    setCreateError(null);
    createMut.mutate(name);
  }

  function submitRename(): void {
    if (!renaming) return;
    const name = renameValue.trim();
    if (!name) return;
    renameMut.mutate({ id: renaming.id, newName: name });
  }

  return (
    <section className="workspace-switcher">
      <header>
        <h2>Workspaces</h2>
        <button type="button" onClick={() => setShowCreate(true)}>
          New
        </button>
      </header>

      {isLoading ? (
        <p>Loading…</p>
      ) : error ? (
        <p className="error">Failed to load: {formatAppError(error)}</p>
      ) : workspaces.length === 0 ? (
        <p className="empty">No workspaces yet. Click New to create one.</p>
      ) : (
        <ul>
          {workspaces.map((ws) => (
            <li
              key={ws.id}
              className={ws.id === activeId ? "active" : undefined}
            >
              <button
                type="button"
                className="name"
                onClick={() => setActiveId(ws.id)}
              >
                {ws.name}
              </button>
              <span className="actions">
                <button type="button" onClick={() => openRename(ws)}>
                  rename
                </button>
                <button
                  type="button"
                  onClick={() => setDeleting(ws)}
                  className="danger"
                >
                  delete
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}

      {showCreate ? (
        <Modal title="New Workspace" onClose={() => setShowCreate(false)}>
          <input
            autoFocus
            value={createName}
            onChange={(e) => setCreateName(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") submitCreate();
            }}
            placeholder="Workspace name"
          />
          {createError ? <p className="error">{createError}</p> : null}
          <div className="modal-actions">
            <button type="button" onClick={() => setShowCreate(false)}>
              Cancel
            </button>
            <button
              type="button"
              onClick={submitCreate}
              disabled={!createName.trim() || createMut.isPending}
            >
              Create
            </button>
          </div>
        </Modal>
      ) : null}

      {renaming ? (
        <Modal
          title={`Rename "${renaming.name}"`}
          onClose={() => setRenaming(null)}
        >
          <input
            autoFocus
            value={renameValue}
            onChange={(e) => setRenameValue(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") submitRename();
            }}
          />
          <div className="modal-actions">
            <button type="button" onClick={() => setRenaming(null)}>
              Cancel
            </button>
            <button
              type="button"
              onClick={submitRename}
              disabled={!renameValue.trim() || renameMut.isPending}
            >
              Save
            </button>
          </div>
        </Modal>
      ) : null}

      {deleting ? (
        <Modal
          title={`Delete "${deleting.name}"?`}
          onClose={() => setDeleting(null)}
        >
          <p>
            This removes the workspace and its repo references from GitWave.
            It does not delete the local repositories themselves.
          </p>
          <div className="modal-actions">
            <button type="button" onClick={() => setDeleting(null)}>
              Cancel
            </button>
            <button
              type="button"
              className="danger"
              onClick={() => deleteMut.mutate(deleting.id)}
              disabled={deleteMut.isPending}
            >
              Delete
            </button>
          </div>
        </Modal>
      ) : null}
    </section>
  );
}

// Re-export the error type so consumers don't have to import from @/lib/api
// separately when handling errors.
export type { AppError };