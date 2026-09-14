import { Disclosure } from "@heroui/react";
import { ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

export interface SidebarSectionProps {
  title: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
  defaultOpen?: boolean;
  /** false = static header: no toggle, never expands (empty datasets). */
  collapsible?: boolean;
  /** Right-click handler for the header row (group-level actions). */
  onHeaderContextMenu?: (event: React.MouseEvent) => void;
  className?: string;
}

/**
 * Flat sidebar section: no card, no border, no rounded corners.
 * A macOS Source List-style header plus a full-width content area.
 */
export function SidebarSection({
  title,
  actions,
  children,
  defaultOpen = true,
  collapsible = true,
  onHeaderContextMenu,
  className,
}: SidebarSectionProps): React.JSX.Element {
  if (!collapsible) {
    return (
      <div className={cn("min-h-0 shrink-0 flex flex-col", className)}>
        <div
          className="flex items-center px-3 py-1 select-none"
          onContextMenu={onHeaderContextMenu}
        >
          <span className="truncate text-[10px] font-semibold text-text-muted uppercase tracking-wider">
            {title}
          </span>
          {actions ? (
            <div className="ml-auto flex shrink-0 items-center gap-1 pl-2">{actions}</div>
          ) : null}
        </div>
      </div>
    );
  }

  return (
    <div className={cn("min-h-0 shrink flex flex-col overflow-hidden", className)}>
      <Disclosure defaultExpanded={defaultOpen} className="flex min-h-0 flex-col">
        <div className="shrink-0 flex items-center gap-1" onContextMenu={onHeaderContextMenu}>
          <Disclosure.Heading className="m-0 flex-1 min-w-0 text-inherit font-inherit leading-inherit">
            <Disclosure.Trigger
              className={cn(
                "flex-1 min-w-0 w-full flex items-center gap-1 px-3 py-1 text-left",
                "text-[10px] font-semibold text-text-muted uppercase tracking-wider",
                "hover:text-text-secondary",
                "bg-transparent border-0 shadow-none rounded-none",
                "select-none",
              )}
            >
              <Disclosure.Indicator className="text-text-muted data-[expanded=false]:rotate-[-90deg] transition-transform duration-fast">
                <ChevronDown size={10} />
              </Disclosure.Indicator>
              <span className="truncate">{title}</span>
            </Disclosure.Trigger>
          </Disclosure.Heading>
          {actions ? <div className="shrink-0 flex items-center gap-1 pr-2">{actions}</div> : null}
        </div>
        <Disclosure.Content className="sidebar-card-scroll min-h-0 overflow-y-auto">
          {children}
        </Disclosure.Content>
      </Disclosure>
    </div>
  );
}
