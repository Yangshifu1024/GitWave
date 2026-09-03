// Sidebar branch list — navigation plus row-scoped operations (checkout via
// double-click, merge / rebase / delete via context menu). Toolbar-scoped
// branch ops (new branch / pull / push) live in the ActionBar.

import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import type { BranchInfo, InlineAuth } from "@/lib/api";
import {
  abortInteractiveRebasePause,
  continueInteractiveRebase,
  deleteBranch,
  deleteRemoteBranch,
  formatAppError,
  getBranches,
  interactiveRebasePaused,
  isAuthError,
  isCancelledSyncError,
  listRemotes,
  mergeBranch,
  mergePreview,
  pushRemote,
  rebaseBranch,
  renameBranch,
  createBranch,
  setBranchUpstream,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useSyncStore, type UiOperation } from "@/stores/syncStore";
import { useAuthPromptStore } from "@/stores/authPromptStore";
import { useTags } from "@/hooks/useTags";
import { useBranchCheckout } from "@/hooks/useBranchCheckout";
import { cn } from "@/lib/utils";
import { copyToClipboard } from "@/lib/commitMenu";
import { withAuthRetry } from "@/lib/authRetry";
import { filterRemoteBranches, remoteShortName, splitBranchPrefix } from "@/lib/branchNames";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { Checkbox } from "@/components/ui/Checkbox";
import { Label } from "@/components/ui/Label";
import { StatusBadge } from "@/components/ui/StatusBadge";
import { ListItem } from "@/components/ui/ListItem";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuTrigger,
} from "@/components/ui/ContextMenu";
import { InteractiveRebaseDialog } from "@/components/InteractiveRebaseDialog";
import { TagManagerModal } from "@/components/TagManagerModal";
import {
  ArrowRight,
  ArrowUp,
  ChevronDown,
  ChevronRight,
  CircleCheck,
  CircleX,
  Copy,
  Folder,
  GitBranch,
  GitMerge,
  GitPullRequestArrow,
  Link2,
  ListOrdered,
  Pencil,
  Tag as TagIcon,
  Trash2,
} from "lucide-react";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { ErrorAlert } from "@/components/ui/ErrorAlert";

function formatTime(time: number, t: TFunction): string {
  if (time <= 0) return "";
  const now = Math.floor(Date.now() / 1000);
  const diff = now - time;
  if (diff < 60) return t("branches.time.justNow");
  if (diff < 3600) return t("branches.time.minutesAgo", { n: Math.floor(diff / 60) });
  if (diff < 86400) return t("branches.time.hoursAgo", { n: Math.floor(diff / 3600) });
  if (diff < 604800) return t("branches.time.daysAgo", { n: Math.floor(diff / 86400) });
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
  /** Text shown in the row (prefix stripped when nested in a folder). */
  displayName?: string;
  /** Folder children sit one indent level deeper than top-level rows. */
  indented?: boolean;
  selected: boolean;
  busy: boolean;
  onSelect: (name: string) => void;
  onCheckout: (name: string) => void;
  /** Pick the push target explicitly (Fork-style submenu on multi-remote). */
  onPush: (branch: BranchInfo, remote: string) => void;
  onDelete: (name: string) => void;
  onMerge: (name: string) => void;
  onRebaseOnto: (name: string) => void;
  onInteractiveRebase: (name: string) => void;
  onNewBranch: (name: string, sha: string | null) => void;
  onNewTag: (branch: BranchInfo) => void;
  onTracking: (branch: BranchInfo) => void;
  onRename: (branch: BranchInfo) => void;
  onCopyName: (name: string) => void;
  /** Configured remote names; drives flat item vs submenu on the push entry. */
  remotes: string[];
}

