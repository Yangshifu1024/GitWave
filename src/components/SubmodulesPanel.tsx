import { useCallback, useEffect, useState } from "react";

import {
  formatAppError,
  initSubmodule,
  listSubmodules,
  updateSubmodule,
  type SubmoduleInfo,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/Toast";
import { Blocks } from "lucide-react";

/** Sidebar section listing `.gitmodules` entries with init / update actions. */
export function SubmodulesPanel(): React.JSX.Element {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const { toast } = useToast();

  const [items, setItems] = useState<SubmoduleInfo[]>([]);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

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
      else await updateSubmodule(workspaceId, name);
      toast({ title: `Submodule ${name} ${op === "init" ? "initialized" : "updated"}` });
      await refresh();
    } catch (e) {
      toast({ title: formatAppError(e), variant: "danger" });
    } finally {
      setBusy(null);
    }
  };

  if (!workspaceId || !repoId) return <></>;

  return (
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
              <p className="truncate text-[11px] text-text-muted" title={sm.url ?? undefined}>
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
              <Button
                variant="ghost"
                size="sm"
                disabled={busy !== null}
                title="git submodule update --init"
                onClick={() => void run(sm.name, "update")}
              >
                Update
              </Button>
            )}
          </div>
        ))
      )}
    </div>
  );
}
