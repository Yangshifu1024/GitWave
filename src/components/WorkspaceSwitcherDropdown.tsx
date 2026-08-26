// Topbar workspace switcher — Radix DropdownMenu listing all workspaces
// with the active one highlighted, plus a "New workspace…" entry that
// opens the create modal inline. Replaces the previous non-functional
// placeholder button in App.tsx so users can verify CRUD via the topbar.

import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  createWorkspace,
  formatAppError,
  listWorkspaces,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Check, ChevronDown, FolderPlus } from "lucide-react";

export function WorkspaceSwitcherDropdown(): React.JSX.Element {
  const queryClient = useQueryClient();
  const activeId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const setActiveId = useWorkspaceUiStore((s) => s.setActiveWorkspaceId);

  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createError, setCreateError] = useState<string | null>(null);

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

  const activeWs = workspaces.find((w) => w.id === activeId);
  const triggerLabel = activeWs?.name ?? "Select workspace";

  function submitCreate(): void {
    const name = createName.trim();
    if (!name) return;
    setCreateError(null);
    createMut.mutate(name);
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
            className="min-w-[220px] rounded-md border border-border-default bg-bg-elevated p-1 shadow-modal z-popover"
          >
            <DropdownMenu.Label className="px-2 py-1 text-xs font-semibold uppercase tracking-wide text-text-muted">
              Workspaces
            </DropdownMenu.Label>

            {isLoading ? (
              <DropdownMenu.Item
                disabled
                className="px-2 py-1 text-sm text-text-muted"
              >
                Loading…
              </DropdownMenu.Item>
            ) : error ? (
              <DropdownMenu.Item
                disabled
                className="px-2 py-1 text-sm text-danger"
              >
                {formatAppError(error)}
              </DropdownMenu.Item>
            ) : workspaces.length === 0 ? (
              <DropdownMenu.Item
                disabled
                className="px-2 py-1 text-sm text-text-muted italic"
              >
                No workspaces yet
              </DropdownMenu.Item>
            ) : (
              workspaces.map((ws) => {
                const isActive = ws.id === activeId;
                return (
                  <DropdownMenu.Item
                    key={ws.id}
                    onSelect={() => setActiveId(ws.id)}
                    className="flex items-center gap-2 px-2 py-1 text-sm rounded-sm cursor-pointer outline-none data-[highlighted]:bg-bg-secondary"
                  >
                    <span className="w-4 shrink-0 text-accent">
                      {isActive ? <Check size={14} /> : null}
                    </span>
                    <span className="truncate">{ws.name}</span>
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
          <Button
            variant="secondary"
            size="sm"
            onClick={() => setCreateOpen(false)}
          >
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
    </>
  );
}