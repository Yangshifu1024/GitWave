// Commit-graph context menu (F011, Fork-style). Menu items render per row
// via `CommitMenuItems`; every confirm / input modal renders once through
// `useCommitMenuActions().renderModals()`. Destructive ops follow P1 — each
// runs behind an explicit confirmation (revert / cherry-pick mirror
// CommitInfoHeader, reset mirrors ReflogPanel).

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Cherry,
  Copy,
  CornerDownRight,
  FileText,
  GitBranch,
  RotateCcw,
  Tag as TagIcon,
  Undo2,
} from "lucide-react";

import type { CommitSummary } from "@/lib/api";
import {
  cherryPickCommit,
  checkoutCommit,
  createBranch,
  formatAppError,
  getWorkingCopy,
  interactiveRebasePaused,
  mergeInProgress,
  popStash,
  resetHeadHard,
  revertCommit,
  saveStash,
} from "@/lib/api";
import { copyCommitInfoText, copyToClipboard, gateCommitCheckout } from "@/lib/commitMenu";
import { useActiveRepoState } from "@/hooks/useActiveRepoState";
import { useTags } from "@/hooks/useTags";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useSyncStore } from "@/stores/syncStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { ContextMenuItem, ContextMenuSeparator } from "@/components/ui/ContextMenu";
import { TagManagerModal } from "@/components/TagManagerModal";

/** HEAD state the items need to disable entries that don't apply. */
export interface CommitMenuState {
  /** Current branch name, or null while detached (disables Reset). */
  currentBranch: string | null;
  /** Tip sha the working copy points at (disables Checkout on itself). */
  headSha: string | null;
}

export type CommitMenuAction =
  | "new-branch"
  | "new-tag"
  | "checkout"
  | "reset"
  | "cherry-pick"
  | "revert"
  | "copy-sha"
  | "copy-info";

/** One commit-row menu: create / switch / rewrite / copy entries. */
export function CommitMenuItems({
  commit,
  onAction,
  state,
}: {
  commit: CommitSummary;
  onAction: (commit: CommitSummary, action: CommitMenuAction) => void;
  state: CommitMenuState;
}): React.JSX.Element {
  const { t } = useTranslation();
  const checkoutDisabled = state.headSha === commit.sha;
  const resetDisabled = state.currentBranch === null;
  return (
    <>
      <ContextMenuItem onSelect={() => onAction(commit, "new-branch")}>
        <GitBranch size={14} />
        {t("commits.menu.newBranch")}
      </ContextMenuItem>
      <ContextMenuItem onSelect={() => onAction(commit, "new-tag")}>
        <TagIcon size={14} />
        {t("commits.menu.newTag")}
      </ContextMenuItem>
      <ContextMenuSeparator />
      <ContextMenuItem
        disabled={checkoutDisabled}
        title={checkoutDisabled ? t("commits.menu.checkoutIsHead") : undefined}
        onSelect={() => onAction(commit, "checkout")}
      >
        <CornerDownRight size={14} />
        {t("commits.menu.checkoutCommit")}
      </ContextMenuItem>
      <ContextMenuItem
        disabled={resetDisabled}
        title={resetDisabled ? t("commits.menu.resetDetached") : undefined}
        onSelect={() => onAction(commit, "reset")}
      >
        <RotateCcw size={14} />
        {t("commits.menu.resetToHere", { branch: state.currentBranch ?? "HEAD" })}
      </ContextMenuItem>
      <ContextMenuSeparator />
      <ContextMenuItem onSelect={() => onAction(commit, "cherry-pick")}>
        <Cherry size={14} />
        {t("commits.menu.cherryPick")}
      </ContextMenuItem>
      <ContextMenuItem onSelect={() => onAction(commit, "revert")}>
        <Undo2 size={14} />
        {t("commits.menu.revert")}
      </ContextMenuItem>
      <ContextMenuSeparator />
      <ContextMenuItem onSelect={() => onAction(commit, "copy-sha")}>
        <Copy size={14} />
        {t("commits.menu.copySha")}
      </ContextMenuItem>
      <ContextMenuItem onSelect={() => onAction(commit, "copy-info")}>
        <FileText size={14} />
        {t("commits.menu.copyInfo")}
      </ContextMenuItem>
    </>
  );
}

type PendingModal =
  | { kind: "new-branch"; commit: CommitSummary }
  | { kind: "new-tag"; commit: CommitSummary }
  | { kind: "reset"; commit: CommitSummary }
  | { kind: "revert"; commit: CommitSummary }
  | { kind: "cherry-pick"; commit: CommitSummary }
  | { kind: "checkout-dirty"; commit: CommitSummary; fileCount: number }
  | { kind: "checkout-blocked"; commit: CommitSummary; reason: "merge" | "rebase" };

export interface CommitMenuController {
  busy: boolean;
  currentBranch: string | null;
  headSha: string | null;
  onAction: (commit: CommitSummary, action: CommitMenuAction) => void;
  renderModals: () => React.JSX.Element | null;
}

