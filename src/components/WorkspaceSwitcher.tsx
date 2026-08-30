import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

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
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ListItem } from "@/components/ui/ListItem";
import { Modal } from "@/components/ui/Modal";
import { EmptyState } from "@/components/ui/EmptyState";
import { FolderPlus, Pencil, Trash2 } from "lucide-react";

export function WorkspaceSwitcher(): React.JSX.Element {
  const queryClient = useQueryClient();
  const activeId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const selectWorkspace = useWorkspaceUiStore((s) => s.selectWorkspace);

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
    mutationFn: ({ id, newName }: { id: string; newName: string }) => renameWorkspace(id, newName),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      setRenaming(null);
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteWorkspace(id),
    onSuccess: (_void, id) => {
      void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      if (activeId === id) selectWorkspace(null);
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
    <div className="flex flex-col">
      {/* Header row */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-border-subtle">
        <h2 className="text-xs font-semibold text-text-muted uppercase tracking-wide">
          Workspaces
        </h2>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setShowCreate(true)}
          aria-label="New workspace"
        >
          <FolderPlus size={14} />
        </Button>
      </div>

      {/* List */}
      {isLoading ? (
        <p className="px-3 py-2 text-sm text-text-muted">Loading…</p>
      ) : error ? (
        <p className="px-3 py-2 text-sm text-danger">Failed to load: {formatAppError(error)}</p>
      ) : workspaces.length === 0 ? (
        <EmptyState
          title="No workspaces"
          description="Create a workspace to get started."
          action={
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(true)}>
              New workspace
            </Button>
          }
          className="py-6"
        />
      ) : (
        <ul className="py-1">
          {workspaces.map((ws) => (
            <li key={ws.id}>
              <ListItem
                selected={ws.id === activeId}
                onClick={() => selectWorkspace(ws.id, ws.last_active_repo_id)}
                leading={null}
                trailing={
                  <span className="flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={(e) => {
                        e.stopPropagation();
                        openRename(ws);
                      }}
                      className="p-1"
                      aria-label={`Rename ${ws.name}`}
                    >
                      <Pencil size={13} />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={(e) => {
                        e.stopPropagation();
                        setDeleting(ws);
                      }}
                      className="p-1 text-danger hover:text-danger"
                      aria-label={`Delete ${ws.name}`}
                    >
                      <Trash2 size={13} />
                    </Button>
                  </span>
                }
              >
                <span className="truncate text-sm">{ws.name}</span>
              </ListItem>
            </li>
          ))}
        </ul>
      )}

      {/* Create modal */}
      {showCreate && (
        <Modal
          open={showCreate}
          onOpenChange={(open) => {
            if (!open) {
              setShowCreate(false);
              setCreateName("");
              setCreateError(null);
            }
          }}
          title="New Workspace"
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setShowCreate(false)}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                onClick={submitCreate}
                disabled={!createName.trim() || createMut.isPending}
              >
                Create
              </Button>
            </>
          }
        >
          <div className="rounded-xl bg-bg-primary p-3">
            <Input
              autoFocus
              value={createName}
              onChange={setCreateName}
              onKeyDown={(e) => {
                if (e.key === "Enter") submitCreate();
              }}
              placeholder="Workspace name"
              error={createError}
            />
          </div>
        </Modal>
      )}

      {/* Rename modal */}
      {renaming && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) setRenaming(null);
          }}
          title={`Rename "${renaming.name}"`}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setRenaming(null)}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                onClick={submitRename}
                disabled={!renameValue.trim() || renameMut.isPending}
              >
                Save
              </Button>
            </>
          }
        >
          <div className="rounded-xl bg-bg-primary p-3">
            <Input
              autoFocus
              value={renameValue}
              onChange={setRenameValue}
              onKeyDown={(e) => {
                if (e.key === "Enter") submitRename();
              }}
            />
          </div>
        </Modal>
      )}

      {/* Delete modal */}
      {deleting && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) setDeleting(null);
          }}
          title={`Delete "${deleting.name}"?`}
          description="This removes the workspace and its repo references from GitWave. It does not delete the local repositories themselves."
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setDeleting(null)}
              >
                Cancel
              </Button>
              <Button
                variant="danger"
                size="sm"
                className="min-w-0 flex-[7]"
                onClick={() => deleteMut.mutate(deleting.id)}
                disabled={deleteMut.isPending}
              >
                Delete
              </Button>
            </>
          }
        />
      )}
    </div>
  );
}

// Re-export the error type so consumers don't have to import from @/lib/api
// separately when handling errors.
export type { AppError };
