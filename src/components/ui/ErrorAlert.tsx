import { useEffect, useState } from "react";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";

export interface ErrorAlertProps {
  message: string | null;
  title?: string;
  onDismiss?: () => void;
}

/**
 * Blocking error dialog. Replaces inline top-of-panel danger bars.
 * Dismissing hides the current message; a new (or re-raised) message opens it again.
 */
export function ErrorAlert({
  message,
  title = "Error",
  onDismiss,
}: ErrorAlertProps): React.JSX.Element {
  const [dismissed, setDismissed] = useState<string | null>(null);

  useEffect(() => {
    if (!message) setDismissed(null);
  }, [message]);

  const open = Boolean(message && message !== dismissed);

  const dismiss = () => {
    setDismissed(message);
    onDismiss?.();
  };

  return (
    <Modal
      open={open}
      onOpenChange={(next) => {
        if (!next) dismiss();
      }}
      title={title}
      description={message}
      size="sm"
    >
      <div className="flex justify-end">
        <Button variant="primary" size="sm" onClick={dismiss}>
          OK
        </Button>
      </div>
    </Modal>
  );
}
