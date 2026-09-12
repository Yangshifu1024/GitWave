// Global command palette (Cmd+K / Ctrl+K). Three layers:
//  1. Static commands — navigation and quick ops, no AI needed.
//  2. Commit search — the typed query also matches commit message/author
//     (backend filter of cmd_get_commit_log); selecting a result locates
//     that commit in the History graph via requestLocate.
//  3. "Ask AI" — the typed request is interpreted into ONE whitelisted
//     action (cmd_ai_palette_intent). Mutating actions show a confirm card
//     before executing; commit / push / merge / rebase are rejected
//     server-side and never executable from here (P1).

import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  ArrowRight,
  CornerDownLeft,
  Download,
  GitCommitHorizontal,
  Settings,
  Sparkles,
} from "lucide-react";
import {
  aiPaletteIntent,
  checkoutBranch,
  createBranch,
  createTag,
  fetchRemote,
  formatAppError,
  getCommitLog,
  getWorkingCopy,
  interactiveRebasePaused,
  mergeInProgress,
  saveStash,
  type CommitSummary,
  type PaletteIntent,
} from "@/lib/api";
import { gateCheckout } from "@/lib/checkoutGate";
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
  const [commitResults, setCommitResults] = useState<CommitSummary[]>([]);
  const [commitsLoading, setCommitsLoading] = useState(false);
  const [commitsError, setCommitsError] = useState<string | null>(null);
  const [selectedIndex, setSelectedIndex] = useState(-1);
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
    setCommitResults([]);
    setCommitsLoading(false);
    setCommitsError(null);
    setSelectedIndex(-1);
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

  // Commit search (F003 history search, moved here from the History toolbar):
  // debounced top-N match on message/author via the backend filter. Selecting
  // a result only locates the commit in the graph — it never filters the list.
  useEffect(() => {
    if (!open) return;
    const needle = query.trim();
    if (!needle || !workspaceId) {
      setCommitResults([]);
      setCommitsLoading(false);
      setCommitsError(null);
      setSelectedIndex(-1);
      return;
    }
    let cancelled = false;
    setCommitsLoading(true);
    const timer = window.setTimeout(() => {
      getCommitLog(workspaceId, 10, needle)
        .then((list) => {
          if (!cancelled) {
            setCommitResults(list);
            setCommitsError(null);
          }
        })
        .catch((e) => {
          if (!cancelled) {
            setCommitResults([]);
            setCommitsError(formatAppError(e));
          }
        })
        .finally(() => {
          if (!cancelled) setCommitsLoading(false);
        });
    }, 300);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [open, query, workspaceId]);

  const locateCommit = (sha: string): void => {
    setOpen(false);
    requestLocate(sha);
  };

  // Arrow keys walk commands and commit results as one list; Enter on an
  // unselected input falls through to the form submit (Ask AI).
  type PaletteRow =
    { kind: "command"; id: string; run: () => void } | { kind: "commit"; id: string; sha: string };
  const rows: PaletteRow[] = [
    ...filtered.map((c) => ({ kind: "command" as const, id: c.id, run: c.run })),
    ...commitResults.map((c) => ({ kind: "commit" as const, id: c.sha, sha: c.sha })),
  ];
  const clampSelection = (next: number): void => {
    setSelectedIndex(rows.length === 0 ? -1 : Math.min(Math.max(next, 0), rows.length - 1));
  };
  const runRow = (row: PaletteRow): void => {
    if (row.kind === "command") row.run();
    else locateCommit(row.sha);
  };

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
            // Same gate as the sidebar checkout (dirty/blocked pre-check);
            // the backend refuses dirty checkouts as the last line of defence.
            const [wc, merging, rebasePaused] = await Promise.all([
              getWorkingCopy(workspaceId).catch(() => null),
              mergeInProgress(workspaceId).catch(() => false),
              interactiveRebasePaused(workspaceId).catch(() => false),
            ]);
            const gate = gateCheckout({
              isCurrent: false,
              dirtyCount: wc?.files.length ?? 0,
              mergeInProgress: merging,
              rebasePaused,
              occupiedWorktree: null,
            });
            if (gate.kind === "blocked") {
              setStatus(gate.message, "danger");
              break;
            }
            if (gate.kind === "dirty") {
              setStatus(t("branches.switch.dirtyDescription", { count: gate.fileCount }), "danger");
              break;
            }
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

  const actionLabelKey = ACTION_LABEL_KEYS[intent?.action ?? ""];

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-bg-overlay pt-[12vh] backdrop-blur-sm animate-in fade-in-0"
      onMouseDown={() => setOpen(false)}
    >
      <div
        className="w-[560px] max-w-[92vw] overflow-hidden rounded-xl border border-border-default bg-bg-elevated shadow-modal animate-in zoom-in-95"
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
            {/* flex-1 makes the field span the row: the TextField is a plain
                flex child and would otherwise shrink to content width,
                stopping short of the Ask AI button's edge. */}
            <div className="min-w-0 flex-1">
              <Input
                ref={inputRef}
                value={query}
                onChange={(v) => {
                  setQuery(v);
                  setIntent(null);
                  setIntentError(null);
                  setSelectedIndex(-1);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Escape") {
                    e.stopPropagation();
                    setOpen(false);
                  } else if (e.key === "ArrowDown") {
                    e.preventDefault();
                    clampSelection(selectedIndex + 1);
                  } else if (e.key === "ArrowUp") {
                    e.preventDefault();
                    clampSelection(selectedIndex - 1);
                  } else if (e.key === "Enter" && selectedIndex >= 0 && rows[selectedIndex]) {
                    e.preventDefault();
                    runRow(rows[selectedIndex]);
                  }
                }}
                placeholder={t("palette.placeholder")}
                variant="search"
                /* Borderless transparent field: the palette header row is the
                   field — a filled InputGroup inside the elevated panel reads
                   as a glaring white box (and doubles the prefix search icon). */
                className="border-0 bg-transparent shadow-none hover:bg-transparent focus-within:bg-transparent focus-visible:bg-transparent"
              />
            </div>
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
            <div className="mx-3 my-2 rounded-lg border border-border-subtle bg-bg-primary px-3 py-2.5">
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

          {intentError ? <p className="px-3 py-2 text-xs text-danger">{intentError}</p> : null}

          {!askAi.isPending ? (
            <div className="px-1.5 py-1">
              {filtered.map((c, i) => (
                <button
                  key={c.id}
                  type="button"
                  onClick={c.run}
                  onMouseEnter={() => setSelectedIndex(i)}
                  className={cn(
                    "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs text-text-secondary",
                    "hover:bg-bg-secondary hover:text-text-primary",
                    selectedIndex === i && "bg-bg-secondary text-text-primary",
                  )}
                >
                  {c.icon}
                  <span className="flex-1">{c.label}</span>
                  {c.hint ? (
                    <span className="rounded border border-border-subtle bg-bg-primary px-1.5 py-0.5 font-mono text-[10px] text-text-muted">
                      {c.hint}
                    </span>
                  ) : null}
                </button>
              ))}

              {commitsLoading ? (
                <p className="px-2.5 py-2 text-xs text-text-muted">
                  {t("palette.searchingCommits")}
                </p>
              ) : null}
              {commitsError ? (
                <p className="px-2.5 py-2 text-xs text-danger">{commitsError}</p>
              ) : null}

              {!commitsLoading && commitResults.length > 0 ? (
                <>
                  <p className="px-2.5 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wide text-text-muted">
                    {t("palette.commitsTitle")}
                  </p>
                  {commitResults.map((c, i) => {
                    const rowIndex = filtered.length + i;
                    return (
                      <button
                        key={c.sha}
                        type="button"
                        onClick={() => locateCommit(c.sha)}
                        onMouseEnter={() => setSelectedIndex(rowIndex)}
                        className={cn(
                          "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs text-text-secondary",
                          "hover:bg-bg-secondary hover:text-text-primary",
                          selectedIndex === rowIndex && "bg-bg-secondary text-text-primary",
                        )}
                      >
                        <GitCommitHorizontal size={14} />
                        <span className="font-mono text-[11px] text-text-muted">
                          {c.sha.slice(0, 7)}
                        </span>
                        <span className="min-w-0 flex-1 truncate">{c.message_summary}</span>
                        <span className="shrink-0 text-[11px] text-text-muted">{c.author}</span>
                      </button>
                    );
                  })}
                </>
              ) : null}

              {filtered.length === 0 &&
              !commitsLoading &&
              commitResults.length === 0 &&
              !commitsError ? (
                <p className="px-2.5 py-2 text-xs text-text-muted">
                  <Sparkles size={11} className="mr-1 inline" />
                  {t("palette.askHint")}
                </p>
              ) : null}
              {query.trim() ? (
                <p className="flex items-center gap-1.5 px-2.5 py-1.5 text-[11px] text-text-muted">
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