function BranchRow({
  branch,
  displayName,
  indented,
  selected,
  busy,
  onSelect,
  onCheckout,
  onPush,
  onDelete,
  onMerge,
  onRebaseOnto,
  onInteractiveRebase,
  onNewBranch,
  onNewTag,
  onTracking,
  onRename,
  onCopyName,
  remotes,
}: BranchRowProps): React.JSX.Element {
  const { t } = useTranslation();

  const row = (
    <ListItem
      selected={selected}
      className={indented ? "pl-8" : "pl-4"}
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
          {branch.is_current && (
            <span className="text-xs text-accent font-medium">{t("branches.row.current")}</span>
          )}
        </div>
      }
    >
      <div className="flex flex-col min-w-0">
        <span
          className={cn(
            "truncate text-[13px]",
            selected
              ? "font-semibold text-text-primary"
              : branch.is_current
                ? "font-medium text-text-primary"
                : "text-text-secondary",
          )}
          title={branch.name}
        >
          {displayName ?? branch.name}
        </span>
        <span
          className="flex items-center gap-1 min-w-0 h-4 text-[11px] text-text-muted"
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
              {branch.last_commit_time > 0 ? ` · ${formatTime(branch.last_commit_time, t)}` : ""}
            </span>
          ) : null}
        </span>
      </div>
    </ListItem>
  );

  // Local rows get the full Fork-style menu (navigation + branch ops; merge /
  // rebase / delete stay hidden on the current branch — they don't apply to
  // HEAD). Remote rows (F012) get a minimal menu: DWIM checkout — same flow
  // as double-click — plus copy name.
  const menuContent =
    branch.kind === "local" ? (
      <>
        <ContextMenuLabel title={branch.name}>{branch.name}</ContextMenuLabel>
        <ContextMenuSeparator />
        <ContextMenuItem
          disabled={busy || branch.is_current}
          title={branch.is_current ? t("branches.guard.current") : undefined}
          onSelect={() => onCheckout(branch.name)}
        >
          <GitBranch size={14} />
          {t("branches.menu.checkout")}
        </ContextMenuItem>
        {remotes.length === 1 ? (
          <ContextMenuItem disabled={busy} onSelect={() => onPush(branch, remotes[0]!)}>
            <ArrowUp size={14} />
            {t("branches.menu.push", { remote: remotes[0] })}
          </ContextMenuItem>
        ) : remotes.length > 1 ? (
          <ContextMenuSub
            disabled={busy}
            icon={<ArrowUp size={14} />}
            label={t("branches.menu.pushGeneric")}
          >
            {remotes.map((remote) => (
              <ContextMenuItem key={remote} onSelect={() => onPush(branch, remote)}>
                {remote}
              </ContextMenuItem>
            ))}
          </ContextMenuSub>
        ) : (
          <ContextMenuItem disabled>
            <ArrowUp size={14} />
            {t("branches.menu.pushGeneric")}
          </ContextMenuItem>
        )}
        <ContextMenuItem
          disabled={busy || !branch.last_commit_sha}
          onSelect={() => onNewBranch(branch.name, branch.last_commit_sha)}
        >
          <GitBranch size={14} />
          {t("branches.menu.new")}
        </ContextMenuItem>
        <ContextMenuItem
          disabled={busy || !branch.last_commit_sha}
          onSelect={() => onNewTag(branch)}
        >
          <TagIcon size={14} />
          {t("branches.menu.newTag")}
        </ContextMenuItem>
        <ContextMenuItem disabled={busy} onSelect={() => onTracking(branch)}>
          <Link2 size={14} />
          {t("branches.menu.tracking")}
        </ContextMenuItem>
        <ContextMenuItem disabled={busy} onSelect={() => onRename(branch)}>
          <Pencil size={14} />
          {t("branches.menu.rename")}
        </ContextMenuItem>
        <ContextMenuItem
          destructive
          disabled={busy || branch.is_current}
          title={branch.is_current ? t("branches.guard.currentBranch") : undefined}
          onSelect={() => onDelete(branch.name)}
        >
          <Trash2 size={14} />
          {t("branches.delete")}
        </ContextMenuItem>
        <ContextMenuItem onSelect={() => onCopyName(branch.name)}>
          <Copy size={14} />
          {t("branches.menu.copyName")}
        </ContextMenuItem>
        {!branch.is_current ? (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem disabled={busy} onSelect={() => onMerge(branch.name)}>
              <GitMerge size={14} />
              {t("branches.menu.mergeIntoCurrent")}
            </ContextMenuItem>
            <ContextMenuItem disabled={busy} onSelect={() => onRebaseOnto(branch.name)}>
              <GitPullRequestArrow size={14} />
              {t("branches.menu.rebaseCurrentOntoThis")}
            </ContextMenuItem>
            <ContextMenuItem disabled={busy} onSelect={() => onInteractiveRebase(branch.name)}>
              <ListOrdered size={14} />
              {t("branches.menu.interactiveRebase")}
            </ContextMenuItem>
          </>
        ) : null}
      </>
    ) : (
      <>
        <ContextMenuLabel title={branch.name}>{branch.name}</ContextMenuLabel>
        <ContextMenuSeparator />
        <ContextMenuItem disabled={busy} onSelect={() => onCheckout(branch.name)}>
          <GitBranch size={14} />
          {t("branches.menu.checkout")}
        </ContextMenuItem>
        <ContextMenuItem onSelect={() => onCopyName(branch.name)}>
          <Copy size={14} />
          {t("branches.menu.copyName")}
        </ContextMenuItem>
      </>
    );

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div>{row}</div>
      </ContextMenuTrigger>
      <ContextMenuContent className="max-w-[260px]">{menuContent}</ContextMenuContent>
    </ContextMenu>
  );
}

