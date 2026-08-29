import { useCallback, useEffect, useState } from "react";
import { Menu, Popover } from "@heroui/react";

import {
  addRemote,
  fetchRemote,
  formatAppError,
  listRemoteDetails,
  removeRemote,
  renameRemote,
  setRemotePushUrl,
  setRemoteUrl,
  type RemoteInfo,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { SidebarSection } from "@/components/ui/SidebarSection";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/ContextMenu";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useSyncStore } from "@/stores/syncStore";
import { Cloud } from "lucide-react";

/**
 * Sidebar card: configured remotes with full CRUD. Fetch per remote reuses
 * the normal fetch pipeline (credentials via the system git credential
 * helper). Adding a remote lives in the header's right-click menu — an
 * empty list collapses the card to a static header.
 */
export function RemotesPanel(): React.JSX.Element {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const setStatus = useStatusAreaStore((s) => s.setStatus);
  const startOp = useSyncStore((s) => s.startOp);
  const endOp = useSyncStore((s) => s.endOp);

  const [items, setItems] = useState<RemoteInfo[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [addName, setAddName] = useState("");
  const [addUrl, setAddUrl] = useState("");
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  const [edit, setEdit] = useState<RemoteInfo | null>(null);
  const [editFetchUrl, setEditFetchUrl] = useState("");
  const [editPushUrl, setEditPushUrl] = useState("");
  const [editNewName, setEditNewName] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<RemoteInfo | null>(null);

  // epoch in deps: auto-refresh bumps it to re-run this manual effect.
  const historyEpoch = useWorkspaceUiStore((s) => s.historyEpoch);

  const refresh = useCallback(async () => {
    void historyEpoch; // re-run trigger: auto-refresh bumps the epoch.
    if (!workspaceId || !repoId) {
      setItems([]);
      return;
    }
    setItems(await listRemoteDetails(workspaceId));
    setError(null);
  }, [workspaceId, repoId, historyEpoch]);

  useEffect(() => {
    refresh().catch((e) => setError(formatAppError(e)));
  }, [refresh]);

  if (!workspaceId || !repoId) return <></>;

  const run = async (key: string, fn: () => Promise<void>, success?: string): Promise<void> => {
    if (busy) return;
    setBusy(key);
    setError(null);
    startOp("remote-op");
    try {
      await fn();
      if (success) setStatus(success);
      await refresh();
    } catch (e) {
      setStatus(formatAppError(e), "danger");
    } finally {
      setBusy(null);
      endOp("remote-op");
    }
  };

  const submitAdd = (): void => {
    if (!addName.trim() || !addUrl.trim()) return;
    void run(
      "add",
      async () => {
        await addRemote(workspaceId, addName.trim(), addUrl.trim());
        setAddName("");
        setAddUrl("");
        setAddOpen(false);
      },
      `Remote "${addName.trim()}" added`,
    );
  };

  const submitEdit = (): void => {
    if (!edit || busy) return;
    const name = edit.name;
    void run(
      `edit-${name}`,
      async () => {
        if (editFetchUrl.trim()) await setRemoteUrl(workspaceId, name, editFetchUrl.trim());
        await setRemotePushUrl(workspaceId, name, editPushUrl.trim() || null);
        if (editNewName.trim() && editNewName.trim() !== name) {
          await renameRemote(workspaceId, name, editNewName.trim());
        }
        setEdit(null);
      },
      `Remote "${name}" updated`,
    );
  };

  return (
    <>
      <SidebarSection
        title="Remotes"
        collapsible={items.length > 0}
        onHeaderContextMenu={(e) => {
          e.preventDefault();
          setMenu({ x: e.clientX, y: e.clientY });
        }}
      >
        <div className="flex flex-col gap-1 px-1 pb-1">
          {error ? <p className="px-2 text-xs text-danger">{error}</p> : null}
          {items.length === 0 ? (
            <p className="px-2 py-1 text-xs text-text-muted italic">No remotes</p>
          ) : (
            items.map((r) => (
              <ContextMenu key={r.name}>
                <ContextMenuTrigger asChild>
                  <div
                    className="group flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-bg-elevated"
                    title={r.fetch_url ?? undefined}
                  >
                    <Cloud size={13} className="shrink-0 text-text-muted" />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-xs font-medium text-text-primary">{r.name}</p>
                      <p className="truncate text-[11px] text-text-muted">
                        {r.fetch_url ?? "(no URL)"}
                      </p>
                    </div>
                  </div>
                </ContextMenuTrigger>
                <ContextMenuContent className="max-w-[240px]">
                  <ContextMenuLabel title={r.fetch_url ?? undefined}>{r.name}</ContextMenuLabel>
                  <ContextMenuSeparator />
                  <ContextMenuItem
                    disabled={busy !== null}
                    onSelect={() =>
                      void run(
                        `fetch-${r.name}`,
                        async () => {
                          await fetchRemote(workspaceId, r.name);
                          bumpHistory(); // remote-tracking refs feed the history graph
                        },
                        `Fetched from ${r.name}`,
                      )
                    }
                  >
                    Fetch
                  </ContextMenuItem>
                  <ContextMenuItem
                    disabled={busy !== null}
                    onSelect={() => {
                      setEdit(r);
                      setEditFetchUrl(r.fetch_url ?? "");
                      setEditPushUrl(r.push_url ?? "");
                      setEditNewName(r.name);
                    }}
                  >
                    Edit…
                  </ContextMenuItem>
                  <ContextMenuSeparator />
                  <ContextMenuItem
                    destructive
                    disabled={busy !== null}
                    onSelect={() => setDeleteTarget(r)}
                  >
                    Remove
                  </ContextMenuItem>
                </ContextMenuContent>
              </ContextMenu>
            ))
          )}
        </div>
      </SidebarSection>

      {/* Group-level actions live in the header's right-click menu. */}
      {menu ? (
        <Popover isOpen onOpenChange={(o) => !o && setMenu(null)}>
          <Popover.Trigger
            aria-hidden
            className="fixed z-popover h-px w-px overflow-hidden p-0 pointer-events-none"
            style={{ left: menu.x, top: menu.y }}
          />
          <Popover.Content
            placement="bottom start"
            offset={2}
            className="z-popover min-w-[160px] rounded-lg bg-bg-elevated border border-border-default shadow-modal p-1"
          >
            <Menu className="outline-none">
              <Menu.Item
                textValue="Add remote"
                onAction={() => {
                  setAddName("");
                  setAddUrl("");
                  setAddOpen(true);
                  setMenu(null);
                }}
                className="relative flex cursor-pointer select-none items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none text-text-primary"
              >
                Add remote
              </Menu.Item>
            </Menu>
          </Popover.Content>
        </Popover>
      ) : null}

      <Modal
        open={addOpen}
        onOpenChange={(o) => !o && setAddOpen(false)}
        title="Add remote"
        size="sm"
      >
        <div className="flex flex-col gap-2">
          <Input value={addName} onChange={setAddName} placeholder="name (e.g. origin)" autoFocus />
          <Input
            value={addUrl}
            onChange={setAddUrl}
            placeholder="URL (https:// or git@…)"
            onKeyDown={(e) => {
              if (e.key === "Enter") submitAdd();
            }}
          />
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setAddOpen(false)}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              disabled={busy !== null || !addName.trim() || !addUrl.trim()}
              onClick={submitAdd}
            >
              Add
            </Button>
          </div>
        </div>
      </Modal>

      {edit ? (
        <Modal
          open
          onOpenChange={(o) => !o && setEdit(null)}
          title={`Edit remote "${edit.name}"`}
          size="sm"
        >
          <div className="flex flex-col gap-2">
            <Input value={editNewName} onChange={setEditNewName} placeholder="name" />
            <Input value={editFetchUrl} onChange={setEditFetchUrl} placeholder="fetch URL" />
            <Input
              value={editPushUrl}
              onChange={setEditPushUrl}
              placeholder="push URL (empty = same as fetch)"
            />
            <div className="flex justify-end gap-2">
              <Button variant="secondary" size="sm" onClick={() => setEdit(null)}>
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                disabled={busy !== null || !editNewName.trim() || !editFetchUrl.trim()}
                onClick={submitEdit}
              >
                Save
              </Button>
            </div>
          </div>
        </Modal>
      ) : null}

      {deleteTarget ? (
        <Modal
          open
          onOpenChange={(o) => !o && setDeleteTarget(null)}
          title={`Remove remote "${deleteTarget.name}"?`}
          description="The remote configuration is removed. Local branches and commits are not touched."
          size="sm"
        >
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setDeleteTarget(null)}>
              Cancel
            </Button>
            <Button
              variant="danger"
              size="sm"
              disabled={busy !== null}
              onClick={() => {
                const name = deleteTarget.name;
                void run(
                  `remove-${name}`,
                  async () => {
                    await removeRemote(workspaceId, name);
                    setDeleteTarget(null);
                  },
                  `Remote "${name}" removed`,
                );
              }}
            >
              Remove
            </Button>
          </div>
        </Modal>
      ) : null}
    </>
  );
}
