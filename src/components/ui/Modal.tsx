import * as Dialog from "@radix-ui/react-dialog";
import { type ReactNode } from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";

export interface ModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string | null;
  size?: "sm" | "md" | "lg" | "xl";
  /** @deprecated Destructive styling — not yet implemented */
  destructive?: boolean;
  children: ReactNode;
}

const sizeClasses = {
  // Responsive: take up to 90vw, capped at the design width. Lets the
  // modal grow with content (long paths, large lists) without overflowing
  // the viewport.
  sm: "w-[90vw] max-w-[480px]",
  md: "w-[90vw] max-w-[640px]",
  lg: "w-[90vw] max-w-[800px]",
  xl: "w-[92vw] max-w-[1200px]",
};

export function Modal({
  open,
  onOpenChange,
  title,
  description,
  size = "md",
  children,
}: ModalProps): React.JSX.Element {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay
          className={cn(
            "fixed inset-0 z-modal",
            "bg-bg-overlay backdrop-blur-sm",
            "data-[state=open]:animate-in data-[state=closed]:animate-out",
            "data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
          )}
        />
        <Dialog.Content
          className={cn(
            "fixed left-1/2 top-1/2 z-modal",
            "-translate-x-1/2 -translate-y-1/2",
            "w-full rounded-xl p-6",
            "bg-bg-elevated shadow-modal",
            "focus:outline-none",
            "data-[state=open]:animate-in data-[state=closed]:animate-out",
            "data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0",
            "data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95",
            "duration-base",
            sizeClasses[size],
          )}
        >
          <div className="flex flex-col gap-4">
            {/* Header */}
            <div className="flex items-start justify-between gap-4">
              <div className="flex flex-col gap-1">
                <Dialog.Title className="text-md font-semibold text-text-primary">
                  {title}
                </Dialog.Title>
                {description ? (
                  <Dialog.Description className="text-sm text-text-secondary break-words">
                    {description}
                  </Dialog.Description>
                ) : null}
              </div>
              <Dialog.Close asChild>
                <Button variant="ghost" size="sm" className="shrink-0 p-1">
                  <X size={16} />
                </Button>
              </Dialog.Close>
            </div>

            {/* Body */}
            <div className="flex flex-col gap-3">{children}</div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
