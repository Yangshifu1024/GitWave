import { cn } from "@/lib/utils";

export type FileStatusKind = "modified" | "added" | "deleted" | "untracked" | "renamed" | "copied";

export interface StatusIconProps {
  kind: FileStatusKind;
  staged?: boolean;
  className?: string;
}

const statusConfig: Record<
  FileStatusKind,
  { char: string; color: string }
> = {
  modified: { char: "M", color: "text-branch-behind" },
  added: { char: "A", color: "text-status-active" },
  deleted: { char: "D", color: "text-danger" },
  untracked: { char: "?", color: "text-text-muted" },
  renamed: { char: "R", color: "text-branch-local" },
  copied: { char: "C", color: "text-branch-current" },
};

/**
 * Single-character file status indicator with semantic color.
 * Unstaged = blue-ish tint; staged = green-ish tint.
 */
export function StatusIcon({
  kind,
  staged = false,
  className,
}: StatusIconProps): React.JSX.Element {
  const { char, color } = statusConfig[kind];

  return (
    <span
      className={cn(
        "inline-flex items-center justify-center",
        "w-5 h-5 rounded-sm font-mono text-xs font-bold",
        color,
        !staged && "opacity-80",
        staged && "opacity-100",
        className,
      )}
      title={`${kind}${staged ? " (staged)" : " (unstaged)"}`}
      aria-label={`${kind}${staged ? " staged" : " unstaged"}`}
    >
      {char}
    </span>
  );
}
