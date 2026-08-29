// Sidebar section listing `.gitmodules` entries with init / update
// (recursive) / deinit actions and an inline "add submodule" form.

import { useCallback, useEffect, useState } from "react";

import {
  addSubmodule,
  deinitSubmodule,
  formatAppError,
  initSubmodule,
  listSubmodules,
  updateSubmodule,
  type SubmoduleInfo,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/Toast";
import { Blocks, CircleAlert, CircleCheck, Plus } from "lucide-react";

export function SubmodulesPanel(): React.JSX.Element {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const { toast } = useToast();

  const [items, setItems] = useState<SubmoduleInfo[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const [url, setUrl] = useState("");
  const [path, setPath] = useState("");
  /** Submodule awaiting deinit confirmation (clears its worktree config). */
  const [pendingDeinit, setPendingDeinit] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!workspaceId || !repoId) {
      setItems([]);
      return;
    }
    setItems(await listSubmodules(workspaceId));
  }, [workspaceId, repoId]);

  useEffect(() => {
    refresh().catch((e) => setError(formatAppError(e)));
  }, [refresh]);

  const run = async (name: string, op: "init" | "update"): Promise<void> => {
    if (!workspaceId || busy) return;
    setBusy(name);
    setError(null);
    try {
      if (op === "init") await initSubmodule(workspaceId, name);
      else await updateSubmodule(workspaceId, name, true);
      toast({
        title: `Submodule ${name} ${op === "init" ? "initialized" : "updated"}`,
      });
      bumpHistory();
      await refresh();
    } catch (e) {
      toast({ title: formatAppError(e), variant: "danger" });
    } finally {
      setBusy(null);
    }
  };

  const submitAdd = async (): Promise<void> => {
    if (!workspaceId || busy) return;
    setBusy("add");
    setError(null);
    try {
      await addSubmodule(workspaceId, url.trim(), path.trim());
      toast({
        title: `Submodule added at ${path.trim()} — staged, ready to commit`,
      });
      setUrl("");
      setPath("");
      setAdding(false);
      await refresh();
    } catch (e) {
      toast({ title: formatAppError(e), variant: "danger" });
    } finally {
      setBusy(null);
    }
  };

  const confirmDeinit = async (): Promise<void> => {
    if (!workspaceId || !pendingDeinit || busy) return;
    setBusy(pendingDeinit);
    setError(null);
    try {
      await deinitSubmodule(workspaceId, pendingDeinit);
      toast({ title: `Submodule ${pendingDeinit} deactivated` });
      setPendingDeinit(null);
      await refresh();
    } catch (e) {
      toast({ title: formatAppError(e), variant: "danger" });
    } finally {
      setBusy(null);
    }
  };

  if (!workspaceId || !repoId) return <></>;

  return (
    <SidebarSection title="Submodules" collapsible={items.length > 0}>
      <div className="flex flex-col gap-1 px-1">
      {error ? <p className="px-2 text-xs text-danger">{error}</p> : null}
      {items.length === 0 ? (
        <p className="px-2 py-1 text-xs text-text-muted italic">No submodules</p>
      ) : (
        items.map((sm) => (
          <div
            key={sm.name}
            className="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-bg-elevated"
          >
            <Blocks size={13} className="shrink-0 text-text-muted" />
            <div className="min-w-0 flex-1">
              <p className="truncate text-xs font-medium text-text-primary" title={sm.path}>
                {sm.name}
              </p>
              <p
                className="flex items-center gap-1 truncate text-[11px] text-text-muted"
                title={sm.url ?? undefined}
              >
                {sm.initialized ? (
                  sm.in_sync ? (
                    <CircleCheck size={10} className="shrink-0 text-success" />
                  ) : (
                    <CircleAlert size={10} className="shrink-0 text-warning" />
                  )
                ) : null}
                {sm.url ?? sm.path}
                {sm.head_sha ? ` · ${sm.head_sha.slice(0, 7)}` : ""}
              </p>
            </div>
            {!sm.initialized ? (
              <Button
                variant="ghost"
                size="sm"
                disabled={busy !== null}
                title="git submodule init"
                onClick={() => void run(sm.name, "init")}
              >
                Init
              </Button>
            ) : (
              <>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy !== null}
                  title="git submodule update --init --recursive"
                  onClick={() => void run(sm.name, "update")}
                >
                  {sm.in_sync ? "Update" : "Sync"}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy !== null}
                  className="text-text-muted hover:text-danger"
                  title="git submodule deinit"
                  onClick={() => setPendingDeinit(sm.name)}
                >
                  Deinit
                </Button>
              </>
            )}
          </div>
        ))
      )}

      {adding ? (
        <div className="flex flex-col gap-1 rounded-md border border-border-subtle p-1.5">
          <Input value={url} onChange={setUrl} placeholder="Submodule URL" />
          <Input value={path} onChange={setPath} placeholder="Path, e.g. libs/util" />
          <div className="flex justify-end gap-1">
            <Button
              variant="ghost"
              size="sm"
              disabled={busy !== null}
              onClick={() => setAdding(false)}
            >
              Cancel
            </Button>
            <Button
              variant="secondary"
              size="sm"
              disabled={busy !== null || !url.trim() || !path.trim()}
              onClick={() => void submitAdd()}
            >
              Add
            </Button>
          </div>
        </div>
      ) : (
        <Button
          variant="ghost"
          size="sm"
          className="self-start px-2 text-text-muted"
          disabled={busy !== null}
          onClick={() => setAdding(true)}
        >
          <Plus size={12} />
          Add submodule
        </Button>
      )}

      {pendingDeinit ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setPendingDeinit(null);
          }}
          title={`Deinit submodule ${pendingDeinit}?`}
          description="Unregisters it from .git/config. The checkout is left untouched; the entry stays in .gitmodules. Run Init to reactivate."
          size="sm"
        >
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setPendingDeinit(null)}>
              Cancel
            </Button>
            <Button
              variant="danger"
              size="sm"
              disabled={busy !== null}
              onClick={() => void confirmDeinit()}
            >
              Deinit
            </Button>
          </div>
        </Modal>
      ) : null}
      </div>
    </SidebarSection>
  );
}
