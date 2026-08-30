import { type InputHTMLAttributes, type ReactNode, forwardRef } from "react";
import { Description, FieldError, InputGroup, TextField } from "@heroui/react";
import { Search } from "lucide-react";
import { Label } from "@/components/ui/Label";
import { cn } from "@/lib/utils";

export interface InputProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "onChange"> {
  /** Error message — renders red border + helper text below */
  error?: string | null;
  /** Field label — rendered above the input and associated with it for a11y */
  label?: ReactNode;
  /** Helper text below the input (hidden while `error` is shown) */
  description?: ReactNode;
  /** Trailing control rendered inside the field's right edge (HeroUI
   *  InputGroup.Suffix) — e.g. PathInput's browse button. */
  suffix?: ReactNode;
  /** Visual style variant */
  variant?: "text" | "search";
  /**
   * Change handler. Accepts a simple `(value: string) => void`.
   */
  onChange?: (value: string) => void;
}

const Input = forwardRef<HTMLInputElement, InputProps>(
  (
    {
      className,
      type = "text",
      variant = "text",
      error,
      label,
      description,
      suffix,
      id,
      onChange,
      disabled,
      value,
      defaultValue,
      placeholder,
      autoFocus,
      onKeyDown,
      name,
      ...props
    },
    ref,
  ) => {
    const isSearch = variant === "search";

    return (
      <TextField
        id={id}
        name={name}
        value={value === undefined ? undefined : String(value)}
        defaultValue={defaultValue === undefined ? undefined : String(defaultValue)}
        onChange={onChange}
        isDisabled={disabled}
        isInvalid={Boolean(error)}
        className="flex min-w-0 flex-col gap-1"
      >
        {label != null ? <Label>{label}</Label> : null}
        <InputGroup fullWidth className={cn("h-8 min-h-8 w-full min-w-0 rounded-md", className)}>
          {isSearch ? (
            <InputGroup.Prefix className="pl-2 text-text-muted" aria-hidden="true">
              <Search size={16} />
            </InputGroup.Prefix>
          ) : null}
          <InputGroup.Input
            ref={ref}
            type={isSearch ? "search" : type}
            placeholder={placeholder}
            autoFocus={autoFocus}
            onKeyDown={onKeyDown}
            /* min-w-0 lets the input shrink inside narrow groups — without it
             * the intrinsic width overflows the group border and pushes the
             * suffix outside it. */
            className="h-8 min-w-0 px-3 py-1 text-sm"
            {...props}
          />
          {suffix != null ? (
            <InputGroup.Suffix className="pr-1 text-text-muted">{suffix}</InputGroup.Suffix>
          ) : null}
        </InputGroup>
        {error ? (
          <FieldError className="text-xs text-danger">{error}</FieldError>
        ) : description != null ? (
          <Description className="text-[11px] text-text-muted">{description}</Description>
        ) : null}
      </TextField>
    );
  },
);
Input.displayName = "Input";

export { Input };
