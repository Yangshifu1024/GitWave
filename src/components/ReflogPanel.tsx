import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { History, RotateCcw, Sparkles, Undo2 } from "lucide-react";

import {
  createBranch,
  explainReflog,
  formatAppError,
  listReflog,
  resetHeadHard,
  type ReflogEntry,
} from "@/lib/api";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { cn } from "@/lib/utils";

// Reflog action → i18n key; labels resolve via t() at render time.
const ACTION_LABEL_KEYS: Record<string, string> = {
  commit: "repo.reflog.actions.commit",
  initial_commit: "repo.reflog.actions.initialCommit",
  amend: "repo.reflog.actions.amend",
  checkout: "repo.reflog.actions.checkout",
  reset: "repo.reflog.actions.reset",
  merge: "repo.reflog.actions.merge",
  rebase: "repo.reflog.actions.rebase",
  pull: "repo.reflog.actions.pull",
  push: "repo.reflog.actions.push",
  branch: "repo.reflog.actions.branch",
  revert: "repo.reflog.actions.revert",
  cherry_pick: "repo.reflog.actions.cherryPick",
  stash: "repo.reflog.actions.stash",
  clone: "repo.reflog.actions.clone",
  other: "repo.reflog.actions.other",
};

function formatTime(time: number): string {
  return new Date(time * 1000).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** A reflog entry plus the resolved recovery position (see recoversOld). */
type RecoveryTarget = ReflogEntry & { oid: string };

/**
 * M2 recovery panel: semantic reflog timeline for HEAD (or the current
 * branch) with deterministic recovery actions and an optional AI
 * explanation. Every mutation is behind an explicit confirmation (P1).
 */
export function ReflogPanel(): React.JSX.Element {
  const { t } = useTranslation();
  const wc = useWorkingCopy();
  const workspaceId = wc.workspaceId;
  const repoId = wc.repoId;
  const branchName = wc.data?.branch;
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const queryClient = useQueryClient();
  const setStatus = useStatusAreaStore((s) => s.setStatus);

  const [scope, setScope] = useState<"HEAD" | "branch">("HEAD");
  const reference =
    scope === "branch" && branchName && branchName !== "(detached)" ? branchName : "HEAD";

  const {
    data: entries = [],
    isLoading,
    error,
  } = useQuery({
    queryKey: ["reflog", workspaceId, reference],
    queryFn: () => listReflog(workspaceId!, reference),
    enabled: Boolean(workspaceId && repoId),
  });

  const [selected, setSelected] = useState<ReflogEntry | null>(null);
  const [busy, setBusy] = useState(false);
  const [branchModal, setBranchModal] = useState<RecoveryTarget | null>(null);
  const [branchName_, setBranchName_] = useState("");
  const [resetModal, setResetModal] = useState<RecoveryTarget | null>(null);
  const [aiText, setAiText] = useState<string | null>(null);
  const [aiBusy, setAiBusy] = useState(false);

  if (!workspaceId || !repoId) return <></>;

  // For reset/amend entries the PREVIOUS position (old_oid) holds what the
  // operation discarded — that is the recovery target, not the new one.

  // For reset/amend entries the PREVIOUS position (old_oid) holds what the
  // operation discarded — that is the recovery target, not the new one.
  const recoversOld = selected?.action === "reset" || selected?.action === "amend";
  const recoveryOid = selected ? (recoversOld ? selected.old_oid : selected.new_oid) : "";

  const afterMutation = (): void => {
    bumpHistory();
    void queryClient.invalidateQueries({ queryKey: ["working-copy"] });
    void queryClient.invalidateQueries({ queryKey: ["reflog", workspaceId] });
    void queryClient.invalidateQueries({ queryKey: ["health", workspaceId] });
  };

  const submitRecoveryBranch = (): void => {
    if (!branchModal || !branchName_.trim() || busy) return;
    setBusy(true);
    createBranch(workspaceId, branchName_.trim(), branchModal.oid)
      .then(() => {
        setStatus(t("repo.reflog.recoveryBranchCreated", { name: branchName_.trim() }));
        setBranchModal(null);
        afterMutation();
      })
      .catch((e) => setStatus(formatAppError(e), "danger"))
      .finally(() => setBusy(false));
  };

  const submitReset = (): void => {
    if (!resetModal || busy) return;
    setBusy(true);
    resetHeadHard(workspaceId, resetModal.oid)
      .then(() => {
        setStatus(t("repo.reflog.branchReset", { oid: resetModal.oid.slice(0, 7) }));
        setResetModal(null);
        setSelected(null);
        afterMutation();
      })
      .catch((e) => setStatus(formatAppError(e), "danger"))
      .finally(() => setBusy(false));
  };

  const runExplain = (entry: ReflogEntry): void => {
    if (aiBusy) return;
    setAiBusy(true);
    setAiText(null);
    explainReflog(workspaceId, entry)
      .then(setAiText)
      .catch((e) => setStatus(formatAppError(e), "danger"))
      .finally(() => setAiBusy(false));
  };

  const suggestion = selected?.action === "reset" ? t("repo.reflog.resetSuggestion") : null;

  return (
    <div className="flex flex-col gap-1 px-1 pb-1">
      {error ? <p className="px-2 text-xs text-danger">{formatAppError(error)}</p> : null}

      {branchName && branchName !== "(detached)" ? (
        <div className="flex items-center gap-0.5 self-start rounded-md border border-border-subtle p-0.5">
          {(["HEAD", "branch"] as const).map((s) => (
            <Button
              key={s}
              variant="ghost"
              size="sm"
              aria-pressed={scope === s}
              className={cn("h-6 px-2 text-[11px]", scope === s && "bg-accent/10 text-accent")}
              onClick={() => {
                setScope(s);
                setSelected(null);
                setAiText(null);
              }}
            >
              {s === "HEAD" ? "HEAD" : branchName}
            </Button>
          ))}
        </div>
      ) : null}

      {isLoading ? (
        <p className="px-2 py-1 text-xs text-text-muted italic">{t("common.loading")}</p>
      ) : entries.length === 0 ? (
        <p className="px-2 py-1 text-xs text-text-muted italic">{t("repo.reflog.empty")}</p>
      ) : (
        <div className="flex max-h-72 flex-col overflow-auto">
          {entries.map((e) => (
            <button
              key={`${e.time}-${e.new_oid}-${e.message}`}
              type="button"
              onClick={() => {
                setSelected(selected === e ? null : e);
                setAiText(null);
              }}
              className={
                "flex min-w-0 items-center gap-2 rounded-md px-2 py-1 text-left hover:bg-bg-elevated " +
                (selected === e ? "bg-accent/10" : "")
              }
              title={e.message}
            >
              <History size={12} className="shrink-0 text-text-muted" />
              <span className="shrink-0 text-[11px] font-medium text-text-secondary">
                {t(ACTION_LABEL_KEYS[e.action] ?? "repo.reflog.actions.other")}
              </span>
              <span className="min-w-0 flex-1 truncate text-[11px] text-text-muted">
                {e.message}
              </span>
              <span className="shrink-0 font-mono text-[10px] text-text-muted tabular-nums">
                {formatTime(e.time)}
              </span>
            </button>
          ))}
        </div>
      )}

      {selected ? (
        <div className="flex flex-col gap-1.5 rounded-md border border-border-subtle p-2">
          <p className="text-[11px] leading-4 text-text-secondary break-words">
            <span className="font-medium text-text-primary">
              {t(ACTION_LABEL_KEYS[selected.action] ?? "repo.reflog.actions.other")}
            </span>{" "}
            · {selected.message}
          </p>
          {suggestion ? <p className="text-[11px] leading-4 text-warning">{suggestion}</p> : null}
          <div className="flex flex-wrap gap-1">
            <Button
              variant="secondary"
              size="sm"
              className="h-6 px-2 text-[11px]"
              disabled={busy}
              onClick={() => {
                setBranchName_(`recovery-${recoveryOid.slice(0, 7)}`);
                setBranchModal({ ...selected, oid: recoveryOid });
              }}
            >
              <Undo2 size={12} />
              {t("repo.reflog.recoveryBranch")}
            </Button>
            <Button
              variant="secondary"
              size="sm"
              className="h-6 px-2 text-[11px]"
              disabled={busy}
              onClick={() => setResetModal({ ...selected, oid: recoveryOid })}
            >
              <RotateCcw size={12} />
              {recoversOld ? t("repo.reflog.resetBackHere") : t("repo.reflog.resetHere")}
            </Button>
            <Button
              variant="secondary"
              size="sm"
              className="h-6 px-2 text-[11px]"
              disabled={aiBusy}
              onClick={() => runExplain(selected)}
            >
              <Sparkles size={12} />
              {t("repo.reflog.aiExplain")}
            </Button>
          </div>
          {aiBusy ? (
            <p className="text-[11px] text-text-muted italic">{t("repo.reflog.thinking")}</p>
          ) : aiText ? (
            <p className="rounded-md bg-bg-primary p-2 text-[11px] leading-4 whitespace-pre-wrap text-text-secondary">
              {aiText}
            </p>
          ) : null}
        </div>
      ) : null}

      {branchModal ? (
        <Modal
          open
          onOpenChange={(o) => !o && setBranchModal(null)}
          title={t("repo.reflog.createBranchTitle")}
          description={
            recoversOld
              ? t("repo.reflog.branchPreviousAt", { oid: branchModal.oid.slice(0, 7) })
              : t("repo.reflog.branchCurrentAt", { oid: branchModal.oid.slice(0, 7) })
          }
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setBranchModal(null)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy || !branchName_.trim()}
                onClick={submitRecoveryBranch}
              >
                {t("repo.reflog.createBranchButton")}
              </Button>
            </>
          }
        >
          <div className="rounded-xl bg-bg-primary p-3">
            <Input value={branchName_} onChange={setBranchName_} autoFocus />
          </div>
        </Modal>
      ) : null}

      {resetModal ? (
        <Modal
          open
          onOpenChange={(o) => !o && setResetModal(null)}
          title={t("repo.reflog.resetTitle", {
            name: branchName || t("repo.reflog.currentBranch"),
            oid: resetModal.oid.slice(0, 7),
          })}
          description={t("repo.reflog.resetDescription")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setResetModal(null)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                variant="danger"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy}
                onClick={submitReset}
              >
                {t("repo.reflog.resetConfirm")}
              </Button>
            </>
          }
        />
      ) : null}
    </div>
  );
}
