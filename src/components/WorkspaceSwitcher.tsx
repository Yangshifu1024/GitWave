import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

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
import { useValidatedWorkspaceSwitch } from "@/hooks/useValidatedWorkspaceSwitch";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ListItem } from "@/components/ui/ListItem";
import { Modal } from "@/components/ui/Modal";
import { EmptyState } from "@/components/ui/EmptyState";
import { FolderPlus, Pencil, Trash2 } from "lucide-react";

export function WorkspaceSwitcher(): React.JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const activeId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const selectWorkspace = useWorkspaceUiStore((s) => s.selectWorkspace);
  const switchWorkspace = useValidatedWorkspaceSwitch();

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
          {t("workspace.title")}
        </h2>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setShowCreate(true)}
          aria-label={t("workspace.new")}
        >
          <FolderPlus size={14} />
        </Button>
      </div>

      {/* List */}
      {isLoading ? (
        <p className="px-3 py-2 text-sm text-text-muted">{t("common.loading")}</p>
      ) : error ? (
        <p className="px-3 py-2 text-sm text-danger">
          {t("workspace.loadFailed", { error: formatAppError(error) })}
        </p>
      ) : workspaces.length === 0 ? (
        <EmptyState
          title={t("workspace.empty.title")}
          description={t("workspace.empty.description")}
          action={
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(true)}>
              {t("workspace.new")}
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
                onClick={() => switchWorkspace(ws.id, ws.last_active_repo_id)}
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
                      aria-label={t("workspace.itemRenameAria", { name: ws.name })}
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
                      aria-label={t("workspace.itemDeleteAria", { name: ws.name })}
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
          title={t("workspace.new")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setShowCreate(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                onClick={submitCreate}
                disabled={!createName.trim() || createMut.isPending}
              >
                {t("workspace.create.submit")}
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
              placeholder={t("workspace.create.namePlaceholder")}
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
          title={t("workspace.rename.title", { name: renaming.name })}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setRenaming(null)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                onClick={submitRename}
                disabled={!renameValue.trim() || renameMut.isPending}
              >
                {t("common.save")}
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
          title={t("workspace.delete.title", { name: deleting.name })}
          description={t("workspace.delete.description")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setDeleting(null)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                variant="danger"
                size="sm"
                className="min-w-0 flex-[7]"
                onClick={() => deleteMut.mutate(deleting.id)}
                disabled={deleteMut.isPending}
              >
                {t("common.delete")}
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
