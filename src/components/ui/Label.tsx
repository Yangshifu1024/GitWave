import { Label as HeroLabel } from "@heroui/react";
import { type ComponentProps } from "react";
import { cn } from "@/lib/utils";

export type LabelProps = ComponentProps<typeof HeroLabel>;

export function Label({ className, ...props }: LabelProps): React.JSX.Element {
  return (
    <HeroLabel className={cn("text-xs font-medium text-text-secondary", className)} {...props} />
  );
}
