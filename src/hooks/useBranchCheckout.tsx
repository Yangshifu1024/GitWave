// F004 safe-branch-switch flow, extended by F012: gate with `gateCheckout`,
// offer the dirty three-choice dialog (cancel / discard / stash & switch),
// then execute. Remote-tracking branches DWIM-resolve to their local target
// (create + track + switch via `cmd_checkout_remote_branch`). The sidebar's
// BranchList uses this hook too — previously it kept its own wired-in copy.

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import {
  checkoutBranch,
  checkoutRemoteBranch,
  formatAppError,
  getWorkingCopy,
  interactiveRebasePaused,
  listRemotes,
  listWorktrees,
  mergeInProgress,
  popStash,
  saveStash,
} from "@/lib/api";
import { localNameForRemote } from "@/lib/branchNames";
import { gateCheckout } from "@/lib/checkoutGate";
import { useActiveRepoState } from "@/hooks/useActiveRepoState";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useSyncStore } from "@/stores/syncStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";

/** A resolved switch request: the API target and the local display name. */
interface SwitchRequest {
  kind: "local" | "remote";
  /** API argument: branch name (local) or remote shorthand (remote). */
  name: string;
  /** Local branch being switched to — used in dialogs and status text. */
  target: string;
}

export type BranchCheckoutDialogState =
  | { kind: "dirty"; name: string; fileCount: number; request: SwitchRequest }
  | { kind: "blocked"; name: string; message: string };

export interface UseBranchCheckoutResult {
  busy: boolean;
  /** Gate and switch to `name`; may open a dirty/blocked dialog instead. */
  request: (name: string, opts: { kind: "local" | "remote"; isCurrent: boolean }) => void;
  /** Dirty three-choice + blocked dialogs; render next to your tree. */
  renderDialogs: () => React.JSX.Element | null;
}

export function useBranchCheckout(options?: {
  /** Called after a successful switch with the local branch switched to. */
  onSwitched?: (target: string) => void;
}): UseBranchCheckoutResult {
  const { t } = useTranslation();
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const setStatus = useStatusAreaStore((s) => s.setStatus);
  const startOp = useSyncStore((s) => s.startOp);
  const endOp = useSyncStore((s) => s.endOp);
  const queryClient = useQueryClient();

  const [busy, setBusy] = useState(false);
  const [dialog, setDialog] = useState<BranchCheckoutDialogState | null>(null);
  // HEAD of the active repo — decides "already on the DWIM target" for
  // remote double-clicks. Shares the ["working-copy"] cache this hook
  // invalidates after switching, so it tracks the new branch.
  const { currentBranch } = useActiveRepoState();
  // Configured remotes, for DWIM name resolution that mirrors the backend's
  // longest-prefix rule (a remote may itself contain slashes). Shares the
  // sidebar's ["remotes"] cache.
  const remotesQuery = useQuery({
    queryKey: ["remotes", workspaceId],
    queryFn: () => listRemotes(workspaceId!),
    enabled: Boolean(workspaceId),
  });
  const remotes = remotesQuery.data ?? [];

  const switchTo = async (req: SwitchRequest, mode: "safe" | "force" | "stash"): Promise<void> => {
    if (!workspaceId) return;
    setBusy(true);
    startOp("checkout");
    try {
      const runCheckout = async (force: boolean): Promise<void> => {
        if (req.kind === "remote") {
          const outcome = await checkoutRemoteBranch(workspaceId, req.name, force);
          if (outcome.alreadyCurrent) return; // already on the DWIM target — silent no-op (F004)
          const message = outcome.created
            ? t("branches.checkout.createdFromRemote", {
                name: outcome.localName,
                remote: req.name,
              })
            : t("branches.checkout.success", { name: outcome.localName });
          setStatus(message);
        } else {
          await checkoutBranch(workspaceId, req.name, force);
          setStatus(t("branches.checkout.success", { name: req.name }));
        }
      };
      if (mode === "stash") {
        await saveStash(workspaceId, t("branches.checkout.stashMessage", { name: req.target }));
        await runCheckout(false);
        try {
          await popStash(workspaceId, 0);
          setStatus(t("branches.checkout.withStash", { name: req.target }));
        } catch {
          setStatus(t("branches.checkout.stashFailed", { name: req.target }), "danger");
        }
      } else {
        await runCheckout(mode === "force");
      }
      void queryClient.invalidateQueries({ queryKey: ["working-copy"] });
      if (req.kind === "remote") {
        // A local branch may have been created — refresh branch consumers.
        void queryClient.invalidateQueries({ queryKey: ["branches", workspaceId] });
      }
      bumpHistory();
      options?.onSwitched?.(req.target);
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
        // F012 DWIM: a remote-tracking name resolves to its local target
        // (same longest-prefix rule as the backend). When the resolution is
        // stale, the backend no-ops with `already_current` instead of
        // touching the worktree — a wrong guess can at worst open a
        // redundant dialog, never discard changes.
        const target = opts.kind === "remote" ? localNameForRemote(name, remotes) : name;
        const isCurrent = opts.kind === "remote" ? currentBranch === target : opts.isCurrent;
        const [merging, worktrees, workingCopy, rebasePaused] = await Promise.all([
          mergeInProgress(workspaceId),
          listWorktrees(workspaceId).catch(() => []),
          getWorkingCopy(workspaceId).catch(() => null),
          interactiveRebasePaused(workspaceId).catch(() => false),
        ]);
        const occupied = worktrees.find((w) => !w.is_main && w.branch === target);
        const gate = gateCheckout({
          isCurrent,
          dirtyCount: workingCopy?.files.length ?? 0,
          mergeInProgress: merging,
          rebasePaused,
          occupiedWorktree: occupied?.name ?? null,
        });
        if (gate.kind === "noop") return;
        if (gate.kind === "blocked") {
          setDialog({ kind: "blocked", name: target, message: gate.message });
          return;
        }
        if (gate.kind === "dirty") {
          setDialog({
            kind: "dirty",
            name: target,
            fileCount: gate.fileCount,
            request: { kind: opts.kind, name, target },
          });
          return;
        }
        await switchTo({ kind: opts.kind, name, target }, "safe");
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
    const req = dialog.request;
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
                setDialog(null);
                void switchTo(req, "force");
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
                setDialog(null);
                void switchTo(req, "stash");
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
