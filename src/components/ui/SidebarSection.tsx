import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { cn } from "@/lib/utils";

export interface SidebarSectionProps {
  title: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
  defaultOpen?: boolean;
  className?: string;
}

/**
 * Collapsible sidebar block. Expanded height follows content; collapsed
 * height is the header only. The parent sidebar scrolls if the stack overflows.
 */
export function SidebarSection({
  title,
  actions,
  children,
  defaultOpen = true,
  className,
}: SidebarSectionProps): React.JSX.Element {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <section className={cn("flex flex-col shrink-0", className)}>
      <div className="shrink-0 flex items-center gap-1">
        <button
          type="button"
          aria-expanded={open}
          aria-label={open ? `Collapse ${title}` : `Expand ${title}`}
          onClick={() => setOpen((value) => !value)}
          className={cn(
            "flex-1 min-w-0 flex items-center gap-1.5 px-3 py-1.5",
            "text-xs font-semibold text-text-muted uppercase tracking-wide",
            "hover:bg-bg-primary/60 hover:text-text-secondary",
            "focus-visible:outline-2 focus-visible:outline-accent focus-visible:outline-offset-[-2px]",
          )}
        >
          {open ? (
            <ChevronDown size={14} className="shrink-0" />
          ) : (
            <ChevronRight size={14} className="shrink-0" />
          )}
          {title}
        </button>
        {actions ? <div className="shrink-0 flex items-center gap-1 pr-2">{actions}</div> : null}
      </div>
      {open ? <div className="flex flex-col">{children}</div> : null}
    </section>
  );
}
