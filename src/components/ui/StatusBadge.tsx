import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center gap-0.5 rounded-sm px-1.5 py-0.5 text-xs font-medium",
  {
    variants: {
      variant: {
        active: "bg-status-active/15 text-status-active",
        missing: "bg-danger/15 text-danger",
        ahead: "bg-branch-ahead/15 text-branch-ahead",
        behind: "bg-branch-behind/15 text-branch-behind",
        conflict: "bg-branch-conflict/15 text-branch-conflict",
      },
    },
    defaultVariants: {
      variant: "active",
    },
  },
);

export interface StatusBadgeProps
  extends VariantProps<typeof badgeVariants> {
  variant: "active" | "missing" | "ahead" | "behind" | "conflict";
  suffix?: string | null;
  className?: string;
}

export function StatusBadge({
  variant,
  suffix = null,
  className,
}: StatusBadgeProps): React.JSX.Element {
  return (
    <span className={cn(badgeVariants({ variant }), className)}>
      {suffix ?? badgeLabel(variant)}
    </span>
  );
}

function badgeLabel(variant: StatusBadgeProps["variant"]): string {
  switch (variant) {
    case "active":
      return "active";
    case "missing":
      return "missing";
    case "ahead":
      return "ahead";
    case "behind":
      return "behind";
    case "conflict":
      return "conflict";
    default:
      return "";
  }
}
