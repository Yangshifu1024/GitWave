// Global command palette (Cmd+K / Ctrl+K). Two layers:
//  1. Static commands — navigation and quick ops, no AI needed.
//  2. "Ask AI" — the typed request is interpreted into ONE whitelisted
//     action (cmd_ai_palette_intent). Mutating actions show a confirm card
//     before executing; commit / push / merge / rebase are rejected
//     server-side and never executable from here (P1).

import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ArrowRight, CornerDownLeft, Download, Search, Settings, Sparkles } from "lucide-react";
import {
  aiPaletteIntent,
  checkoutBranch,
  createBranch,
  createTag,
  fetchRemote,
  formatAppError,
  getWorkingCopy,
  saveStash,
  type PaletteIntent,
} from "@/lib/api";
import { useUiStore } from "@/stores/uiStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useSyncStore } from "@/stores/syncStore";
import { CommitExplainModal } from "@/components/CommitExplainModal";
import { cn } from "@/lib/utils";

const ACTION_LABEL_KEYS: Record<string, string> = {
  explain_commit: "palette.action.explain_commit",
  locate_commit: "palette.action.locate_commit",
  create_branch: "palette.action.create_branch",
  checkout_branch: "palette.action.checkout_branch",
  create_tag: "palette.action.create_tag",
  stash_changes: "palette.action.stash_changes",
  fetch_remotes: "palette.action.fetch_remotes",
  none: "palette.action.none",
};

