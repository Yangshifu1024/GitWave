import { Modal as HeroModal, Description } from "@heroui/react";
import { type ReactNode } from "react";

export interface ModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string | null;
  size?: "sm" | "md" | "lg" | "xl";
  /** @deprecated Destructive styling — not yet implemented */
  destructive?: boolean;
  /** Actions rendered in HeroUI's Modal.Footer (right-aligned, mt-5 below
   *  the body). Omit to keep actions inside the body flow. */
  footer?: ReactNode;
  children?: ReactNode;
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
  footer,
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
        {/* No chrome overrides: HeroUI's .modal__dialog already ships the
            official surface — bg-overlay, shadow-overlay, p-6 and radius
            min(32px, --radius-3xl). These classes only constrain the width. */}
        <HeroModal.Dialog className={sizeClasses[size]}>
          {({ close }) => (
            <>
              {/* Official anatomy: the close button floats at end-4 top-4
                  (.modal__close-trigger), outside the header flow. */}
              <HeroModal.CloseTrigger onPress={close} />
              <HeroModal.Header className="pe-10">
                <HeroModal.Heading>{title}</HeroModal.Heading>
                {description ? (
                  <Description className="text-sm text-text-secondary break-words">
                    {description}
                  </Description>
                ) : null}
              </HeroModal.Header>
              {/* No p-0 here: HeroUI's .modal__body ships -m-[3px] + p-[3px]
                  so focus rings on first/last fields have clip room inside
                  the overflow-y-auto scroll box. p-0 made the ring flush with
                  the clip edge and got it cut off (top/left/right). */}
              <HeroModal.Body className="flex flex-col gap-3">{children}</HeroModal.Body>
              {footer ? <HeroModal.Footer>{footer}</HeroModal.Footer> : null}
            </>
          )}
        </HeroModal.Dialog>
      </HeroModal.Container>
    </HeroModal.Backdrop>
  );
}