interface BranchListProps {
  /** Fired when the user clicks a branch row, with the full branch (name + tip sha). */
  onBranchSelect?: (branch: BranchInfo) => void;
}

export function BranchList({ onBranchSelect }: BranchListProps): React.JSX.Element {
  const { t } = useTranslation();
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistoryEpoch = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const historyEpoch = useWorkspaceUiStore((s) => s.historyEpoch);
  const { data: tags = [], invalidate: invalidateTags } = useTags();
  // Configured remotes for the branch-menu push entry (flat item vs Fork-style
  // submenu on multi-remote). Shares the ActionBar dialogs' cache.
  const remotesQuery = useQuery({
    queryKey: ["remotes", activeWorkspaceId],
    queryFn: () => listRemotes(activeWorkspaceId!),
    enabled: Boolean(activeWorkspaceId),
  });
  const remotes = useMemo(() => remotesQuery.data ?? [], [remotesQuery.data]);
  const [branches, setBranches] = useState<BranchInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [irebaseOnto, setIrebaseOnto] = useState<string | null>(null);
  const [irebasePaused, setIrebasePaused] = useState(false);
  const [selectedName, setSelectedName] = useState<string | null>(null);
  const [deleteDialog, setDeleteDialog] = useState<{ name: string; deleteRemote: boolean } | null>(
    null,
  );
  /** Fork-style confirmation before "Merge into current" executes. */
  const [mergeDialog, setMergeDialog] = useState<{ name: string } | null>(null);
  const [newBranchBase, setNewBranchBase] = useState<{ name: string; sha: string } | null>(null);
  const [newBranchName, setNewBranchName] = useState("");
  const [newBranchError, setNewBranchError] = useState<string | null>(null);
  // F011 extension: Fork-style branch menu actions. pushConfirm carries the
  // user-chosen remote (submenu pick on multi-remote repos).
  const [pushConfirm, setPushConfirm] = useState<{ branch: BranchInfo; remote: string } | null>(
    null,
  );
  const [tagBranch, setTagBranch] = useState<BranchInfo | null>(null);
  const [renameDialog, setRenameDialog] = useState<{
    branch: BranchInfo;
    name: string;
    error: string | null;
  } | null>(null);
  const [trackingDialog, setTrackingDialog] = useState<BranchInfo | null>(null);
  const [trackingPick, setTrackingPick] = useState("");

  const refresh = useCallback(() => {
    bumpHistoryEpoch();
  }, [bumpHistoryEpoch]);

  const setStatus = useStatusAreaStore((s) => s.setStatus);
  const startOp = useSyncStore((s) => s.startOp);
  const endOp = useSyncStore((s) => s.endOp);
  // Sidebar operation results (checkout, merge, rebase, delete) surface in
  // the ActionBar status area — the single operation-status surface.
  const showNotice = (text: string, variant: "success" | "danger" = "success") => {
    setStatus(text, variant);
  };

  // F004/F012 checkout flow (gate + dirty three-choice + DWIM remote
  // checkout) lives in the shared hook; the sidebar used to keep its own copy.
  const {
    busy: checkoutBusy,
    request: requestCheckout,
    renderDialogs: renderCheckoutDialogs,
  } = useBranchCheckout({
    onSwitched: (target) => {
      setSelectedName(target);
      refresh();
    },
  });

  useEffect(() => {
    // Repo switch: drop the previous repo's selection immediately, but keep
    // rendering the old rows — the refetch lands in milliseconds and the rows
    // are inert while `loading`, so blanking would only flash the UI.
    setSelectedName(null);
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

  const run = async (op: UiOperation, fn: () => Promise<void>) => {
    if (!activeWorkspaceId || busy) return;
    setBusy(true);
    setError(null);
    startOp(op);
    try {
      await fn();
      refresh();
      const paused = await interactiveRebasePaused(activeWorkspaceId);
      setIrebasePaused(paused);
    } catch (e) {
      setError(formatAppError(e));
    } finally {
      setBusy(false);
      endOp(op);
    }
  };

  /** Resolve the row's branch, then let the shared hook gate and switch. */
  const handleCheckout = (name: string) => {
    const branch = branches.find((b) => b.name === name);
    if (!branch) return;
    requestCheckout(name, { kind: branch.kind, isCurrent: branch.is_current });
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
    void run("delete", async () => {
      if (!deleteDialog) return;
      const { name, deleteRemote } = deleteDialog;
      await deleteBranch(activeWorkspaceId!, name);
      let remoteDeleted = false;
      if (deleteRemote) {
        try {
          for (const remote of deleteCounterparts) {
            // F012: an auth-challenged remote delete opens the in-app
            // prompt and retries with the entered credentials.
            await withAuthRetry(remote, (auth) =>
              deleteRemoteBranch(activeWorkspaceId!, remote, name, auth),
            );
            remoteDeleted = true;
          }
        } catch (e) {
          // Dismissing the prompt keeps the (already applied) local
          // deletion — a user cancel, not an error; the remaining remote
          // counterparts simply stay.
          if (!isCancelledSyncError(e)) throw e;
        }
      }
      setDeleteDialog(null);
      showNotice(
        remoteDeleted
          ? t("branches.deleteDialog.deletedWithRemote", { name })
          : t("branches.deleteDialog.deleted", { name }),
      );
    });

  const handleMerge = (name: string) => setMergeDialog({ name });

  const openNewBranch = (name: string, sha: string | null): void => {
    if (!sha) return;
    setNewBranchName("");
    setNewBranchError(null);
    setNewBranchBase({ name, sha });
  };

  const submitNewBranch = async (): Promise<void> => {
    const name = newBranchName.trim();
    if (!newBranchBase || !name || !activeWorkspaceId) return;
    setBusy(true);
    setNewBranchError(null);
    try {
      await createBranch(activeWorkspaceId, name, newBranchBase.sha);
      setNewBranchBase(null);
      setNewBranchName("");
      refresh();
      showNotice(t("branches.newBranch.created", { name, base: newBranchBase.name }));
    } catch (e) {
      setNewBranchError(formatAppError(e));
    } finally {
      setBusy(false);
    }
  };

  // ─── Fork-style branch menu actions (F011 extension) ──────────────────────

  const copyBranchName = (name: string): void => {
    void copyToClipboard(name).then((ok) =>
      showNotice(
        ok ? t("commits.menu.copied") : t("commits.explain.clipboardUnavailable"),
        ok ? "success" : "danger",
      ),
    );
  };

  const openRename = (branch: BranchInfo): void =>
    setRenameDialog({ branch, name: branch.name, error: null });

  const openTracking = (branch: BranchInfo): void => {
    setTrackingPick(branch.upstream ?? "");
    setTrackingDialog(branch);
  };

  /** Plain (non-force) push of the branch to the user-chosen remote. */
  const submitPush = (): void => {
    const target = pushConfirm;
    // One network op at a time: a second concurrent push would double-claim
    // the "push" status slot and can double-prompt for credentials.
    if (!activeWorkspaceId || !target || busy || useSyncStore.getState().isBusy()) return;
    const { branch, remote } = target;
    const run = (auth?: InlineAuth): void => {
      setBusy(true);
      startOp("push", remote);
      pushRemote(activeWorkspaceId, { remote, branch: branch.name, auth })
        .then(() => showNotice(t("branches.push.done", { name: branch.name, remote })))
        .catch((e) => {
          if (isCancelledSyncError(e)) {
            showNotice(t("status.sync.cancelled"));
          } else if (isAuthError(e) && auth === undefined) {
            // F012: collect credentials in-app and retry once; a second
            // auth failure surfaces as a plain error (no prompt loop).
            useAuthPromptStore.getState().show(remote, (a) => run(a));
          } else {
            showNotice(formatAppError(e), "danger");
          }
        })
        .finally(() => {
          setBusy(false);
          endOp("push");
          setPushConfirm(null);
        });
    };
    run();
  };

  const submitRename = (): void => {
    const dialog = renameDialog;
    const name = dialog?.name.trim();
    if (!activeWorkspaceId || !dialog || !name || busy) return;
    setBusy(true);
    setRenameDialog({ ...dialog, error: null });
    renameBranch(activeWorkspaceId, dialog.branch.name, name)
      .then(() => {
        showNotice(t("branches.rename.done", { old: dialog.branch.name, name }));
        setRenameDialog(null);
        refresh();
      })
      .catch((e) =>
        setRenameDialog((prev) => (prev ? { ...prev, error: formatAppError(e) } : prev)),
      )
      .finally(() => setBusy(false));
  };

  const submitTracking = (): void => {
    const branch = trackingDialog;
    if (!activeWorkspaceId || !branch || busy) return;
    const upstream = trackingPick.trim() || null;
    setBusy(true);
    setBranchUpstream(activeWorkspaceId, branch.name, upstream)
      .then(() => {
        showNotice(
          upstream
            ? t("branches.tracking.done", { name: branch.name, upstream })
            : t("branches.tracking.cleared", { name: branch.name }),
        );
        setTrackingDialog(null);
        refresh();
      })
      .catch((e) => showNotice(formatAppError(e), "danger"))
      .finally(() => setBusy(false));
  };

  const handleMergeConfirmed = (name: string, noFf: boolean) =>
    void run("merge", async () => {
      const result = await mergeBranch(activeWorkspaceId!, name, noFf);
      if (result.conflicts.length > 0) {
        showNotice(
          t("branches.merge.withConflicts", {
            name,
            n: result.conflicts.length,
            files: result.conflicts.join(", "),
          }),
          "danger",
        );
      } else {
        showNotice(t("branches.merge.success", { name, kind: result.kind.replace(/_/g, " ") }));
      }
    });

  const handleRebaseOnto = (name: string) =>
    void run("rebase", async () => {
      const result = await rebaseBranch(activeWorkspaceId!, name);
      if (result.kind === "conflicts" || result.conflicts.length > 0) {
        showNotice(
          result.conflicts.length
            ? t("branches.rebase.hitConflicts", { name, files: result.conflicts.join(", ") })
            : t("branches.rebase.hitConflictsPlain", { name }),
          "danger",
        );
      } else {
        showNotice(t("branches.rebase.success", { name, kind: result.kind.replace(/_/g, " ") }));
      }
    });

  const handleContinueIrebase = () =>
    void run("rebase", async () => {
      const result = await continueInteractiveRebase(activeWorkspaceId!);
      if (result.kind === "conflicts") {
        showNotice(
          t("branches.irebase.continueConflicts", { files: result.conflicts.join(", ") }),
          "danger",
        );
      } else if (result.kind === "paused_for_edit") {
        showNotice(t("branches.irebase.stillPaused"));
      } else {
        showNotice(t("branches.irebase.continued", { kind: result.kind.replace(/_/g, " ") }));
      }
    });

  const handleAbortIrebasePause = () =>
    void run("rebase", async () => {
      await abortInteractiveRebasePause(activeWorkspaceId!);
      showNotice(t("branches.irebase.pauseCleared"));
    });

  const visibleBranches = filterRemoteBranches(branches);
  const localBranches = visibleBranches.filter((b) => b.kind === "local");
  const remoteBranches = visibleBranches.filter((b) => b.kind === "remote");

  // Remote branches grouped by their remote (first path segment), in first-seen order.
  const remoteGroups = (() => {
    const map = new Map<string, BranchInfo[]>();
    for (const b of remoteBranches) {
      const slash = b.name.indexOf("/");
      const remote = slash === -1 ? b.name : b.name.slice(0, slash);
      const list = map.get(remote) ?? [];
      list.push(b);
      map.set(remote, list);
    }
    return [...map.entries()];
  })();

  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({});
  // Flip the *effective* collapsed state (raw value falls back to the
  // group's default), otherwise a default-collapsed group needs two clicks
  // — the first write (`!undefined` = true) is a visual no-op.
  const toggleGroup = (key: string, defaultCollapsed: boolean): void =>
    setCollapsedGroups((prev) => ({ ...prev, [key]: !(prev[key] ?? defaultCollapsed) }));

  const renderGroup = (
    label: string,
    groupKey: string,
    groupBranches: BranchInfo[],
  ): React.JSX.Element | null => {
    if (groupBranches.length === 0) return null;
    // Local defaults expanded; remote groups default collapsed.
    const collapsed = collapsedGroups[groupKey] ?? groupKey !== "local";
    // Fork-style sub-grouping: branches sharing the first display-name
    // segment (after the remote prefix for remote-tracking branches)
    // collapse into a folder; unprefixed branches stay at the top level.
    const display = (b: BranchInfo): string =>
      b.kind === "remote" ? remoteShortName(b.name) : b.name;
    const roots: BranchInfo[] = [];
    const folders = new Map<string, BranchInfo[]>();
    for (const b of groupBranches) {
      const { prefix } = splitBranchPrefix(display(b));
      if (prefix === null) {
        roots.push(b);
      } else {
        const list = folders.get(prefix) ?? [];
        if (list.length === 0) folders.set(prefix, list);
        list.push(b);
      }
    }
    const folderList = [...folders.entries()].sort(([a], [b]) => a.localeCompare(b));
    const renderRows = (list: BranchInfo[], nameOf: (b: BranchInfo) => string, indented: boolean) =>
      list.map((branch) => (
        <BranchRow
          key={branch.name}
          branch={branch}
          displayName={nameOf(branch)}
          indented={indented}
          selected={branch.name === selectedName}
          busy={busy || loading || checkoutBusy}
          onSelect={handleSelect}
          onCheckout={handleCheckout}
          onDelete={(name) => setDeleteDialog({ name, deleteRemote: false })}
          onMerge={handleMerge}
          onRebaseOnto={handleRebaseOnto}
          onInteractiveRebase={(name) => setIrebaseOnto(name)}
          onNewBranch={openNewBranch}
          onPush={(b, remote) => setPushConfirm({ branch: b, remote })}
          onNewTag={(b) => setTagBranch(b)}
          onTracking={openTracking}
          onRename={openRename}
          onCopyName={copyBranchName}
          remotes={remotes}
        />
      ));
    return (
      <div className={groupKey === "local" ? undefined : "mt-2"}>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-expanded={!collapsed}
          onClick={() => toggleGroup(groupKey, groupKey !== "local")}
          className="h-auto w-full justify-start flex items-center gap-1.5 pl-3 pr-3 py-1 text-[11px] font-semibold text-text-muted uppercase tracking-wider hover:text-text-secondary rounded-none border-0 shadow-none bg-transparent"
        >
          {collapsed ? <ChevronRight size={12} /> : <ChevronDown size={12} />}
          {label}
          <span className="font-normal normal-case">({groupBranches.length})</span>
        </Button>
        {!collapsed && (
          <>
            {/* Top-level rows (unprefixed) at the base indent; folder
                children one level deeper. */}
            {renderRows(roots, display, false)}
            {folderList.map(([prefix, list]) => {
              const folderKey = `${groupKey}:${prefix}`;
              // Default collapsed, except the folder holding the selected
              // branch; an explicit toggle always wins over the default.
              const folderCollapsed =
                collapsedGroups[folderKey] ?? !list.some((b) => b.name === selectedName);
              return (
                <div key={folderKey}>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    aria-expanded={!folderCollapsed}
                    onClick={() =>
                      toggleGroup(folderKey, !list.some((b) => b.name === selectedName))
                    }
                    className="h-auto w-full justify-start flex items-center gap-1.5 pl-6 pr-3 py-1 text-[11px] font-medium text-text-secondary hover:text-text-primary rounded-none border-0 shadow-none bg-transparent"
                  >
                    {folderCollapsed ? <ChevronRight size={10} /> : <ChevronDown size={10} />}
                    <Folder size={11} className="shrink-0 text-text-muted" />
                    <span className="truncate">{prefix}</span>
                    <span className="font-normal text-text-muted">({list.length})</span>
                  </Button>
                  {!folderCollapsed &&
                    renderRows(list, (b) => splitBranchPrefix(display(b)).rest, true)}
                </div>
              );
            })}
          </>
        )}
      </div>
    );
  };

  const renderBody = (): React.JSX.Element => {
    if (!activeWorkspaceId) {
      return (
        <div className="flex items-center justify-center py-6 text-text-muted text-sm px-3 text-center">
          {t("branches.list.selectWorkspace")}
        </div>
      );
    }
    if (!activeRepoId) {
      return (
        <div className="flex items-center justify-center py-6 text-text-muted text-sm px-3 text-center">
          {t("branches.list.selectRepo")}
        </div>
      );
    }
    if (loading && branches.length === 0) {
      // Cold start only: nothing cached to keep on screen yet, so render an
      // empty body instead of a spinner; the list pops in when data lands.
      return <div className="h-8" />;
    }
    if (branches.length === 0) {
      return (
        <div className="flex items-center justify-center py-6 text-text-muted text-sm">
          {t("branches.list.empty")}
        </div>
      );
    }
    return (
      <>
        {renderGroup(t("branches.list.localGroup"), "local", localBranches)}
        {remoteGroups.map(([remote, list]) => renderGroup(remote, `remote:${remote}`, list))}
      </>
    );
  };

  return (
    <>
      <SidebarSection title={t("branches.title")}>
        {irebasePaused ? (
          <div className="shrink-0 flex items-center gap-2 px-3 py-2 border-b border-border-subtle">
            <Button variant="primary" size="sm" disabled={busy} onClick={handleContinueIrebase}>
              {t("branches.irebase.continue")}
            </Button>
            <Button variant="ghost" size="sm" disabled={busy} onClick={handleAbortIrebasePause}>
              {t("branches.irebase.discardPause")}
            </Button>
          </div>
        ) : null}

        <div>{renderBody()}</div>
      </SidebarSection>

      <ErrorAlert message={error} onDismiss={() => setError(null)} />

      {renderCheckoutDialogs()}

      {newBranchBase ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setNewBranchBase(null);
          }}
          title={t("branches.newBranch.title", { name: newBranchBase.name })}
          description={t("branches.newBranch.description")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setNewBranchBase(null)}
              >
                {t("branches.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy || !newBranchName.trim()}
                onClick={() => void submitNewBranch()}
              >
                {t("branches.newBranch.create")}
              </Button>
            </>
          }
        >
          <div className="rounded-xl bg-bg-primary p-3">
            <Input
              autoFocus
              value={newBranchName}
              onChange={setNewBranchName}
              onKeyDown={(e) => {
                if (e.key === "Enter" && newBranchName.trim()) void submitNewBranch();
              }}
              placeholder={t("branches.newBranch.placeholder")}
              error={newBranchError}
            />
          </div>
        </Modal>
      ) : null}

      {deleteDialog ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setDeleteDialog(null);
          }}
          title={t("branches.deleteDialog.title")}
          description={t("branches.deleteDialog.description")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setDeleteDialog(null)}
              >
                {t("branches.cancel")}
              </Button>
              <Button
                variant="danger"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy}
                onClick={handleConfirmDelete}
              >
                {t("branches.delete")}
              </Button>
            </>
          }
        >
          <div className="flex items-center gap-2 rounded-xl bg-bg-primary p-3">
            <span className="w-16 shrink-0 text-sm text-text-secondary">
              {t("branches.deleteDialog.branchLabel")}
            </span>
            <span className="flex min-w-0 items-center gap-1 text-sm text-text-primary">
              <GitBranch size={12} className="shrink-0 text-accent" />
              <span className="truncate" title={deleteDialog.name}>
                {deleteDialog.name}
              </span>
            </span>
          </div>
          <div className="rounded-xl bg-bg-primary p-3">
            <Checkbox
              checked={deleteDialog.deleteRemote}
              disabled={busy || deleteCounterparts.length === 0}
              onChange={(deleteRemote) => setDeleteDialog({ ...deleteDialog, deleteRemote })}
              className={cn(deleteCounterparts.length === 0 && "text-text-muted")}
            >
              {t("branches.deleteDialog.alsoDeleteRemote")}
            </Checkbox>
          </div>
        </Modal>
      ) : null}

      {mergeDialog && activeWorkspaceId ? (
        <MergeConfirmDialog
          workspaceId={activeWorkspaceId}
          name={mergeDialog.name}
          currentBranch={branches.find((b) => b.is_current)?.name ?? "—"}
          onClose={() => setMergeDialog(null)}
          onConfirm={(noFf) => {
            setMergeDialog(null);
            handleMergeConfirmed(mergeDialog.name, noFf);
          }}
        />
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

      {/* Push: plain push of the branch to the remote picked in the row
          menu, behind a Fork-style confirm (same product decision as the
          toolbar push). */}
      {pushConfirm ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open && !busy) setPushConfirm(null);
          }}
          title={t("commits.sync.pushTitle")}
          description={t("branches.push.confirmDescription", {
            name: pushConfirm.branch.name,
            remote: pushConfirm.remote,
          })}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                disabled={busy}
                onClick={() => setPushConfirm(null)}
              >
                {t("branches.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy}
                onClick={submitPush}
              >
                {t("commits.sync.push")}
              </Button>
            </>
          }
        />
      ) : null}

      {tagBranch?.last_commit_sha && activeWorkspaceId ? (
        <TagManagerModal
          workspaceId={activeWorkspaceId}
          sha={tagBranch.last_commit_sha}
          tags={tags}
          onClose={() => setTagBranch(null)}
          onChanged={() => {
            invalidateTags();
            bumpHistoryEpoch();
          }}
          onError={(message) => showNotice(message, "danger")}
          onCreated={(name) =>
            showNotice(
              t("commits.tag.created", { sha: tagBranch.last_commit_sha.slice(0, 7), name }),
            )
          }
        />
      ) : null}

      {renameDialog ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setRenameDialog(null);
          }}
          title={t("branches.rename.title")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setRenameDialog(null)}
              >
                {t("branches.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={
                  busy ||
                  !renameDialog.name.trim() ||
                  renameDialog.name.trim() === renameDialog.branch.name
                }
                onClick={() => void submitRename()}
              >
                {t("branches.rename.confirm")}
              </Button>
            </>
          }
        >
          <div className="rounded-xl bg-bg-primary p-3">
            <Input
              autoFocus
              value={renameDialog.name}
              onChange={(name) => setRenameDialog({ ...renameDialog, name })}
              onKeyDown={(e) => {
                if (e.key === "Enter" && renameDialog.name.trim()) void submitRename();
              }}
              error={renameDialog.error}
            />
          </div>
        </Modal>
      ) : null}

      {trackingDialog ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setTrackingDialog(null);
          }}
          title={t("branches.tracking.title", { name: trackingDialog.name })}
          description={t("branches.tracking.description", {
            upstream: trackingDialog.upstream ?? t("branches.tracking.none"),
          })}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setTrackingDialog(null)}
              >
                {t("branches.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy}
                onClick={submitTracking}
              >
                {t("branches.tracking.confirm")}
              </Button>
            </>
          }
        >
          <div className="rounded-xl bg-bg-primary p-3">
            <Select
              aria-label={t("branches.tracking.aria")}
              value={trackingPick}
              onChange={setTrackingPick}
              disabled={busy}
              options={[
                { value: "", label: t("branches.tracking.none") },
                ...remoteBranches.map((b) => ({ value: b.name, label: b.name })),
              ]}
            />
          </div>
        </Modal>
      ) : null}
    </>
  );
}

