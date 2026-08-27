// Sidebar branch list — navigation plus row-scoped operations (checkout via
// double-click, merge / rebase / delete via context menu). Toolbar-scoped
// branch ops (new branch / pull / push) live in the ActionBar.

import { useCallback, useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { BranchInfo } from "@/lib/api";
import {
  abortInteractiveRebasePause,
  continueInteractiveRebase,
  checkoutBranch,
  deleteBranch,
  deleteRemoteBranch,
  formatAppError,
  getBranches,
  getWorkingCopy,
  interactiveRebasePaused,
  listWorktrees,
  mergeBranch,
  mergeInProgress,
  popStash,
  rebaseBranch,
  saveStash,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { cn } from "@/lib/utils";
import { gateCheckout } from "@/lib/checkoutGate";
import { filterRemoteBranches, remoteShortName } from "@/lib/branchNames";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { StatusBadge } from "@/components/ui/StatusBadge";
import { ListItem } from "@/components/ui/ListItem";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/ContextMenu";
import { InteractiveRebaseDialog } from "@/components/InteractiveRebaseDialog";
import {
  ArrowRight,
  GitBranch,
  GitMerge,
  GitPullRequestArrow,
  ListOrdered,
  Trash2,
} from "lucide-react";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { ErrorAlert } from "@/components/ui/ErrorAlert";

function formatTime(time: number): string {
  if (time <= 0) return "";
  const now = Math.floor(Date.now() / 1000);
  const diff = now - time;
  if (diff < 60) return "just now";
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  if (diff < 604800) return `${Math.floor(diff / 86400)}d ago`;
  return new Date(time * 1000).toLocaleDateString();
}

function shortSha(sha: string): string {
  return sha.slice(0, 7);
}

function BranchIcon({ kind }: { kind: "local" | "remote" }): React.JSX.Element {
  return (
    <GitBranch
      size={14}
      className={cn("shrink-0", kind === "local" ? "text-accent" : "text-text-muted")}
    />
  );
}

interface BranchRowProps {
  branch: BranchInfo;
  selected: boolean;
  busy: boolean;
  onSelect: (name: string) => void;
  onCheckout: (name: string) => void;
  onDelete: (name: string) => void;
  onMerge: (name: string) => void;
  onRebaseOnto: (name: string) => void;
  onInteractiveRebase: (name: string) => void;
}

function BranchRow({
  branch,
  selected,
  busy,
  onSelect,
  onCheckout,
  onDelete,
  onMerge,
  onRebaseOnto,
  onInteractiveRebase,
}: BranchRowProps): React.JSX.Element {
  const hasActions = !branch.is_current && branch.kind === "local";

  const row = (
    <ListItem
      selected={selected}
      onClick={() => {
        if (busy) return;
        onSelect(branch.name);
      }}
      onDoubleClick={() => {
        if (busy) return;
        onCheckout(branch.name);
      }}
      leading={<BranchIcon kind={branch.kind} />}
      trailing={
        <div className="flex items-center gap-1">
          {branch.ahead > 0 && <StatusBadge variant="ahead" suffix={`\u2191${branch.ahead}`} />}
          {branch.behind > 0 && <StatusBadge variant="behind" suffix={`\u2193${branch.behind}`} />}
          {branch.is_current && <span className="text-xs text-accent font-medium">current</span>}
        </div>
      }
    >
      <div className="flex flex-col min-w-0">
        <span
          className={cn(
            "truncate text-sm",
            branch.is_current ? "font-medium text-text-primary" : "text-text-secondary",
          )}
          title={branch.name}
        >
          {branch.name}
        </span>
        <span
          className="flex items-center gap-1 min-w-0 h-4 text-xs text-text-muted"
          title={
            branch.upstream
              ? branch.upstream
              : branch.last_commit_sha
                ? `${branch.last_commit_sha}${branch.last_commit_time > 0 ? ` · ${new Date(branch.last_commit_time * 1000).toLocaleString()}` : ""}`
                : undefined
          }
        >
          {branch.upstream ? (
            <>
              <ArrowRight size={10} className="shrink-0" />
              <span className="truncate">{branch.upstream}</span>
            </>
          ) : branch.last_commit_sha ? (
            <span className="truncate">
              <span className="font-mono">{shortSha(branch.last_commit_sha)}</span>
              {branch.last_commit_time > 0 ? ` · ${formatTime(branch.last_commit_time)}` : ""}
            </span>
          ) : null}
        </span>
      </div>
    </ListItem>
  );

  if (!hasActions) return row;

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div>{row}</div>
      </ContextMenuTrigger>
      <ContextMenuContent className="max-w-[240px]">
        <ContextMenuLabel title={branch.name}>{branch.name}</ContextMenuLabel>
        <ContextMenuSeparator />
        <ContextMenuItem disabled={busy} onSelect={() => onMerge(branch.name)}>
          <GitMerge size={14} />
          Merge into current
        </ContextMenuItem>
        <ContextMenuItem disabled={busy} onSelect={() => onRebaseOnto(branch.name)}>
          <GitPullRequestArrow size={14} />
          Rebase current onto this
        </ContextMenuItem>
        <ContextMenuItem disabled={busy} onSelect={() => onInteractiveRebase(branch.name)}>
          <ListOrdered size={14} />
          Interactive rebase
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem destructive disabled={busy} onSelect={() => onDelete(branch.name)}>
          <Trash2 size={14} />
          Delete
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}

type BranchNotice = { text: string; variant: "success" | "danger" };

interface BranchListProps {
  /** Fired when the user clicks a branch row, with the full branch (name + tip sha). */
  onBranchSelect?: (branch: BranchInfo) => void;
}

export function BranchList({ onBranchSelect }: BranchListProps): React.JSX.Element {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistoryEpoch = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const historyEpoch = useWorkspaceUiStore((s) => s.historyEpoch);
  const queryClient = useQueryClient();
  const [branches, setBranches] = useState<BranchInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<BranchNotice | null>(null);
  const [irebaseOnto, setIrebaseOnto] = useState<string | null>(null);
  const [irebasePaused, setIrebasePaused] = useState(false);
  const [selectedName, setSelectedName] = useState<string | null>(null);
  const [deleteDialog, setDeleteDialog] = useState<{ name: string; deleteRemote: boolean } | null>(
    null,
  );
  const [switchDialog, setSwitchDialog] = useState<
    | { kind: "dirty"; name: string; fileCount: number }
    | { kind: "blocked"; name: string; message: string }
    | null
  >(null);

  const refresh = useCallback(() => {
    bumpHistoryEpoch();
  }, [bumpHistoryEpoch]);

  const showNotice = useCallback((text: string, variant: BranchNotice["variant"] = "success") => {
    setNotice({ text, variant });
  }, []);

  useEffect(() => {
    if (!notice) return;
    const timer = window.setTimeout(() => setNotice(null), 5000);
    return () => window.clearTimeout(timer);
  }, [notice]);

  useEffect(() => {
    // Repo switch: drop the previous repo's rows immediately so a click during
    // the reload window can't select a branch that belongs to the old repo.
    setSelectedName(null);
    setBranches([]);
  }, [activeRepoId]);

  useEffect(() => {
    if (!activeWorkspaceId || !activeRepoId) {
      setBranches([]);
      setError(null);
      setIrebasePaused(false);
      setLoading(false);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    getBranches(activeWorkspaceId)
      .then((updated) => {
        if (cancelled) return;
        setBranches(updated);
        setSelectedName((prev) => {
          if (prev && filterRemoteBranches(updated).some((b) => b.name === prev)) return prev;
          return updated.find((b) => b.is_current)?.name ?? null;
        });
      })
      .catch((e) => {
        if (!cancelled) setError(formatAppError(e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    interactiveRebasePaused(activeWorkspaceId)
      .then((paused) => {
        if (!cancelled) setIrebasePaused(paused);
      })
      .catch(() => {
        if (!cancelled) setIrebasePaused(false);
      });
    return () => {
      cancelled = true;
    };
  }, [activeWorkspaceId, activeRepoId, historyEpoch]);

  const run = async (fn: () => Promise<void>) => {
    if (!activeWorkspaceId || busy) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await fn();
      refresh();
      const paused = await interactiveRebasePaused(activeWorkspaceId);
      setIrebasePaused(paused);
    } catch (e) {
      setError(formatAppError(e));
    } finally {
      setBusy(false);
    }
  };

  const checkoutOnto = async (name: string, mode: "safe" | "force" | "stash") => {
    if (!activeWorkspaceId) return;
    if (mode === "stash") {
      await saveStash(activeWorkspaceId, `switch to ${name}`);
      await checkoutBranch(activeWorkspaceId, name, false);
      try {
        await popStash(activeWorkspaceId, 0);
        showNotice(`Checked out ${name} and re-applied stash`);
      } catch {
        showNotice(`Checked out ${name}. Stash re-apply failed; the stash was kept.`, "danger");
      }
    } else {
      await checkoutBranch(activeWorkspaceId, name, mode === "force");
      showNotice(`Checked out ${name}`);
    }
    setSelectedName(name);
    void queryClient.invalidateQueries({ queryKey: ["working-copy"] });
  };

  const handleCheckout = (name: string) => {
    if (!activeWorkspaceId || busy) return;
    const branch = branches.find((b) => b.name === name);
    if (!branch) return;
    void (async () => {
      setError(null);
      try {
        const [merging, worktrees, workingCopy] = await Promise.all([
          mergeInProgress(activeWorkspaceId),
          listWorktrees(activeWorkspaceId).catch(() => []),
          getWorkingCopy(activeWorkspaceId).catch(() => null),
        ]);
        const occupied = worktrees.find((w) => !w.is_main && w.branch === name);
        const gate = gateCheckout({
          isCurrent: branch.is_current,
          branchKind: branch.kind,
          dirtyCount: workingCopy?.files.length ?? 0,
          mergeInProgress: merging,
          rebasePaused: irebasePaused,
          occupiedWorktree: occupied?.name ?? null,
        });
        if (gate.kind === "noop") return;
        if (gate.kind === "blocked") {
          setSwitchDialog({ kind: "blocked", name, message: gate.message });
          return;
        }
        if (gate.kind === "dirty") {
          setSwitchDialog({ kind: "dirty", name, fileCount: gate.fileCount });
          return;
        }
        await run(() => checkoutOnto(name, "safe"));
      } catch (e) {
        setError(formatAppError(e));
      }
    })();
  };

  const handleSelect = (name: string) => {
    setSelectedName(name);
    const branch = branches.find((b) => b.name === name);
    if (branch) onBranchSelect?.(branch);
  };

  const deleteCounterparts = deleteDialog
    ? [
        ...new Set(
          branches
            .filter((b) => b.kind === "remote" && remoteShortName(b.name) === deleteDialog.name)
            .map((b) => b.name.slice(0, b.name.indexOf("/"))),
        ),
      ]
    : [];

  const handleConfirmDelete = () =>
    void run(async () => {
      if (!deleteDialog) return;
      const { name, deleteRemote } = deleteDialog;
      await deleteBranch(activeWorkspaceId!, name);
      if (deleteRemote) {
        for (const remote of deleteCounterparts) {
          await deleteRemoteBranch(activeWorkspaceId!, remote, name);
        }
      }
      setDeleteDialog(null);
      showNotice(
        deleteRemote && deleteCounterparts.length > 0
          ? `Deleted ${name} and its remote counterpart`
          : `Deleted ${name}`,
      );
    });

  const handleMerge = (name: string) =>
    void run(async () => {
      const result = await mergeBranch(activeWorkspaceId!, name);
      if (result.conflicts.length > 0) {
        showNotice(
          `Merged ${name} with ${result.conflicts.length} conflict(s): ${result.conflicts.join(", ")}`,
          "danger",
        );
      } else {
        showNotice(`Merged ${name} (${result.kind.replace(/_/g, " ")})`);
      }
    });

  const handleRebaseOnto = (name: string) =>
    void run(async () => {
      const result = await rebaseBranch(activeWorkspaceId!, name);
      if (result.kind === "conflicts" || result.conflicts.length > 0) {
        showNotice(
          `Rebase onto ${name} hit conflicts${
            result.conflicts.length ? `: ${result.conflicts.join(", ")}` : ""
          }`,
          "danger",
        );
      } else {
        showNotice(`Rebased onto ${name} (${result.kind.replace(/_/g, " ")})`);
      }
    });

  const handleContinueIrebase = () =>
    void run(async () => {
      const result = await continueInteractiveRebase(activeWorkspaceId!);
      if (result.kind === "conflicts") {
        showNotice(`Continue rebase conflicts: ${result.conflicts.join(", ")}`, "danger");
      } else if (result.kind === "paused_for_edit") {
        showNotice("Still paused for edit — amend then Continue again.");
      } else {
        showNotice(`Continued interactive rebase (${result.kind.replace(/_/g, " ")})`);
      }
    });

  const handleAbortIrebasePause = () =>
    void run(async () => {
      await abortInteractiveRebasePause(activeWorkspaceId!);
      showNotice("Cleared interactive rebase pause state");
    });

  const visibleBranches = filterRemoteBranches(branches);
  const localBranches = visibleBranches.filter((b) => b.kind === "local");
  const remoteBranches = visibleBranches.filter((b) => b.kind === "remote");

  const renderBody = (): React.JSX.Element => {
    if (!activeWorkspaceId) {
      return (
        <div className="flex items-center justify-center py-6 text-text-muted text-sm px-3 text-center">
          Select a workspace to view branches
        </div>
      );
    }
    if (!activeRepoId) {
      return (
        <div className="flex items-center justify-center py-6 text-text-muted text-sm px-3 text-center">
          Select a repository to view branches
        </div>
      );
    }
    if (loading && branches.length === 0) {
      return (
        <div className="flex items-center justify-center py-6 text-text-muted text-sm">
          Loading branches...
        </div>
      );
    }
    if (branches.length === 0) {
      return (
        <div className="flex items-center justify-center py-6 text-text-muted text-sm">
          No branches found
        </div>
      );
    }
    return (
      <>
        {localBranches.length > 0 && (
          <div>
            <div className="px-4 py-2 text-xs font-semibold text-text-muted uppercase tracking-wider">
              Local
            </div>
            {localBranches.map((branch) => (
              <BranchRow
                key={branch.name}
                branch={branch}
                selected={branch.name === selectedName}
                busy={busy}
                onSelect={handleSelect}
                onCheckout={handleCheckout}
                onDelete={(name) => setDeleteDialog({ name, deleteRemote: false })}
                onMerge={handleMerge}
                onRebaseOnto={handleRebaseOnto}
                onInteractiveRebase={(name) => setIrebaseOnto(name)}
              />
            ))}
          </div>
        )}

        {remoteBranches.length > 0 && (
          <div className="mt-2">
            <div className="px-4 py-2 text-xs font-semibold text-text-muted uppercase tracking-wider">
              Remote
            </div>
            {remoteBranches.map((branch) => (
              <BranchRow
                key={branch.name}
                branch={branch}
                selected={branch.name === selectedName}
                busy={busy}
                onSelect={handleSelect}
                onCheckout={handleCheckout}
                onDelete={(name) => setDeleteDialog({ name, deleteRemote: false })}
                onMerge={handleMerge}
                onRebaseOnto={handleRebaseOnto}
                onInteractiveRebase={(name) => setIrebaseOnto(name)}
              />
            ))}
          </div>
        )}
      </>
    );
  };

  return (
    <>
      <SidebarSection title="Branches" className="border-b-0">
        {irebasePaused ? (
          <div className="shrink-0 flex items-center gap-2 px-3 py-2 border-b border-border-subtle">
            <Button variant="primary" size="sm" disabled={busy} onClick={handleContinueIrebase}>
              Continue rebase
            </Button>
            <Button variant="ghost" size="sm" disabled={busy} onClick={handleAbortIrebasePause}>
              Discard pause
            </Button>
          </div>
        ) : null}

        {notice ? (
          <div
            className={cn(
              "shrink-0 px-3 py-2 text-xs border-b",
              notice.variant === "success" && "bg-success/20 text-success border-b-success/40",
              notice.variant === "danger" && "bg-danger/20 text-danger border-b-danger/40",
            )}
            role="status"
          >
            {notice.text}
          </div>
        ) : null}

        <div>{renderBody()}</div>
      </SidebarSection>

      <ErrorAlert message={error} onDismiss={() => setError(null)} />

      {switchDialog?.kind === "dirty" ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setSwitchDialog(null);
          }}
          title={`Switch to ${switchDialog.name}?`}
          description={`Working copy has ${switchDialog.fileCount} uncommitted file${switchDialog.fileCount === 1 ? "" : "s"}. Discard them, or stash, switch, and re-apply the stash on the new branch.`}
          size="sm"
        >
          <div className="flex flex-wrap justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => setSwitchDialog(null)}>
              Cancel
            </Button>
            <Button
              variant="danger"
              size="sm"
              disabled={busy}
              onClick={() => {
                const target = switchDialog.name;
                setSwitchDialog(null);
                void run(() => checkoutOnto(target, "force"));
              }}
            >
              Discard
            </Button>
            <Button
              variant="primary"
              size="sm"
              disabled={busy}
              onClick={() => {
                const target = switchDialog.name;
                setSwitchDialog(null);
                void run(() => checkoutOnto(target, "stash"));
              }}
            >
              Stash & switch
            </Button>
          </div>
        </Modal>
      ) : null}

      {switchDialog?.kind === "blocked" ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setSwitchDialog(null);
          }}
          title={`Cannot switch to ${switchDialog.name}`}
          description={switchDialog.message}
          size="sm"
        >
          <div className="flex justify-end">
            <Button variant="primary" size="sm" onClick={() => setSwitchDialog(null)}>
              OK
            </Button>
          </div>
        </Modal>
      ) : null}

      {deleteDialog ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setDeleteDialog(null);
          }}
          title="Delete Branch"
          description="Delete local branch from your repository"
          size="sm"
        >
          <div className="flex items-center gap-2">
            <span className="w-16 shrink-0 text-right text-xs text-text-secondary">Branch</span>
            <span className="flex min-w-0 items-center gap-1 text-xs font-mono text-text-primary">
              <GitBranch size={12} className="shrink-0 text-accent" />
              <span className="truncate" title={deleteDialog.name}>
                {deleteDialog.name}
              </span>
            </span>
          </div>
          <label
            className={cn(
              "mt-3 flex items-center gap-2 text-xs",
              deleteCounterparts.length > 0 ? "text-text-primary" : "text-text-muted",
            )}
            title={deleteCounterparts.length > 0 ? undefined : "No corresponding remote branch"}
          >
            <input
              type="checkbox"
              className="accent-accent"
              checked={deleteDialog.deleteRemote}
              disabled={busy || deleteCounterparts.length === 0}
              onChange={(e) => setDeleteDialog({ ...deleteDialog, deleteRemote: e.target.checked })}
            />
            Also delete corresponding remote branch
          </label>
          <div className="mt-3 flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setDeleteDialog(null)}>
              Cancel
            </Button>
            <Button variant="danger" size="sm" disabled={busy} onClick={handleConfirmDelete}>
              Delete
            </Button>
          </div>
        </Modal>
      ) : null}

      {irebaseOnto && activeWorkspaceId ? (
        <InteractiveRebaseDialog
          open={true}
          workspaceId={activeWorkspaceId}
          upstream={irebaseOnto}
          onClose={() => setIrebaseOnto(null)}
          onDone={(msg) => {
            showNotice(msg);
            refresh();
            void interactiveRebasePaused(activeWorkspaceId).then(setIrebasePaused);
          }}
        />
      ) : null}
    </>
  );
}