export function CommandPalette({
  requestLocate,
}: {
  requestLocate: (sha: string) => void;
}): React.JSX.Element | null {
  const { t } = useTranslation();
  const open = useUiStore((s) => s.paletteOpen);
  const setOpen = useUiStore((s) => s.setPaletteOpen);
  const setSettingsOpen = useUiStore((s) => s.setSettingsOpen);
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const queryClient = useQueryClient();
  const setStatus = useStatusAreaStore((s) => s.setStatus);

  const [query, setQuery] = useState("");
  const [intent, setIntent] = useState<PaletteIntent | null>(null);
  const [intentError, setIntentError] = useState<string | null>(null);
  const [explain, setExplain] = useState<{ sha: string } | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  // App-standard global shortcut, same modifier check as Toolbar's Ctrl+,.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent): void => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen(!useUiStore.getState().paletteOpen);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [setOpen]);

  // Fresh state every time the palette opens; focus the input.
  useEffect(() => {
    if (!open) return;
    setQuery("");
    setIntent(null);
    setIntentError(null);
    // Focus after mount so the input is attached.
    window.setTimeout(() => inputRef.current?.focus(), 0);
  }, [open]);

  const staticCommands = useMemo(
    () => [
      {
        id: "settings",
        label: t("palette.command.openSettings"),
        icon: <Settings size={14} />,
        hint: "Ctrl+,",
        run: () => {
          setOpen(false);
          setSettingsOpen(true);
        },
      },
      {
        id: "fetch",
        label: t("palette.command.fetchRemote"),
        icon: <Download size={14} />,
        hint: "",
        run: () => {
          if (!workspaceId || useSyncStore.getState().isBusy()) return;
          setOpen(false);
          // Own lifecycle: the backend emits sync-progress for any fetch, so
          // startOp/endOp here keep the status area from sticking in "sync".
          const sync = useSyncStore.getState();
          sync.startOp("fetch");
          fetchRemote(workspaceId)
            .then(() => setStatus(t("status.fetchComplete")))
            .catch((e) => setStatus(formatAppError(e), "danger"))
            .finally(() => sync.endOp("fetch"));
        },
      },
    ],
    [workspaceId, setOpen, setSettingsOpen, setStatus, t],
  );

  const filtered = staticCommands.filter((c) =>
    c.label.toLowerCase().includes(query.trim().toLowerCase()),
  );

  const askAi = useMutation({
    mutationFn: () => {
      if (!workspaceId) throw new Error(t("palette.noWorkspace"));
      return aiPaletteIntent(workspaceId, query);
    },
    onSuccess: (res) => {
      setIntent(res);
      setIntentError(null);
    },
    onError: (e) => {
      setIntent(null);
      setIntentError(formatAppError(e));
    },
  });

  const execute = (next: PaletteIntent): void => {
    if (!workspaceId) return;
    const params = next.params as Record<string, string | undefined>;
    void (async () => {
      try {
        switch (next.action) {
          case "explain_commit": {
            setOpen(false);
            setExplain({ sha: params.sha ?? "" });
            return;
          }
          case "locate_commit": {
            setOpen(false);
            requestLocate(params.sha ?? "");
            return;
          }
          case "create_branch": {
            const from =
              params.from?.trim() || (await getWorkingCopy(workspaceId)).sha || undefined;
            if (!from) throw new Error(t("palette.noBaseCommit"));
            await createBranch(workspaceId, params.name ?? "", from);
            bumpHistory();
            setStatus(t("status.createdBranch", { name: params.name }));
            break;
          }
          case "checkout_branch": {
            await checkoutBranch(workspaceId, params.name ?? "", false);
            bumpHistory();
            void queryClient.invalidateQueries({ queryKey: ["working-copy"] });
            setStatus(t("status.checkedOut", { name: params.name }));
            break;
          }
          case "create_tag": {
            const target = params.sha?.trim() || (await getWorkingCopy(workspaceId)).sha || null;
            await createTag(workspaceId, params.name ?? "", target || null, null);
            bumpHistory();
            setStatus(t("status.createdTag", { name: params.name }));
            break;
          }
          case "stash_changes": {
            await saveStash(workspaceId, params.message || undefined);
            void queryClient.invalidateQueries({ queryKey: ["working-copy"] });
            setStatus(t("status.changesStashed"));
            break;
          }
          case "fetch_remotes": {
            if (!useSyncStore.getState().isBusy()) {
              const sync = useSyncStore.getState();
              sync.startOp("fetch");
              try {
                await fetchRemote(workspaceId);
                setStatus(t("status.fetchComplete"));
              } finally {
                sync.endOp("fetch");
              }
            }
            break;
          }
          case "none": {
            setStatus(next.explanation || t("palette.noMatch"), "info");
            break;
          }
        }
        setOpen(false);
      } catch (e) {
        const message = e instanceof Error ? e.message : formatAppError(e);
        setIntentError(message);
      }
    })();
  };

  if (!open) return null;

  const showResults = query.trim().length === 0 || filtered.length > 0;
  const actionLabelKey = ACTION_LABEL_KEYS[intent?.action ?? ""];

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/40 pt-[12vh] animate-in fade-in-0"
      onMouseDown={() => setOpen(false)}
    >
      <div
        className="w-[560px] max-w-[92vw] overflow-hidden rounded-xl border border-border-subtle bg-bg-overlay shadow-xl animate-in zoom-in-95"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (query.trim() && !askAi.isPending) {
              setIntent(null);
              setIntentError(null);
              askAi.mutate();
            }
          }}
        >
          <div className="flex items-center gap-2 border-b border-border-subtle px-3 py-2">
            <Search size={15} className="shrink-0 text-text-muted" />
            <Input
              ref={inputRef}
              value={query}
              onChange={(v) => {
                setQuery(v);
                setIntent(null);
                setIntentError(null);
              }}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  e.stopPropagation();
                  setOpen(false);
                }
              }}
              placeholder={t("palette.placeholder")}
              variant="search"
            />
            {query.trim() ? (
              <Button type="submit" variant="primary" size="sm" disabled={askAi.isPending}>
                <Sparkles size={13} />
                {askAi.isPending ? t("palette.thinking") : t("palette.askAi")}
              </Button>
            ) : null}
          </div>
        </form>

        <div className="max-h-[46vh] overflow-y-auto">
          {askAi.isPending ? (
            <p className="px-3 py-3 text-xs text-text-muted">{t("palette.interpreting")}</p>
          ) : null}

          {intent ? (
            <div className="border-b border-border-subtle px-3 py-2.5">
              <p className="flex items-center gap-1.5 text-xs font-semibold text-text-primary">
                <ArrowRight size={13} />
                {actionLabelKey ? t(actionLabelKey) : intent.action}
                {intent.requires_confirm ? (
                  <span className="rounded bg-bg-secondary px-1 text-[10px] font-medium uppercase text-text-muted">
                    {t("palette.needsConfirmation")}
                  </span>
                ) : null}
              </p>
              <p className="mt-0.5 text-xs text-text-secondary">
                {intent.explanation || t("palette.runAction")}
              </p>
              {Object.keys(intent.params).length > 0 ? (
                <p className="mt-0.5 font-mono text-[11px] text-text-muted">
                  {Object.entries(intent.params)
                    .map(([k, v]) => `${k}: ${String(v)}`)
                    .join(" · ")}
                </p>
              ) : null}
              <div className="mt-1.5 flex justify-end gap-2">
                <Button variant="secondary" size="sm" onClick={() => setIntent(null)}>
                  {t("common.cancel")}
                </Button>
                <Button variant="primary" size="sm" onClick={() => execute(intent)}>
                  {intent.requires_confirm ? t("common.confirm") : t("palette.run")}
                </Button>
              </div>
            </div>
          ) : null}

          {intentError ? (
            <p className="border-b border-border-subtle px-3 py-2 text-xs text-danger">
              {intentError}
            </p>
          ) : null}

          {showResults && !askAi.isPending ? (
            <div className="py-1">
              {filtered.length > 0 ? (
                filtered.map((c) => (
                  <button
                    key={c.id}
                    type="button"
                    onClick={c.run}
                    className={cn(
                      "flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs text-text-secondary",
                      "hover:bg-bg-secondary hover:text-text-primary",
                    )}
                  >
                    {c.icon}
                    <span className="flex-1">{c.label}</span>
                    {c.hint ? (
                      <span className="font-mono text-[10px] text-text-muted">{c.hint}</span>
                    ) : null}
                  </button>
                ))
              ) : (
                <p className="px-3 py-2 text-xs text-text-muted">
                  <Sparkles size={11} className="mr-1 inline" />
                  {t("palette.askHint")}
                </p>
              )}
              {query.trim() ? (
                <p className="flex items-center gap-1.5 px-3 py-1.5 text-[11px] text-text-muted">
                  <CornerDownLeft size={11} />
                  {t("palette.enterAsks")}
                </p>
              ) : null}
            </div>
          ) : null}
        </div>

        {explain && workspaceId ? (
          <CommitExplainModal
            workspaceId={workspaceId}
            sha={explain.sha}
            open
            onClose={() => setExplain(null)}
          />
        ) : null}
      </div>
    </div>
  );
}
