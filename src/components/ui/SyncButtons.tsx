import { Download, Upload, RefreshCw } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";

export interface SyncButtonsProps {
  ahead?: number;
  behind?: number;
  onFetch: () => void;
  onPull: () => void;
  onPush: () => void;
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
  variant: "primary" | "secondary" | "ghost";
}

export function SyncButtons({
  ahead = 0,
  behind = 0,
  onFetch,
  onPull,
  onPush,
  inProgress = {},
  className,
}: SyncButtonsProps): React.JSX.Element {
  const buttons: SyncButtonConfig[] = [
    {
      label: "Fetch",
      icon: <Download size={14} />,
      onClick: onFetch,
      variant: "ghost",
      inProgress: inProgress.fetch,
    },
    {
      label: "Pull",
      icon: <RefreshCw size={14} />,
      onClick: onPull,
      disabled: behind === 0,
      badge: behind > 0 ? `↓${behind}` : null,
      inProgress: inProgress.pull,
      variant: "ghost",
    },
    {
      label: "Push",
      icon: <Upload size={14} />,
      onClick: onPush,
      disabled: ahead === 0,
      badge: ahead > 0 ? `↑${ahead}` : null,
      inProgress: inProgress.push,
      variant: "ghost",
    },
  ];

  return (
    <div className={cn("flex items-center gap-1", className)}>
      {buttons.map((btn) => (
        <Button
          key={btn.label}
          variant={btn.variant}
          size="sm"
          disabled={btn.disabled || btn.inProgress}
          onClick={btn.onClick}
          className={cn("relative")}
          title={btn.label}
        >
          {/* Spinner overlay when in progress */}
          {btn.inProgress ? (
            <RefreshCw size={14} className="animate-spin" />
          ) : (
            btn.icon
          )}

          {/* Badge (ahead/behind count) */}
          {btn.badge && !btn.inProgress && (
            <span
              className={cn(
                "ml-1 rounded-sm px-1 py-0.5 text-xs font-medium",
                btn.label === "Push"
                  ? "bg-branch-ahead/15 text-branch-ahead"
                  : "bg-branch-behind/15 text-branch-behind",
              )}
            >
              {btn.badge}
            </span>
          )}
        </Button>
      ))}
    </div>
  );
}
