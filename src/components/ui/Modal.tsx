import { Modal as HeroModal, Description } from "@heroui/react";
import { type ReactNode } from "react";
import { cn } from "@/lib/utils";

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
    <HeroModal.Backdrop
      isDismissable
      isOpen={open}
      onOpenChange={onOpenChange}
      className="z-modal bg-bg-overlay backdrop-blur-sm"
    >
      <HeroModal.Container placement="center" className="z-modal">
        <HeroModal.Dialog
          className={cn(
            "rounded-xl p-6 bg-bg-elevated shadow-modal focus:outline-none",
            sizeClasses[size],
          )}
        >
          {({ close }) => (
            <div className="flex flex-col gap-4">
              <div className="flex items-start justify-between gap-4">
                <div className="flex flex-col gap-1">
                  <HeroModal.Heading className="text-md font-semibold text-text-primary">
                    {title}
                  </HeroModal.Heading>
                  {description ? (
                    <Description className="text-sm text-text-secondary break-words">
                      {description}
                    </Description>
                  ) : null}
                </div>
                <HeroModal.CloseTrigger className="shrink-0" onPress={close} />
              </div>
              <HeroModal.Body className="flex flex-col gap-3 p-0">{children}</HeroModal.Body>
            </div>
          )}
        </HeroModal.Dialog>
      </HeroModal.Container>
    </HeroModal.Backdrop>
  );
}
