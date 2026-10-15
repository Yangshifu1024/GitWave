import { GitBranch, GitCommit } from "lucide-react";
import { Chip } from "@heroui/react";
import { cn } from "@/lib/utils";

export interface BranchIndicatorProps {
  branch: string;
  ahead?: number;
  behind?: number;
  upstream?: string | null;
  sha?: string | null;
  className?: string;
}

/**
 * Displays the current branch name with optional ahead/behind chips.
 * When sha is provided (detached HEAD state), shows detached mode instead.
 */
export function BranchIndicator({
  branch,
  ahead = 0,
  behind = 0,
  upstream = null,
  sha = null,
  className,
}: BranchIndicatorProps): React.JSX.Element {
  const isDetached = branch === "(detached)";

  return (
    <div className={cn("flex min-w-0 items-center gap-2 text-sm", className)}>
      {isDetached ? (
        <>
          <GitCommit size={14} className="text-text-muted shrink-0" />
          <span className="font-mono text-text-muted">
            detached @ {sha?.slice(0, 7) ?? "??????"}
          </span>
        </>
      ) : (
        <>
          <GitBranch size={14} className="text-branch-current shrink-0" />
          <span className="min-w-0 font-medium text-text-primary truncate" title={branch}>
            {branch}
          </span>
          {upstream && (
            <span className="min-w-0 text-text-muted text-xs truncate" title={upstream}>
              → {upstream}
            </span>
          )}
        </>
      )}

      {ahead > 0 && !isDetached && (
        <Chip
          size="sm"
          className={cn(
            "inline-flex shrink-0 items-center gap-0.5",
            "rounded-sm px-1.5 py-0.5",
            "text-xs font-medium",
            "bg-branch-ahead/15 text-branch-ahead",
            "border-0 shadow-none",
          )}
          title={`${ahead} commit${ahead === 1 ? "" : "s"} ahead of ${upstream ?? "upstream"}`}
        >
          ↑{ahead}
        </Chip>
      )}

      {behind > 0 && !isDetached && (
        <Chip
          size="sm"
          className={cn(
            "inline-flex shrink-0 items-center gap-0.5",
            "rounded-sm px-1.5 py-0.5",
            "text-xs font-medium",
            "bg-branch-behind/15 text-branch-behind",
            "border-0 shadow-none",
          )}
          title={`${behind} commit${behind === 1 ? "" : "s"} behind ${upstream ?? "upstream"}`}
        >
          ↓{behind}
        </Chip>
      )}
    </div>
  );
}
