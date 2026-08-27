import { type ButtonHTMLAttributes, forwardRef } from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  [
    "inline-flex items-center justify-center gap-2",
    "font-medium text-sm",
    "rounded-md transition-colors duration-200",
    "focus-visible:outline-2 focus-visible:outline-accent focus-visible:outline-offset-1",
    "disabled:opacity-50 disabled:cursor-not-allowed",
  ].join(" "),
  {
    variants: {
      variant: {
        primary: "bg-accent text-text-inverse hover:bg-accent-hover",
        secondary:
          "bg-bg-elevated border border-border-default text-text-primary hover:bg-bg-secondary",
        danger:
          "bg-bg-elevated border border-danger text-danger hover:bg-danger hover:text-text-inverse",
        ghost: "bg-transparent hover:bg-bg-primary/55 text-text-primary",
      },
      size: {
        sm: "h-7 px-2 text-xs",
        md: "h-8 px-3 text-sm",
      },
    },
    defaultVariants: {
      variant: "secondary",
      size: "md",
    },
  },
);

export interface ButtonProps
  extends ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> {
  variant?: "primary" | "secondary" | "danger" | "ghost";
  size?: "sm" | "md";
}

const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, disabled, type = "button", ...props }, ref) => (
    <button
      ref={ref}
      type={type}
      disabled={disabled}
      className={cn(buttonVariants({ variant, size }), className)}
      {...props}
    />
  ),
);
Button.displayName = "Button";

export { Button, buttonVariants };
