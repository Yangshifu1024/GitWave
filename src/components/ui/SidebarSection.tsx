import { Card as HeroCard, Disclosure } from "@heroui/react";
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
 * Collapsible sidebar card: a HeroUI Card wrapping a HeroUI Disclosure.
 * The sidebar is a fixed-height flex column, so each card's height is capped
 * at the space flex assigns it — long content scrolls inside the card
 * (header stays pinned) instead of pushing sibling cards out of view.
 * `bg-bg-elevated` is explicit because the theme bridge does not alias
 * HeroUI's `--surface`, so the Card default would not follow the theme.
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
  const cardClass = cn(
    "min-h-0 shrink overflow-hidden rounded-lg p-0 gap-0",
    "border border-border-subtle bg-bg-elevated shadow-none",
    className,
  );

  if (!collapsible) {
    // Static header card: an empty dataset has nothing to expand, so no
    // Disclosure is mounted at all.
    return (
      <HeroCard className={cardClass}>
        <div
          className="flex items-center px-3 py-1.5"
          onContextMenu={onHeaderContextMenu}
        >
          <span className="truncate text-[11px] font-semibold text-text-muted uppercase tracking-wider">
            {title}
          </span>
          {actions ? (
            <div className="ml-auto flex shrink-0 items-center gap-1 pl-2">{actions}</div>
          ) : null}
        </div>
      </HeroCard>
    );
  }

  return (
    <HeroCard className={cardClass}>
      <Disclosure defaultExpanded={defaultOpen} className="flex min-h-0 flex-col">
        <div
          className="shrink-0 flex items-center gap-1"
          onContextMenu={onHeaderContextMenu}
        >
          <Disclosure.Heading className="m-0 flex-1 min-w-0 text-inherit font-inherit leading-inherit">
            <Disclosure.Trigger
              className={cn(
                "flex-1 min-w-0 w-full flex items-center gap-1.5 px-3 py-1.5 text-left",
                "text-[11px] font-semibold text-text-muted uppercase tracking-wider",
                "hover:bg-bg-primary/60 hover:text-text-secondary",
                "bg-transparent border-0 shadow-none rounded-none",
              )}
            >
              <span className="truncate">{title}</span>
            </Disclosure.Trigger>
          </Disclosure.Heading>
          {actions ? (
            <div className="shrink-0 flex items-center gap-1 pr-2">{actions}</div>
          ) : null}
        </div>
        {/* Content is the box flex actually clamps, so the scroll belongs
            here: an inner height-auto wrapper would never overflow itself.
            sidebar-card-scroll keeps the gutter reserved and the scrollbar
            invisible until hover — expansion never flashes one. */}
        <Disclosure.Content className="sidebar-card-scroll min-h-0 overflow-y-auto">
          {children}
        </Disclosure.Content>
      </Disclosure>
    </HeroCard>
  );
}
