import { Download, Upload, RefreshCw } from "lucide-react";
import { Chip } from "@heroui/react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";

export interface SyncButtonsProps {
  ahead?: number;
  behind?: number;
  onFetch?: () => void;
  onPull?: () => void;
  onPush?: () => void;
  /** When set, overrides the default (always enabled for fetch; ahead/behind for pull/push). */
  fetchDisabled?: boolean;
  pullDisabled?: boolean;
  pushDisabled?: boolean;
  inProgress?: {
    fetch?: boolean;
    pull?: boolean;
    push?: boolean;
  };
  className?: string;
}

interface SyncButtonConfig {
  label: string;
  icon: React.ReactNode;
  onClick: () => void;
  disabled?: boolean;
  badge?: string | null;
  inProgress?: boolean;
}

export function SyncButtons({
  ahead = 0,
  behind = 0,
  onFetch,
  onPull,
  onPush,
  fetchDisabled = false,
  pullDisabled,
  pushDisabled,
  inProgress = {},
  className,
}: SyncButtonsProps): React.JSX.Element {
  const buttons: SyncButtonConfig[] = [];

  if (onFetch) {
    buttons.push({
      label: "Fetch",
      icon: <Download size={14} />,
      onClick: onFetch,
      disabled: fetchDisabled,
      inProgress: inProgress.fetch,
    });
  }
  if (onPull) {
    buttons.push({
      label: "Pull",
      icon: <RefreshCw size={14} />,
      onClick: onPull,
      disabled: pullDisabled ?? behind === 0,
      badge: behind > 0 ? `↓${behind}` : null,
      inProgress: inProgress.pull,
    });
  }
  if (onPush) {
    buttons.push({
      label: "Push",
      icon: <Upload size={14} />,
      onClick: onPush,
      disabled: pushDisabled ?? ahead === 0,
      badge: ahead > 0 ? `↑${ahead}` : null,
      inProgress: inProgress.push,
    });
  }

  return (
    <div className={cn("flex items-center gap-1", className)}>
      {buttons.map((btn) => (
        <Button
          key={btn.label}
          variant="ghost"
          size="sm"
          disabled={btn.disabled || btn.inProgress}
          onClick={btn.onClick}
          className="relative"
          title={btn.label}
          aria-label={btn.label}
        >
          {btn.inProgress ? <RefreshCw size={14} className="animate-spin" /> : btn.icon}

          {btn.badge && !btn.inProgress && (
            <Chip
              size="sm"
              className={cn(
                "ml-1 rounded-sm px-1 py-0.5 text-xs font-medium border-0 shadow-none",
                btn.label === "Push"
                  ? "bg-branch-ahead/15 text-branch-ahead"
                  : "bg-branch-behind/15 text-branch-behind",
              )}
            >
              {btn.badge}
            </Chip>
          )}
        </Button>
      ))}
    </div>
  );
}
