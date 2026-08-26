import { useEffect, useState } from "react";
import type { BranchInfo } from "@/lib/api";
import { formatAppError, getBranches, checkoutBranch, deleteBranch } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import { StatusBadge } from "@/components/ui/StatusBadge";
import { ListItem } from "@/components/ui/ListItem";
import { GitBranch, Trash2, ArrowRight } from "lucide-react";

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
  onCheckout: (name: string) => void;
  onDelete: (name: string) => void;
}

function BranchRow({ branch, onCheckout, onDelete }: BranchRowProps): React.JSX.Element {
  return (
    <ListItem
      selected={branch.is_current}
      onClick={() => !branch.is_current && onCheckout(branch.name)}
      leading={<BranchIcon kind={branch.kind} />}
      trailing={
        <div className="flex items-center gap-2">
          {branch.ahead > 0 && <StatusBadge variant="ahead" suffix={`\u2191${branch.ahead}`} />}
          {branch.behind > 0 && <StatusBadge variant="behind" suffix={`\u2193${branch.behind}`} />}
          {branch.upstream && (
            <span className="text-xs text-text-muted flex items-center gap-1">
              <ArrowRight size={10} />
              {branch.upstream}
            </span>
          )}
          {!branch.is_current && branch.kind === "local" && (
            <Button
              variant="ghost"
              size="sm"
              className="p-1 text-danger hover:text-danger hover:bg-danger/10"
              onClick={(e) => {
                e.stopPropagation();
                onDelete(branch.name);
              }}
              title="Delete branch"
            >
              <Trash2 size={12} />
            </Button>
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
  const [branches, setBranches] = useState<BranchInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!activeWorkspaceId || !activeRepoId) {
      setBranches([]);
      setError(null);
      return;
    }
    setLoading(true);
    setError(null);
    getBranches(activeWorkspaceId)
      .then(setBranches)
      .catch((e) => setError(formatAppError(e)))
      .finally(() => setLoading(false));
  }, [activeWorkspaceId, activeRepoId]);

  const handleCheckout = async (name: string) => {
    if (!activeWorkspaceId) return;
    try {
      await checkoutBranch(activeWorkspaceId, name);
      // Refresh branches
      const updated = await getBranches(activeWorkspaceId);
      setBranches(updated);
      onBranchSelect?.(name);
    } catch {
      // silently fail — UI will show stale state
    }
  };

  const handleDelete = async (name: string) => {
    if (!activeWorkspaceId) return;
    try {
      await deleteBranch(activeWorkspaceId, name);
      // Refresh branches
      const updated = await getBranches(activeWorkspaceId);
      setBranches(updated);
    } catch {
      // silently fail
    }
  };

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

  if (error) {
    return (
      <div className="flex items-center justify-center h-full text-danger text-sm px-4">
        {error}
      </div>
    );
  }

  const localBranches = branches.filter((b) => b.kind === "local");
  const remoteBranches = branches.filter((b) => b.kind === "remote");

  if (branches.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        No branches found
      </div>
    );
  }

  return (
    <div className="h-full overflow-auto">
      {localBranches.length > 0 && (
        <div>
          <div className="px-4 py-2 text-xs font-semibold text-text-muted uppercase tracking-wider">
            Local
          </div>
          {localBranches.map((branch) => (
            <BranchRow
              key={branch.name}
              branch={branch}
              onCheckout={() => {
                void handleCheckout(branch.name);
              }}
              onDelete={() => {
                void handleDelete(branch.name);
              }}
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
              onCheckout={() => {
                void handleCheckout(branch.name);
              }}
              onDelete={() => {
                void handleDelete(branch.name);
              }}
            />
          ))}
        </div>
      )}
    </div>
  );
}
