// Sidebar branch list — navigation plus row-scoped operations (checkout via
// double-click, merge / rebase / delete via context menu). Toolbar-scoped
// branch ops (new branch / pull / push) live in the ActionBar.

import { useCallback, useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
  mergePreview,
  popStash,
  rebaseBranch,
  saveStash,
  createBranch,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useSyncStore, type UiOperation } from "@/stores/syncStore";
import { cn } from "@/lib/utils";
import { gateCheckout } from "@/lib/checkoutGate";
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
  ContextMenuTrigger,
} from "@/components/ui/ContextMenu";
import { InteractiveRebaseDialog } from "@/components/InteractiveRebaseDialog";
import {
  ArrowRight,
  ChevronDown,
  ChevronRight,
  CircleCheck,
  CircleX,
  Folder,
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
  /** Text shown in the row (prefix stripped when nested in a folder). */
  displayName?: string;
  /** Folder children sit one indent level deeper than top-level rows. */
  indented?: boolean;
  selected: boolean;
  busy: boolean;
  onSelect: (name: string) => void;
  onCheckout: (name: string) => void;
  onDelete: (name: string) => void;
  onMerge: (name: string) => void;
  onRebaseOnto: (name: string) => void;
  onInteractiveRebase: (name: string) => void;
  onNewBranch: (name: string, sha: string | null) => void;
}

