import { type InputHTMLAttributes, forwardRef } from "react";
import { FieldError, InputGroup, TextField } from "@heroui/react";
import { Search } from "lucide-react";
import { cn } from "@/lib/utils";

export interface InputProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "onChange"> {
  /** Error message — renders red border + helper text below */
  error?: string | null;
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
        <InputGroup fullWidth className={cn("h-8 min-h-8 rounded-md", className)}>
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
            className="h-8 px-3 py-1 text-sm"
            {...props}
          />
        </InputGroup>
        {error ? <FieldError className="text-xs text-danger">{error}</FieldError> : null}
      </TextField>
    );
  },
);
Input.displayName = "Input";

export { Input };
