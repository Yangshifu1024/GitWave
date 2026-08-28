import { ChevronDown, ChevronRight } from "lucide-react";
import { Disclosure } from "@heroui/react";
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
  return (
    <Disclosure
      defaultExpanded={defaultOpen}
      className={cn("flex flex-col shrink-0 border-b border-border-subtle", className)}
    >
      {({ isExpanded }) => (
        <>
          <div className="shrink-0 flex items-center gap-1">
            <Disclosure.Heading className="m-0 flex-1 min-w-0 text-inherit font-inherit leading-inherit">
              <Disclosure.Trigger
                aria-label={isExpanded ? `Collapse ${title}` : `Expand ${title}`}
                className={cn(
                  "flex-1 min-w-0 w-full flex items-center gap-1.5 px-3 py-1.5",
                  "text-[11px] font-semibold text-text-muted uppercase tracking-wider",
                  "hover:bg-bg-primary/60 hover:text-text-secondary",
                  "bg-transparent border-0 shadow-none rounded-none",
                )}
              >
                {isExpanded ? (
                  <ChevronDown size={12} className="shrink-0" />
                ) : (
                  <ChevronRight size={12} className="shrink-0" />
                )}
                {title}
              </Disclosure.Trigger>
            </Disclosure.Heading>
            {actions ? (
              <div className="shrink-0 flex items-center gap-1 pr-2">{actions}</div>
            ) : null}
          </div>
          {isExpanded ? <div className="flex flex-col">{children}</div> : null}
        </>
      )}
    </Disclosure>
  );
}
