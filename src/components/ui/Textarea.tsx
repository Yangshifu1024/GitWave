import { TextArea as HeroTextArea } from "@heroui/react";
import { forwardRef, type TextareaHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

export interface TextareaProps extends Omit<
  TextareaHTMLAttributes<HTMLTextAreaElement>,
  "onChange"
> {
  /** Simple `(value: string) => void` handler, mirroring the shared Input. */
  onChange?: (value: string) => void;
  rows?: number;
}

/**
 * Shared multiline input on HeroUI's TextArea. Full width by default —
 * every caller would otherwise have to remember w-full.
 */
const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(
  ({ className, onChange, rows = 3, ...props }, ref) => {
    return (
      <HeroTextArea
        ref={ref}
        fullWidth
        rows={rows}
        className={cn("min-w-0 text-xs", className)}
        onChange={
          onChange
            ? (e: React.ChangeEvent<HTMLTextAreaElement>) => onChange(e.target.value)
            : undefined
        }
        {...props}
      />
    );
  },
);
Textarea.displayName = "Textarea";

export { Textarea };
