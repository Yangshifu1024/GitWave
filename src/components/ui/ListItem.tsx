import { type ReactNode } from "react";
import { cn } from "@/lib/utils";

export interface ListItemProps {
  selected?: boolean;
  onClick?: () => void;
  leading?: ReactNode | null;
  trailing?: ReactNode | null;
  children: ReactNode;
  className?: string;
}

export function ListItem({
  selected = false,
  onClick,
  leading = null,
  trailing = null,
  children,
  className,
}: ListItemProps): React.JSX.Element {
  return (
    <div
      role={onClick ? "button" : undefined}
      tabIndex={onClick ? 0 : undefined}
      onClick={onClick}
      onKeyDown={onClick ? (e) => e.key === "Enter" && onClick() : undefined}
      className={cn(
        "group flex items-center gap-2 px-3 py-2",
        "rounded-md transition-colors duration-150",
        "text-sm text-text-primary",
        selected && "bg-accent/10",
        !selected && "hover:bg-bg-secondary cursor-pointer",
        onClick && "cursor-pointer",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-inset",
        className,
      )}
    >
      {/* Leading slot */}
      {leading ? <span className="shrink-0 text-text-muted">{leading}</span> : null}

      {/* Content */}
      <span className="flex-1 min-w-0">{children}</span>

      {/* Trailing slot */}
      {trailing ? <span className="shrink-0 flex items-center gap-1">{trailing}</span> : null}
    </div>
  );
}
