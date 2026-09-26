import { type FileStatusKind } from "@/components/ui/StatusIcon";
import { StatusIcon } from "@/components/ui/StatusIcon";
import { Button } from "@/components/ui/Button";
import { Surface } from "@heroui/react";
import { useTranslation } from "react-i18next";
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
  active?: boolean;
  className?: string;
  tabIndex?: number;
  onFocus?: () => void;
  onKeyDown?: (event: React.KeyboardEvent<HTMLDivElement>) => void;
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
  active = false,
  className,
  tabIndex = 0,
  onFocus,
  onKeyDown,
}: FileListItemProps): React.JSX.Element {
  const { t } = useTranslation();
  const { path, kind, staged, additions, deletions } = change;

  return (
    <Surface
      variant="transparent"
      role="option"
      aria-selected={selected}
      aria-current={active ? "true" : undefined}
      tabIndex={tabIndex}
      onFocus={onFocus}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget) return;
        onKeyDown?.(e);
        if (!e.defaultPrevented && (e.key === "Enter" || e.key === " ")) {
          e.preventDefault();
          onClick?.(e);
        }
      }}
      className={cn(
        "flex items-center gap-2 px-3 py-1.5",
        "rounded-md transition-colors duration-fast",
        // UI (sans) font, one step below body size — the path is a label,
        // not code; the +/- stats below stay mono for column alignment.
        "text-xs",
        "cursor-pointer shadow-none",
        selected && "bg-accent/10",
        active && "ring-1 ring-inset ring-accent/30",
        !selected && "hover:bg-bg-secondary",
        className,
      )}
    >
      {/* Status icon — clicking toggles stage */}
      <Button
        type="button"
        tabIndex={tabIndex}
        variant="ghost"
        size="sm"
        onClick={(e) => {
          e.stopPropagation();
          onStageToggle?.();
        }}
        className="shrink-0 h-auto p-0 border-0 bg-transparent rounded"
        title={t(staged ? "changes.action.unstage" : "changes.action.stage")}
        aria-label={t(
          staged ? "changes.fileSection.unstageFile" : "changes.fileSection.stageFile",
          { path },
        )}
      >
        <StatusIcon kind={kind} staged={staged} />
      </Button>

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
    </Surface>
  );
}
