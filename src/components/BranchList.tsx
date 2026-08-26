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
import { InteractiveRebaseDialog } from "@/components/InteractiveRebaseDialog";
import { ArrowRight, GitBranch, GitMerge, GitPullRequestArrow, ListOrdered, Plus, Trash2 } from "lucide-react";
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
  busy: boolean;
  onCheckout: (name: string) => void;
  onDelete: (name: string) => void;
  onMerge: (name: string) => void;
  onRebaseOnto: (name: string) => void;
  onInteractiveRebase: (name: string) => void;
}

function BranchRow({
  branch,
  busy,
  onCheckout,
  onDelete,
  onMerge,
  onRebaseOnto,
  onInteractiveRebase,
}: BranchRowProps): React.JSX.Element {
  return (
    <ListItem
      selected={branch.is_current}
      onClick={() => !branch.is_current && !busy && onCheckout(branch.name)}
      leading={<BranchIcon kind={branch.kind} />}
      trailing={
        <div className="flex items-center gap-1">
          {branch.ahead > 0 && <StatusBadge variant="ahead" suffix={`\u2191${branch.ahead}`} />}
          {branch.behind > 0 && <StatusBadge variant="behind" suffix={`\u2193${branch.behind}`} />}
          {branch.upstream && (
            <span className="text-xs text-text-muted flex items-center gap-1 mr-1">
              <ArrowRight size={10} />
              {branch.upstream}
            </span>
          )}
          {!branch.is_current && branch.kind === "local" && (
            <>
              <Button
                variant="ghost"
                size="sm"
                className="p-1"
                disabled={busy}
                onClick={(e) => {
                  e.stopPropagation();
                  onMerge(branch.name);
                }}
                title={`Merge ${branch.name} into current`}
              >
                <GitMerge size={12} />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="p-1"
                disabled={busy}
                onClick={(e) => {
                  e.stopPropagation();
                  onRebaseOnto(branch.name);
                }}
                title={`Rebase current onto ${branch.name}`}
              >
                <GitPullRequestArrow size={12} />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="p-1"
                disabled={busy}
                onClick={(e) => {
                  e.stopPropagation();
                  onInteractiveRebase(branch.name);
                }}
                title={`Interactive rebase onto ${branch.name}`}
              >
                <ListOrdered size={12} />
              </Button>
              <Button
                variant="ghost"
                size="sm"
                className="p-1 text-danger hover:text-danger hover:bg-danger/10"
                disabled={busy}
                onClick={(e) => {
                  e.stopPropagation();
                  onDelete(branch.name);
                }}
                title="Delete branch"
              >
                <Trash2 size={12} />
              </Button>
            </>
          )}
          {branch.is_current && <span className="text-xs text-accent font-medium">current</span>}
        </div>
      }
    >
      <div className="flex flex-col">
        <span
          className={cn(
            "text-sm",
            branch.is_current ? "font-medium text-text-primary" : "text-text-secondary",
          )}
        >
          {branch.name}
        </span>
        {branch.kind === "remote" && <span className="text-xs text-text-muted">remote branch</span>}
      </div>
    </ListItem>
  );
}

interface BranchListProps {
  onBranchSelect?: (name: string) => void;
}

export function BranchList({ onBranchSelect }: BranchListProps): React.JSX.Element {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistoryEpoch = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const [branches, setBranches] = useState<BranchInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [newName, setNewName] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [irebaseOnto, setIrebaseOnto] = useState<string | null>(null);
  const [irebasePaused, setIrebasePaused] = useState(false);

  const refresh = useCallback(async () => {
    if (!activeWorkspaceId) return;
    const updated = await getBranches(activeWorkspaceId);
    setBranches(updated);
    bumpHistoryEpoch();
  }, [activeWorkspaceId, bumpHistoryEpoch]);

  useEffect(() => {
    if (!activeWorkspaceId || !activeRepoId) {
      setBranches([]);
      setError(null);
      setIrebasePaused(false);
      return;
    }
    setLoading(true);
    setError(null);
    getBranches(activeWorkspaceId)
      .then(setBranches)
      .catch((e) => setError(formatAppError(e)))
      .finally(() => setLoading(false));
    interactiveRebasePaused(activeWorkspaceId)
      .then(setIrebasePaused)
      .catch(() => setIrebasePaused(false));
  }, [activeWorkspaceId, activeRepoId]);

  const run = async (fn: () => Promise<void>) => {
    if (!activeWorkspaceId || busy) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await fn();
      await refresh();
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
      onBranchSelect?.(name);
      setNotice(`Checked out ${name}`);
    });

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

  if (!activeWorkspaceId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Select a workspace to view branches
      </div>
    );
  }

  if (!activeRepoId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Select a repository to view branches
      </div>
    );
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        Loading branches...
      </div>
    );
  }

  const localBranches = branches.filter((b) => b.kind === "local");
  const remoteBranches = branches.filter((b) => b.kind === "remote");

  return (
    <div className="h-full min-h-0 flex flex-col overflow-hidden">
      <div className="shrink-0 flex items-center gap-2 px-3 py-2 border-b border-border-subtle">
        <Button
          variant="secondary"
          size="sm"
          disabled={busy || !current}
          onClick={() => setShowCreate((v) => !v)}
        >
          <Plus size={14} />
          New branch
        </Button>
        {current ? (
          <span className="text-xs text-text-muted truncate">
            from <span className="text-text-secondary font-medium">{current.name}</span>
          </span>
        ) : null}
        {irebasePaused ? (
          <>
            <Button variant="primary" size="sm" disabled={busy} onClick={handleContinueIrebase}>
              Continue rebase
            </Button>
            <Button variant="ghost" size="sm" disabled={busy} onClick={handleAbortIrebasePause}>
              Discard pause
            </Button>
          </>
        ) : null}
      </div>

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

      {error ? (
        <div className="shrink-0 px-3 py-2 text-xs text-danger border-b border-border-subtle">
          {error}
        </div>
      ) : null}
      {notice ? (
        <div className="shrink-0 px-3 py-2 text-xs text-text-secondary border-b border-border-subtle">
          {notice}
        </div>
      ) : null}

      <div className="flex-1 min-h-0 overflow-auto">
        {branches.length === 0 ? (
          <div className="flex items-center justify-center h-full text-text-muted text-sm">
            No branches found
          </div>
        ) : (
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
                    busy={busy}
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
              <div className="mt-4">
                <div className="px-4 py-2 text-xs font-semibold text-text-muted uppercase tracking-wider">
                  Remote
                </div>
                {remoteBranches.map((branch) => (
                  <BranchRow
                    key={branch.name}
                    branch={branch}
                    busy={busy}
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
        )}
      </div>

      {irebaseOnto && activeWorkspaceId ? (
        <InteractiveRebaseDialog
          open={true}
          workspaceId={activeWorkspaceId}
          upstream={irebaseOnto}
          onClose={() => setIrebaseOnto(null)}
          onDone={(msg) => {
            setNotice(msg);
            void refresh();
            void interactiveRebasePaused(activeWorkspaceId).then(setIrebasePaused);
          }}
        />
      ) : null}
    </div>
  );
}
