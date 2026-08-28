import { type ReactNode } from "react";
import { cn } from "@/lib/utils";

export interface ListItemProps {
  selected?: boolean;
  onClick?: () => void;
  onDoubleClick?: () => void;
  leading?: ReactNode | null;
  trailing?: ReactNode | null;
  children: ReactNode;
  className?: string;
}

export function ListItem({
  selected = false,
  onClick,
  onDoubleClick,
  leading = null,
  trailing = null,
  children,
  className,
}: ListItemProps): React.JSX.Element {
  const interactive = Boolean(onClick || onDoubleClick);
  return (
    <div
      role={interactive ? "button" : undefined}
      tabIndex={interactive ? 0 : undefined}
      onClick={onClick}
      onDoubleClick={
        onDoubleClick
          ? (event) => {
              event.preventDefault();
              onDoubleClick();
            }
          : undefined
      }
      onKeyDown={
        onClick
          ? (e) => {
              if (e.key === "Enter") onClick();
            }
          : undefined
      }
      className={cn(
        "group flex items-center gap-2 px-3 py-1.5 select-none",
        "transition-colors duration-fast",
        "text-sm text-text-primary",
        "border-l-[3px]",
        selected && "bg-accent/10 border-l-accent hover:bg-accent/15",
        !selected && "border-l-transparent hover:bg-bg-primary/70 cursor-pointer",
        onClick && "cursor-pointer",
        onDoubleClick && "cursor-pointer",
        "focus-visible:outline-2 focus-visible:outline-accent focus-visible:outline-offset-[-2px]",
        className,
      )}
    >
      {leading ? <span className="shrink-0 text-text-muted">{leading}</span> : null}
      <span className="flex-1 min-w-0">{children}</span>
      {trailing ? <span className="shrink-0 flex items-center gap-1">{trailing}</span> : null}
    </div>
  );
}