function BranchRow({
  branch,
  displayName,
  indented,
  selected,
  busy,
  onSelect,
  onCheckout,
  onDelete,
  onMerge,
  onRebaseOnto,
  onInteractiveRebase,
  onNewBranch,
}: BranchRowProps): React.JSX.Element {
  // Every local branch gets a context menu — the current branch's menu is
  // New-only (merge / rebase / delete don't apply to HEAD).
  const hasActions = branch.kind === "local";

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
          {branch.is_current && <span className="text-xs text-accent font-medium">current</span>}
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
        <ContextMenuItem
          disabled={busy || !branch.last_commit_sha}
          onSelect={() => onNewBranch(branch.name, branch.last_commit_sha)}
        >
          <GitBranch size={14} />
          New
        </ContextMenuItem>
        {!branch.is_current ? (
          <>
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
          </>
        ) : null}
      </ContextMenuContent>
    </ContextMenu>
  );
}

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
  const [switchDialog, setSwitchDialog] = useState<
    | { kind: "dirty"; name: string; fileCount: number }
    | { kind: "blocked"; name: string; message: string }
    | null
  >(null);

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

  const checkoutOnto = async (name: string, mode: "safe" | "force" | "stash") => {
    if (!activeWorkspaceId) return;
    if (mode === "stash") {
      await saveStash(activeWorkspaceId, `switch to ${name}`);
      await checkoutBranch(activeWorkspaceId, name, false);
      try {
        await popStash(activeWorkspaceId, 0);
        setStatus(`Checked out ${name} and re-applied stash`);
      } catch {
        setStatus(`Checked out ${name}. Stash re-apply failed; the stash was kept.`, "danger");
      }
    } else {
      await checkoutBranch(activeWorkspaceId, name, mode === "force");
      setStatus(`Checked out ${name}`);
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
        await run("checkout", () => checkoutOnto(name, "safe"));
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
    void run("delete", async () => {
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
      showNotice(`Created branch ${name} from ${newBranchBase.name}`);
    } catch (e) {
      setNewBranchError(formatAppError(e));
    } finally {
      setBusy(false);
    }
  };

  const handleMergeConfirmed = (name: string, noFf: boolean) =>
    void run("merge", async () => {
      const result = await mergeBranch(activeWorkspaceId!, name, noFf);
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
    void run("rebase", async () => {
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
    void run("rebase", async () => {
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
    void run("rebase", async () => {
      await abortInteractiveRebasePause(activeWorkspaceId!);
      showNotice("Cleared interactive rebase pause state");
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
          busy={busy || loading}
          onSelect={handleSelect}
          onCheckout={handleCheckout}
          onDelete={(name) => setDeleteDialog({ name, deleteRemote: false })}
          onMerge={handleMerge}
          onRebaseOnto={handleRebaseOnto}
          onInteractiveRebase={(name) => setIrebaseOnto(name)}
          onNewBranch={openNewBranch}
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
      // Cold start only: nothing cached to keep on screen yet, so render an
      // empty body instead of a spinner; the list pops in when data lands.
      return <div className="h-8" />;
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
        {renderGroup("Local", "local", localBranches)}
        {remoteGroups.map(([remote, list]) => renderGroup(remote, `remote:${remote}`, list))}
      </>
    );
  };

  return (
    <>
      <SidebarSection title="Branches">
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
          footer={
            <>
              <Button
                variant="ghost"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setSwitchDialog(null)}
              >
                Cancel
              </Button>
              <Button
                variant="danger"
                size="sm"
                className="min-w-0 flex-[3]"
                disabled={busy}
                onClick={() => {
                  const target = switchDialog.name;
                  setSwitchDialog(null);
                  void run("checkout", () => checkoutOnto(target, "force"));
                }}
              >
                Discard
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[4]"
                disabled={busy}
                onClick={() => {
                  const target = switchDialog.name;
                  setSwitchDialog(null);
                  void run("checkout", () => checkoutOnto(target, "stash"));
                }}
              >
                Stash & switch
              </Button>
            </>
          }
        />
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
          footer={
            <Button
              variant="primary"
              size="sm"
              className="w-full"
              onClick={() => setSwitchDialog(null)}
            >
              OK
            </Button>
          }
        />
      ) : null}

      {newBranchBase ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setNewBranchBase(null);
          }}
          title={`New branch from "${newBranchBase.name}"`}
          description="Creates a local branch at this tip; the current branch stays checked out."
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setNewBranchBase(null)}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy || !newBranchName.trim()}
                onClick={() => void submitNewBranch()}
              >
                Create
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
              placeholder="feature/my-branch"
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
          title="Delete Branch"
          description="Delete local branch from your repository"
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setDeleteDialog(null)}
              >
                Cancel
              </Button>
              <Button
                variant="danger"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy}
                onClick={handleConfirmDelete}
              >
                Delete
              </Button>
            </>
          }
        >
          <div className="flex items-center gap-2 rounded-xl bg-bg-primary p-3">
            <span className="w-16 shrink-0 text-sm text-text-secondary">Branch</span>
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
              Also delete corresponding remote branch
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
      title="Merge branch"
      description={`Merge ${name} into ${currentBranch}.`}
      size="sm"
      footer={
        <>
          <Button variant="secondary" size="sm" className="min-w-0 flex-[3]" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            className="min-w-0 flex-[7]"
            disabled={upToDate}
            onClick={() => onConfirm(noFf)}
          >
            Merge
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
        <div className="flex items-center gap-2">
          <span className="w-16 shrink-0 text-sm text-text-secondary">Merge</span>
          <GitBranch size={13} className="shrink-0 text-text-muted" />
          <span className="min-w-0 truncate text-sm text-text-primary" title={name}>
            {name}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="w-16 shrink-0 text-sm text-text-secondary">Into</span>
          <GitBranch size={13} className="shrink-0 text-text-muted" />
          <span className="min-w-0 truncate text-sm text-text-primary">{currentBranch}</span>
        </div>
        <div className="flex items-center gap-2">
          <Label className="w-16 shrink-0 text-sm text-text-secondary" htmlFor="merge-option">
            Option
          </Label>
          <Select
            id="merge-option"
            aria-label="Merge option"
            className="h-auto min-w-0 flex-1 bg-bg-primary border-border-subtle px-1.5 py-1.5 text-sm"
            value={noFf ? "no_ff" : "auto"}
            onChange={(next) => setNoFf(next === "no_ff")}
            options={[
              { value: "auto", label: "Auto — fast-forward when possible" },
              { value: "no_ff", label: "No fast-forward — always create a merge commit" },
            ]}
          />
        </div>
      </div>
      <div className="flex items-center gap-1.5 text-xs text-text-muted">
        {isLoading ? (
          <span>Checking merge…</span>
        ) : upToDate ? (
          <span className="flex items-center gap-1.5">
            <CircleX size={14} className="shrink-0" />
            Already up to date
          </span>
        ) : conflictCount > 0 ? (
          <span className="flex items-center gap-1.5 text-danger">
            <CircleX size={14} className="shrink-0" />
            May conflict in {conflictCount} file{conflictCount === 1 ? "" : "s"} (resolve after
            merge)
          </span>
        ) : (
          <span className="flex items-center gap-1.5 text-success">
            <CircleCheck size={14} className="shrink-0" />
            Merge can be done without conflicts
          </span>
        )}
      </div>
    </Modal>
  );
}
