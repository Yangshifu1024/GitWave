import { Tooltip as HeroTooltip } from "@heroui/react";
import { type ReactNode } from "react";
import { cn } from "@/lib/utils";

export interface TooltipProps {
  content: string;
  children: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  delayDuration?: number;
}

export function Tooltip({
  content,
  children,
  side = "top",
  delayDuration = 300,
}: TooltipProps): React.JSX.Element {
  return (
    <HeroTooltip delay={delayDuration}>
      <HeroTooltip.Trigger className="inline-flex">{children}</HeroTooltip.Trigger>
      <HeroTooltip.Content
        placement={side}
        offset={4}
        showArrow
        className={cn(
          "z-popover rounded-sm",
          "bg-bg-elevated border border-border-default",
          "px-2 py-1 text-xs text-text-primary",
          "shadow-modal",
        )}
      >
        {content}
        <HeroTooltip.Arrow />
      </HeroTooltip.Content>
    </HeroTooltip>
  );
}
