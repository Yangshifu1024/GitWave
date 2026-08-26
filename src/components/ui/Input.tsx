import {
  type InputHTMLAttributes,
  type ChangeEvent,
  forwardRef,
} from "react";
import { Search } from "lucide-react";
import { cn } from "@/lib/utils";

export interface InputProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "onChange"> {
  /** Error message — renders red border + helper text below */
  error?: string | null;
  /** Visual style variant */
  variant?: "text" | "search";
  /**
   * Change handler. Accepts either a React event handler (standard) or a
   * simple `(value: string) => void` for convenience.
   */
  onChange?: (value: string) => void;
}

/** Extract the string value from a change event. */
function eventValue(e: ChangeEvent<HTMLInputElement>): string {
  return e.currentTarget.value;
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
      ...props
    },
    ref,
  ) => {
    const isSearch = variant === "search";
    const inputId = id ?? `input-${Math.random().toString(36).slice(2, 9)}`;
    const errorId = error ? `${inputId}-error` : undefined;

    const handleChange = (e: ChangeEvent<HTMLInputElement>): void => {
      if (onChange) {
        onChange(eventValue(e));
      }
    };

    return (
      <div className="flex flex-col gap-1">
        <div className="relative">
          {isSearch && (
            <Search
              size={16}
              className="absolute left-2 top-1/2 -translate-y-1/2 text-text-muted pointer-events-none"
              aria-hidden="true"
            />
          )}
          <input
            ref={ref}
            id={inputId}
            type={isSearch ? "search" : type}
            aria-invalid={error ? true : undefined}
            aria-describedby={errorId}
            onChange={handleChange}
            className={cn(
              [
                "flex h-8 w-full rounded-md",
                "bg-bg-elevated border text-text-primary text-sm",
                "px-3 py-1",
                "placeholder:text-text-muted",
                "transition-colors duration-200",
                "focus:outline-none focus:ring-2 focus:ring-accent focus:ring-offset-2",
                "disabled:opacity-50 disabled:cursor-not-allowed",
              ].join(" "),
              isSearch ? "pl-8" : "",
              error
                ? "border-danger focus:ring-danger"
                : "border-border-default hover:border-border-strong",
              className,
            )}
            {...props}
          />
        </div>
        {error ? (
          <p id={errorId} className="text-xs text-danger">
            {error}
          </p>
        ) : null}
      </div>
    );
  },
);
Input.displayName = "Input";

export { Input };
