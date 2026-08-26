// Topbar workspace switcher — Radix DropdownMenu listing all workspaces
// with the active one highlighted, plus create / rename / delete actions.
// Replaces the previous non-functional placeholder + restores the rename
// and delete operations that Day 4 restructure had inadvertently dropped.

import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
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
import { Modal } from "@/components/ui/Modal";
import { Check, ChevronDown, FolderPlus, Pencil, Trash2 } from "lucide-react";

export function WorkspaceSwitcherDropdown(): React.JSX.Element {
  const queryClient = useQueryClient();
  const activeId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const setActiveId = useWorkspaceUiStore((s) => s.setActiveWorkspaceId);

  // ── Create ──
  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createError, setCreateError] = useState<string | null>(null);

  // ── Rename ──
  const [renaming, setRenaming] = useState<WorkspaceSummary | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [renameError, setRenameError] = useState<string | null>(null);

  // ── Delete ──
  const [deleting, setDeleting] = useState<WorkspaceSummary | null>(null);

  const {
    data: workspaces = [],
    isLoading,
    error,
  } = useQuery({
    queryKey: ["workspaces"],
    queryFn: listWorkspaces,
  });

  const activeWs = workspaces.find((w) => w.id === activeId);
  const triggerLabel = activeWs?.name ?? "Select workspace";

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
      if (activeId === id) setActiveId(null);
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

  return (
    <>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="gap-1 px-2 min-w-[180px] justify-between"
            aria-label="Switch workspace"
          >
            <span className="truncate">{triggerLabel}</span>
            <ChevronDown size={14} />
          </Button>
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content
            align="start"
            sideOffset={4}
            className="min-w-[260px] rounded-md border border-border-default bg-bg-elevated p-1 shadow-modal z-popover"
          >
            <DropdownMenu.Label className="px-2 py-1 text-xs font-semibold uppercase tracking-wide text-text-muted">
              Workspaces
            </DropdownMenu.Label>

            {isLoading ? (
              <DropdownMenu.Item disabled className="px-2 py-1 text-sm text-text-muted">
                Loading…
              </DropdownMenu.Item>
            ) : error ? (
              <DropdownMenu.Item disabled className="px-2 py-1 text-sm text-danger">
                {formatAppError(error)}
              </DropdownMenu.Item>
            ) : workspaces.length === 0 ? (
              <DropdownMenu.Item disabled className="px-2 py-1 text-sm text-text-muted italic">
                No workspaces yet
              </DropdownMenu.Item>
            ) : (
              workspaces.map((ws) => {
                const isActive = ws.id === activeId;
                return (
                  <DropdownMenu.Item
                    key={ws.id}
                    onSelect={() => setActiveId(ws.id)}
                    className="group flex items-center gap-2 px-2 py-1 text-sm rounded-sm cursor-pointer outline-none data-[highlighted]:bg-bg-secondary"
                  >
                    <span className="w-4 shrink-0 text-accent">
                      {isActive ? <Check size={14} /> : null}
                    </span>
                    <span className="flex-1 truncate">{ws.name}</span>
                    {/* Hover-revealed actions; onSelect closes the menu so we
                        prevent that with onClick + e.preventDefault. */}
                    <button
                      type="button"
                      aria-label={`Rename ${ws.name}`}
                      onClick={(e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        openRename(ws);
                      }}
                      className="opacity-0 group-hover:opacity-100 group-data-[highlighted]:opacity-100 text-text-secondary hover:text-accent p-1 rounded-sm"
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
                      className="opacity-0 group-hover:opacity-100 group-data-[highlighted]:opacity-100 text-text-secondary hover:text-danger p-1 rounded-sm"
                    >
                      <Trash2 size={13} />
                    </button>
                  </DropdownMenu.Item>
                );
              })
            )}

            <DropdownMenu.Separator className="my-1 h-px bg-border-subtle" />

            <DropdownMenu.Item
              onSelect={() => setCreateOpen(true)}
              className="flex items-center gap-2 px-2 py-1 text-sm rounded-sm cursor-pointer outline-none data-[highlighted]:bg-bg-secondary"
            >
              <FolderPlus size={14} />
              <span>New workspace…</span>
            </DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>

      {/* ── Create modal ──────────────────────────────────────────────── */}
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

      {/* ── Rename modal ──────────────────────────────────────────────── */}
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

      {/* ── Delete confirm modal ───────────────────────────────────────── */}
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
    </>
  );
}