/**
 * Fork-style confirmation before "Merge into current" runs: shows the
 * source/target pair, a fast-forward option, and a conflict pre-check
 * computed server-side without touching the working tree.
 */
function MergeConfirmDialog({
  workspaceId,
  name,
  currentBranch,
  onClose,
  onConfirm,
}: {
  workspaceId: string;
  name: string;
  currentBranch: string;
  onClose: () => void;
  onConfirm: (noFf: boolean) => void;
}): React.JSX.Element {
  const { t } = useTranslation();
  const [noFf, setNoFf] = useState(false);
  const { data: preview, isLoading } = useQuery({
    queryKey: ["merge-preview", workspaceId, name],
    queryFn: () => mergePreview(workspaceId, name),
  });

  const upToDate = preview?.up_to_date ?? false;
  const conflictCount = preview?.conflicts.length ?? 0;

  return (
    <Modal
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("branches.merge.title")}
      description={t("branches.merge.description", { name, current: currentBranch })}
      size="sm"
      footer={
        <>
          <Button variant="secondary" size="sm" className="min-w-0 flex-[3]" onClick={onClose}>
            {t("branches.cancel")}
          </Button>
          <Button
            variant="primary"
            size="sm"
            className="min-w-0 flex-[7]"
            disabled={upToDate}
            onClick={() => onConfirm(noFf)}
          >
            {t("branches.merge.confirm")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
        <div className="flex items-center gap-2">
          <span className="w-16 shrink-0 text-sm text-text-secondary">
            {t("branches.merge.sourceLabel")}
          </span>
          <GitBranch size={13} className="shrink-0 text-text-muted" />
          <span className="min-w-0 truncate text-sm text-text-primary" title={name}>
            {name}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="w-16 shrink-0 text-sm text-text-secondary">
            {t("branches.merge.targetLabel")}
          </span>
          <GitBranch size={13} className="shrink-0 text-text-muted" />
          <span className="min-w-0 truncate text-sm text-text-primary">{currentBranch}</span>
        </div>
        <div className="flex items-center gap-2">
          <Label className="w-16 shrink-0 text-sm text-text-secondary" htmlFor="merge-option">
            {t("branches.merge.optionLabel")}
          </Label>
          <Select
            id="merge-option"
            aria-label={t("branches.merge.optionAria")}
            className="h-auto min-w-0 flex-1 bg-bg-primary border-border-subtle px-1.5 py-1.5 text-sm"
            value={noFf ? "no_ff" : "auto"}
            onChange={(next) => setNoFf(next === "no_ff")}
            options={[
              { value: "auto", label: t("branches.merge.optionAuto") },
              { value: "no_ff", label: t("branches.merge.optionNoFf") },
            ]}
          />
        </div>
      </div>
      <div className="flex items-center gap-1.5 text-xs text-text-muted">
        {isLoading ? (
          <span>{t("branches.merge.checking")}</span>
        ) : upToDate ? (
          <span className="flex items-center gap-1.5">
            <CircleX size={14} className="shrink-0" />
            {t("branches.merge.upToDate")}
          </span>
        ) : conflictCount > 0 ? (
          <span className="flex items-center gap-1.5 text-danger">
            <CircleX size={14} className="shrink-0" />
            {t("branches.merge.mayConflict", { count: conflictCount })}
          </span>
        ) : (
          <span className="flex items-center gap-1.5 text-success">
            <CircleCheck size={14} className="shrink-0" />
            {t("branches.merge.noConflicts")}
          </span>
        )}
      </div>
    </Modal>
  );
}
