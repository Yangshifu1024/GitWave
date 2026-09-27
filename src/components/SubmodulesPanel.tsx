// Sidebar section listing `.gitmodules` entries with init / update
// (recursive) / deinit actions and an inline "add submodule" form.

import { useCallback, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import {
  addSubmodule,
  deinitSubmodule,
  formatAppError,
  initSubmodule,
  isCancelledSyncError,
  listSubmodules,
  updateSubmodule,
} from "@/lib/api";
import { remoteHost, withAuthRetry } from "@/lib/authRetry";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { Blocks, CircleAlert, CircleCheck, Plus } from "lucide-react";

export function SubmodulesPanel(): React.JSX.Element {
  const { t } = useTranslation();
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const setStatus = useStatusAreaStore((s) => s.setStatus);

  const [busy, setBusy] = useState<string | null>(null);
  const [actionError, setError] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const [url, setUrl] = useState("");
  const [path, setPath] = useState("");
  /** Submodule awaiting deinit confirmation (clears its worktree config). */
  const [pendingDeinit, setPendingDeinit] = useState<string | null>(null);

  // epoch in deps: auto-refresh bumps it to re-run this manual effect.
  const historyEpoch = useWorkspaceUiStore((s) => s.historyEpoch);

  const query = useQuery({
    queryKey: ["submodules", workspaceId, repoId, historyEpoch],
    queryFn: () => listSubmodules(workspaceId!),
    enabled: Boolean(workspaceId && repoId),
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[1] === workspaceId && previousQuery.queryKey[2] === repoId
        ? previous
        : undefined,
  });
  const refreshScope = JSON.stringify([workspaceId, repoId, historyEpoch]);
  const [previousScope, setPreviousScope] = useState(refreshScope);
  if (previousScope !== refreshScope) {
    setPreviousScope(refreshScope);
    setError(null);
  }
  const items = query.data ?? [];
  const error = actionError ?? (query.error ? formatAppError(query.error) : null);
  const { refetch } = query;
  const refresh = useCallback(async () => {
    await refetch({ throwOnError: true });
    setError(null);
  }, [refetch]);

  const run = async (name: string, op: "init" | "update"): Promise<void> => {
    if (!workspaceId || busy) return;
    setBusy(name);
    setError(null);
    try {
      if (op === "init") {
        await initSubmodule(workspaceId, name);
      } else {
        // F012: an auth-challenged submodule remote opens the in-app prompt
        // and retries with the entered credentials (at most one prompt).
        const host = remoteHost(items.find((i) => i.name === name)?.url ?? "") || name;
        await withAuthRetry(host, (auth) => updateSubmodule(workspaceId, name, true, auth));
      }
      setStatus(
        t(op === "init" ? "submodules.status.initialized" : "submodules.status.updated", { name }),
      );
      bumpHistory();
      await refresh();
    } catch (e) {
      // A dismissed auth prompt is a user cancel, not a failure.
      if (isCancelledSyncError(e)) {
        setStatus(t("status.sync.cancelled"));
      } else {
        setStatus(formatAppError(e), "danger");
      }
    } finally {
      setBusy(null);
    }
  };

  const submitAdd = async (): Promise<void> => {
    if (!workspaceId || busy) return;
    setBusy("add");
    setError(null);
    try {
      await withAuthRetry(remoteHost(url.trim()) || url.trim(), (auth) =>
        addSubmodule(workspaceId, url.trim(), path.trim(), auth),
      );
      setStatus(t("submodules.status.added", { path: path.trim() }));
      setUrl("");
      setPath("");
      setAdding(false);
      await refresh();
    } catch (e) {
      if (isCancelledSyncError(e)) {
        setStatus(t("status.sync.cancelled"));
      } else {
        setStatus(formatAppError(e), "danger");
      }
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
      setStatus(t("submodules.status.deactivated", { name: pendingDeinit }));
      setPendingDeinit(null);
      await refresh();
    } catch (e) {
      setStatus(formatAppError(e), "danger");
    } finally {
      setBusy(null);
    }
  };

  if (!workspaceId || !repoId) return <></>;

  return (
    <SidebarSection title={t("submodules.title")} collapsible={items.length > 0}>
      <div className="flex flex-col gap-1 px-1">
        {error ? <p className="px-2 text-xs text-danger">{error}</p> : null}
        {items.length === 0 ? (
          <p className="px-2 py-1 text-xs text-text-muted italic">{t("submodules.empty")}</p>
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
                  {t("submodules.actions.init")}
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
                    {sm.in_sync ? t("submodules.actions.update") : t("submodules.actions.sync")}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={busy !== null}
                    className="text-text-muted hover:text-danger"
                    title="git submodule deinit"
                    onClick={() => setPendingDeinit(sm.name)}
                  >
                    {t("submodules.actions.deinit")}
                  </Button>
                </>
              )}
            </div>
          ))
        )}

        {adding ? (
          <div className="flex flex-col gap-1 rounded-md border border-border-subtle p-1.5">
            <Input value={url} onChange={setUrl} placeholder={t("submodules.add.urlPlaceholder")} />
            <Input
              value={path}
              onChange={setPath}
              placeholder={t("submodules.add.pathPlaceholder")}
            />
            <div className="flex justify-end gap-1">
              <Button
                variant="ghost"
                size="sm"
                disabled={busy !== null}
                onClick={() => setAdding(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                variant="secondary"
                size="sm"
                disabled={busy !== null || !url.trim() || !path.trim()}
                onClick={() => void submitAdd()}
              >
                {t("common.add")}
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
            {t("submodules.add.button")}
          </Button>
        )}

        {pendingDeinit ? (
          <Modal
            open
            onOpenChange={(open) => {
              if (!open) setPendingDeinit(null);
            }}
            title={t("submodules.deinit.title", { name: pendingDeinit })}
            description={t("submodules.deinit.description")}
            size="sm"
            footer={
              <>
                <Button
                  variant="secondary"
                  size="sm"
                  className="min-w-0 flex-[3]"
                  onClick={() => setPendingDeinit(null)}
                >
                  {t("common.cancel")}
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  className="min-w-0 flex-[7]"
                  disabled={busy !== null}
                  onClick={() => void confirmDeinit()}
                >
                  {t("submodules.actions.deinit")}
                </Button>
              </>
            }
          />
        ) : null}
      </div>
    </SidebarSection>
  );
}