export function useCommitMenuActions(workspaceId: string | null): CommitMenuController {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const setStatus = useStatusAreaStore((s) => s.setStatus);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const startOp = useSyncStore((s) => s.startOp);
  const endOp = useSyncStore((s) => s.endOp);
  const { currentBranch, headSha } = useActiveRepoState();
  const { data: tags = [], invalidate: invalidateTags } = useTags();

  const [modal, setModal] = useState<PendingModal | null>(null);
  const [busy, setBusy] = useState(false);
  const [branchName, setBranchName] = useState("");
  const [branchError, setBranchError] = useState<string | null>(null);

  const shortSha = (sha: string): string => sha.slice(0, 7);

  const afterMutation = (): void => {
    void queryClient.invalidateQueries({ queryKey: ["working-copy"] });
    void queryClient.invalidateQueries({ queryKey: ["branches", workspaceId] });
    void queryClient.invalidateQueries({ queryKey: ["commit-details", workspaceId] });
    // Health metrics (dirty/unpushed) and the reflog move with every mutation.
    void queryClient.invalidateQueries({ queryKey: ["health", workspaceId] });
    void queryClient.invalidateQueries({ queryKey: ["reflog", workspaceId] });
    bumpHistory();
  };

  const copy = (text: string): void => {
    void copyToClipboard(text).then((ok) =>
      setStatus(
        ok ? t("commits.menu.copied") : t("commits.explain.clipboardUnavailable"),
        ok ? undefined : "danger",
      ),
    );
  };

  /** mode mirrors useBranchCheckout.switchTo, but onto a detached commit. */
  const checkoutOnto = (commit: CommitSummary, mode: "safe" | "force" | "stash"): void => {
    if (!workspaceId) return;
    const short = shortSha(commit.sha);
    setBusy(true);
    startOp("checkout");
    void (async () => {
      try {
        if (mode === "stash") {
          await saveStash(workspaceId, t("branches.checkout.stashMessage", { name: short }));
          await checkoutCommit(workspaceId, commit.sha, false);
          try {
            await popStash(workspaceId, 0);
            setStatus(t("branches.checkout.withStash", { name: short }));
          } catch {
            setStatus(t("branches.checkout.stashFailed", { name: short }), "danger");
          }
        } else {
          await checkoutCommit(workspaceId, commit.sha, mode === "force");
          setStatus(t("branches.checkout.success", { name: short }));
        }
        afterMutation();
      } catch (e) {
        setStatus(formatAppError(e), "danger");
      } finally {
        setBusy(false);
        setModal(null);
        endOp("checkout");
      }
    })();
  };

  const runCheckout = (commit: CommitSummary): void => {
    if (!workspaceId) return;
    void (async () => {
      try {
        const [merging, rebasePaused, workingCopy] = await Promise.all([
          mergeInProgress(workspaceId),
          interactiveRebasePaused(workspaceId).catch(() => false),
          getWorkingCopy(workspaceId).catch(() => null),
        ]);
        const gate = gateCommitCheckout({
          isHead: commit.sha === workingCopy?.sha,
          dirtyCount: workingCopy?.files.length ?? 0,
          mergeInProgress: merging,
          rebasePaused,
        });
        if (gate.kind === "noop") return;
        if (gate.kind === "blocked") {
          setModal({ kind: "checkout-blocked", commit, reason: gate.reason });
          return;
        }
        if (gate.kind === "dirty") {
          setModal({ kind: "checkout-dirty", commit, fileCount: gate.fileCount });
          return;
        }
        checkoutOnto(commit, "safe");
      } catch (e) {
        setStatus(formatAppError(e), "danger");
      }
    })();
  };

  const submitNewBranch = (commit: CommitSummary): void => {
    const name = branchName.trim();
    if (!workspaceId || !name) return;
    setBusy(true);
    setBranchError(null);
    createBranch(workspaceId, name, commit.sha)
      .then(() => {
        setStatus(t("branches.newBranch.created", { name, base: shortSha(commit.sha) }));
        setModal(null);
        afterMutation();
      })
      .catch((e) => setBranchError(formatAppError(e)))
      .finally(() => setBusy(false));
  };

  const submitReset = (commit: CommitSummary): void => {
    if (!workspaceId) return;
    setBusy(true);
    resetHeadHard(workspaceId, commit.sha)
      .then(() => {
        setStatus(t("repo.reflog.branchReset", { oid: shortSha(commit.sha) }));
        setModal(null);
        afterMutation();
      })
      .catch((e) => setStatus(formatAppError(e), "danger"))
      .finally(() => setBusy(false));
  };

  /** revert / cherry-pick — same flow as the CommitInfoHeader buttons. */
  const runRewrite = (commit: CommitSummary, op: "revert" | "cherry-pick"): void => {
    if (!workspaceId) return;
    const short = shortSha(commit.sha);
    setBusy(true);
    const request =
      op === "revert"
        ? revertCommit(workspaceId, commit.sha)
        : cherryPickCommit(workspaceId, commit.sha);
    request
      .then(() => {
        setStatus(
          op === "revert"
            ? t("commits.revert.done", { sha: short })
            : t("commits.cherryPick.done", { sha: short }),
        );
        setModal(null);
        afterMutation();
      })
      .catch((e) => setStatus(formatAppError(e), "danger"))
      .finally(() => setBusy(false));
  };

  const onAction = (commit: CommitSummary, action: CommitMenuAction): void => {
    switch (action) {
      case "copy-sha":
        copy(commit.sha);
        break;
      case "copy-info":
        copy(copyCommitInfoText(commit));
        break;
      case "new-branch":
        setBranchName("");
        setBranchError(null);
        setModal({ kind: "new-branch", commit });
        break;
      case "new-tag":
        setModal({ kind: "new-tag", commit });
        break;
      case "checkout":
        runCheckout(commit);
        break;
      case "reset":
        setModal({ kind: "reset", commit });
        break;
      case "revert":
      case "cherry-pick":
        setModal({ kind: action, commit });
        break;
    }
  };

  const renderModals = (): React.JSX.Element | null => {
    if (!modal) return null;
    const commit = modal.commit;
    const short = shortSha(commit.sha);

    if (modal.kind === "new-tag") {
      return (
        <TagManagerModal
          workspaceId={workspaceId!}
          sha={commit.sha}
          tags={tags}
          onClose={() => setModal(null)}
          onChanged={() => {
            invalidateTags();
            bumpHistory();
          }}
          onError={(message) => setStatus(message, "danger")}
          onCreated={(name) => setStatus(t("commits.tag.created", { sha: short, name }))}
        />
      );
    }

    if (modal.kind === "new-branch") {
      return (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setModal(null);
          }}
          title={t("branches.newBranch.title", { name: short })}
          description={t("branches.newBranch.description")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setModal(null)}
              >
                {t("commits.action.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy || !branchName.trim()}
                onClick={() => submitNewBranch(commit)}
              >
                {t("branches.newBranch.create")}
              </Button>
            </>
          }
        >
          <div className="rounded-xl bg-bg-primary p-3">
            <Input
              autoFocus
              value={branchName}
              onChange={setBranchName}
              onKeyDown={(e) => {
                if (e.key === "Enter" && branchName.trim()) submitNewBranch(commit);
              }}
              placeholder={t("branches.newBranch.placeholder")}
              error={branchError}
            />
          </div>
        </Modal>
      );
    }

    if (modal.kind === "reset") {
      return (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setModal(null);
          }}
          title={t("repo.reflog.resetTitle", { name: currentBranch ?? "HEAD", oid: short })}
          description={t("repo.reflog.resetDescription")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setModal(null)}
              >
                {t("commits.action.cancel")}
              </Button>
              <Button
                variant="danger"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy}
                onClick={() => submitReset(commit)}
              >
                {t("repo.reflog.resetConfirm")}
              </Button>
            </>
          }
        />
      );
    }

    if (modal.kind === "revert" || modal.kind === "cherry-pick") {
      return (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setModal(null);
          }}
          title={
            modal.kind === "revert"
              ? t("commits.revert.confirmTitle", { sha: short })
              : t("commits.cherryPick.confirmTitle", { sha: short })
          }
          description={
            modal.kind === "revert"
              ? t("commits.revert.confirmDescription", { subject: commit.message_summary })
              : t("commits.cherryPick.confirmDescription", { subject: commit.message_summary })
          }
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setModal(null)}
              >
                {t("commits.action.cancel")}
              </Button>
              <Button
                variant={modal.kind === "revert" ? "danger" : "primary"}
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy}
                onClick={() => runRewrite(commit, modal.kind)}
              >
                {modal.kind === "revert"
                  ? t("commits.action.revert")
                  : t("commits.action.cherryPick")}
              </Button>
            </>
          }
        />
      );
    }

    if (modal.kind === "checkout-blocked") {
      return (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setModal(null);
          }}
          title={t("branches.switch.blockedTitle", { name: short })}
          description={t(
            modal.reason === "merge" ? "commits.menu.blockedMerge" : "commits.menu.blockedRebase",
          )}
          size="sm"
          footer={
            <Button variant="primary" size="sm" className="w-full" onClick={() => setModal(null)}>
              {t("commits.action.close")}
            </Button>
          }
        />
      );
    }

    // checkout-dirty: the F004 three-choice dialog, onto a detached commit.
    return (
      <Modal
        open
        onOpenChange={(open) => {
          if (!open) setModal(null);
        }}
        title={t("branches.switch.dirtyTitle", { name: short })}
        description={t("branches.switch.dirtyDescription", { count: modal.fileCount })}
        size="sm"
        footer={
          <>
            <Button
              variant="ghost"
              size="sm"
              className="min-w-0 flex-[3]"
              onClick={() => setModal(null)}
            >
              {t("commits.action.cancel")}
            </Button>
            <Button
              variant="danger"
              size="sm"
              className="min-w-0 flex-[3]"
              disabled={busy}
              onClick={() => checkoutOnto(commit, "force")}
            >
              {t("branches.switch.discard")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              className="min-w-0 flex-[4]"
              disabled={busy}
              onClick={() => checkoutOnto(commit, "stash")}
            >
              {t("branches.switch.stashAndSwitch")}
            </Button>
          </>
        }
      />
    );
  };

  return { busy, currentBranch, headSha, onAction, renderModals };
}
