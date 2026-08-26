import { type ReactNode } from "react";
import { cn } from "@/lib/utils";

export interface EmptyStateProps {
  icon?: ReactNode | null;
  title: string;
  description?: string | null;
  action?: ReactNode | null;
  className?: string;
}

/**
 * Renders an empty-state placeholder with icon, title, description, and optional action.
 */
export function EmptyState({
  icon,
  title,
  description = null,
  action = null,
  className,
}: EmptyStateProps): React.JSX.Element {
  return (
    <div
      className={cn(
        "flex flex-col items-center justify-center gap-3 p-8",
        "text-center",
        className,
      )}
    >
      {icon ? <div className="text-text-muted opacity-60">{icon}</div> : null}
      <div className="flex flex-col gap-1">
        <p className="text-sm font-medium text-text-primary">{title}</p>
        {description ? <p className="text-xs text-text-secondary max-w-xs">{description}</p> : null}
      </div>
      {action ? <div className="mt-1">{action}</div> : null}
    </div>
  );
}
