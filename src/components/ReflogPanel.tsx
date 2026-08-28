import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
import { useToast } from "@/components/ui/Toast";
import { cn } from "@/lib/utils";

const ACTION_LABELS: Record<string, string> = {
  commit: "Commit",
  initial_commit: "Initial commit",
  amend: "Amend",
  checkout: "Checkout",
  reset: "Reset",
  merge: "Merge",
  rebase: "Rebase",
  pull: "Pull",
  push: "Push",
  branch: "Branch",
  revert: "Revert",
  cherry_pick: "Cherry-pick",
  stash: "Stash",
  clone: "Clone",
  other: "Operation",
};

function formatTime(time: number): string {
  return new Date(time * 1000).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * M2 recovery panel: semantic reflog timeline for HEAD (or the current
 * branch) with deterministic recovery actions and an optional AI
 * explanation. Every mutation is behind an explicit confirmation (P1).
 */
export function ReflogPanel(): React.JSX.Element {
  const wc = useWorkingCopy();
  const workspaceId = wc.workspaceId;
  const repoId = wc.repoId;
  const branchName = wc.data?.branch;
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const queryClient = useQueryClient();
  const { toast } = useToast();

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
  const [branchModal, setBranchModal] = useState<ReflogEntry | null>(null);
  const [branchName_, setBranchName_] = useState("");
  const [resetModal, setResetModal] = useState<ReflogEntry | null>(null);
  const [aiText, setAiText] = useState<string | null>(null);
  const [aiBusy, setAiBusy] = useState(false);

  if (!workspaceId || !repoId) return <></>;

  const afterMutation = (): void => {
    bumpHistory();
    void queryClient.invalidateQueries({ queryKey: ["working-copy"] });
    void queryClient.invalidateQueries({ queryKey: ["reflog", workspaceId] });
  };

  const submitRecoveryBranch = (): void => {
    if (!branchModal || !branchName_.trim() || busy) return;
    setBusy(true);
    createBranch(workspaceId, branchName_.trim(), branchModal.new_oid)
      .then(() => {
        toast({ title: `Recovery branch "${branchName_.trim()}" created` });
        setBranchModal(null);
        afterMutation();
      })
      .catch((e) => toast({ title: formatAppError(e), variant: "danger" }))
      .finally(() => setBusy(false));
  };

  const submitReset = (): void => {
    if (!resetModal || busy) return;
    setBusy(true);
    resetHeadHard(workspaceId, resetModal.new_oid)
      .then(() => {
        toast({ title: `Branch reset to ${resetModal.new_oid.slice(0, 7)}` });
        setResetModal(null);
        setSelected(null);
        afterMutation();
      })
      .catch((e) => toast({ title: formatAppError(e), variant: "danger" }))
      .finally(() => setBusy(false));
  };

  const runExplain = (entry: ReflogEntry): void => {
    if (aiBusy) return;
    setAiBusy(true);
    setAiText(null);
    explainReflog(workspaceId, entry)
      .then(setAiText)
      .catch((e) => toast({ title: formatAppError(e), variant: "danger" }))
      .finally(() => setAiBusy(false));
  };

  const suggestion =
    selected?.action === "reset"
      ? "The branch was reset — the discarded commits are still reachable from the previous position. Create a recovery branch to keep them."
      : null;

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
        <p className="px-2 py-1 text-xs text-text-muted italic">Loading…</p>
      ) : entries.length === 0 ? (
        <p className="px-2 py-1 text-xs text-text-muted italic">No reflog entries</p>
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
                {ACTION_LABELS[e.action] ?? "Operation"}
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
              {ACTION_LABELS[selected.action] ?? "Operation"}
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
                setBranchName_(`recovery-${selected.new_oid.slice(0, 7)}`);
                setBranchModal(selected);
              }}
            >
              <Undo2 size={12} />
              Recovery branch
            </Button>
            <Button
              variant="secondary"
              size="sm"
              className="h-6 px-2 text-[11px]"
              disabled={busy}
              onClick={() => setResetModal(selected)}
            >
              <RotateCcw size={12} />
              Reset branch here
            </Button>
            <Button
              variant="secondary"
              size="sm"
              className="h-6 px-2 text-[11px]"
              disabled={aiBusy}
              onClick={() => runExplain(selected)}
            >
              <Sparkles size={12} />
              AI explain
            </Button>
          </div>
          {aiBusy ? (
            <p className="text-[11px] text-text-muted italic">Thinking…</p>
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
          title="Create recovery branch"
          description={`Branch the current state at ${branchModal.new_oid.slice(0, 7)} so the commits stay reachable.`}
          size="sm"
        >
          <Input value={branchName_} onChange={setBranchName_} autoFocus />
          <div className="mt-3 flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setBranchModal(null)}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              disabled={busy || !branchName_.trim()}
              onClick={submitRecoveryBranch}
            >
              Create branch
            </Button>
          </div>
        </Modal>
      ) : null}

      {resetModal ? (
        <Modal
          open
          onOpenChange={(o) => !o && setResetModal(null)}
          title={`Reset "${branchName || "current branch"}" to ${resetModal.new_oid.slice(0, 7)}?`}
          description="Hard reset: the branch moves and uncommitted changes are discarded. This cannot be undone automatically — the reflog keeps a record."
          size="sm"
        >
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setResetModal(null)}>
              Cancel
            </Button>
            <Button variant="danger" size="sm" disabled={busy} onClick={submitReset}>
              Reset — discard changes
            </Button>
          </div>
        </Modal>
      ) : null}
    </div>
  );
}
