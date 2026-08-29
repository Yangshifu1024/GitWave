import { Card as HeroCard, Disclosure } from "@heroui/react";
import { cn } from "@/lib/utils";

export interface SidebarSectionProps {
  title: string;
  actions?: React.ReactNode;
  children: React.ReactNode;
  defaultOpen?: boolean;
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
  className,
}: SidebarSectionProps): React.JSX.Element {
  return (
    <HeroCard
      className={cn(
        "min-h-0 shrink overflow-hidden rounded-lg p-0 gap-0",
        "border border-border-subtle bg-bg-elevated shadow-none",
        className,
      )}
    >
      <Disclosure defaultExpanded={defaultOpen} className="flex min-h-0 flex-col">
        <div className="shrink-0 flex items-center gap-1">
          <Disclosure.Heading className="m-0 flex-1 min-w-0 text-inherit font-inherit leading-inherit">
            <Disclosure.Trigger
              className={cn(
                "flex-1 min-w-0 w-full flex items-center gap-1.5 px-3 py-1.5 text-left",
                "text-[11px] font-semibold text-text-muted uppercase tracking-wider",
                "hover:bg-bg-primary/60 hover:text-text-secondary",
                "bg-transparent border-0 shadow-none rounded-none",
              )}
            >
              {/* HeroUI omits data-expanded when collapsed (dataAttr(false) is
                  undefined), so default to the collapsed ▶ and flip to ▼ only
                  when the attribute is present; the attribute selector also
                  outranks the plain utility. */}
              <Disclosure.Indicator className="ms-0 size-3 shrink-0 -rotate-90 data-[expanded=true]:rotate-0" />
              <span className="truncate">{title}</span>
            </Disclosure.Trigger>
          </Disclosure.Heading>
          {actions ? (
            <div className="shrink-0 flex items-center gap-1 pr-2">{actions}</div>
          ) : null}
        </div>
        {/* Content is the box flex actually clamps, so the scroll belongs
            here: an inner height-auto wrapper would never overflow itself. */}
        <Disclosure.Content className="min-h-0 overflow-y-auto">
          {children}
        </Disclosure.Content>
      </Disclosure>
    </HeroCard>
  );
}
