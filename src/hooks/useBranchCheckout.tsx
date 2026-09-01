// F004 safe-branch-switch flow for list rows outside the sidebar (commit
// graph ref badges): gate with `gateCheckout`, offer the dirty three-choice
// dialog (cancel / discard / stash & switch), then execute. The sidebar's
// BranchList keeps its own wired-in copy until it migrates to this hook.

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import {
  checkoutBranch,
  formatAppError,
  getWorkingCopy,
  interactiveRebasePaused,
  listWorktrees,
  mergeInProgress,
  popStash,
  saveStash,
} from "@/lib/api";
import { gateCheckout } from "@/lib/checkoutGate";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useSyncStore } from "@/stores/syncStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";

export type BranchCheckoutDialogState =
  | { kind: "dirty"; name: string; fileCount: number }
  | { kind: "blocked"; name: string; message: string };

export interface UseBranchCheckoutResult {
  busy: boolean;
  /** Gate and switch to `name`; may open a dirty/blocked dialog instead. */
  request: (name: string, opts: { kind: "local" | "remote"; isCurrent: boolean }) => void;
  /** Dirty three-choice + blocked dialogs; render next to your tree. */
  renderDialogs: () => React.JSX.Element | null;
}

export function useBranchCheckout(): UseBranchCheckoutResult {
  const { t } = useTranslation();
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const setStatus = useStatusAreaStore((s) => s.setStatus);
  const startOp = useSyncStore((s) => s.startOp);
  const endOp = useSyncStore((s) => s.endOp);
  const queryClient = useQueryClient();

  const [busy, setBusy] = useState(false);
  const [dialog, setDialog] = useState<BranchCheckoutDialogState | null>(null);

  /** mode mirrors BranchList.checkoutOnto minus the sidebar selection. */
  const switchTo = async (name: string, mode: "safe" | "force" | "stash"): Promise<void> => {
    if (!workspaceId) return;
    setBusy(true);
    startOp("checkout");
    try {
      if (mode === "stash") {
        await saveStash(workspaceId, t("branches.checkout.stashMessage", { name }));
        await checkoutBranch(workspaceId, name, false);
        try {
          await popStash(workspaceId, 0);
          setStatus(t("branches.checkout.withStash", { name }));
        } catch {
          setStatus(t("branches.checkout.stashFailed", { name }), "danger");
        }
      } else {
        await checkoutBranch(workspaceId, name, mode === "force");
        setStatus(t("branches.checkout.success", { name }));
      }
      void queryClient.invalidateQueries({ queryKey: ["working-copy"] });
      bumpHistory();
    } catch (e) {
      setStatus(formatAppError(e), "danger");
    } finally {
      setBusy(false);
      endOp("checkout");
    }
  };

  const request = (name: string, opts: { kind: "local" | "remote"; isCurrent: boolean }): void => {
    if (!workspaceId || busy) return;
    void (async () => {
      try {
        const [merging, worktrees, workingCopy, rebasePaused] = await Promise.all([
          mergeInProgress(workspaceId),
          listWorktrees(workspaceId).catch(() => []),
          getWorkingCopy(workspaceId).catch(() => null),
          interactiveRebasePaused(workspaceId).catch(() => false),
        ]);
        const occupied = worktrees.find((w) => !w.is_main && w.branch === name);
        const gate = gateCheckout({
          isCurrent: opts.isCurrent,
          branchKind: opts.kind,
          dirtyCount: workingCopy?.files.length ?? 0,
          mergeInProgress: merging,
          rebasePaused,
          occupiedWorktree: occupied?.name ?? null,
        });
        if (gate.kind === "noop") return;
        if (gate.kind === "blocked") {
          setDialog({ kind: "blocked", name, message: gate.message });
          return;
        }
        if (gate.kind === "dirty") {
          setDialog({ kind: "dirty", name, fileCount: gate.fileCount });
          return;
        }
        await switchTo(name, "safe");
      } catch (e) {
        setStatus(formatAppError(e), "danger");
      }
    })();
  };

  const renderDialogs = (): React.JSX.Element | null => {
    if (!dialog) return null;
    if (dialog.kind === "blocked") {
      return (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setDialog(null);
          }}
          title={t("branches.switch.blockedTitle", { name: dialog.name })}
          description={dialog.message}
          size="sm"
          footer={
            <Button variant="primary" size="sm" className="w-full" onClick={() => setDialog(null)}>
              {t("branches.ok")}
            </Button>
          }
        />
      );
    }
    return (
      <Modal
        open
        onOpenChange={(open) => {
          if (!open) setDialog(null);
        }}
        title={t("branches.switch.dirtyTitle", { name: dialog.name })}
        description={t("branches.switch.dirtyDescription", { count: dialog.fileCount })}
        size="sm"
        footer={
          <>
            <Button
              variant="ghost"
              size="sm"
              className="min-w-0 flex-[3]"
              onClick={() => setDialog(null)}
            >
              {t("branches.cancel")}
            </Button>
            <Button
              variant="danger"
              size="sm"
              className="min-w-0 flex-[3]"
              disabled={busy}
              onClick={() => {
                const target = dialog.name;
                setDialog(null);
                void switchTo(target, "force");
              }}
            >
              {t("branches.switch.discard")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              className="min-w-0 flex-[4]"
              disabled={busy}
              onClick={() => {
                const target = dialog.name;
                setDialog(null);
                void switchTo(target, "stash");
              }}
            >
              {t("branches.switch.stashAndSwitch")}
            </Button>
          </>
        }
      />
    );
  };

  return { busy, request, renderDialogs };
}
