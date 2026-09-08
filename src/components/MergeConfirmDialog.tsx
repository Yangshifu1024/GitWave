// Fork-style merge confirmation dialog, shared by the sidebar BranchList and
// the history RefBadgeContextMenu: shows the source/target pair, an
// auto vs --no_ff option, and a conflict pre-check computed server-side
// without touching the working tree.

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { CircleCheck, CircleX, GitBranch } from "lucide-react";

import { mergePreview } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Label } from "@/components/ui/Label";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";

export function MergeConfirmDialog({
  workspaceId,
  name,
  currentBranch,
  onClose,
  onConfirm,
}: {
  workspaceId: string;
  name: string;
  currentBranch: string;
  onClose: () => void;
  onConfirm: (noFf: boolean) => void;
}): React.JSX.Element {
  const { t } = useTranslation();
  const [noFf, setNoFf] = useState(false);
  const { data: preview, isLoading } = useQuery({
    queryKey: ["merge-preview", workspaceId, name],
    queryFn: () => mergePreview(workspaceId, name),
  });

  const upToDate = preview?.up_to_date ?? false;
  const conflictCount = preview?.conflicts.length ?? 0;

  return (
    <Modal
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("branches.merge.title")}
      description={t("branches.merge.description", { name, current: currentBranch })}
      size="sm"
      footer={
        <>
          <Button variant="secondary" size="sm" className="min-w-0 flex-[3]" onClick={onClose}>
            {t("branches.cancel")}
          </Button>
          <Button
            variant="primary"
            size="sm"
            className="min-w-0 flex-[7]"
            disabled={upToDate}
            onClick={() => onConfirm(noFf)}
          >
            {t("branches.merge.confirm")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
        <div className="flex items-center gap-2">
          <span className="w-16 shrink-0 text-sm text-text-secondary">
            {t("branches.merge.sourceLabel")}
          </span>
          <GitBranch size={13} className="shrink-0 text-text-muted" />
          <span className="min-w-0 truncate text-sm text-text-primary" title={name}>
            {name}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="w-16 shrink-0 text-sm text-text-secondary">
            {t("branches.merge.targetLabel")}
          </span>
          <GitBranch size={13} className="shrink-0 text-text-muted" />
          <span className="min-w-0 truncate text-sm text-text-primary">{currentBranch}</span>
        </div>
        <div className="flex items-center gap-2">
          <Label className="w-16 shrink-0 text-sm text-text-secondary" htmlFor="merge-option">
            {t("branches.merge.optionLabel")}
          </Label>
          <Select
            id="merge-option"
            aria-label={t("branches.merge.optionAria")}
            className="h-auto min-w-0 flex-1 bg-bg-primary border-border-subtle px-1.5 py-1.5 text-sm"
            value={noFf ? "no_ff" : "auto"}
            onChange={(next) => setNoFf(next === "no_ff")}
            options={[
              { value: "auto", label: t("branches.merge.optionAuto") },
              { value: "no_ff", label: t("branches.merge.optionNoFf") },
            ]}
          />
        </div>
      </div>
      <div className="flex items-center gap-1.5 text-xs text-text-muted">
        {isLoading ? (
          <span>{t("branches.merge.checking")}</span>
        ) : upToDate ? (
          <span className="flex items-center gap-1.5">
            <CircleX size={14} className="shrink-0" />
            {t("branches.merge.upToDate")}
          </span>
        ) : conflictCount > 0 ? (
          <span className="flex items-center gap-1.5 text-danger">
            <CircleX size={14} className="shrink-0" />
            {t("branches.merge.mayConflict", { count: conflictCount })}
          </span>
        ) : (
          <span className="flex items-center gap-1.5 text-success">
            <CircleCheck size={14} className="shrink-0" />
            {t("branches.merge.noConflicts")}
          </span>
        )}
      </div>
    </Modal>
  );
}
