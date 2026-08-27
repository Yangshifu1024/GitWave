import { type FileStatusKind } from "@/components/ui/StatusIcon";
import { StatusIcon } from "@/components/ui/StatusIcon";
import { cn } from "@/lib/utils";

export interface FileChange {
  path: string;
  kind: FileStatusKind;
  staged: boolean;
  additions: number;
  deletions: number;
  old_path?: string | null;
}

export interface FileListItemProps {
  change: FileChange;
  onClick?: (event: React.MouseEvent<HTMLDivElement> | React.KeyboardEvent<HTMLDivElement>) => void;
  onStageToggle?: () => void;
  selected?: boolean;
  className?: string;
}

/**
 * Single file change row within the Working Copy file list.
 * Clicking the row opens diff; clicking the StatusIcon toggles stage.
 */
export function FileListItem({
  change,
  onClick,
  onStageToggle,
  selected = false,
  className,
}: FileListItemProps): React.JSX.Element {
  const { path, kind, staged, additions, deletions } = change;

  return (
    <div
      role="option"
      aria-selected={selected}
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === "Enter") onClick?.(e);
      }}
      className={cn(
        "flex items-center gap-2 px-3 py-1.5",
        "rounded-md transition-colors duration-150",
        "text-sm font-mono",
        "cursor-pointer",
        selected && "bg-accent/10",
        !selected && "hover:bg-bg-secondary",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-inset",
        className,
      )}
    >
      {/* Status icon — clicking toggles stage */}
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          onStageToggle?.();
        }}
        className="shrink-0 p-0 border-0 bg-transparent cursor-pointer focus:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-1 rounded"
        title={staged ? "Unstage" : "Stage"}
        aria-label={staged ? `Unstage ${path}` : `Stage ${path}`}
      >
        <StatusIcon kind={kind} staged={staged} />
      </button>

      {/* File path */}
      <span className="flex-1 min-w-0 truncate text-text-primary" title={path}>
        {path}
      </span>

      {/* +/- stats */}
      {additions > 0 || deletions > 0 ? (
        <span className="shrink-0 flex items-center gap-1.5 font-mono text-xs">
          {additions > 0 && <span className="text-status-active">+{additions}</span>}
          {deletions > 0 && <span className="text-danger">-{deletions}</span>}
        </span>
      ) : null}
    </div>
  );
}
