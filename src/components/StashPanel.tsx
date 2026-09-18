import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { StashEntry } from "@/lib/api";
import {
  applyStash,
  dropStash,
  formatAppError,
  getStashDiff,
  getWorkingCopy,
  popStash,
} from "@/lib/api";
import { stashFileSummary, stashLabel, stashTitle } from "@/lib/stashActions";
import { useStashes } from "@/hooks/useStashes";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { ListItem } from "@/components/ui/ListItem";
import { Modal } from "@/components/ui/Modal";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { Tooltip } from "@/components/ui/Tooltip";
import { StashDetailModal } from "@/components/StashDetailModal";
import { Archive } from "lucide-react";

/** apply / pop hold back until the dirty pre-check came back clean. */
interface DirtyPrompt {
  entry: StashEntry;
  action: "apply" | "pop";
  count: number;
}

/** Drop confirmation; the name excerpt arrives after the diff is fetched. */
interface DropPrompt {
  entry: StashEntry;
  excerpt: { count: number; names: string[] } | null;
  loading: boolean;
}

export function StashPanel(): React.JSX.Element {
  const { t } = useTranslation();
  const { data: entries = [], error: listError, workspaceId, repoId, invalidate } = useStashes();
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const setStatus = useStatusAreaStore((s) => s.setStatus);

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [detail, setDetail] = useState<StashEntry | null>(null);
  const [dirtyPrompt, setDirtyPrompt] = useState<DirtyPrompt | null>(null);
  const [dropPrompt, setDropPrompt] = useState<DropPrompt | null>(null);

  // Single-flight gate for stash ops. `busy` alone cannot close the window that
  // precedes `run`: the dirty pre-check awaits a full worktree diff (hundreds of
  // ms in a large repo) without touching state, so a click landing in that gap
  // still saw `busy === false` and started a second apply / pop in parallel — in
  // the pop case the extra `stash_pop(0)` dropped an entry the user never picked.
  // The ref flips synchronously, before any await.
  const inFlight = useRef(false);

  /** Takes the op lock; null when another stash op is already in flight. */
  const lock = (): (() => void) | null => {
    if (busy || inFlight.current) return null;
    inFlight.current = true;
    return () => {
      inFlight.current = false;
    };
  };

  // The stash list is repo-scoped: on a repo switch drop the previous repo's
  // dialogs and errors so they never act on another repo's stash.
  useEffect(() => {
    setDetail(null);
    setDirtyPrompt(null);
    setDropPrompt(null);
    setError(null);
  }, [repoId]);

  const refresh = (): void => {
    invalidate();
  };

  /** Returns false when the op was skipped (busy / no workspace) or failed. */
  const run = async (fn: () => Promise<void>, success: string): Promise<boolean> => {
    if (!workspaceId || busy) return false;
    setBusy(true);
    setError(null);
    try {
      await fn();
      refresh();
      bumpHistory();
      // Operation results surface in the ActionBar status area (same surface
      // BranchList uses) instead of a panel-local toast.
      setStatus(success);
      return true;
    } catch (e) {
      setError(formatAppError(e));
      return false;
    } finally {
      setBusy(false);
    }
  };

  /** Uncommitted files an apply / pop has to merge with (dirty pre-check). */
  const dirtyFileCount = async (): Promise<number> => {
    if (!workspaceId) return 0;
    const wc = await getWorkingCopy(workspaceId).catch(() => null);
    return wc?.files.length ?? 0;
  };

  const runApply = (entry: StashEntry): Promise<boolean> =>
    run(async () => {
      await applyStash(workspaceId!, entry.index);
    }, t("changes.stash.applied"));

  const runPop = async (entry: StashEntry): Promise<void> => {
    const ok = await run(async () => {
      await popStash(workspaceId!, entry.index);
    }, t("changes.stash.popped"));
    // The entry is gone from the list: the detail modal behind must not keep
    // showing (and offering actions on) a dropped stash.
    if (ok) {
      setDetail(null);
      setDirtyPrompt(null);
    }
  };

  const requestApplyOrPop = async (entry: StashEntry, action: "apply" | "pop"): Promise<void> => {
    // The lock spans pre-check + dialog + execution: the whole sequence is one
    // non-reentrant critical section, and cancelling the dialog is safe because
    // the lock is already released by then.
    const release = lock();
    if (!release) return;
    try {
      const count = await dirtyFileCount();
      if (count > 0) {
        setDirtyPrompt({ entry, action, count });
        return;
      }
      if (action === "apply") await runApply(entry);
      else await runPop(entry);
    } finally {
      release();
    }
  };

  const confirmDirty = (): void => {
    const prompt = dirtyPrompt;
    setDirtyPrompt(null);
    if (!prompt) return;
    // Re-take the lock for the execution half: sharing one lock is simpler than
    // reasoning about two, and it keeps a confirm click from racing the row
    // buttons behind the dialog.
    const release = lock();
    if (!release) return;
    void (prompt.action === "apply" ? runApply(prompt.entry) : runPop(prompt.entry)).finally(
      release,
    );
  };

  const requestDrop = async (entry: StashEntry): Promise<void> => {
    // Same lock as apply / pop: this one awaits a stash diff before the dialog
    // has its file excerpt, so it needs the single-flight gate too.
    const release = lock();
    if (!release) return;
    // Open on the known facts (label + message) and fill the excerpt in when
    // the diff lands; a failed diff degrades to "message + index only" rather
    // than blocking the drop.
    try {
      if (!workspaceId) return;
      setDropPrompt({ entry, excerpt: null, loading: true });
      const summary = await getStashDiff(workspaceId, entry.oid).catch(() => null);
      setDropPrompt((prev) =>
        prev && prev.entry.oid === entry.oid
          ? { entry, excerpt: summary ? stashFileSummary(summary.files) : null, loading: false }
          : prev,
      );
    } finally {
      release();
    }
  };

  const confirmDrop = (): void => {
    const prompt = dropPrompt;
    setDropPrompt(null);
    if (!prompt) return;
    const release = lock();
    if (!release) return;
    void run(async () => {
      await dropStash(workspaceId!, prompt.entry.index);
    }, t("changes.stash.dropped"))
      .then((ok) => {
        if (ok) setDetail(null);
      })
      .finally(release);
  };

  // Empty dataset = static header (nothing to expand); with entries the
  // section is expandable. Load errors stay expandable so they are readable.
  const hasEntries = entries.length > 0;
  const collapsible = hasEntries || Boolean(listError);

  return (
    <SidebarSection title={t("changes.stash.title")} collapsible={collapsible}>
      {listError ? (
        <p className="px-3 py-1.5 text-xs text-danger">{formatAppError(listError)}</p>
      ) : null}

      <ErrorAlert message={error} onDismiss={() => setError(null)} />

      {entries.length === 0 ? (
        <p className="px-3 py-1.5 text-xs text-text-muted">{t("changes.stash.noStashes")}</p>
      ) : (
        entries.map((e) => (
          <div key={`${e.index}-${e.oid}`} className="min-w-0">
            <ListItem
              selected={detail?.oid === e.oid}
              onClick={() => setDetail(e)}
              leading={<Archive size={14} className="text-accent shrink-0" />}
            >
              <div className="flex min-w-0 flex-col">
                <span className="truncate text-xs text-text-primary">
                  {stashTitle(e, t("changes.stash.noMessage"))}
                </span>
                <span className="font-mono text-[10px] text-text-muted">{e.oid.slice(0, 7)}</span>
              </div>
            </ListItem>

            {/* Actions sit outside the ListItem on purpose: ListItem renders
                role="button" once it gets onClick, and real buttons nested in
                a button role are unreachable for a11y and double-fire on
                Enter. */}
            <div className="flex items-center gap-1 pb-1.5 pl-8 pr-3">
              <StashActionButton
                label={t("changes.stash.view")}
                tooltip={t("changes.stash.viewTooltip")}
                ariaLabel={`${t("changes.stash.view")} ${stashLabel(e)}`}
                disabled={busy}
                onClick={() => setDetail(e)}
              />
              <StashActionButton
                label={t("changes.stash.apply")}
                tooltip={t("changes.stash.applyTooltip")}
                ariaLabel={`${t("changes.stash.apply")} ${stashLabel(e)}`}
                disabled={busy}
                onClick={() => void requestApplyOrPop(e, "apply")}
              />
              <StashActionButton
                label={t("changes.stash.pop")}
                tooltip={t("changes.stash.popTooltip")}
                ariaLabel={`${t("changes.stash.pop")} ${stashLabel(e)}`}
                disabled={busy}
                onClick={() => void requestApplyOrPop(e, "pop")}
              />
              <StashActionButton
                label={t("changes.stash.drop")}
                tooltip={t("changes.stash.dropTooltip")}
                ariaLabel={`${t("changes.stash.drop")} ${stashLabel(e)}`}
                disabled={busy}
                danger
                onClick={() => void requestDrop(e)}
              />
            </div>
          </div>
        ))
      )}

      {detail && workspaceId ? (
        <StashDetailModal
          key={detail.oid}
          workspaceId={workspaceId}
          entry={detail}
          busy={busy}
          onClose={() => setDetail(null)}
          onApply={() => void requestApplyOrPop(detail, "apply")}
          onPop={() => void requestApplyOrPop(detail, "pop")}
          onDrop={() => void requestDrop(detail)}
        />
      ) : null}

      {dirtyPrompt ? (
        <Modal
          open
          onOpenChange={(next) => {
            if (!next) setDirtyPrompt(null);
          }}
          title={t("changes.stash.dirtyTitle")}
          description={t("changes.stash.dirtyDescription", { count: dirtyPrompt.count })}
          size="sm"
          footer={
            <div className="flex justify-end gap-2">
              <Button variant="secondary" size="sm" autoFocus onClick={() => setDirtyPrompt(null)}>
                {t("changes.stash.dirtyCancel")}
              </Button>
              <Button variant="primary" size="sm" onClick={confirmDirty}>
                {t("changes.stash.dirtyContinue")}
              </Button>
            </div>
          }
        >
          <p className="text-sm text-text-secondary">
            {stashTitle(dirtyPrompt.entry, t("changes.stash.noMessage"))}
          </p>
        </Modal>
      ) : null}

      {dropPrompt ? (
        <Modal
          open
          onOpenChange={(next) => {
            if (!next) setDropPrompt(null);
          }}
          title={t("changes.stash.dropConfirmTitle", { label: stashLabel(dropPrompt.entry) })}
          description={stashTitle(dropPrompt.entry, t("changes.stash.noMessage"))}
          size="sm"
          footer={
            <div className="flex justify-end gap-2">
              <Button variant="secondary" size="sm" autoFocus onClick={() => setDropPrompt(null)}>
                {t("changes.stash.dirtyCancel")}
              </Button>
              <Button variant="danger" size="sm" onClick={() => void confirmDrop()}>
                {t("changes.stash.dropConfirm")}
              </Button>
            </div>
          }
        >
          <div className="flex flex-col gap-2 text-sm">
            {dropPrompt.loading ? (
              <p className="text-xs text-text-muted">
                {t("changes.stash.dropConfirmFilesLoading")}
              </p>
            ) : dropPrompt.excerpt ? (
              <p className="text-text-secondary">
                {t("changes.stash.dropConfirmFiles", {
                  count: dropPrompt.excerpt.count,
                  names: dropPrompt.excerpt.names.join(", "),
                })}
                {dropPrompt.excerpt.count > dropPrompt.excerpt.names.length
                  ? ` ${t("changes.stash.dropConfirmMore", {
                      count: dropPrompt.excerpt.count - dropPrompt.excerpt.names.length,
                    })}`
                  : ""}
              </p>
            ) : null}
            <p className="text-xs text-danger">{t("changes.stash.dropConfirmIrreversible")}</p>
          </div>
        </Modal>
      ) : null}
    </SidebarSection>
  );
}

function StashActionButton({
  label,
  tooltip,
  ariaLabel,
  disabled,
  danger = false,
  onClick,
}: {
  label: string;
  tooltip: string;
  ariaLabel: string;
  disabled: boolean;
  danger?: boolean;
  onClick: () => void;
}): React.JSX.Element {
  return (
    <Tooltip content={tooltip}>
      <Button
        variant="ghost"
        size="sm"
        className={
          danger ? "px-1.5 text-[11px] text-danger hover:bg-danger/10" : "px-1.5 text-[11px]"
        }
        disabled={disabled}
        aria-label={ariaLabel}
        onClick={onClick}
      >
        {label}
      </Button>
    </Tooltip>
  );
}
