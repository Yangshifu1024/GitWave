// Sidebar workspace list — select / create / rename / delete / AI actions.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createWorkspace,
  deleteWorkspace,
  formatAppError,
  listWorkspaces,
  renameWorkspace,
  type WorkspaceSummary,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ListItem } from "@/components/ui/ListItem";
import { Modal } from "@/components/ui/Modal";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { AiProviderSettings } from "@/components/AiProviderSettings";
import { FolderPlus, Pencil, Sparkles, Trash2 } from "lucide-react";
import { SidebarSection } from "@/components/ui/SidebarSection";

export function WorkspaceSwitcherDropdown(): React.JSX.Element {
  const queryClient = useQueryClient();
  const activeId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const selectWorkspace = useWorkspaceUiStore((s) => s.selectWorkspace);

  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createError, setCreateError] = useState<string | null>(null);

  const [renaming, setRenaming] = useState<WorkspaceSummary | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [renameError, setRenameError] = useState<string | null>(null);

  const [deleting, setDeleting] = useState<WorkspaceSummary | null>(null);
  const [aiWorkspace, setAiWorkspace] = useState<WorkspaceSummary | null>(null);

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
      setCreateName("");
      setCreateError(null);
      setCreateOpen(false);
    },
    onError: (e: unknown) => setCreateError(formatAppError(e)),
  });

  const renameMut = useMutation({
    mutationFn: ({ id, newName }: { id: string; newName: string }) => renameWorkspace(id, newName),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      setRenaming(null);
    },
    onError: (e: unknown) => setRenameError(formatAppError(e)),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteWorkspace(id),
    onSuccess: (_void, id) => {
      void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      if (activeId === id) selectWorkspace(null);
      setDeleting(null);
    },
  });

  function submitCreate(): void {
    const name = createName.trim();
    if (!name) return;
    setCreateError(null);
    createMut.mutate(name);
  }

  function openRename(ws: WorkspaceSummary): void {
    setRenaming(ws);
    setRenameValue(ws.name);
    setRenameError(null);
  }

  function submitRename(): void {
    if (!renaming) return;
    const name = renameValue.trim();
    if (!name) return;
    setRenameError(null);
    renameMut.mutate({ id: renaming.id, newName: name });
  }

  function confirmDelete(): void {
    if (!deleting) return;
    deleteMut.mutate(deleting.id);
  }

  const actionBtn =
    "opacity-0 group-hover:opacity-100 text-text-secondary hover:text-accent p-1 rounded-sm";

  return (
    <>
      <SidebarSection
        title="Workspaces"
        actions={
          <Button
            variant="ghost"
            size="sm"
            className="p-1"
            onClick={() => setCreateOpen(true)}
            aria-label="New workspace"
          >
            <FolderPlus size={14} />
          </Button>
        }
      >
        {isLoading ? (
          <p className="px-3 py-2 text-sm text-text-muted">Loading…</p>
        ) : error ? (
          <p className="px-3 py-2 text-sm text-text-muted">Failed to load workspaces.</p>
        ) : workspaces.length === 0 ? (
          <p className="px-3 py-2 text-sm text-text-muted italic">No workspaces yet</p>
        ) : (
          <ul className="pb-1">
            {workspaces.map((ws) => (
              <li key={ws.id}>
                <ListItem
                  selected={ws.id === activeId}
                  onClick={() => selectWorkspace(ws.id, ws.last_active_repo_id)}
                  trailing={
                    <span className="flex items-center gap-0.5">
                      <button
                        type="button"
                        aria-label={`AI provider for ${ws.name}`}
                        title="AI provider"
                        onClick={(e) => {
                          e.preventDefault();
                          e.stopPropagation();
                          setAiWorkspace(ws);
                        }}
                        className={actionBtn}
                      >
                        <Sparkles size={13} />
                      </button>
                      <button
                        type="button"
                        aria-label={`Rename ${ws.name}`}
                        onClick={(e) => {
                          e.preventDefault();
                          e.stopPropagation();
                          openRename(ws);
                        }}
                        className={actionBtn}
                      >
                        <Pencil size={13} />
                      </button>
                      <button
                        type="button"
                        aria-label={`Delete ${ws.name}`}
                        onClick={(e) => {
                          e.preventDefault();
                          e.stopPropagation();
                          setDeleting(ws);
                        }}
                        className="opacity-0 group-hover:opacity-100 text-text-secondary hover:text-danger p-1 rounded-sm"
                      >
                        <Trash2 size={13} />
                      </button>
                    </span>
                  }
                >
                  <span className="truncate">{ws.name}</span>
                </ListItem>
              </li>
            ))}
          </ul>
        )}
      </SidebarSection>

      <Modal
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) {
            setCreateName("");
            setCreateError(null);
          }
        }}
        title="New Workspace"
        size="sm"
      >
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
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={() => setCreateOpen(false)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={submitCreate}
            disabled={!createName.trim() || createMut.isPending}
          >
            Create
          </Button>
        </div>
      </Modal>

      <Modal
        open={renaming !== null}
        onOpenChange={(open) => {
          if (!open) setRenaming(null);
        }}
        title={renaming ? `Rename "${renaming.name}"` : ""}
        size="sm"
      >
        <Input
          autoFocus
          value={renameValue}
          onChange={setRenameValue}
          onKeyDown={(e) => {
            if (e.key === "Enter") submitRename();
          }}
          error={renameError}
        />
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={() => setRenaming(null)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={submitRename}
            disabled={!renameValue.trim() || renameMut.isPending}
          >
            Save
          </Button>
        </div>
      </Modal>

      <Modal
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        title={deleting ? `Delete "${deleting.name}"?` : ""}
        description="This removes the workspace and its repo references from GitWave. It does not delete the local repositories themselves."
        size="sm"
      >
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={() => setDeleting(null)}>
            Cancel
          </Button>
          <Button variant="danger" size="sm" onClick={confirmDelete} disabled={deleteMut.isPending}>
            Delete
          </Button>
        </div>
      </Modal>

      <ErrorAlert
        message={error ? formatAppError(error) : null}
      />
      <AiProviderSettings
        workspaceId={aiWorkspace?.id ?? null}
        workspaceName={aiWorkspace?.name}
        open={aiWorkspace !== null}
        onOpenChange={(open) => {
          if (!open) setAiWorkspace(null);
        }}
      />
    </>
  );
}
