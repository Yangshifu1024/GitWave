import { useCallback, useEffect, useState } from "react";

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
import { useToast } from "@/components/ui/Toast";
import { Cloud, Pencil, Plus, Trash2 } from "lucide-react";

/**
 * Sidebar section body: configured remotes with full CRUD (M1). Fetch
 * per remote reuses the normal fetch pipeline (credentials via the system
 * git credential helper).
 */
export function RemotesPanel(): React.JSX.Element {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const { toast } = useToast();

  const [items, setItems] = useState<RemoteInfo[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [addName, setAddName] = useState("");
  const [addUrl, setAddUrl] = useState("");
  const [edit, setEdit] = useState<RemoteInfo | null>(null);
  const [editFetchUrl, setEditFetchUrl] = useState("");
  const [editPushUrl, setEditPushUrl] = useState("");
  const [editNewName, setEditNewName] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<RemoteInfo | null>(null);

  const refresh = useCallback(async () => {
    if (!workspaceId || !repoId) {
      setItems([]);
      return;
    }
    setItems(await listRemoteDetails(workspaceId));
  }, [workspaceId, repoId]);

  useEffect(() => {
    refresh().catch((e) => setError(formatAppError(e)));
  }, [refresh]);

  if (!workspaceId || !repoId) return <></>;

  const run = async (key: string, fn: () => Promise<void>, success?: string): Promise<void> => {
    if (busy) return;
    setBusy(key);
    setError(null);
    try {
      await fn();
      if (success) toast({ title: success });
      await refresh();
    } catch (e) {
      toast({ title: formatAppError(e), variant: "danger" });
    } finally {
      setBusy(null);
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
    <div className="flex flex-col gap-1 px-1 pb-1">
      {error ? <p className="px-2 text-xs text-danger">{error}</p> : null}
      {items.length === 0 ? (
        <p className="px-2 py-1 text-xs text-text-muted italic">No remotes</p>
      ) : (
        items.map((r) => (
          <div
            key={r.name}
            className="group flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-bg-elevated"
          >
            <Cloud size={13} className="shrink-0 text-text-muted" />
            <div className="min-w-0 flex-1">
              <p className="truncate text-xs font-medium text-text-primary">{r.name}</p>
              <p className="truncate text-[11px] text-text-muted" title={r.fetch_url ?? undefined}>
                {r.fetch_url ?? "(no URL)"}
              </p>
            </div>
            <div className="flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100">
              <Button
                variant="ghost"
                size="sm"
                disabled={busy !== null}
                title={`Fetch from ${r.name}`}
                onClick={() =>
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
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="p-1"
                aria-label={`Edit remote ${r.name}`}
                title="Edit remote"
                disabled={busy !== null}
                onClick={() => {
                  setEdit(r);
                  setEditFetchUrl(r.fetch_url ?? "");
                  setEditPushUrl(r.push_url ?? "");
                  setEditNewName(r.name);
                }}
              >
                <Pencil size={12} />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="p-1 text-text-muted hover:text-danger"
                aria-label={`Remove remote ${r.name}`}
                title="Remove remote"
                disabled={busy !== null}
                onClick={() => setDeleteTarget(r)}
              >
                <Trash2 size={12} />
              </Button>
            </div>
          </div>
        ))
      )}

      <Button
        variant="ghost"
        size="sm"
        className="self-start px-2 text-text-muted"
        onClick={() => {
          setAddName("");
          setAddUrl("");
          setAddOpen(true);
        }}
      >
        <Plus size={13} />
        Add remote
      </Button>

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
    </div>
  );
}
