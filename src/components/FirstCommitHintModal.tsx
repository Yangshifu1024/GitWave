import { useTranslation } from "react-i18next";

import { useUiStore } from "@/stores/uiStore";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";

export interface FirstCommitHintModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Shown from the commit graph's empty state: a repository without commits has
 *  no files to commit yet, so the hint explains the two ways to get a first
 *  commit instead of leaving the user at a dead end. */
export function FirstCommitHintModal({
  open,
  onOpenChange,
}: FirstCommitHintModalProps): React.JSX.Element {
  const { t } = useTranslation();
  // Clone is owned by ActionBar's repository dialogs; ask for the same action
  // the Repository menu fires rather than reaching into it from here.
  const requestMenuAction = useUiStore((s) => s.requestMenuAction);

  const steps = [
    t("branches.graph.firstCommitHint.step1"),
    t("branches.graph.firstCommitHint.step2", { clone: t("menu.repository.clone.text") }),
  ];

  const startClone = (): void => {
    // Close the hint first: the clone dialog must not stack under it.
    onOpenChange(false);
    requestMenuAction("repo:clone");
  };

  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={t("branches.graph.firstCommitHint.title")}
      description={t("branches.graph.firstCommitHint.description")}
      size="sm"
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={startClone}>
            {t("menu.repository.clone.text")}
          </Button>
          <Button variant="primary" size="sm" autoFocus onClick={() => onOpenChange(false)}>
            {t("common.close")}
          </Button>
        </div>
      }
    >
      <ul className="flex flex-col gap-2">
        {steps.map((step, index) => (
          <li key={step} className="flex items-start gap-2 rounded-md bg-bg-elevated px-3 py-2">
            <span className="mt-px flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-bg-panel text-[10px] font-medium text-text-muted">
              {index + 1}
            </span>
            <span className="text-xs leading-5 text-text-secondary">{step}</span>
          </li>
        ))}
      </ul>
    </Modal>
  );
}
