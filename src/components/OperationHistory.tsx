import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { useOperationHistory } from "@/stores/operationHistory";
const labels = {
  fetch: "status.sync.fetch",
  pull: "status.sync.pull",
  push: "status.sync.push",
  stage: "changes.action.stage",
  unstage: "changes.action.unstage",
  discard: "changes.action.discard",
  ignore: "changes.ignore.title",
  commit: "changes.messageBox.commit",
} as const;
export function OperationHistory() {
  const { t, i18n } = useTranslation();
  const [open, setOpen] = useState(false);
  const entries = useOperationHistory((s) => s.entries);
  return (
    <>
      <Button size="sm" variant="ghost" onClick={() => setOpen(true)}>
        {t("status.log.title")}
      </Button>
      <Modal open={open} onOpenChange={setOpen} title={t("status.log.title")}>
        {!entries.length && <p>{t("status.log.empty")}</p>}
        <ol className="max-h-[55vh] overflow-auto">
          {entries.map((e) => (
            <li className="flex flex-wrap gap-2 text-xs py-1" key={e.id}>
              <time>{new Date(e.at).toLocaleTimeString(i18n.language)}</time>
              <span title={e.workspaceId + " / " + e.repoId}>{e.label}</span>
              <strong>{t(labels[e.operation])}</strong>
              <span>{t("status.log." + e.outcome)}</span>
            </li>
          ))}
        </ol>
      </Modal>
    </>
  );
}
