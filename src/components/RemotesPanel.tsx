import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
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
      t("remotes.added", { name: addName.trim() }),
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
      t("remotes.updated", { name }),
    );
  };

  return (
    <>
      <SidebarSection
        title={t("remotes.title")}
        collapsible={items.length > 0}
        onHeaderContextMenu={(e) => {
          e.preventDefault();
          setMenu({ x: e.clientX, y: e.clientY });
        }}
      >
        <div className="flex flex-col gap-1 px-1 pb-1">
          {error ? <p className="px-2 text-xs text-danger">{error}</p> : null}
          {items.length === 0 ? (
            <p className="px-2 py-1 text-xs text-text-muted italic">{t("remotes.empty")}</p>
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
                        {r.fetch_url ?? t("remotes.noUrl")}
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
                        t("remotes.fetched", { name: r.name }),
                      )
                    }
                  >
                    {t("remotes.actions.fetch")}
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
                    {t("remotes.actions.edit")}
                  </ContextMenuItem>
                  <ContextMenuSeparator />
                  <ContextMenuItem
                    destructive
                    disabled={busy !== null}
                    onSelect={() => setDeleteTarget(r)}
                  >
                    {t("common.remove")}
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
                textValue={t("remotes.actions.add")}
                onAction={() => {
                  setAddName("");
                  setAddUrl("");
                  setAddOpen(true);
                  setMenu(null);
                }}
                className="relative flex cursor-pointer select-none items-center gap-2 rounded-md px-2 py-1.5 text-sm outline-none text-text-primary"
              >
                {t("remotes.actions.add")}
              </Menu.Item>
            </Menu>
          </Popover.Content>
        </Popover>
      ) : null}

      <Modal
        open={addOpen}
        onOpenChange={(o) => !o && setAddOpen(false)}
        title={t("remotes.add.title")}
        size="sm"
        footer={
          <>
            <Button
              variant="secondary"
              size="sm"
              className="min-w-0 flex-[3]"
              onClick={() => setAddOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              className="min-w-0 flex-[7]"
              disabled={busy !== null || !addName.trim() || !addUrl.trim()}
              onClick={submitAdd}
            >
              {t("common.add")}
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
          <Input
            value={addName}
            onChange={setAddName}
            placeholder={t("remotes.add.namePlaceholder")}
            autoFocus
          />
          <Input
            value={addUrl}
            onChange={setAddUrl}
            placeholder={t("remotes.add.urlPlaceholder")}
            onKeyDown={(e) => {
              if (e.key === "Enter") submitAdd();
            }}
          />
        </div>
      </Modal>

      {edit ? (
        <Modal
          open
          onOpenChange={(o) => !o && setEdit(null)}
          title={t("remotes.edit.title", { name: edit.name })}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setEdit(null)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy !== null || !editNewName.trim() || !editFetchUrl.trim()}
                onClick={submitEdit}
              >
                {t("common.save")}
              </Button>
            </>
          }
        >
          <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
            <Input
              value={editNewName}
              onChange={setEditNewName}
              placeholder={t("remotes.edit.namePlaceholder")}
            />
            <Input
              value={editFetchUrl}
              onChange={setEditFetchUrl}
              placeholder={t("remotes.edit.fetchPlaceholder")}
            />
            <Input
              value={editPushUrl}
              onChange={setEditPushUrl}
              placeholder={t("remotes.edit.pushPlaceholder")}
            />
          </div>
        </Modal>
      ) : null}

      {deleteTarget ? (
        <Modal
          open
          onOpenChange={(o) => !o && setDeleteTarget(null)}
          title={t("remotes.delete.title", { name: deleteTarget.name })}
          description={t("remotes.delete.description")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setDeleteTarget(null)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                variant="danger"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy !== null}
                onClick={() => {
                  const name = deleteTarget.name;
                  void run(
                    `remove-${name}`,
                    async () => {
                      await removeRemote(workspaceId, name);
                      setDeleteTarget(null);
                    },
                    t("remotes.removed", { name }),
                  );
                }}
              >
                {t("common.remove")}
              </Button>
            </>
          }
        />
      ) : null}
    </>
  );
}
