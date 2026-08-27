import { useCallback, useEffect, useState } from "react";
import type { BranchInfo } from "@/lib/api";
import {
  abortInteractiveRebasePause,
  continueInteractiveRebase,
  checkoutBranch,
  createBranch,
  deleteBranch,
  formatAppError,
  getBranches,
  interactiveRebasePaused,
  mergeBranch,
  rebaseBranch,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
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
import { ArrowRight, GitBranch, GitMerge, GitPullRequestArrow, ListOrdered, Plus, Trash2 } from "lucide-react";
import { SyncButtons } from "@/components/ui/SyncButtons";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";

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
        if (branch.is_current) {
          onSelect(branch.name);
          return;
        }
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

interface BranchListProps {
  onBranchSelect?: (name: string) => void;
}

export function BranchList({ onBranchSelect }: BranchListProps): React.JSX.Element {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistoryEpoch = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const historyEpoch = useWorkspaceUiStore((s) => s.historyEpoch);
  const wc = useWorkingCopy();
  const [branches, setBranches] = useState<BranchInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [newName, setNewName] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [irebaseOnto, setIrebaseOnto] = useState<string | null>(null);
  const [irebasePaused, setIrebasePaused] = useState(false);
  const [selectedName, setSelectedName] = useState<string | null>(null);

  const refresh = useCallback(() => {
    bumpHistoryEpoch();
  }, [bumpHistoryEpoch]);

  useEffect(() => {
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
          if (prev && updated.some((b) => b.name === prev)) return prev;
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

  const current = branches.find((b) => b.is_current && b.kind === "local");

  const handleCreate = () =>
    void run(async () => {
      const name = newName.trim();
      if (!name) throw new Error("Branch name is required");
      if (!activeWorkspaceId) return;
      const fromSha = current?.last_commit_sha;
      if (!fromSha) throw new Error("No current branch tip to branch from");
      await createBranch(activeWorkspaceId, name, fromSha);
      setNewName("");
      setShowCreate(false);
      setNotice(`Created branch ${name}`);
    });

  const handleCheckout = (name: string) =>
    void run(async () => {
      await checkoutBranch(activeWorkspaceId!, name);
      setSelectedName(name);
      onBranchSelect?.(name);
      setNotice(`Checked out ${name}`);
    });

  const handleSelect = (name: string) => {
    setSelectedName(name);
    onBranchSelect?.(name);
  };

  const handleDelete = (name: string) =>
    void run(async () => {
      await deleteBranch(activeWorkspaceId!, name);
      setNotice(`Deleted ${name}`);
    });

  const handleMerge = (name: string) =>
    void run(async () => {
      const result = await mergeBranch(activeWorkspaceId!, name);
      if (result.conflicts.length > 0) {
        setNotice(
          `Merged ${name} with ${result.conflicts.length} conflict(s): ${result.conflicts.join(", ")}`,
        );
      } else {
        setNotice(`Merged ${name} (${result.kind.replace(/_/g, " ")})`);
      }
    });

  const handleRebaseOnto = (name: string) =>
    void run(async () => {
      const result = await rebaseBranch(activeWorkspaceId!, name);
      if (result.kind === "conflicts" || result.conflicts.length > 0) {
        setNotice(
          `Rebase onto ${name} hit conflicts${
            result.conflicts.length ? `: ${result.conflicts.join(", ")}` : ""
          }`,
        );
      } else {
        setNotice(`Rebased onto ${name} (${result.kind.replace(/_/g, " ")})`);
      }
    });

  const handleContinueIrebase = () =>
    void run(async () => {
      const result = await continueInteractiveRebase(activeWorkspaceId!);
      if (result.kind === "conflicts") {
        setNotice(`Continue rebase conflicts: ${result.conflicts.join(", ")}`);
      } else if (result.kind === "paused_for_edit") {
        setNotice("Still paused for edit — amend then Continue again.");
      } else {
        setNotice(`Continued interactive rebase (${result.kind.replace(/_/g, " ")})`);
      }
    });

  const handleAbortIrebasePause = () =>
    void run(async () => {
      await abortInteractiveRebasePause(activeWorkspaceId!);
      setNotice("Cleared interactive rebase pause state");
    });

  const selectedBranch = branches.find((b) => b.name === selectedName) ?? null;
  const canSync = Boolean(selectedName && activeRepoId);
  const bannerError = error ?? wc.actionError;

  const localBranches = branches.filter((b) => b.kind === "local");
  const remoteBranches = branches.filter((b) => b.kind === "remote");

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
                onDelete={handleDelete}
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
                onDelete={handleDelete}
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
      <SidebarSection
        title="Branches"
        className="border-b-0"
        actions={
          <>
            <Button
              variant="ghost"
              size="sm"
              disabled={busy || !current}
              onClick={() => setShowCreate((v) => !v)}
              aria-label="New branch"
              title="New branch"
            >
              <Plus size={14} />
            </Button>
            <SyncButtons
              ahead={selectedBranch?.ahead ?? wc.data?.ahead ?? 0}
              behind={selectedBranch?.behind ?? wc.data?.behind ?? 0}
              onPull={wc.pull}
              onPush={wc.push}
              pullDisabled={!canSync}
              pushDisabled={!canSync}
              inProgress={wc.syncPending}
            />
          </>
        }
      >
        {showCreate ? (
          <div className="shrink-0 flex items-center gap-2 px-3 py-2 border-b border-border-subtle bg-bg-elevated">
            <div className="flex-1 min-w-0">
              <Input
                placeholder="feature/my-branch"
                value={newName}
                onChange={setNewName}
                disabled={busy}
                onKeyDown={(e) => {
                  if (e.key === "Enter") handleCreate();
                }}
              />
            </div>
            <Button variant="primary" size="sm" disabled={busy || !newName.trim()} onClick={handleCreate}>
              Create
            </Button>
          </div>
        ) : null}

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
          <div className="shrink-0 px-3 py-2 text-xs text-text-secondary border-b border-border-subtle">
            {notice}
          </div>
        ) : null}

        <div>{renderBody()}</div>
      </SidebarSection>

      <ErrorAlert
        message={bannerError}
        onDismiss={() => {
          setError(null);
          wc.setActionError(null);
        }}
      />

      {irebaseOnto && activeWorkspaceId ? (
        <InteractiveRebaseDialog
          open={true}
          workspaceId={activeWorkspaceId}
          upstream={irebaseOnto}
          onClose={() => setIrebaseOnto(null)}
          onDone={(msg) => {
            setNotice(msg);
            refresh();
            void interactiveRebasePaused(activeWorkspaceId).then(setIrebasePaused);
          }}
        />
      ) : null}
    </>
  );
}
