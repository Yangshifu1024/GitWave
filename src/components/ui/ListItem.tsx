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
        "group flex items-center gap-2 px-3 select-none",
        "h-7 text-[13px] text-text-primary",
        "border-l-2 rounded-none shadow-none",
        "transition-colors duration-fast",
        selected && "bg-accent/[0.06] border-l-accent",
        !selected && "border-l-transparent hover:bg-black/[0.03] dark:hover:bg-white/[0.03]",
        interactive && "cursor-pointer",
        className,
      )}
    >
      {leading ? <span className="shrink-0 text-text-muted">{leading}</span> : null}
      <span className="flex-1 min-w-0">{children}</span>
      {trailing ? <span className="shrink-0 flex items-center gap-1">{trailing}</span> : null}
    </div>
  );
}
