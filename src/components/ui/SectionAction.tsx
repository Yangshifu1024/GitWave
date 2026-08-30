import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import { Tooltip } from "@/components/ui/Tooltip";

export interface SectionActionProps {
  children: React.ReactNode;
  onClick?: () => void;
  disabled?: boolean;
  inProgress?: boolean;
  className?: string;
  title?: string;
  tooltip?: string;
  "aria-label"?: string;
}

export function SectionAction({
  children,
  onClick,
  disabled = false,
  inProgress = false,
  className,
  title,
  tooltip,
  "aria-label": ariaLabel,
}: SectionActionProps): React.JSX.Element {
  const label = tooltip ?? title ?? ariaLabel;
  const button = (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      onClick={onClick}
      disabled={disabled || inProgress}
      title={label}
      aria-label={ariaLabel ?? label}
      className={cn(
        "h-auto gap-0.5 px-1 py-0.5",
        "text-[10px] font-semibold uppercase tracking-wide",
        "text-text-muted hover:text-accent",
        "disabled:opacity-40 disabled:pointer-events-none",
        className,
      )}
    >
      {inProgress ? <RefreshCw size={10} className="animate-spin" /> : null}
      {children}
    </Button>
  );

  if (tooltip) {
    return <Tooltip content={tooltip}>{button}</Tooltip>;
  }

  return button;
}

export interface FetchButtonProps {
  onFetch?: () => void;
  disabled?: boolean;
  inProgress?: boolean;
}

export function FetchButton({
  onFetch,
  disabled = false,
  inProgress = false,
}: FetchButtonProps): React.JSX.Element | null {
  const { t } = useTranslation();
  if (!onFetch) return null;
  return (
    <SectionAction
      onClick={onFetch}
      disabled={disabled}
      inProgress={inProgress}
      tooltip={t("status.sync.fetchTooltip")}
      title={t("status.sync.fetch")}
      aria-label={t("status.sync.fetch")}
    >
      {t("status.sync.fetch")}
    </SectionAction>
  );
}

export interface BranchSyncButtonsProps {
  ahead?: number;
  behind?: number;
  onPull?: () => void;
  onPush?: () => void;
  pullDisabled?: boolean;
  pushDisabled?: boolean;
  inProgress?: { pull?: boolean; push?: boolean };
  syncBusy?: boolean;
}

export function BranchSyncButtons({
  ahead = 0,
  behind = 0,
  onPull,
  onPush,
  pullDisabled,
  pushDisabled,
  inProgress = {},
  syncBusy = false,
}: BranchSyncButtonsProps): React.JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      {onPull ? (
        <SectionAction
          onClick={onPull}
          disabled={syncBusy || (pullDisabled ?? behind === 0)}
          inProgress={inProgress.pull}
          tooltip={t("status.sync.pullTooltip")}
          title={t("status.sync.pull")}
          aria-label={t("status.sync.pull")}
        >
          {t("status.sync.pull")}
          {behind > 0 && !inProgress.pull ? (
            <span className="text-branch-behind normal-case">↓{behind}</span>
          ) : null}
        </SectionAction>
      ) : null}
      {onPush ? (
        <SectionAction
          onClick={onPush}
          disabled={syncBusy || (pushDisabled ?? ahead === 0)}
          inProgress={inProgress.push}
          tooltip={t("status.sync.pushTooltip")}
          title={t("status.sync.push")}
          aria-label={t("status.sync.push")}
        >
          {t("status.sync.push")}
          {ahead > 0 && !inProgress.push ? (
            <span className="text-branch-ahead normal-case">↑{ahead}</span>
          ) : null}
        </SectionAction>
      ) : null}
    </>
  );
}
