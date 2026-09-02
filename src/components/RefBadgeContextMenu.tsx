// Badge-scoped context menu on history-row refs (F011): checkout / delete /
// copy without scrolling the sidebar lists. Wraps a RefBadge in its own
// ContextMenu — the trigger stops propagation so the row menu doesn't also
// open.

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Copy, GitBranch, Trash2 } from "lucide-react";

import type { CommitRef } from "@/lib/api";
import { deleteBranch, deleteRemoteBranch, deleteTag, formatAppError } from "@/lib/api";
import { copyToClipboard, parseRemoteBranchName } from "@/lib/commitMenu";
import { useActiveRepoState } from "@/hooks/useActiveRepoState";
import { useBranchCheckout } from "@/hooks/useBranchCheckout";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/ContextMenu";
import type { ReactNode } from "react";

type DeleteTarget = "branch" | "remote" | "tag";

export function RefBadgeContextMenu({
  r,
  onSelect,
  children,
}: {
  r: CommitRef;
  /** Select the owning commit row when the menu opens (Fork behavior). */
  onSelect?: () => void;
  children: ReactNode;
}): React.JSX.Element {
  const { t } = useTranslation();
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const setStatus = useStatusAreaStore((s) => s.setStatus);
  const queryClient = useQueryClient();
  const { currentBranch } = useActiveRepoState();
  const checkout = useBranchCheckout();
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState<DeleteTarget | null>(null);

  const isCurrent = r.name === currentBranch;
  const remoteRef = r.kind === "remote_branch" ? parseRemoteBranchName(r.name) : null;

  const copyName = (): void => {
    void copyToClipboard(r.name).then((ok) =>
      setStatus(
        ok ? t("commits.menu.copied") : t("commits.explain.clipboardUnavailable"),
        ok ? undefined : "danger",
      ),
    );
  };

  const run = (op: () => Promise<unknown>, done: string, invalidate: () => void): void => {
    if (!workspaceId) return;
    setBusy(true);
    op()
      .then(() => {
        setStatus(done);
        setConfirm(null);
        // Invalidate only after the delete lands, so a racing refetch
        // doesn't resurrect the deleted ref in the sidebar lists.
        invalidate();
        bumpHistory();
      })
      .catch((e) => setStatus(formatAppError(e), "danger"))
      .finally(() => setBusy(false));
  };

  const confirmMeta: Record<DeleteTarget, { title: string; description: string; button: string }> =
    {
      branch: {
        title: t("branches.deleteDialog.title"),
        description: t("branches.deleteDialog.description"),
        button: t("branches.delete"),
      },
      remote: {
        title: t("branches.deleteRemote.title"),
        description: t("branches.deleteRemote.description", { name: r.name }),
        button: t("branches.delete"),
      },
      tag: {
        title: t("repo.tags.deleteDialog.title"),
        description: t("repo.tags.deleteDialog.description", { name: r.name }),
        button: t("branches.delete"),
      },
    };

  const submitDelete = (): void => {
    const invalidateBranches = (): void => {
      void queryClient.invalidateQueries({ queryKey: ["branches", workspaceId] });
    };
    if (confirm === "branch") {
      run(
        () => deleteBranch(workspaceId!, r.name),
        t("branches.deleteDialog.deleted", { name: r.name }),
        invalidateBranches,
      );
    } else if (confirm === "remote" && remoteRef) {
      run(
        () => deleteRemoteBranch(workspaceId!, remoteRef.remote, remoteRef.branch),
        t("branches.deleteRemote.deleted", {
          name: `${remoteRef.remote}/${remoteRef.branch}`,
        }),
        invalidateBranches,
      );
    } else if (confirm === "tag") {
      run(
        () => deleteTag(workspaceId!, r.name),
        t("repo.tags.deleteDialog.deleted", { name: r.name }),
        () => {
          void queryClient.invalidateQueries({ queryKey: ["tags"] });
        },
      );
    }
  };

  return (
    <>
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <span className="inline-flex" onContextMenu={() => onSelect?.()}>
            {children}
          </span>
        </ContextMenuTrigger>
        <ContextMenuContent className="max-w-[240px]">
          <ContextMenuLabel title={r.name}>{r.name}</ContextMenuLabel>
          <ContextMenuSeparator />
          {r.kind === "local_branch" ? (
            <>
              <ContextMenuItem
                disabled={checkout.busy || isCurrent}
                title={isCurrent ? t("branches.row.current") : undefined}
                onSelect={() => {
                  onSelect?.();
                  checkout.request(r.name, { kind: "local", isCurrent });
                }}
              >
                <GitBranch size={14} />
                {t("branches.menu.checkout")}
              </ContextMenuItem>
              <ContextMenuItem
                destructive
                disabled={busy || isCurrent}
                title={isCurrent ? t("branches.guard.currentBranch") : undefined}
                onSelect={() => setConfirm("branch")}
              >
                <Trash2 size={14} />
                {t("branches.delete")}
              </ContextMenuItem>
            </>
          ) : null}
          {r.kind === "remote_branch" ? (
            <>
              <ContextMenuItem
                disabled={checkout.busy}
                onSelect={() => {
                  onSelect?.();
                  // F012 DWIM: creates/tracks the local branch via the hook.
                  checkout.request(r.name, { kind: "remote", isCurrent: false });
                }}
              >
                <GitBranch size={14} />
                {t("branches.menu.checkout")}
              </ContextMenuItem>
              <ContextMenuItem
                destructive
                disabled={busy || !remoteRef}
                onSelect={() => setConfirm("remote")}
              >
                <Trash2 size={14} />
                {t("branches.deleteRemote.menuItem")}
              </ContextMenuItem>
            </>
          ) : null}
          {r.kind === "tag" ? (
            <ContextMenuItem destructive disabled={busy} onSelect={() => setConfirm("tag")}>
              <Trash2 size={14} />
              {t("repo.tags.menu.delete")}
            </ContextMenuItem>
          ) : null}
          <ContextMenuItem onSelect={copyName}>
            <Copy size={14} />
            {r.kind === "tag" ? t("branches.menu.copyTagName") : t("branches.menu.copyName")}
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>

      {confirm ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setConfirm(null);
          }}
          title={confirmMeta[confirm].title}
          description={confirmMeta[confirm].description}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setConfirm(null)}
              >
                {t("branches.cancel")}
              </Button>
              <Button
                variant="danger"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy}
                onClick={submitDelete}
              >
                {confirmMeta[confirm].button}
              </Button>
            </>
          }
        />
      ) : null}

      {checkout.renderDialogs()}
    </>
  );
}
