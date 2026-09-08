import { useTranslation } from "react-i18next";

import type { DirtyRepoSummary } from "@/lib/api";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";

export interface QuitConfirmModalProps {
  open: boolean;
  dirtyRepos: DirtyRepoSummary[];
  onCancel: () => void;
  onConfirm: () => void;
}

/** Shown by the quit guard when repos in the active workspace still have
 *  uncommitted changes. Cancel returns to the app; confirm really exits. */
export function QuitConfirmModal({
  open,
  dirtyRepos,
  onCancel,
  onConfirm,
}: QuitConfirmModalProps): React.JSX.Element {
  const { t } = useTranslation();

  return (
    <Modal
      open={open}
      onOpenChange={(next) => {
        if (!next) onCancel();
      }}
      title={t("quitGuard.title")}
      description={t("quitGuard.description")}
      size="sm"
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="secondary" size="sm" autoFocus onClick={onCancel}>
            {t("quitGuard.cancel")}
          </Button>
          <Button variant="primary" size="sm" onClick={onConfirm}>
            {t("quitGuard.confirm")}
          </Button>
        </div>
      }
    >
      <ul className="flex flex-col gap-1.5">
        {dirtyRepos.map((repo) => (
          <li
            key={repo.repo_id}
            className="flex items-center justify-between gap-3 rounded-md bg-bg-elevated px-3 py-2"
          >
            <span className="truncate text-sm text-text-primary">{repo.nickname ?? repo.path}</span>
            <span className="shrink-0 text-xs text-text-muted tabular-nums">
              {t("quitGuard.fileCount", { count: repo.file_count })}
            </span>
          </li>
        ))}
      </ul>
    </Modal>
  );
}
